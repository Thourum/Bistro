package handler

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

// newHandlerTestDB opens an :memory: SQLite DB with all migrations applied.
// Returns the underlying *sql.DB because handler code expects *sql.DB (via prLister).
func newHandlerTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("newHandlerTestDB open: %v", err)
	}
	if err := store.NewMigrationRunner(nil, db.DB).Run(context.Background()); err != nil {
		t.Fatalf("newHandlerTestDB migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db.DB
}

func TestRequestComments_ReturnsNewColumns(t *testing.T) {
	db := newHandlerTestDB(t)
	if _, err := db.Exec(`INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('p1','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','b','h')`); err != nil {
		t.Fatalf("insert pr: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO review_threads (id, pr_id) VALUES ('t1','p1')`); err != nil {
		t.Fatalf("insert review_thread: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, commit_sha, original_sha, side, position, original_position,
		 in_reply_to, thread_id, patchset_number)
		VALUES
		('c1','p1','alice','nit','2026-01-01','2026-01-01',
		 'review','sha-c','sha-o','RIGHT',5,5,
		 'github-review-comment-0','t1',NULL)`); err != nil {
		t.Fatalf("insert comment: %v", err)
	}

	h := NewRequestCommentsHandler(db, testRepoLogger())
	payload := []byte(`{"pr_id":"p1"}`)
	resp, err := h(context.Background(), ws.InboundMessage{
		ID:   "req-new",
		Type: "REQUEST_COMMENTS",
		Data: payload,
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	env, ok := resp.(map[string]interface{})
	if !ok {
		t.Fatalf("response is not a map")
	}
	data, ok := env["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not a map")
	}
	comments, ok := data["comments"].([]commentRow)
	if !ok {
		t.Fatalf("comments is %T, want []commentRow", data["comments"])
	}
	if len(comments) != 1 {
		t.Fatalf("got %d rows want 1", len(comments))
	}
	c := comments[0]
	if c.Kind == nil || *c.Kind != "review" {
		t.Errorf("Kind: got %v", c.Kind)
	}
	if c.CommitSHA == nil || *c.CommitSHA != "sha-c" {
		t.Errorf("CommitSHA: got %v", c.CommitSHA)
	}
	if c.ThreadID == nil || *c.ThreadID != "t1" {
		t.Errorf("ThreadID: got %v", c.ThreadID)
	}
	if c.Side == nil || *c.Side != "RIGHT" {
		t.Errorf("Side: got %v", c.Side)
	}
}

// ── Anchor fields in COMMENTS_READY (Phase 2.3) ───────────────────────────

func TestRequestComments_AnchoredTrueWhenHeadSHAMatches(t *testing.T) {
	db := newHandlerTestDB(t)
	// Insert a PR at head sha2.
	if _, err := db.Exec(`INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pa1','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','b','sha2')`); err != nil {
		t.Fatalf("insert pr: %v", err)
	}
	// Patchset row so the freshness join resolves to number=1.
	if _, err := db.Exec(`INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pa1', 1, 'sha2', 'b', '2026-01-01')`); err != nil {
		t.Fatalf("insert patchset: %v", err)
	}
	// Anchor computed on patchset 1: both head and patchset_number match.
	if _, err := db.Exec(`INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, commit_sha, anchored_line, anchor_head_sha, outdated, anchor_patchset_number, anchor_current_patchset_number)
		VALUES
		('c1','pa1','alice','lgtm','2026-01-01','2026-01-01',
		 'review','sha-commit',12,'sha2',0,1,1)`); err != nil {
		t.Fatalf("insert comment: %v", err)
	}

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, err := h(context.Background(), ws.InboundMessage{
		ID:   "req-anc",
		Type: "REQUEST_COMMENTS",
		Data: []byte(`{"pr_id":"pa1"}`),
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	c := comments[0]

	// anchor_head_sha == prs.head_sha → Anchored=true.
	if !c.Anchored {
		t.Errorf("Anchored should be true when anchor_head_sha matches prs.head_sha")
	}
	if c.AnchoredLine == nil || *c.AnchoredLine != 12 {
		t.Errorf("AnchoredLine: got %v want 12", c.AnchoredLine)
	}
	if c.Outdated {
		t.Error("Outdated should be false")
	}
}

func TestRequestComments_AnchoredFalseWhenHeadSHAStale(t *testing.T) {
	db := newHandlerTestDB(t)
	// PR at head 'sha3' but comment cached at 'sha2' (old head).
	if _, err := db.Exec(`INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pa2','github','o','r',2,'T','a','open','2026-01-01','2026-01-01','main','f','b','sha3')`); err != nil {
		t.Fatalf("insert pr: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, commit_sha, anchored_line, anchor_head_sha, outdated)
		VALUES
		('c2','pa2','bob','comment','2026-01-01','2026-01-01',
		 'review','sha-commit',7,'sha2',0)`); err != nil {
		t.Fatalf("insert comment: %v", err)
	}

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, _ := h(context.Background(), ws.InboundMessage{
		ID: "req-stale", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pa2"}`),
	})
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	// anchor_head_sha ('sha2') != prs.head_sha ('sha3') → Anchored=false.
	if comments[0].Anchored {
		t.Error("Anchored should be false when anchor_head_sha is stale (sha2 != sha3)")
	}
}

func TestRequestComments_AnchoredFalseWhenNullAnchor(t *testing.T) {
	db := newHandlerTestDB(t)
	// Comment with no anchor yet (anchored_line is NULL).
	if _, err := db.Exec(`INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pa3','github','o','r',3,'T','a','open','2026-01-01','2026-01-01','main','f','b','sha4')`); err != nil {
		t.Fatalf("insert pr: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at, kind)
		VALUES ('c3','pa3','carol','todo','2026-01-01','2026-01-01','review')`); err != nil {
		t.Fatalf("insert comment: %v", err)
	}

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, _ := h(context.Background(), ws.InboundMessage{
		ID: "req-null", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pa3"}`),
	})
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	// anchored_line is NULL → Anchored must be false.
	if comments[0].Anchored {
		t.Error("Anchored should be false when anchored_line is NULL")
	}
}

func TestRequestComments_OutdatedComment(t *testing.T) {
	db := newHandlerTestDB(t)
	if _, err := db.Exec(`INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pa4','github','o','r',4,'T','a','open','2026-01-01','2026-01-01','main','f','b','sha5')`); err != nil {
		t.Fatalf("insert pr: %v", err)
	}
	// Patchset row number=1 for the current PR state.
	if _, err := db.Exec(`INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pa4', 1, 'sha5', 'b', '2026-01-01')`); err != nil {
		t.Fatalf("insert patchset: %v", err)
	}
	// Anchor: outdated with anchor_patchset_number=1 matching current patchset.
	if _, err := db.Exec(`INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, anchor_head_sha, outdated, outdated_near_line, anchor_patchset_number, anchor_current_patchset_number)
		VALUES
		('c4','pa4','dave','old','2026-01-01','2026-01-01',
		 'review','sha5',1,9,1,1)`); err != nil {
		t.Fatalf("insert comment: %v", err)
	}

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, _ := h(context.Background(), ws.InboundMessage{
		ID: "req-out", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pa4"}`),
	})
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1, got %d", len(comments))
	}
	c := comments[0]
	if !c.Outdated {
		t.Error("Outdated should be true")
	}
	if c.OutdatedNearLine == nil || *c.OutdatedNearLine != 9 {
		t.Errorf("OutdatedNearLine: got %v want 9", c.OutdatedNearLine)
	}
	// anchored_line is NULL → Anchored=false even though anchor_head_sha matches.
	if c.Anchored {
		t.Error("Anchored should be false for outdated comment (no anchored_line)")
	}
}

// ── Stale cache suppression (blocker 2 fix) ───────────────────────────────

// TestRequestComments_StaleCacheSuppressesAllAnchorFields verifies that when
// anchor_head_sha does NOT match the current prs.head_sha (stale cache), ALL
// anchor-derived fields are zeroed in the response — including a stale outdated=true
// that must not leak to the client.
func TestRequestComments_StaleCacheSuppressesAllAnchorFields(t *testing.T) {
	db := newHandlerTestDB(t)
	// PR is currently at sha-NEW but comment was anchored against sha-OLD.
	if _, err := db.Exec(`INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('ps1','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','b','sha-NEW')`); err != nil {
		t.Fatalf("insert pr: %v", err)
	}
	// Comment with stale anchor: anchor_head_sha='sha-OLD' but PR is now at 'sha-NEW'.
	// Also has outdated=1 and outdated_near_line=5 — these must NOT appear in response.
	if _, err := db.Exec(`INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, commit_sha, anchored_line, anchor_head_sha, outdated, outdated_near_line)
		VALUES
		('sc1','ps1','alice','stale','2026-01-01','2026-01-01',
		 'review','sha-commit',7,'sha-OLD',1,5)`); err != nil {
		t.Fatalf("insert stale comment: %v", err)
	}

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, _ := h(context.Background(), ws.InboundMessage{
		ID: "req-stale2", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"ps1"}`),
	})
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	c := comments[0]

	// All anchor-derived fields must be zeroed/nil when cache is stale.
	if c.Anchored {
		t.Error("Anchored must be false for stale cache")
	}
	if c.AnchoredLine != nil {
		t.Errorf("AnchoredLine must be nil for stale cache, got %d", *c.AnchoredLine)
	}
	if c.AnchorHeadSHA != nil {
		t.Errorf("AnchorHeadSHA must be nil for stale cache, got %q", *c.AnchorHeadSHA)
	}
	if c.Outdated {
		t.Error("Outdated must be false for stale cache (stale outdated=true must not leak)")
	}
	if c.OutdatedNearLine != nil {
		t.Errorf("OutdatedNearLine must be nil for stale cache, got %d", *c.OutdatedNearLine)
	}
}

// TestRequestComments_FreshCacheShowsAnchorFields confirms that a fresh cache
// (anchor_head_sha == prs.head_sha AND anchor_patchset_number == current patchset)
// exposes all anchor fields correctly.
func TestRequestComments_FreshCacheShowsAnchorFields(t *testing.T) {
	db := newHandlerTestDB(t)
	if _, err := db.Exec(`INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pf1','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','b','sha-X')`); err != nil {
		t.Fatalf("insert pr: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pf1', 1, 'sha-X', 'b', '2026-01-01')`); err != nil {
		t.Fatalf("insert patchset: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, commit_sha, anchored_line, anchor_head_sha, outdated, anchor_patchset_number, anchor_current_patchset_number)
		VALUES
		('fc1','pf1','bob','fresh','2026-01-01','2026-01-01',
		 'review','sha-commit',15,'sha-X',0,1,1)`); err != nil {
		t.Fatalf("insert fresh comment: %v", err)
	}

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, _ := h(context.Background(), ws.InboundMessage{
		ID: "req-fresh", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pf1"}`),
	})
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	c := comments[0]

	if !c.Anchored {
		t.Error("Anchored must be true for fresh cache")
	}
	if c.AnchoredLine == nil || *c.AnchoredLine != 15 {
		t.Errorf("AnchoredLine: got %v want 15", c.AnchoredLine)
	}
	if c.AnchorHeadSHA == nil || *c.AnchorHeadSHA != "sha-X" {
		t.Errorf("AnchorHeadSHA: got %v want sha-X", c.AnchorHeadSHA)
	}
}

// TestRequestComments_StaleOutdatedTrue_NotLeaked is a focused regression test:
// a comment that was marked outdated=true in a prior sync (anchor_head_sha='sha-OLD')
// must show outdated=false after a force-push moves the PR head to 'sha-FORCE'.
func TestRequestComments_StaleOutdatedTrue_NotLeaked(t *testing.T) {
	db := newHandlerTestDB(t)
	if _, err := db.Exec(`INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pl1','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','b','sha-FORCE')`); err != nil {
		t.Fatalf("insert pr: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, anchor_head_sha, outdated, outdated_near_line)
		VALUES
		('ol1','pl1','dave','stale-out','2026-01-01','2026-01-01',
		 'review','sha-OLD',1,11)`); err != nil {
		t.Fatalf("insert stale outdated comment: %v", err)
	}

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, _ := h(context.Background(), ws.InboundMessage{
		ID: "req-leak", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pl1"}`),
	})
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1, got %d", len(comments))
	}
	c := comments[0]
	if c.Outdated {
		t.Error("stale outdated=true must NOT leak when anchor_head_sha != prs.head_sha")
	}
	if c.OutdatedNearLine != nil {
		t.Errorf("stale outdated_near_line must be nil, got %d", *c.OutdatedNearLine)
	}
}

