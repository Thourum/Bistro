package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/samuelasvanyi/bistro/internal/provider"
	"github.com/samuelasvanyi/bistro/internal/provider/mock"
	"github.com/samuelasvanyi/bistro/internal/ws"
)

// setupRepoTestDB creates an in-memory SQLite DB with the repos table.
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
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
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

	handler := newRequestRepoListHandler(func() provider.Provider { return nil }, db, testRepoLogger())

	msg := ws.InboundMessage{ID: "req-1", Type: "REQUEST_REPO_LIST"}
	resp, err := handler(msg)
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
	handler := newRequestRepoListHandler(getProvider, db, testRepoLogger())

	msg := ws.InboundMessage{ID: "req-2", Type: "REQUEST_REPO_LIST"}
	resp, err := handler(msg)
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
}

func TestRequestRepoListHandler_ProviderError(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	errProv := mock.NewMockProvider().WithError(errors.New("api error"))
	getProvider := func() provider.Provider { return errProv }
	handler := newRequestRepoListHandler(getProvider, db, testRepoLogger())

	msg := ws.InboundMessage{ID: "req-3", Type: "REQUEST_REPO_LIST"}
	resp, err := handler(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := resp.(map[string]interface{})
	data := m["data"].(map[string]interface{})
	if data["error"] == nil {
		t.Error("expected error field when provider fails")
	}
}

func TestToggleWatchHandler_TogglesWatch(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	// Insert a repo to toggle.
	_, err := db.Exec(`INSERT INTO repos (id, provider, owner, repo) VALUES ('github:acme/api', 'github', 'acme', 'api')`)
	if err != nil {
		t.Fatalf("insert repo: %v", err)
	}

	syncCh := make(chan string, 8)
	handler := newToggleWatchHandler(db, syncCh, testRepoLogger())

	payload, _ := json.Marshal(map[string]string{"repo_id": "github:acme/api"})
	msg := ws.InboundMessage{ID: "req-4", Type: "TOGGLE_WATCH_REPO", Data: payload}

	resp, err := handler(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := resp.(map[string]interface{})
	if m["type"] != "WATCH_REPO_RESULT" {
		t.Errorf("expected WATCH_REPO_RESULT, got %v", m["type"])
	}
	data := m["data"].(map[string]interface{})
	if !data["watched"].(bool) {
		t.Error("expected watched=true after first toggle")
	}

	// Verify sync was triggered.
	select {
	case repoID := <-syncCh:
		if repoID != "github:acme/api" {
			t.Errorf("expected github:acme/api on sync channel, got %s", repoID)
		}
	default:
		t.Error("expected sync trigger after watch enabled")
	}

	// Toggle again — should set to unwatched, no sync trigger.
	resp2, err := handler(msg)
	if err != nil {
		t.Fatalf("unexpected error on second toggle: %v", err)
	}
	m2 := resp2.(map[string]interface{})
	data2 := m2["data"].(map[string]interface{})
	if data2["watched"].(bool) {
		t.Error("expected watched=false after second toggle")
	}
}

func TestToggleWatchHandler_MissingRepoID(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	syncCh := make(chan string, 8)
	handler := newToggleWatchHandler(db, syncCh, testRepoLogger())

	payload, _ := json.Marshal(map[string]string{})
	msg := ws.InboundMessage{ID: "req-5", Type: "TOGGLE_WATCH_REPO", Data: payload}

	_, err := handler(msg)
	if err == nil {
		t.Error("expected error for missing repo_id")
	}
}

func TestToggleWatchHandler_InvalidPayload(t *testing.T) {
	db := setupRepoTestDB(t)
	defer db.Close()

	syncCh := make(chan string, 8)
	handler := newToggleWatchHandler(db, syncCh, testRepoLogger())

	msg := ws.InboundMessage{ID: "req-6", Type: "TOGGLE_WATCH_REPO", Data: json.RawMessage(`not-json`)}

	_, err := handler(msg)
	if err == nil {
		t.Error("expected error for invalid payload")
	}
}
