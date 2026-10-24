package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
	_ "github.com/mattn/go-sqlite3"
)

func setupRepoTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE repos (
		id         TEXT PRIMARY KEY,
		provider   TEXT NOT NULL,
		owner      TEXT NOT NULL,
		repo       TEXT NOT NULL,
		watched    INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
		local_path TEXT,
		source TEXT,
		last_used_at DATETIME,
		location_status TEXT,
		location_error_code TEXT,
		location_error TEXT
	)`)
	if err != nil {
		t.Fatalf("create repos table: %v", err)
	}
	return db
}

func testRepoLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestRequestRepoListHandler_NotAuthenticated(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	handler := NewRequestRepoListHandler(func() provider.Provider { return nil }, db, testRepoLogger())

	msg := ws.InboundMessage{ID: "req-1", Type: "REQUEST_REPO_LIST"}
	resp, err := handler(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := resp.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map response, got %T", resp)
	}
	if m["type"] != "REPO_LIST_READY" {
		t.Errorf("expected REPO_LIST_READY, got %v", m["type"])
	}
	data := m["data"].(map[string]interface{})
	if data["error"] == nil || data["error"] == "" {
		t.Error("expected error field for unauthenticated call")
	}
}

func TestRequestRepoListHandler_ReturnsRepos(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	prov := mock.NewMockProvider().WithRepos([]*provider.Repo{
		{Owner: "acme", Name: "api", FullName: "acme/api", Private: false},
		{Owner: "acme", Name: "web", FullName: "acme/web", Private: true},
	})

	getProvider := func() provider.Provider { return prov }
	handler := NewRequestRepoListHandler(getProvider, db, testRepoLogger())

	msg := ws.InboundMessage{ID: "req-2", Type: "REQUEST_REPO_LIST"}
	resp, err := handler(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := resp.(map[string]interface{})
	if m["type"] != "REPO_LIST_READY" {
		t.Errorf("expected REPO_LIST_READY, got %v", m["type"])
	}
	data := m["data"].(map[string]interface{})
	repos, ok := data["repos"].([]repoRow)
	if !ok {
		t.Fatalf("expected []repoRow, got %T", data["repos"])
	}
	if len(repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(repos))
	}
	for _, repo := range repos {
		if repo.Source != nil || repo.LocationStatus != nil || repo.LocationErrorCode != nil || repo.LocationError != nil {
			t.Fatalf("expected empty location fields for provider-only repo, got %+v", repo)
		}
	}
}

func TestRepoListMixedCaseDiscoveryPreservesCanonicalWatchedClone(t *testing.T) {
	database := setupRepoTestDB(t)
	defer database.Close()
	ctx := context.Background()
	locations := store.NewRepoLocationStore(database)
	if err := locations.UpsertLocal(ctx, "github", "Itterno", "Bistro", t.TempDir(), true); err != nil {
		t.Fatal(err)
	}
	queueProvider := &queueOperationProvider{read: func(context.Context) (*provider.ReviewQueue, error) {
		return &provider.ReviewQueue{Repositories: []*provider.Repo{{Owner: "Itterno", Name: "Bistro"}}}, nil
	}}
	if _, err := fetchAndPersistReviewQueue(ctx, database, queueProvider); err != nil {
		t.Fatal(err)
	}
	prov := mock.NewMockProvider().WithRepos([]*provider.Repo{{Owner: "Itterno", Name: "Bistro", FullName: "Itterno/Bistro"}})
	request := NewRequestRepoListHandler(func() provider.Provider { return prov }, database, testRepoLogger())
	for range 2 {
		reply, err := request(ctx, ws.InboundMessage{ID: "mixed-case", Type: "REQUEST_REPO_LIST"})
		if err != nil {
			t.Fatal(err)
		}
		repos := reply.(map[string]interface{})["data"].(map[string]interface{})["repos"].([]repoRow)
		if len(repos) != 1 || repos[0].ID != "github:itterno/bistro" || !repos[0].Watched || repos[0].LocalPath == nil {
			t.Fatalf("discovery duplicated or replaced the watched clone: %+v", repos)
		}
	}
}

func TestRequestRepoListHandler_ReturnsRepoLocationFields(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	_, err := db.Exec(
		`INSERT INTO repos (id, provider, owner, repo, watched, source, location_status, location_error_code, location_error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"github:acme/api", "github", "acme", "api", 1, "local", "missing", "path_missing", "folder not found",
	)
	if err != nil {
		t.Fatalf("insert repo: %v", err)
	}

	prov := mock.NewMockProvider().WithRepos([]*provider.Repo{
		{Owner: "acme", Name: "api", FullName: "acme/api", Private: false},
	})

	handler := NewRequestRepoListHandler(func() provider.Provider { return prov }, db, testRepoLogger())

	resp, err := handler(context.Background(), ws.InboundMessage{ID: "req-location", Type: "REQUEST_REPO_LIST"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := resp.(map[string]interface{})
	data := m["data"].(map[string]interface{})
	repos, ok := data["repos"].([]repoRow)
	if !ok {
		t.Fatalf("expected []repoRow, got %T", data["repos"])
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	got := repos[0]
	if got.Source == nil || *got.Source != "local" {
		t.Fatalf("expected source local, got %+v", got.Source)
	}
	if got.LocationStatus == nil || *got.LocationStatus != "missing" {
		t.Fatalf("expected location_status missing, got %+v", got.LocationStatus)
	}
	if got.LocationErrorCode == nil || *got.LocationErrorCode != "path_missing" {
		t.Fatalf("expected location_error_code path_missing, got %+v", got.LocationErrorCode)
	}
	if got.LocationError == nil || *got.LocationError != "folder not found" {
		t.Fatalf("expected location_error folder not found, got %+v", got.LocationError)
	}
}

func TestRequestRepoListHandler_DatabaseErrorDoesNotExposeSQL(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()
	if _, err := db.Exec(`DROP TABLE repos`); err != nil {
		t.Fatalf("drop repos table: %v", err)
	}

	prov := mock.NewMockProvider().WithRepos([]*provider.Repo{
		{Owner: "acme", Name: "api", FullName: "acme/api"},
	})
	handler := NewRequestRepoListHandler(func() provider.Provider { return prov }, db, testRepoLogger())

	resp, err := handler(context.Background(), ws.InboundMessage{ID: "req-db-error", Type: "REQUEST_REPO_LIST"})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	message := resp.(map[string]interface{})
	data := message["data"].(map[string]interface{})
	if data["error"] != "unable to load repositories" {
		t.Fatalf("expected safe repository-list error, got %v", data["error"])
	}
}

func TestRequestRepoListHandler_ProviderError(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	errProv := mock.NewMockProvider().WithError(errors.New("api error"))
	getProvider := func() provider.Provider { return errProv }
	handler := NewRequestRepoListHandler(getProvider, db, testRepoLogger())

	msg := ws.InboundMessage{ID: "req-3", Type: "REQUEST_REPO_LIST"}
	resp, err := handler(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := resp.(map[string]interface{})
	data := m["data"].(map[string]interface{})
	if data["error"] == nil {
		t.Error("expected error field when provider fails")
	}
}

func TestToggleWatchHandler_RespectsWatchedBooleanAndTriggersSyncWhenEnabled(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	_, err := db.Exec(`INSERT INTO repos (id, provider, owner, repo) VALUES ('github:acme/api', 'github', 'acme', 'api')`)
	if err != nil {
		t.Fatalf("insert repo: %v", err)
	}

	syncCh := make(chan string, 8)
	handler := NewToggleWatchHandler(db, WatchSyncTrigger{Queue: syncCh, Pending: new(atomic.Bool)}, testRepoLogger())

	payload, _ := json.Marshal(map[string]interface{}{"repo_id": "github:acme/api", "watched": true})
	msg := ws.InboundMessage{ID: "req-4", Type: "TOGGLE_WATCH_REPO", Data: payload}

	resp, err := handler(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := resp.(map[string]interface{})
	if m["type"] != "WATCH_REPO_RESULT" {
		t.Errorf("expected WATCH_REPO_RESULT, got %v", m["type"])
	}
	data := m["data"].(map[string]interface{})
	if !data["watched"].(bool) {
		t.Error("expected watched=true")
	}

	select {
	case repoID := <-syncCh:
		if repoID != "github:acme/api" {
			t.Errorf("expected github:acme/api on sync channel, got %s", repoID)
		}
	default:
		t.Error("expected sync trigger after watch enabled")
	}

	unwatchPayload, _ := json.Marshal(map[string]interface{}{"repo_id": "github:acme/api", "watched": false})
	unwatchMsg := ws.InboundMessage{ID: "req-4b", Type: "TOGGLE_WATCH_REPO", Data: unwatchPayload}
	resp2, err := handler(context.Background(), unwatchMsg)
	if err != nil {
		t.Fatalf("unexpected error on unwatch: %v", err)
	}
	m2 := resp2.(map[string]interface{})
	data2 := m2["data"].(map[string]interface{})
	if data2["watched"].(bool) {
		t.Error("expected watched=false")
	}
	select {
	case repoID := <-syncCh:
		t.Errorf("did not expect sync trigger after unwatch, got %s", repoID)
	default:
	}
}

func TestToggleWatchHandler_MissingWatchedFallsBackToToggle(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	_, err := db.Exec(`INSERT INTO repos (id, provider, owner, repo) VALUES ('github:acme/api', 'github', 'acme', 'api')`)
	if err != nil {
		t.Fatalf("insert repo: %v", err)
	}

	syncCh := make(chan string, 8)
	handler := NewToggleWatchHandler(db, WatchSyncTrigger{Queue: syncCh, Pending: new(atomic.Bool)}, testRepoLogger())

	payload, _ := json.Marshal(map[string]string{"repo_id": "github:acme/api"})
	msg := ws.InboundMessage{ID: "req-4c", Type: "TOGGLE_WATCH_REPO", Data: payload}

	resp, err := handler(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	if !data["watched"].(bool) {
		t.Error("expected missing watched payload to toggle to true")
	}
}

func TestToggleWatchHandler_MissingRepoID(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	syncCh := make(chan string, 8)
	handler := NewToggleWatchHandler(db, WatchSyncTrigger{Queue: syncCh, Pending: new(atomic.Bool)}, testRepoLogger())

	payload, _ := json.Marshal(map[string]string{})
	msg := ws.InboundMessage{ID: "req-5", Type: "TOGGLE_WATCH_REPO", Data: payload}

	_, err := handler(context.Background(), msg)
	if err == nil {
		t.Error("expected error for missing repo_id")
	}
}

func TestCacheOnlyRepoListIncludesCloneWithoutProvider(t *testing.T) {
	database := setupRepoTestDB(t)
	defer database.Close()
	_, err := database.Exec(`INSERT INTO repos(id,provider,owner,repo,watched,local_path,source)
        VALUES('github:acme/api','github','acme','api',1,'/projects/api','local')`)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	request := NewRequestRepoListHandler(func() provider.Provider { calls++; return nil }, database, testRepoLogger())
	reply, err := request(context.Background(), ws.InboundMessage{ID: "cached", Type: "REQUEST_REPO_LIST", Data: json.RawMessage(`{"cache_only":true}`)})
	if err != nil {
		t.Fatal(err)
	}
	data := reply.(map[string]interface{})["data"].(map[string]interface{})
	repos := data["repos"].([]repoRow)
	if calls != 0 || len(repos) != 1 || !repos[0].Watched || repos[0].LocalPath == nil || *repos[0].LocalPath != "/projects/api" {
		t.Fatalf("not a local cache read: calls=%d repos=%+v", calls, repos)
	}
}

func TestRepoListRejectsInvalidCacheFlagBeforeProvider(t *testing.T) {
	for _, payload := range []string{`{`, `[]`, `null`, `{"cache_only":"true"}`, `{"cache_only":null}`} {
		t.Run(payload, func(t *testing.T) {
			database := setupRepoTestDB(t)
			defer database.Close()
			request := NewRequestRepoListHandler(func() provider.Provider {
				t.Fatal("invalid payload must not reach provider")
				return nil
			}, database, testRepoLogger())
			reply, err := request(context.Background(), ws.InboundMessage{ID: "invalid", Data: json.RawMessage(payload)})
			if err != nil {
				t.Fatal(err)
			}
			data := reply.(map[string]interface{})["data"].(map[string]interface{})
			if data["error"] != "unable to load repositories" {
				t.Fatalf("unsafe error: %+v", data)
			}
		})
	}
}

func TestRepoListFalseCacheFlagStillRefreshes(t *testing.T) {
	database := setupRepoTestDB(t)
	defer database.Close()
	prov := mock.NewMockProvider().WithRepos([]*provider.Repo{{Owner: "acme", Name: "api", FullName: "acme/api"}})
	request := NewRequestRepoListHandler(func() provider.Provider { return prov }, database, testRepoLogger())
	reply, err := request(context.Background(), ws.InboundMessage{ID: "refresh", Data: json.RawMessage(`{"cache_only":false}`)})
	if err != nil {
		t.Fatal(err)
	}
	repos := reply.(map[string]interface{})["data"].(map[string]interface{})["repos"].([]repoRow)
	if len(repos) != 1 || repos[0].ID != "github:acme/api" {
		t.Fatalf("missing refresh: %+v", repos)
	}
}

func TestToggleWatchHandler_InvalidPayload(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	syncCh := make(chan string, 8)
	handler := NewToggleWatchHandler(db, WatchSyncTrigger{Queue: syncCh, Pending: new(atomic.Bool)}, testRepoLogger())

	msg := ws.InboundMessage{ID: "req-6", Type: "TOGGLE_WATCH_REPO", Data: json.RawMessage(`not-json`)}

	_, err := handler(context.Background(), msg)
	if err == nil {
		t.Error("expected error for invalid payload")
	}
}

func TestRequestRepoListHandler_InsertFailureStillReturnsOtherRepos(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER reject_bad BEFORE INSERT ON repos WHEN NEW.id = 'github:acme/bad'
		BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	prov := mock.NewMockProvider().WithRepos([]*provider.Repo{
		{Owner: "acme", Name: "bad", FullName: "acme/bad"},
		{Owner: "acme", Name: "api", FullName: "acme/api"},
	})
	handler := NewRequestRepoListHandler(func() provider.Provider { return prov }, db, testRepoLogger())
	resp, err := handler(context.Background(), ws.InboundMessage{ID: "req-bad", Type: "REQUEST_REPO_LIST"})
	if err != nil {
		t.Fatal(err)
	}
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	repos, ok := data["repos"].([]repoRow)
	if !ok || len(repos) != 1 || repos[0].ID != "github:acme/api" || data["error"] != nil {
		t.Fatalf("data = %+v", data)
	}
}