// ── Blocker 2: anchor_patchset_number freshness tests ─────────────────────

// mustExecDB is a test helper for handler tests that use a *sql.DB.
func mustExecDB(t *testing.T, db *sql.DB, q string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("mustExecDB(%q): %v", q, err)
	}
}

// TestRequestComments_RetargetStaleAnchor_Suppressed is the primary regression
// test for the anchor_patchset_number blocker: same head SHA under different
// base (retarget) must not return the old anchor as fresh.
//
// P1: base=sha-BASE-OLD, head=sha-HEAD (anchor computed here, patchset_number=1)
// P2: base=sha-BASE-NEW, head=sha-HEAD (retarget, new patchset number=2)
// PR now at P2. Anchor anchor_patchset_number=1 but current patchset=2 → stale.
func TestRequestComments_RetargetStaleAnchor_Suppressed(t *testing.T) {
	db := newHandlerTestDB(t)

	// PR retargeted: same head, new base → new patchset.
	mustExecDB(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pr-ret','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f',
		        'sha-BASE-NEW','sha-HEAD')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-ret', 1, 'sha-HEAD', 'sha-BASE-OLD', '2026-01-01')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-ret', 2, 'sha-HEAD', 'sha-BASE-NEW', '2026-01-01')`)
	// Comment: anchor computed on P1, anchor_patchset_number=1, anchor_head_sha=sha-HEAD.
	// Current PR is on P2. head matches, but patchset number does NOT.
	mustExecDB(t, db, `INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, anchored_line, anchor_head_sha, outdated, anchor_patchset_number)
		VALUES
		('c-ret','pr-ret','alice','nit','2026-01-01','2026-01-01',
		 'review', 7, 'sha-HEAD', 0, 1)`)

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, err := h(context.Background(), ws.InboundMessage{
		ID: "req-ret", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pr-ret"}`),
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	c := comments[0]

	// anchor_head_sha matches BUT anchor_patchset_number=1 != current=2 → STALE.
	if c.Anchored {
		t.Error("Anchored must be false: head matches but patchset number is stale (retarget)")
	}
	if c.AnchoredLine != nil {
		t.Errorf("AnchoredLine must be nil for stale retarget anchor, got %d", *c.AnchoredLine)
	}
	if c.AnchorPatchsetNumber != nil {
		t.Errorf("AnchorPatchsetNumber must be nil for stale, got %d", *c.AnchorPatchsetNumber)
	}
}

