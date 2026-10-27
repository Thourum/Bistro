package handler

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/itterno/bistro/internal/ws"
)

type queueTestProvider struct{ provider.Provider }

type queueOperationProvider struct {
	provider.Provider
	read func(context.Context) (*provider.ReviewQueue, error)
}

func (p *queueOperationProvider) GetReviewQueue(ctx context.Context, _ time.Time) (*provider.ReviewQueue, error) {
	return p.read(ctx)
}

func TestReviewQueueCancellationPreventsPersistenceAndLateWorkers(t *testing.T) {
	for _, cancelledLifetime := range []string{"run", "provider"} {
		t.Run(cancelledLifetime, func(t *testing.T) {
			database := setupRepoTestDB(t)
			defer database.Close()
			runCtx, cancelRun := context.WithCancel(context.Background())
			defer cancelRun()
			providerCtx, cancelProvider := context.WithCancel(context.Background())
			defer cancelProvider()
			started := make(chan struct{}, 1)
			p := &queueOperationProvider{Provider: mock.NewMockProvider(), read: func(ctx context.Context) (*provider.ReviewQueue, error) {
				started <- struct{}{}
				<-ctx.Done()
				return (&queueTestProvider{}).GetReviewQueue(ctx, time.Now())
			}}
			messages := make(chan any, 1)
			request, wait := NewRequestReviewQueueHandler(ReviewQueueDeps{Context: runCtx,
				GetProvider: func() (provider.Provider, context.Context) { return p, providerCtx },
				DB:          database, Broadcast: func(message any) { messages <- message }, Log: testRepoLogger()})
			defer wait()
			if _, err := request(runCtx, ws.InboundMessage{ID: "cancelled"}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("worker did not start")
			}
			if cancelledLifetime == "run" {
				cancelRun()
			} else {
				cancelProvider()
			}
			wait()
			wait()
			var count int
			if err := database.QueryRow(`SELECT count(*) FROM repos`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("cancelled queue persisted %d repositories", count)
			}
			select {
			case message := <-messages:
				t.Fatalf("stale reply: %+v", message)
			default:
			}
			if _, err := request(runCtx, ws.InboundMessage{ID: "late"}); err != nil {
				t.Fatal(err)
			}
			wait()
			select {
			case <-started:
				t.Fatal("worker registered after wait")
			default:
			}
		})
	}
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

func TestReviewQueueLogsFailureWithoutUsernameOrAPIResponse(t *testing.T) {
	database := setupRepoTestDB(t)
	defer database.Close()
	requestURL, err := url.Parse("https://api.github.com/search/issues?q=review-requested:private-login")
	if err != nil {
		t.Fatal(err)
	}
	apiFailure := &gh.ErrorResponse{Response: &http.Response{StatusCode: http.StatusForbidden, Request: &http.Request{Method: http.MethodGet, URL: requestURL}}, Message: "private response content"}
	p := &queueOperationProvider{Provider: mock.NewMockProvider(), read: func(context.Context) (*provider.ReviewQueue, error) {
		return nil, fmt.Errorf("search requested reviews: %w", apiFailure)
	}}
	var output bytes.Buffer
	request, wait := NewRequestReviewQueueHandler(ReviewQueueDeps{Context: context.Background(),
		GetProvider: func() (provider.Provider, context.Context) { return p, context.Background() },
		DB:          database, Broadcast: func(any) {}, Log: slog.New(slog.NewTextHandler(&output, nil))})
	if _, err := request(context.Background(), ws.InboundMessage{ID: "private"}); err != nil {
		t.Fatal(err)
	}
	wait()
	if strings.Contains(output.String(), "private-login") || strings.Contains(output.String(), "private response content") {
		t.Fatalf("user data in logs: %s", output.String())
	}
	if !strings.Contains(output.String(), "403") || !strings.Contains(output.String(), "search requested reviews") {
		t.Fatalf("missing technical context: %s", output.String())
	}
}

func TestReviewQueueFailuresReplyWithSafeCorrelatedError(t *testing.T) {
	for _, read := range []func(context.Context) (*provider.ReviewQueue, error){
		func(context.Context) (*provider.ReviewQueue, error) {
			return nil, errors.New("private technical detail")
		},
		func(context.Context) (*provider.ReviewQueue, error) { return nil, context.DeadlineExceeded },
		func(context.Context) (*provider.ReviewQueue, error) { return nil, nil },
	} {
		database := setupRepoTestDB(t)
		p := &queueOperationProvider{Provider: mock.NewMockProvider(), read: read}
		messages := make(chan any, 1)
		request, wait := NewRequestReviewQueueHandler(ReviewQueueDeps{Context: context.Background(),
			GetProvider: func() (provider.Provider, context.Context) { return p, context.Background() },
			DB:          database, Broadcast: func(message any) { messages <- message }, Log: testRepoLogger()})
		if reply, err := request(context.Background(), ws.InboundMessage{ID: "failed"}); err != nil || reply != nil {
			t.Fatalf("request: %v %v", reply, err)
		}
		wait()
		select {
		case reply := <-messages:
			message := reply.(map[string]any)
			if message["id"] != "failed" || message["type"] != "REVIEW_QUEUE_READY" || message["data"].(map[string]string)["error"] != "Unable to read review requests. Try again later." {
				t.Fatalf("unsafe reply: %+v", message)
			}
		default:
			t.Fatal("missing failure reply")
		}
		if err := database.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReviewQueueRejectsPayloadAndMissingCapability(t *testing.T) {
	for _, test := range []struct {
		payload string
		prov    provider.Provider
		ctx     context.Context
		want    string
	}{
		{payload: `[]`, want: "Unable to read review requests. Try again later."},
		{payload: `null`, want: "Unable to read review requests. Try again later."},
		{payload: `{`, want: "Unable to read review requests. Try again later."},
		{payload: `{"unexpected":true}`, want: "Unable to read review requests. Try again later."},
		{payload: `{}`, want: "not authenticated"},
		{payload: `{}`, prov: &queueTestProvider{}, want: "not authenticated"},
		{payload: `{}`, prov: mock.NewMockProvider(), ctx: context.Background(), want: "Unable to read review requests. Try again later."},
	} {
		request, wait := NewRequestReviewQueueHandler(ReviewQueueDeps{Context: context.Background(),
			GetProvider: func() (provider.Provider, context.Context) { return test.prov, test.ctx }, Log: testRepoLogger()})
		reply, err := request(context.Background(), ws.InboundMessage{ID: "rejected", Data: json.RawMessage(test.payload)})
		wait()
		if err != nil {
			t.Fatal(err)
		}
		message := reply.(map[string]any)
		if message["id"] != "rejected" || message["data"].(map[string]string)["error"] != test.want {
			t.Fatalf("unexpected rejection: %+v", message)
		}
	}
}

func (p *queueTestProvider) GetReviewQueue(context.Context, time.Time) (*provider.ReviewQueue, error) {
	return &provider.ReviewQueue{
		Requested:        []provider.RequestedRepo{{RepoID: "github:acme/api", Count: 3}},
		RecentlyReviewed: []string{"github:acme/web"},
		Repositories:     []*provider.Repo{{Owner: "acme", Name: "api", FullName: "acme/api"}, {Owner: "acme", Name: "web", FullName: "acme/web"}},
	}, nil
}
func TestReviewQueuePreservesWatchAndRepliesAfterPersistence(t *testing.T) {
	database := setupRepoTestDB(t)
	defer database.Close()
	if _, err := database.Exec(`INSERT INTO repos(id,provider,owner,repo,watched) VALUES('github:acme/api','github','acme','api',1)`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &queueTestProvider{Provider: mock.NewMockProvider()}
	messages := make(chan any, 1)
	request, wait := NewRequestReviewQueueHandler(ReviewQueueDeps{Context: ctx,
		GetProvider: func() (provider.Provider, context.Context) { return p, ctx },
		DB:          database, Broadcast: func(message any) { messages <- message }, Log: testRepoLogger()})
	defer wait()
	reply, err := request(ctx, ws.InboundMessage{ID: "queue", Type: "REQUEST_REVIEW_QUEUE", Data: json.RawMessage(`{}`)})
	if err != nil || reply != nil {
		t.Fatalf("start must yield the socket: %v %v", reply, err)
	}
	select {
	case message := <-messages:
		encoded, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			ID   string               `json:"id"`
			Type string               `json:"type"`
			Data provider.ReviewQueue `json:"data"`
		}
		if err := json.Unmarshal(encoded, &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.ID != "queue" || envelope.Type != "REVIEW_QUEUE_READY" || len(envelope.Data.Requested) != 1 {
			t.Fatalf("reply = %s", encoded)
		}
		var on, off int
		if err := database.QueryRow(`SELECT watched FROM repos WHERE id='github:acme/api'`).Scan(&on); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRow(`SELECT watched FROM repos WHERE id='github:acme/web'`).Scan(&off); err != nil {
			t.Fatal(err)
		}
		if on != 1 || off != 0 {
			t.Fatalf("watch choices changed: api=%d web=%d", on, off)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queue response missing")
	}
}
