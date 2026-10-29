package services

import (
	"context"
	"testing"

	"github.com/itterno/bistro/internal/store/storetest"
)

// TestRequestComments_NullAnchorPatchsetNumber_Stale covers legacy anchors missing
// both anchor identities: the current-identity guard rejects either comment origin.
func TestRequestComments_NullAnchorPatchsetNumber_Stale(t *testing.T) {
	tests := []struct {
		name           string
		patchsetNumber *int
	}{
		{name: "unknown_comment_patchset"},
		{name: "current_comment_patchset", patchsetNumber: new(1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := storetest.Open(t).DB
			mustExecDB(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
				created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
				VALUES ('pr-null','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f',
				        'sha-BASE','sha-HEAD')`)
			mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
				VALUES ('pr-null', 1, 'sha-HEAD', 'sha-BASE', '2026-01-01')`)
			mustExecDB(t, db, `INSERT INTO comments
				(id, pr_id, author, body, created_at, updated_at,
				 kind, patchset_number, anchored_line, anchor_head_sha, outdated)
				VALUES ('c-null','pr-null','alice','old','2026-01-01','2026-01-01',
				 'review', ?, 5, 'sha-HEAD', 0)`, tt.patchsetNumber)

			h := (&Comments{DB: db, Log: testRepoLogger()})
			resp, err := h.List(context.Background(), PRRef{PRID: "pr-null"})
			if err != nil {
				t.Fatalf("REQUEST_COMMENTS: %v", err)
			}
			comments := resp.Comments
			if len(comments) != 1 {
				t.Fatalf("expected 1, got %d", len(comments))
			}
			if comments[0].Anchored {
				t.Error("Anchored must be false for a legacy anchor missing patchset identity")
			}
			if comments[0].AnchoredLine != nil {
				t.Errorf("AnchoredLine must be nil, got %d", *comments[0].AnchoredLine)
			}
		})
	}
}

// TestRequestComments_CurrentPositionAnchor_Fresh confirms Case A still works:
// current-position comment with anchor_patchset_number == current_patchset_number.
func TestRequestComments_CurrentPositionAnchor_Fresh(t *testing.T) {
	db := storetest.Open(t).DB

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

	h := (&Comments{DB: db, Log: testRepoLogger()})
	resp, _ := h.List(context.Background(), PRRef{PRID: "pr-cp"})
	comments := resp.Comments
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
	db := storetest.Open(t).DB

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

	h := (&Comments{DB: db, Log: testRepoLogger()})
	resp, _ := h.List(context.Background(), PRRef{PRID: "pr-rt2"})
	comments := resp.Comments
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

// TestRequestComments_NullCurrentPatchsetNumber_Stale verifies that
// anchor_current_patchset_number=NULL is always stale, for both original- and
// current-position comments (pre-migration 019 data).
func TestRequestComments_NullCurrentPatchsetNumber_Stale(t *testing.T) {
	db := storetest.Open(t).DB

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

	h := (&Comments{DB: db, Log: testRepoLogger()})
	resp, _ := h.List(context.Background(), PRRef{PRID: "pr-ncp"})
	comments := resp.Comments
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