// TestRequestComments_FreshAnchorAfterRetarget_Passes verifies that after a retarget
// and re-resolution on the new patchset, the anchor IS returned as fresh.
func TestRequestComments_FreshAnchorAfterRetarget_Passes(t *testing.T) {
	db := newHandlerTestDB(t)

	mustExecDB(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pr-fresh','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f',
		        'sha-BASE-NEW','sha-HEAD')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-fresh', 1, 'sha-HEAD', 'sha-BASE-OLD', '2026-01-01')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-fresh', 2, 'sha-HEAD', 'sha-BASE-NEW', '2026-01-01')`)
	// Anchor re-computed on P2: anchor_patchset_number=2, anchor_head_sha=sha-HEAD.
	mustExecDB(t, db, `INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, anchored_line, anchor_head_sha, outdated, anchor_patchset_number, anchor_current_patchset_number)
		VALUES
		('c-fresh','pr-fresh','alice','nit','2026-01-01','2026-01-01',
		 'review', 9, 'sha-HEAD', 0, 2, 2)`)

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, _ := h(context.Background(), ws.InboundMessage{
		ID: "req-fresh2", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pr-fresh"}`),
	})
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1, got %d", len(comments))
	}
	c := comments[0]

	// anchor_head_sha=sha-HEAD == prs.head_sha AND anchor_patchset_number=2 == current=2 → FRESH.
	if !c.Anchored {
		t.Error("Anchored must be true: both head and patchset number match after re-resolution")
	}
	if c.AnchoredLine == nil || *c.AnchoredLine != 9 {
		t.Errorf("AnchoredLine: got %v want 9", c.AnchoredLine)
	}
}

