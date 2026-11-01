package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	gh "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/store/repolocation"
)

type queueTestProvider struct{ provider.Provider }

type queueOperationProvider struct {
	provider.Provider
	read func(context.Context) (*provider.ReviewQueue, error)
}

func (p *queueOperationProvider) GetReviewQueue(ctx context.Context, _ time.Time) (*provider.ReviewQueue, error) {
	return p.read(ctx)
}

func (p *queueTestProvider) GetReviewQueue(context.Context, time.Time) (*provider.ReviewQueue, error) {
	return &provider.ReviewQueue{
		Requested:        []provider.RequestedRepo{{RepoID: "github:acme/api", Count: 3}},
		RecentlyReviewed: []string{"github:acme/web"},
		Repositories:     []*provider.Repo{{Owner: "acme", Name: "api", FullName: "acme/api"}, {Owner: "acme", Name: "web", FullName: "acme/web"}},
	}, nil
}
func TestReviewQueueDiscoveryMergesWithLocalRepository(t *testing.T) {
	database := setupRepoTestDB(t)
	defer database.Close()
	ctx := context.Background()
	p := &queueOperationProvider{read: func(context.Context) (*provider.ReviewQueue, error) {
		return &provider.ReviewQueue{Repositories: []*provider.Repo{{Owner: "Itterno", Name: "Bistro"}}}, nil
	}}
	if _, err := fetchAndPersistReviewQueue(ctx, database, p); err != nil {
		t.Fatal(err)
	}
	var id, owner, repo string
	if err := database.QueryRow(`SELECT id, owner, repo FROM repos`).Scan(&id, &owner, &repo); err != nil {
		t.Fatal(err)
	}
	if id != "github:itterno/bistro" || owner != "Itterno" || repo != "Bistro" {
		t.Fatalf("discovery identity or display casing lost: %s %s/%s", id, owner, repo)
	}
	localPath := t.TempDir()
	if err := repolocation.New(database).UpsertLocal(ctx, "github", "Itterno", "Bistro", localPath, false); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := database.QueryRow(`SELECT count(*) FROM repos`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("queue discovery and scan created %d rows", count)
	}
	var storedPath string
	if err := database.QueryRow(`SELECT local_path FROM repos WHERE id = ?`, id).Scan(&storedPath); err != nil {
		t.Fatal(err)
	}
	if storedPath != localPath {
		t.Fatalf("local path = %q, want %q", storedPath, localPath)
	}
}

func TestReviewQueuePersistenceRollsBackInvalidRepository(t *testing.T) {
	for _, invalid := range []*provider.Repo{nil, {Owner: "bad/owner", Name: "repo"}} {
		database := setupRepoTestDB(t)
		p := &queueOperationProvider{read: func(context.Context) (*provider.ReviewQueue, error) {
			return &provider.ReviewQueue{Repositories: []*provider.Repo{{Owner: "acme", Name: "valid"}, invalid}}, nil
		}}
		if _, err := fetchAndPersistReviewQueue(context.Background(), database, p); err == nil {
			t.Fatal("invalid repository accepted")
		}
		var count int
		if err := database.QueryRow(`SELECT count(*) FROM repos`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("partial persistence: %d repositories", count)
		}
		if err := database.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReviewQueuePreservesWatchAfterPersistence(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO repos(id,provider,owner,repo,watched) VALUES('github:acme/api','github','acme','api',1)`); err != nil {
		t.Fatal(err)
	}
	p := &queueTestProvider{Provider: mock.NewMockProvider()}
	s := &ReviewQueue{DB: db, GetProvider: func() (provider.Provider, context.Context) { return p, context.Background() }}
	queue, err := s.Get(context.Background())
	if err != nil || len(queue.Requested) != 1 || queue.Requested[0].RepoID != "github:acme/api" || len(queue.RecentlyReviewed) != 1 {
		t.Fatalf("queue = %+v, %v", queue, err)
	}
	var on, off int
	if err := db.QueryRow(`SELECT watched FROM repos WHERE id='github:acme/api'`).Scan(&on); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT watched FROM repos WHERE id='github:acme/web'`).Scan(&off); err != nil {
		t.Fatal(err)
	}
	if on != 1 || off != 0 {
		t.Fatalf("watch lost: on=%d off=%d", on, off)
	}
}

func TestReviewQueueErrorsAndTimeout(t *testing.T) {
	for _, test := range []struct {
		name string
		read func(context.Context) (*provider.ReviewQueue, error)
		code Code
	}{
		{"provider failure", func(context.Context) (*provider.ReviewQueue, error) {
			return nil, errors.New("private technical detail")
		}, CodeInternal},
		{"nil queue", func(context.Context) (*provider.ReviewQueue, error) { return nil, nil }, CodeInternal},
		{"timeout", func(ctx context.Context) (*provider.ReviewQueue, error) { <-ctx.Done(); return nil, ctx.Err() }, CodeTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupRepoTestDB(t)
			defer db.Close()
			p := &queueOperationProvider{read: test.read}
			s := &ReviewQueue{DB: db, Log: testRepoLogger(), GetProvider: func() (provider.Provider, context.Context) { return p, context.Background() }}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			_, err := s.Get(ctx)
			serviceError := assertServiceCode(t, err, test.code)
			if test.code == CodeInternal && serviceError.Message != reviewQueueError {
				t.Fatalf("unsafe message: %q", serviceError.Message)
			}
		})
	}
}

