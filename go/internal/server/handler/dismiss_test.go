package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/itterno/bistro/internal/store/dismissal"
	"testing"

	"github.com/itterno/bistro/internal/actions"

	"github.com/itterno/bistro/internal/ws"
	_ "github.com/mattn/go-sqlite3"
)

// fakeApplier records applied actions and always succeeds (queue semantics
// are the orchestrator's concern, not this handler's).
type fakeApplier struct {
	applied []*actions.ActionRequest
}

func (f *fakeApplier) ApplyAction(_ context.Context, req *actions.ActionRequest) *actions.ActionResult {
	f.applied = append(f.applied, req)
	return &actions.ActionResult{PRID: req.PRID, Action: req.Action, Success: true}
}

func setupDismissTestDB(t *testing.T) *sql.DB {
	t.Helper()
	// file::memory:?cache=shared + a single connection: a plain ":memory:"
	// DSN gives every pooled connection its own empty database, which flakes
	// as soon as database/sql opens a second connection.
	db, err := sql.Open("sqlite3", "file:"+t.Name()+"?mode=memory&cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	stmts := []string{
		`CREATE TABLE comments (
			id TEXT PRIMARY KEY, pr_id TEXT NOT NULL, author TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL DEFAULT '', thread_id TEXT,
			created_at DATETIME NOT NULL DEFAULT '2026-06-01T10:00:00Z',
			updated_at DATETIME NOT NULL DEFAULT '2026-06-01T10:00:00Z'
		)`,
		`CREATE TABLE review_threads (
			id TEXT PRIMARY KEY, pr_id TEXT NOT NULL,
			resolved INTEGER NOT NULL DEFAULT 0, state_known INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE comment_dismissals (
			comment_id TEXT NOT NULL,
			account_provider TEXT NOT NULL DEFAULT 'github',
			account_login TEXT NOT NULL DEFAULT '',
			dismissed_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
			PRIMARY KEY (comment_id, account_provider, account_login),
			FOREIGN KEY(comment_id) REFERENCES comments(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE current_user (provider TEXT PRIMARY KEY, login TEXT NOT NULL)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	// Dismissals require a signed-in account (per-account namespacing).
	if _, err := db.Exec(`INSERT INTO current_user (provider, login) VALUES ('github','samdev')`); err != nil {
		t.Fatalf("seed current_user: %v", err)
	}
	return db
}

func dismissDeps(db *sql.DB, applier ActionApplier, scope string) DismissDeps {
	return DismissDeps{
		DB:         db,
		Dismissals: dismissal.New(db),
		GetApplier: func() ActionApplier { return applier },
		Scope:      func() string { return scope },
		Log:        testRepoLogger(),
	}
}

func callDismiss(t *testing.T, deps DismissDeps, prID string, ids []string) map[string]interface{} {
	t.Helper()
	payload, _ := json.Marshal(map[string]interface{}{"pr_id": prID, "comment_ids": ids})
	resp, err := NewDismissCommentsHandler(deps)(context.Background(),
		ws.InboundMessage{ID: "r1", Type: "DISMISS_COMMENTS", Data: payload})
	if err != nil {
		t.Fatalf("dismiss handler: %v", err)
	}
	env := resp.(map[string]interface{})
	if env["type"] != "DISMISS_RESULT" {
		t.Fatalf("response type = %v, want DISMISS_RESULT", env["type"])
	}
	return env["data"].(map[string]interface{})
}

func dismissalRows(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.Query(`SELECT comment_id, account_provider || '/' || account_login FROM comment_dismissals`)
	if err != nil {
		t.Fatalf("query dismissals: %v", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, acct string
		if err := rows.Scan(&id, &acct); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[id] = acct
	}
	return out
}

func TestDismissLocalOnly(t *testing.T) {
	db := setupDismissTestDB(t)
	applier := &fakeApplier{}
	deps := dismissDeps(db, applier, "local")
	mustExec(t, db, `INSERT INTO comments (id, pr_id, thread_id) VALUES ('c1','pr-1','t1'), ('c2','pr-1',NULL)`)
	mustExec(t, db, `INSERT INTO review_threads (id, pr_id, state_known, resolved) VALUES ('t1','pr-1',1,0)`)

	data := callDismiss(t, deps, "pr-1", []string{"c1", "c2"})

	if got := dismissalRows(t, db); len(got) != 2 {
		t.Errorf("dismissal rows = %v, want 2", got)
	}
	if len(applier.applied) != 0 {
		t.Errorf("local scope must enqueue nothing, got %d actions", len(applier.applied))
	}
	if ids := data["resolved_thread_ids"].([]string); len(ids) != 0 {
		t.Errorf("resolved_thread_ids = %v, want empty", ids)
	}
	if data["batch_id"] == "" {
		t.Errorf("batch_id missing")
	}
}

func TestDismissWithProviderWriteback(t *testing.T) {
	db := setupDismissTestDB(t)
	applier := &fakeApplier{}
	deps := dismissDeps(db, applier, "local+provider")
	// Three threads: resolvable, already resolved, state unknown.
	mustExec(t, db, `INSERT INTO review_threads (id, pr_id, state_known, resolved) VALUES
		('t-open','pr-1',1,0), ('t-resolved','pr-1',1,1), ('t-unknown','pr-1',0,0)`)
	mustExec(t, db, `INSERT INTO comments (id, pr_id, thread_id) VALUES
		('c1','pr-1','t-open'), ('c2','pr-1','t-resolved'), ('c3','pr-1','t-unknown')`)

	data := callDismiss(t, deps, "pr-1", []string{"c1", "c2", "c3"})

	ids := data["resolved_thread_ids"].([]string)
	if len(ids) != 1 || ids[0] != "t-open" {
		t.Errorf("resolved_thread_ids = %v, want [t-open]", ids)
	}
	if len(applier.applied) != 1 || applier.applied[0].Action != actions.ResolveThreadCommand || applier.applied[0].ThreadID != "t-open" {
		t.Errorf("applied actions = %+v, want one ResolveThread for t-open", applier.applied)
	}
	if got := dismissalRows(t, db); len(got) != 3 {
		t.Errorf("all three comments must be locally dismissed, got %v", got)
	}
}

func TestDismissIssueCommentsStayLocal(t *testing.T) {
	db := setupDismissTestDB(t)
	applier := &fakeApplier{}
	deps := dismissDeps(db, applier, "local+provider")
	mustExec(t, db, `INSERT INTO comments (id, pr_id, thread_id) VALUES ('c1','pr-1',NULL)`)

	data := callDismiss(t, deps, "pr-1", []string{"c1"})

	if len(applier.applied) != 0 {
		t.Errorf("threadless comment must not enqueue actions: %+v", applier.applied)
	}
	if got := dismissalRows(t, db); !reflectHas(got, "c1") {
		t.Errorf("comment not dismissed locally: %v", got)
	}
	if ids := data["resolved_thread_ids"].([]string); len(ids) != 0 {
		t.Errorf("resolved_thread_ids = %v, want empty", ids)
	}
}

func TestUndismiss(t *testing.T) {
	db := setupDismissTestDB(t)
	applier := &fakeApplier{}
	deps := dismissDeps(db, applier, "local+provider")
	mustExec(t, db, `INSERT INTO comments (id, pr_id, thread_id) VALUES ('c1','pr-1','t1'), ('c2','pr-1','t2')`)
	mustExec(t, db, `INSERT INTO review_threads (id, pr_id, state_known, resolved) VALUES ('t1','pr-1',1,0), ('t2','pr-1',1,0)`)
	mustExec(t, db, `INSERT INTO comment_dismissals (comment_id, account_provider, account_login) VALUES ('c1','github','samdev'), ('c2','github','samdev')`)

	// Undo lists ONLY t1 — t2 must not be unresolved even though c2 is undismissed.
	payload, _ := json.Marshal(map[string]interface{}{
		"comment_ids":          []string{"c1", "c2"},
		"unresolve_thread_ids": []string{"t1"},
	})
	resp, err := NewUndismissCommentsHandler(deps)(context.Background(),
		ws.InboundMessage{ID: "r2", Type: "UNDISMISS_COMMENTS", Data: payload})
	if err != nil {
		t.Fatalf("undismiss handler: %v", err)
	}

	if got := dismissalRows(t, db); len(got) != 0 {
		t.Errorf("dismissal rows remain: %v", got)
	}
	if len(applier.applied) != 1 || applier.applied[0].Action != actions.UnresolveThreadCommand || applier.applied[0].ThreadID != "t1" {
		t.Errorf("applied = %+v, want exactly one UnresolveThread for t1", applier.applied)
	}
	env := resp.(map[string]interface{})
	if env["type"] != "UNDISMISS_RESULT" {
		t.Errorf("response type = %v", env["type"])
	}
}

func TestDismissResolvesCurrentAccount(t *testing.T) {
	db := setupDismissTestDB(t)
	deps := dismissDeps(db, &fakeApplier{}, "local")
	mustExec(t, db, `INSERT INTO comments (id, pr_id) VALUES ('c1','pr-1')`)

	callDismiss(t, deps, "pr-1", []string{"c1"})

	got := dismissalRows(t, db)
	if got["c1"] != "github/samdev" {
		t.Errorf("dismissal account = %q, want github/samdev", got["c1"])
	}
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %s: %v", query, err)
	}
}

func reflectHas(m map[string]string, k string) bool {
	_, ok := m[k]
	return ok
}