// TestRequestComments_NullAnchorPatchsetNumber_Stale verifies that a comment with
// anchor_patchset_number=NULL (anchor computed before migration 018) is treated
// as stale and all anchor fields are suppressed.
func TestRequestComments_NullAnchorPatchsetNumber_Stale(t *testing.T) {
	db := newHandlerTestDB(t)

	mustExecDB(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pr-null','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f',
		        'sha-BASE','sha-HEAD')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-null', 1, 'sha-HEAD', 'sha-BASE', '2026-01-01')`)
	// anchor_patchset_number is NULL — pre-migration 018 anchor.
	mustExecDB(t, db, `INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, anchored_line, anchor_head_sha, outdated)
		VALUES
		('c-null','pr-null','alice','old','2026-01-01','2026-01-01',
		 'review', 5, 'sha-HEAD', 0)`)

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, _ := h(context.Background(), ws.InboundMessage{
		ID: "req-null2", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pr-null"}`),
	})
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1, got %d", len(comments))
	}
	c := comments[0]
	// anchor_patchset_number is NULL → cannot verify identity → stale.
	if c.Anchored {
		t.Error("Anchored must be false when anchor_patchset_number is NULL")
	}
	if c.AnchoredLine != nil {
		t.Errorf("AnchoredLine must be nil for pre-migration anchor, got %d", *c.AnchoredLine)
	}
}