func TestReviewQueueValidation(t *testing.T) {
	for _, test := range []struct {
		prov provider.Provider
		ctx  context.Context
		code Code
	}{
		{code: CodeUnauthenticated},
		{prov: &queueTestProvider{}, code: CodeUnauthenticated},
		{prov: mock.NewMockProvider(), ctx: context.Background(), code: CodeInvalid},
	} {
		s := &ReviewQueue{GetProvider: func() (provider.Provider, context.Context) { return test.prov, test.ctx }}
		_, err := s.Get(context.Background())
		assertServiceCode(t, err, test.code)
	}
}

func TestReviewQueueCancellationPreventsPersistence(t *testing.T) {
	for _, lifetime := range []string{"caller", "run", "provider"} {
		t.Run(lifetime, func(t *testing.T) {
			db := setupRepoTestDB(t)
			defer db.Close()
			callerCtx, cancelCaller := context.WithCancel(context.Background())
			defer cancelCaller()
			runCtx, cancelRun := context.WithCancel(context.Background())
			defer cancelRun()
			providerCtx, cancelProvider := context.WithCancel(context.Background())
			defer cancelProvider()
			started := make(chan struct{})
			p := &queueOperationProvider{read: func(ctx context.Context) (*provider.ReviewQueue, error) {
				close(started)
				<-ctx.Done()
				return (&queueTestProvider{}).GetReviewQueue(ctx, time.Now())
			}}
			s := &ReviewQueue{Context: runCtx, DB: db, GetProvider: func() (provider.Provider, context.Context) { return p, providerCtx }}
			done := make(chan error, 1)
			go func() { _, err := s.Get(callerCtx); done <- err }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("fetch did not start")
			}
			switch lifetime {
			case "caller":
				cancelCaller()
			case "run":
				cancelRun()
			case "provider":
				cancelProvider()
			}
			select {
			case err := <-done:
				assertServiceCode(t, err, CodeCancelled)
			case <-time.After(time.Second):
				t.Fatal("fetch did not cancel")
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM repos`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("cancelled fetch persisted %d repos", count)
			}
		})
	}
}

func TestReviewQueueLogsFailureWithoutPersonalData(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()
	requestURL, err := url.Parse("https://api.github.com/search/issues?q=review-requested:private-login")
	if err != nil {
		t.Fatal(err)
	}
	apiFailure := &gh.ErrorResponse{Response: &http.Response{StatusCode: http.StatusForbidden, Request: &http.Request{Method: http.MethodGet, URL: requestURL}}, Message: "private response content"}
	p := &queueOperationProvider{read: func(context.Context) (*provider.ReviewQueue, error) {
		return nil, fmt.Errorf("search requested reviews: %w", apiFailure)
	}}
	var output bytes.Buffer
	s := &ReviewQueue{DB: db, GetProvider: func() (provider.Provider, context.Context) { return p, context.Background() }, Log: slog.New(slog.NewTextHandler(&output, nil))}
	_, err = s.Get(context.Background())
	if message := assertServiceCode(t, err, CodeInternal).Message; message != reviewQueueError {
		t.Fatalf("unsafe message: %q", message)
	}
	if strings.Contains(output.String(), "private-login") || strings.Contains(output.String(), "private response content") {
		t.Fatal("personal data in logs")
	}
	if !strings.Contains(output.String(), "403") || !strings.Contains(output.String(), "search requested reviews") {
		t.Fatalf("technical context missing: %s", output.String())
	}
}
