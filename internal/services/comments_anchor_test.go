package services

import (
	"context"
	"database/sql"
	"testing"

	"github.com/itterno/bistro/internal/store/storetest"
)

// mustExecDB is a test helper for handler tests that use a *sql.DB.
func mustExecDB(t *testing.T, db *sql.DB, q string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("mustExecDB(%q): %v", q, err)
	}
}

func TestRequestComments_ReturnsNewColumns(t *testing.T) {
	db := storetest.Open(t).DB
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

	h := (&Comments{DB: db, Log: testRepoLogger()})
	resp, err := h.List(context.Background(), PRRef{PRID: "p1"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	comments := resp.Comments
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
	db := storetest.Open(t).DB
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

	h := (&Comments{DB: db, Log: testRepoLogger()})
	resp, err := h.List(context.Background(), PRRef{PRID: "pa1"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	comments := resp.Comments
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
	db := storetest.Open(t).DB
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

	h := (&Comments{DB: db, Log: testRepoLogger()})
	resp, _ := h.List(context.Background(), PRRef{PRID: "pa2"})
	comments := resp.Comments
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	// anchor_head_sha ('sha2') != prs.head_sha ('sha3') → Anchored=false.
	if comments[0].Anchored {
		t.Error("Anchored should be false when anchor_head_sha is stale (sha2 != sha3)")
	}
}

func TestRequestComments_AnchoredFalseWhenNullAnchor(t *testing.T) {
	db := storetest.Open(t).DB
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

	h := (&Comments{DB: db, Log: testRepoLogger()})
	resp, _ := h.List(context.Background(), PRRef{PRID: "pa3"})
	comments := resp.Comments
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	// anchored_line is NULL → Anchored must be false.
	if comments[0].Anchored {
		t.Error("Anchored should be false when anchored_line is NULL")
	}
}

func TestRequestComments_OutdatedComment(t *testing.T) {
	db := storetest.Open(t).DB
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

	h := (&Comments{DB: db, Log: testRepoLogger()})
	resp, _ := h.List(context.Background(), PRRef{PRID: "pa4"})
	comments := resp.Comments
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