// ── Original-position freshness tests (blocker fix) ───────────────────────

// TestRequestComments_OriginalPositionAnchor_Fresh is the primary regression
// test: a comment from P1 re-resolved against P2.head must be returned as
// fresh (Case B freshness: anchor_patchset_number == comments.patchset_number).
//
// Setup: PR currently on P2 (head=sha-P2, base=b). Comment has patchset_number=1
// (was made on P1). Anchor was re-resolved against P2.head with correct P1
// geometry, so anchor_patchset_number=1 and anchor_head_sha=sha-P2.
func TestRequestComments_OriginalPositionAnchor_Fresh(t *testing.T) {
	db := newHandlerTestDB(t)

	mustExecDB(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pr-op','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','b','sha-P2')`)
	// P1 and P2 patchsets.
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-op', 1, 'sha-P1', 'b', '2026-01-01')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-op', 2, 'sha-P2', 'b', '2026-01-01')`)
	// Comment: from P1, re-resolved against P2.head.
	// anchor_patchset_number=1 (source patchset), anchor_head_sha=sha-P2 (current head).
	mustExecDB(t, db, `INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, patchset_number, anchored_line, anchor_head_sha, outdated, anchor_patchset_number, anchor_current_patchset_number)
		VALUES
		('c-op','pr-op','alice','nit','2026-01-01','2026-01-01',
		 'review', 1, 7, 'sha-P2', 0, 1, 2)`)

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, err := h(context.Background(), ws.InboundMessage{
		ID: "req-op", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pr-op"}`),
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	c := comments[0]

	// Case B: anchor_patchset_number(1) == comments.patchset_number(1) AND
	//         anchor_head_sha(sha-P2) == prs.head_sha(sha-P2) → FRESH.
	if !c.Anchored {
		t.Error("Anchored must be true: original-position re-anchored against current head with correct source patchset")
	}
	if c.AnchoredLine == nil || *c.AnchoredLine != 7 {
		t.Errorf("AnchoredLine: got %v want 7", c.AnchoredLine)
	}
}

