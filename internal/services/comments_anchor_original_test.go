package services

import (
	"context"
	"testing"

	"github.com/itterno/bistro/internal/store/storetest"
)

// ── Original-position freshness tests (blocker fix) ───────────────────────

// TestRequestComments_OriginalPositionAnchor_Fresh is the primary regression
// test: a comment from P1 re-resolved against P2.head must be returned as
// fresh (Case B freshness: anchor_patchset_number == comments.patchset_number).
//
// Setup: PR currently on P2 (head=sha-P2, base=b). Comment has patchset_number=1
// (was made on P1). Anchor was re-resolved against P2.head with correct P1
// geometry, so anchor_patchset_number=1 and anchor_head_sha=sha-P2.
func TestRequestComments_OriginalPositionAnchor_Fresh(t *testing.T) {
	db := storetest.Open(t).DB

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

	h := (&Comments{DB: db, Log: testRepoLogger()})
	resp, err := h.List(context.Background(), PRRef{PRID: "pr-op"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	comments := resp.Comments
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
	db := storetest.Open(t).DB

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

	h := (&Comments{DB: db, Log: testRepoLogger()})
	resp, _ := h.List(context.Background(), PRRef{PRID: "pr-op2"})
	comments := resp.Comments
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

// ── Migration 019: anchor_current_patchset_number tests ──────────────────

// TestRequestComments_OriginalPositionAnchor_BeforeRetarget_Stale is the primary
// regression test for migration 019:
//   - Comment from P1 (patchset_number=1).
//   - Anchor was resolved before retarget with anchor_current_patchset_number=1
//     (anchor_head_sha=sha-H, anchor_patchset_number=1, anchor_current_ps=1).
//   - PR retargeted: same head sha-H, new base → P2 is now current.
//   - Anchor must be STALE because anchor_current_patchset_number=1 != current=2.
func TestRequestComments_OriginalPositionAnchor_BeforeRetarget_Stale(t *testing.T) {
	db := storetest.Open(t).DB

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

	h := (&Comments{DB: db, Log: testRepoLogger()})
	resp, _ := h.List(context.Background(), PRRef{PRID: "pr-brt"})
	comments := resp.Comments
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
	db := storetest.Open(t).DB

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

	h := (&Comments{DB: db, Log: testRepoLogger()})
	resp, _ := h.List(context.Background(), PRRef{PRID: "pr-art"})
	comments := resp.Comments
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
