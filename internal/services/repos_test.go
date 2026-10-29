package services

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/store/storetest"
)

func setupRepoTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return storetest.Open(t).DB
}

func testRepoLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestReposListErrors(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()
	s := &Repos{DB: db, GetProvider: func() provider.Provider { return nil }}
	_, err := s.List(context.Background(), RepoListRequest{})
	assertServiceCode(t, err, CodeUnauthenticated)
	s.GetProvider = func() provider.Provider {
		return mock.NewMockProvider().WithError(errors.New("private technical detail"))
	}
	_, err = s.List(context.Background(), RepoListRequest{})
	if message := assertServiceCode(t, err, CodeInternal).Message; message != "unable to load repositories" {
		t.Fatalf("unsafe message %q", message)
	}
	if _, err := db.Exec(`DROP TABLE repos`); err != nil {
		t.Fatal(err)
	}
	_, err = s.List(context.Background(), RepoListRequest{CacheOnly: true})
	if message := assertServiceCode(t, err, CodeInternal).Message; message != "unable to load repositories" {
		t.Fatalf("unsafe message %q", message)
	}
}

func TestReposListCacheAndRefreshPreserveLocation(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO repos(id,provider,owner,repo,watched,local_path,source,location_status,location_error_code,location_error)
 VALUES('github:acme/api','github','Acme','API',1,'/projects/api','local','missing','path_missing','folder not found')`); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s := &Repos{DB: db, GetProvider: func() provider.Provider {
		calls++
		return mock.NewMockProvider().WithRepos([]*provider.Repo{{Owner: "Acme", Name: "API"}, {Owner: "acme", Name: "web"}})
	}}
	cached, err := s.List(context.Background(), RepoListRequest{CacheOnly: true})
	if err != nil || calls != 0 || len(cached.Repos) != 1 {
		t.Fatalf("cache = %+v, %v, calls %d", cached, err, calls)
	}
	for range 2 {
		response, err := s.List(context.Background(), RepoListRequest{})
		if err != nil || len(response.Repos) != 2 {
			t.Fatalf("refresh = %+v, %v", response, err)
		}
		for _, repo := range response.Repos {
			if repo.ID == "github:acme/api" && (!repo.Watched || repo.Owner != "Acme" || repo.Repo != "API" || repo.LocalPath == nil || *repo.LocalPath != "/projects/api" || repo.Source == nil || *repo.Source != "local" || repo.LocationStatus == nil || *repo.LocationStatus != "missing" || repo.LocationErrorCode == nil || *repo.LocationErrorCode != "path_missing" || repo.LocationError == nil || *repo.LocationError != "folder not found") {
				t.Fatalf("location lost: %+v", repo)
			}
			if repo.ID == "github:acme/web" && repo.Source != nil {
				t.Fatalf("provider-only location: %+v", repo)
			}
		}
	}
}

func TestReposListContinuesAfterInsertFailure(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER reject_bad BEFORE INSERT ON repos WHEN NEW.id = 'github:acme/bad' BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	s := &Repos{DB: db, GetProvider: func() provider.Provider {
		return mock.NewMockProvider().WithRepos([]*provider.Repo{{Owner: "acme", Name: "bad"}, {Owner: "acme", Name: "api"}})
	}, Log: testRepoLogger()}
	response, err := s.List(context.Background(), RepoListRequest{})
	if err != nil || len(response.Repos) != 1 || response.Repos[0].ID != "github:acme/api" {
		t.Fatalf("response = %+v, %v", response, err)
	}
}

func TestReposSetWatched(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO repos(id,provider,owner,repo) VALUES('github:acme/api','github','acme','api')`); err != nil {
		t.Fatal(err)
	}
	queue := make(chan string, 1)
	pending := new(atomic.Bool)
	s := &Repos{DB: db, SyncTrigger: WatchSyncTrigger{Queue: queue, Pending: pending}}
	enabled, disabled := true, false
	response, err := s.SetWatched(context.Background(), SetWatchedRequest{RepoID: "github:Acme/API", Watched: &enabled})
	if err != nil || !response.Watched || response.RepoID != "github:acme/api" {
		t.Fatalf("watch = %+v, %v", response, err)
	}
	if got := <-queue; got != response.RepoID {
		t.Fatalf("trigger %q", got)
	}
	_, err = s.SetWatched(context.Background(), SetWatchedRequest{RepoID: response.RepoID, Watched: &enabled})
	if err != nil || len(queue) != 0 {
		t.Fatalf("idempotent watch triggered: %v", err)
	}
	response, err = s.SetWatched(context.Background(), SetWatchedRequest{RepoID: response.RepoID, Watched: &disabled})
	if err != nil || response.Watched || len(queue) != 0 {
		t.Fatalf("unwatch = %+v, %v", response, err)
	}
	queue <- "busy"
	response, err = s.SetWatched(context.Background(), SetWatchedRequest{RepoID: response.RepoID})
	if err != nil || !response.Watched || !pending.Load() {
		t.Fatalf("toggle overflow = %+v, %v", response, err)
	}
	var watched int
	if err := db.QueryRow(`SELECT watched FROM repos`).Scan(&watched); err != nil || watched != 1 {
		t.Fatalf("stored = %d, %v", watched, err)
	}
	_, err = s.SetWatched(context.Background(), SetWatchedRequest{})
	assertServiceCode(t, err, CodeInvalid)
	_, err = s.SetWatched(context.Background(), SetWatchedRequest{RepoID: "missing"})
	assertServiceCode(t, err, CodeNotFound)
}