// TestRequestComments_OriginalPositionAnchor_WrongAnchorPatchset_Stale verifies
// that an original-position comment is suppressed when anchor_patchset_number
// doesn't match comments.patchset_number (wrong geometry was used for anchor).
func TestRequestComments_OriginalPositionAnchor_WrongAnchorPatchset_Stale(t *testing.T) {
	db := newHandlerTestDB(t)

	mustExecDB(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pr-op2','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','b','sha-P2')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-op2', 1, 'sha-P1', 'b', '2026-01-01')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-op2', 2, 'sha-P2', 'b', '2026-01-01')`)
	// Comment from P1, but anchor_patchset_number=2 (wrong: resolved with current patchset
	// geometry instead of original P1 geometry).
	mustExecDB(t, db, `INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, patchset_number, anchored_line, anchor_head_sha, outdated, anchor_patchset_number)
		VALUES
		('c-op2','pr-op2','alice','nit','2026-01-01','2026-01-01',
		 'review', 1, 9, 'sha-P2', 0, 2)`)

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, _ := h(context.Background(), ws.InboundMessage{
		ID: "req-op2", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pr-op2"}`),
	})
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1, got %d", len(comments))
	}
	c := comments[0]

	// anchor_patchset_number(2) != comments.patchset_number(1) → STALE.
	if c.Anchored {
		t.Error("Anchored must be false: anchor_patchset_number doesn't match source patchset")
	}
	if c.AnchoredLine != nil {
		t.Errorf("AnchoredLine must be nil for wrong-geometry anchor, got %d", *c.AnchoredLine)
	}
}

// TestRequestComments_CurrentPositionAnchor_Fresh confirms Case A still works:
// current-position comment with anchor_patchset_number == current_patchset_number.
func TestRequestComments_CurrentPositionAnchor_Fresh(t *testing.T) {
	db := newHandlerTestDB(t)

	mustExecDB(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pr-cp','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','b','sha-P3')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-cp', 3, 'sha-P3', 'b', '2026-01-01')`)
	// Current-position comment: patchset_number=3 matches current patchset.
	mustExecDB(t, db, `INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, patchset_number, anchored_line, anchor_head_sha, outdated, anchor_patchset_number, anchor_current_patchset_number)
		VALUES
		('c-cp','pr-cp','bob','good','2026-01-01','2026-01-01',
		 'review', 3, 11, 'sha-P3', 0, 3, 3)`)

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, _ := h(context.Background(), ws.InboundMessage{
		ID: "req-cp", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pr-cp"}`),
	})
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1, got %d", len(comments))
	}
	c := comments[0]

	// Case A: anchor_patchset_number(3) == current_patchset_number(3) → FRESH.
	if !c.Anchored {
		t.Error("Anchored must be true for current-position comment with matching patchset")
	}
	if c.AnchoredLine == nil || *c.AnchoredLine != 11 {
		t.Errorf("AnchoredLine: got %v want 11", c.AnchoredLine)
	}
}

