package handler

import (
	"context"
	"testing"

	"github.com/itterno/bistro/internal/store/storetest"
	"github.com/itterno/bistro/internal/ws"
)

// ── Stale cache suppression (blocker 2 fix) ───────────────────────────────

// TestRequestComments_StaleCacheSuppressesAllAnchorFields verifies that when
// anchor_head_sha does NOT match the current prs.head_sha (stale cache), ALL
// anchor-derived fields are zeroed in the response — including a stale outdated=true
// that must not leak to the client.
func TestRequestComments_StaleCacheSuppressesAllAnchorFields(t *testing.T) {
	db := storetest.Open(t).DB
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
	db := storetest.Open(t).DB
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
	db := storetest.Open(t).DB
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

// TestRequestComments_RetargetStaleAnchor_Suppressed is the primary regression
// test for the anchor_patchset_number blocker: same head SHA under different
// base (retarget) must not return the old anchor as fresh.
//
// P1: base=sha-BASE-OLD, head=sha-HEAD (anchor computed here, patchset_number=1)
// P2: base=sha-BASE-NEW, head=sha-HEAD (retarget, new patchset number=2)
// PR now at P2. Anchor anchor_patchset_number=1 but current patchset=2 → stale.
func TestRequestComments_RetargetStaleAnchor_Suppressed(t *testing.T) {
	db := storetest.Open(t).DB

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
	db := storetest.Open(t).DB

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