// TestRequestComments_SameHeadRetarget_CurrentPosition_Stale verifies that
// Case A (current-position) still suppresses anchors after a retarget.
// Same head, new base → new patchset number; old anchor was computed on P1.
func TestRequestComments_SameHeadRetarget_CurrentPosition_Stale(t *testing.T) {
	db := newHandlerTestDB(t)

	mustExecDB(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pr-rt2','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','sha-NEW-BASE','sha-SAME')`)
	// P1: old base, P2: new base (retarget) — same head.
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-rt2', 1, 'sha-SAME', 'sha-OLD-BASE', '2026-01-01')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-rt2', 2, 'sha-SAME', 'sha-NEW-BASE', '2026-01-01')`)
	// Current-position comment: anchor computed on P1 geometry.
	// anchor_head_sha matches but anchor_patchset_number(1) != current(2).
	mustExecDB(t, db, `INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, patchset_number, anchored_line, anchor_head_sha, outdated, anchor_patchset_number)
		VALUES
		('c-rt2','pr-rt2','carol','msg','2026-01-01','2026-01-01',
		 'review', 2, 5, 'sha-SAME', 0, 1)`)

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, _ := h(context.Background(), ws.InboundMessage{
		ID: "req-rt2", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pr-rt2"}`),
	})
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1, got %d", len(comments))
	}
	c := comments[0]

	// Case A: patchset_number(2)==current(2) but anchor_patchset_number(1)!=current(2) → STALE.
	if c.Anchored {
		t.Error("Anchored must be false: current-position with stale anchor geometry (retarget)")
	}
	if c.AnchoredLine != nil {
		t.Errorf("AnchoredLine must be nil after retarget stale, got %d", *c.AnchoredLine)
	}
}

// TestRequestComments_NullAnchorPatchsetNumber_AlwaysStale ensures null identity
// always suppresses, regardless of position type.
func TestRequestComments_NullAnchorPatchsetNumber_AlwaysStale(t *testing.T) {
	db := newHandlerTestDB(t)

	mustExecDB(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pr-null2','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','b','sha-H')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-null2', 1, 'sha-H', 'b', '2026-01-01')`)
	// anchor_patchset_number is NULL — pre-migration 018 or failed identity.
	mustExecDB(t, db, `INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, patchset_number, anchored_line, anchor_head_sha, outdated)
		VALUES
		('c-null2','pr-null2','dave','x','2026-01-01','2026-01-01',
		 'review', 1, 4, 'sha-H', 0)`)

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, _ := h(context.Background(), ws.InboundMessage{
		ID: "req-null3", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pr-null2"}`),
	})
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1, got %d", len(comments))
	}
	if comments[0].Anchored {
		t.Error("Anchored must be false when anchor_patchset_number is NULL")
	}
	if comments[0].AnchoredLine != nil {
		t.Errorf("AnchoredLine must be nil when anchor_patchset_number is NULL")
	}
}

// ── Migration 019: anchor_current_patchset_number tests ──────────────────

// TestRequestComments_OriginalPositionAnchor_BeforeRetarget_Stale is the primary
// regression test for migration 019:
// - Comment from P1 (patchset_number=1).
// - Anchor was resolved before retarget with anchor_current_patchset_number=1
//   (anchor_head_sha=sha-H, anchor_patchset_number=1, anchor_current_ps=1).
// - PR retargeted: same head sha-H, new base → P2 is now current.
// - Anchor must be STALE because anchor_current_patchset_number=1 != current=2.
func TestRequestComments_OriginalPositionAnchor_BeforeRetarget_Stale(t *testing.T) {
	db := newHandlerTestDB(t)

	mustExecDB(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pr-brt','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','sha-NEW-BASE','sha-H')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-brt', 1, 'sha-H', 'sha-OLD-BASE', '2026-01-01')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-brt', 2, 'sha-H', 'sha-NEW-BASE', '2026-01-01')`)
	// Anchor computed BEFORE retarget: anchor_current_patchset_number=1 (P1 was current).
	// After retarget, P2 is current. anchor_current_ps(1) != current(2) → STALE.
	mustExecDB(t, db, `INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, patchset_number, anchored_line, anchor_head_sha, outdated,
		 anchor_patchset_number, anchor_current_patchset_number)
		VALUES
		('c-brt','pr-brt','alice','nit','2026-01-01','2026-01-01',
		 'review', 1, 5, 'sha-H', 0, 1, 1)`)

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, _ := h(context.Background(), ws.InboundMessage{
		ID: "req-brt", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pr-brt"}`),
	})
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1, got %d", len(comments))
	}
	c := comments[0]

	// anchor_current_patchset_number=1 but current=2 → STALE regardless of head match.
	if c.Anchored {
		t.Error("Anchored must be false: anchor_current_patchset_number(1) != current_patchset(2)")
	}
	if c.AnchoredLine != nil {
		t.Errorf("AnchoredLine must be nil for stale pre-retarget anchor, got %d", *c.AnchoredLine)
	}
}

// TestRequestComments_OriginalPositionAnchor_AfterRetarget_Fresh verifies
// that after a retarget and re-resolution, the anchor IS fresh:
// anchor_patchset_number=1 (source), anchor_current_patchset_number=2 (current P2).
func TestRequestComments_OriginalPositionAnchor_AfterRetarget_Fresh(t *testing.T) {
	db := newHandlerTestDB(t)

	mustExecDB(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pr-art','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','sha-NEW-BASE','sha-H')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-art', 1, 'sha-H', 'sha-OLD-BASE', '2026-01-01')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-art', 2, 'sha-H', 'sha-NEW-BASE', '2026-01-01')`)
	// Re-resolved AFTER retarget: anchor_current_patchset_number=2 (P2 is current now).
	mustExecDB(t, db, `INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, patchset_number, anchored_line, anchor_head_sha, outdated,
		 anchor_patchset_number, anchor_current_patchset_number)
		VALUES
		('c-art','pr-art','alice','nit','2026-01-01','2026-01-01',
		 'review', 1, 8, 'sha-H', 0, 1, 2)`)

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, _ := h(context.Background(), ws.InboundMessage{
		ID: "req-art", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pr-art"}`),
	})
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1, got %d", len(comments))
	}
	c := comments[0]

	// anchor_head_sha == prs.head_sha AND anchor_current_ps(2) == current(2)
	// AND anchor_patchset_number(1) == comments.patchset_number(1) → FRESH.
	if !c.Anchored {
		t.Error("Anchored must be true: re-resolved after retarget with correct identities")
	}
	if c.AnchoredLine == nil || *c.AnchoredLine != 8 {
		t.Errorf("AnchoredLine: got %v want 8", c.AnchoredLine)
	}
}

// TestRequestComments_NullCurrentPatchsetNumber_Stale verifies that
// anchor_current_patchset_number=NULL is always stale, for both original- and
// current-position comments (pre-migration 019 data).
func TestRequestComments_NullCurrentPatchsetNumber_Stale(t *testing.T) {
	db := newHandlerTestDB(t)

	mustExecDB(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pr-ncp','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','b','sha-H')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-ncp', 1, 'sha-H', 'b', '2026-01-01')`)
	// anchor_current_patchset_number is NULL (pre-migration 019 data).
	mustExecDB(t, db, `INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, patchset_number, anchored_line, anchor_head_sha, outdated, anchor_patchset_number)
		VALUES
		('c-ncp','pr-ncp','bob','x','2026-01-01','2026-01-01',
		 'review', 1, 3, 'sha-H', 0, 1)`)

	h := NewRequestCommentsHandler(db, testRepoLogger())
	resp, _ := h(context.Background(), ws.InboundMessage{
		ID: "req-ncp", Type: "REQUEST_COMMENTS", Data: []byte(`{"pr_id":"pr-ncp"}`),
	})
	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	comments := data["comments"].([]commentRow)
	if len(comments) != 1 {
		t.Fatalf("expected 1, got %d", len(comments))
	}
	// anchor_current_patchset_number=NULL → stale (pre-migration 019).
	if comments[0].Anchored {
		t.Error("Anchored must be false when anchor_current_patchset_number is NULL")
	}
	if comments[0].AnchoredLine != nil {
		t.Errorf("AnchoredLine must be nil when anchor_current_patchset_number is NULL")
	}
}
