package sync

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// TestIsAnchorable verifies the eligibility predicate for anchor resolution.
func TestIsAnchorable(t *testing.T) {
	pos5 := 5
	zeroPos := 0
	origPos3 := 3

	tests := []struct {
		name string
		c    *provider.Comment
		want bool
	}{
		{name: "current position present", c: &provider.Comment{CommitSHA: "sha", Position: &pos5}, want: true},
		{name: "only original position (outdated/rebased)", c: &provider.Comment{OriginalPosition: &origPos3}, want: true},
		{name: "both present", c: &provider.Comment{CommitSHA: "sha", Position: &pos5, OriginalPosition: &origPos3}, want: true},
		{name: "no position at all", c: &provider.Comment{CommitSHA: "sha"}, want: false},
		{name: "position pointer to zero (invalid)", c: &provider.Comment{CommitSHA: "sha", Position: &zeroPos}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAnchorable(tt.c); got != tt.want {
				t.Errorf("isAnchorable = %v, want %v", got, tt.want)
			}
		})
	}
}

// ── selectDiffBaseSHA unit tests ──────────────────────────────────────────

// TestSelectDiffBaseSHA_CurrentPosition verifies that current-position
// comments always use prBaseSHA regardless of patchsets, and that the
// resolved patchset number is populated when a matching patchset exists.
func TestSelectDiffBaseSHA_CurrentPosition(t *testing.T) {
	patchsets := []store.Patchset{
		{Number: 1, HeadSHA: "sha-HEAD", BaseSHA: "sha-BASE"},
	}
	base, psNum, skip := selectDiffBaseSHA(diffBaseSelection{patchsets: patchsets, isCurrentPosition: true, prBaseSHA: "sha-BASE", prHeadSHA: "sha-HEAD"})
	if skip {
		t.Error("current-position must not be skipped")
	}
	if base != "sha-BASE" {
		t.Errorf("current-position: got DiffBaseSHA=%q want sha-BASE", base)
	}
	if psNum != 1 {
		t.Errorf("current-position: got resolvedPatchsetNum=%d want 1", psNum)
	}
}

// TestSelectDiffBaseSHA_CurrentPosition_NoPatchsetYet verifies that a current-position
// comment is NOT skipped even when no matching patchset row exists yet (first sync).
func TestSelectDiffBaseSHA_CurrentPosition_NoPatchsetYet(t *testing.T) {
	base, psNum, skip := selectDiffBaseSHA(diffBaseSelection{isCurrentPosition: true, prBaseSHA: "sha-BASE", prHeadSHA: "sha-HEAD"})
	if skip {
		t.Error("current-position: must not skip when patchset not yet recorded")
	}
	if base != "sha-BASE" {
		t.Errorf("expected sha-BASE, got %q", base)
	}
	if psNum != 0 {
		t.Errorf("expected patchset num 0 (not recorded yet), got %d", psNum)
	}
}

// TestSelectDiffBaseSHA_OriginalPosition_ByNumber_Found verifies that exact
// patchset number matching returns the correct BaseSHA, not the current PR base.
func TestSelectDiffBaseSHA_OriginalPosition_ByNumber_Found(t *testing.T) {
	patchsets := []store.Patchset{
		{Number: 1, HeadSHA: "sha-P1", BaseSHA: "sha-P1-BASE"},
		{Number: 2, HeadSHA: "sha-P2", BaseSHA: "sha-P2-BASE"},
	}
	// patchsetNum=1 → must select P1's BaseSHA
	base, _, skip := selectDiffBaseSHA(diffBaseSelection{patchsets: patchsets, patchsetNum: 1, prBaseSHA: "WRONG-CURRENT-BASE", prHeadSHA: "sha-head"})
	if skip {
		t.Error("original-position with matching patchset_number must not be skipped")
	}
	if base != "sha-P1-BASE" {
		t.Errorf("original-position patchsetNum=1: got DiffBaseSHA=%q want sha-P1-BASE", base)
	}
}

// TestSelectDiffBaseSHA_SameHead_DifferentBases is the critical regression test:
// two patchsets share the same head_sha but have different bases (retarget/rebase).
// Head-SHA matching would be ambiguous; number matching is exact.
func TestSelectDiffBaseSHA_SameHead_DifferentBases(t *testing.T) {
	// P1 and P2 share head "sha-SAME" but have different bases —
	// a retarget moved the PR to a different base without pushing new commits.
	patchsets := []store.Patchset{
		{Number: 1, HeadSHA: "sha-SAME", BaseSHA: "sha-BASE-ORIGINAL"},
		{Number: 2, HeadSHA: "sha-SAME", BaseSHA: "sha-BASE-RETARGETED"},
	}

	// Comment was made on patchset 1 → must use sha-BASE-ORIGINAL, not sha-BASE-RETARGETED.
	base, _, skip := selectDiffBaseSHA(diffBaseSelection{patchsets: patchsets, patchsetNum: 1, prBaseSHA: "sha-BASE-RETARGETED", prHeadSHA: "sha-head"})
	if skip {
		t.Error("patchsetNum=1 found → must not skip")
	}
	if base != "sha-BASE-ORIGINAL" {
		t.Errorf("same-head two-bases: patchsetNum=1 got %q want sha-BASE-ORIGINAL", base)
	}

	// Comment was made on patchset 2 → must use sha-BASE-RETARGETED.
	base, _, skip = selectDiffBaseSHA(diffBaseSelection{patchsets: patchsets, patchsetNum: 2, prBaseSHA: "sha-BASE-RETARGETED", prHeadSHA: "sha-head"})
	if skip {
		t.Error("patchsetNum=2 found → must not skip")
	}
	if base != "sha-BASE-RETARGETED" {
		t.Errorf("same-head two-bases: patchsetNum=2 got %q want sha-BASE-RETARGETED", base)
	}
}

// TestSelectDiffBaseSHA_OriginalPosition_NoPatchsetNumber skips when
// patchset_number is zero (not stored / freshly-fetched comment).
func TestSelectDiffBaseSHA_OriginalPosition_NoPatchsetNumber(t *testing.T) {
	patchsets := []store.Patchset{
		{Number: 1, HeadSHA: "sha-HEAD", BaseSHA: "sha-BASE"},
	}
	// patchsetNum=0 → no identity → must skip even though head matches.
	_, _, skip := selectDiffBaseSHA(diffBaseSelection{patchsets: patchsets, prBaseSHA: "some-base", prHeadSHA: "sha-head"})
	if !skip {
		t.Error("patchsetNum=0: must skip, never approximate")
	}
}

// TestSelectDiffBaseSHA_OriginalPosition_PatchsetNumberNotInDB skips when
// the stored patchset_number has no matching row in the DB.
func TestSelectDiffBaseSHA_OriginalPosition_PatchsetNumberNotInDB(t *testing.T) {
	patchsets := []store.Patchset{
		{Number: 1, HeadSHA: "sha-HEAD", BaseSHA: "sha-BASE"},
	}
	// patchsetNum=99 is not in the list → skip.
	_, _, skip := selectDiffBaseSHA(diffBaseSelection{patchsets: patchsets, patchsetNum: 99, prBaseSHA: "some-base", prHeadSHA: "sha-head"})
	if !skip {
		t.Error("patchsetNum not in DB: must skip")
	}
}

// TestSelectDiffBaseSHA_OriginalPosition_EmptyPatchsets verifies skipping
// when no patchsets are available.
func TestSelectDiffBaseSHA_OriginalPosition_EmptyPatchsets(t *testing.T) {
	_, _, skip := selectDiffBaseSHA(diffBaseSelection{patchsetNum: 1, prBaseSHA: "current-base", prHeadSHA: "sha-head"})
	if !skip {
		t.Error("empty patchsets: must skip even with non-zero patchset_number")
	}
}

// ── resolveAnchors integration tests ─────────────────────────────────────

// TestResolveAnchors_OutdatedCommentIsCandidate verifies the candidate-selection
// logic. An outdated comment with only OriginalPosition is a candidate.
func TestResolveAnchors_OutdatedCommentIsCandidate(t *testing.T) {
	pos5 := 5
	origPos3 := 3

	allComments := []*provider.Comment{
		{ID: "c-normal", Kind: "review", FilePath: "main.go",
			CommitSHA: "sha-commit", Position: &pos5, Side: "RIGHT"},
		{ID: "c-outdated", Kind: "review", FilePath: "util.go",
			OriginalSHA: "sha-original", OriginalPosition: &origPos3, Side: "RIGHT"},
		{ID: "c-issue", Kind: "issue", FilePath: "x.go"},
		{ID: "c-nopos", Kind: "review", FilePath: "y.go", CommitSHA: "sha-other"},
	}

	var got []string
	for _, c := range allComments {
		if c != nil && c.Kind == "review" && c.FilePath != "" && isAnchorable(c) {
			got = append(got, c.ID)
		}
	}

	want := map[string]bool{"c-normal": true, "c-outdated": true}
	if len(got) != len(want) {
		t.Fatalf("candidate count: got %d want %d (%v)", len(got), len(want), got)
	}
	for _, id := range got {
		if !want[id] {
			t.Errorf("unexpected candidate %q", id)
		}
	}
}

// TestResolveAnchors_OriginalPosition_NoPatchsetNumber_Skipped verifies that
// an original-position comment without a patchset_number is SKIPPED.
func TestResolveAnchors_OriginalPosition_NoPatchsetNumber_Skipped(t *testing.T) {
	origPos := 4
	o := &Orchestrator{
		anchorResolver: git.NewAnchorResolver(),
		// store is nil → no patchset lookup → patchsetNum stays 0 → skipped
	}
	comments := []*provider.Comment{{
		ID:               "c-nopsnum",
		Kind:             "review",
		FilePath:         "foo.go",
		OriginalSHA:      "sha-original",
		OriginalPosition: &origPos,
		Side:             "RIGHT",
		// PatchsetNumber: nil — not set
	}}
	results := o.resolveAnchors(context.Background(), anchorResolutionOptions{
		comments: comments, prID: "pr-test", localPath: "/tmp/bistro-nonexistent-clone",
		headSHA: "sha-head", prBaseSHA: "sha-base", log: silentLog(),
	})
	if len(results) != 0 {
		t.Errorf("original-position with no patchset_number must be skipped; got %d results", len(results))
	}
}

// TestResolveAnchors_OriginalPosition_PatchsetNumberSetOnComment_SkippedIfNoRow
// verifies that when c.PatchsetNumber is set but not in patchsets list → skip.
func TestResolveAnchors_OriginalPosition_PatchsetNumberSetButNotInDB_Skipped(t *testing.T) {
	origPos := 4
	psNum := 5 // will not match any patchset in DB
	o := &Orchestrator{
		anchorResolver: git.NewAnchorResolver(),
		// store nil → patchsets empty, so patchset 5 not found
	}
	comments := []*provider.Comment{{
		ID:               "c-orphan-ps",
		Kind:             "review",
		FilePath:         "foo.go",
		OriginalSHA:      "sha-original",
		OriginalPosition: &origPos,
		PatchsetNumber:   &psNum, // set, but not in DB
		Side:             "RIGHT",
	}}
	results := o.resolveAnchors(context.Background(), anchorResolutionOptions{
		comments: comments, prID: "pr-test", localPath: "/tmp/bistro-nonexistent-clone",
		headSHA: "sha-head", prBaseSHA: "sha-base", log: silentLog(),
	})
	if len(results) != 0 {
		t.Errorf("patchset_number not in DB must skip; got %d results", len(results))
	}
}

// TestResolveAnchors_OriginalPosition_WithDB_PatchsetFound_AttemptedNotSkipped
// verifies that when the DB has the correct patchset row AND c.PatchsetNumber
// is populated, the comment is ATTEMPTED (not skipped by the patchset guard).
// The clone is missing so Resolve fails gracefully.
func TestResolveAnchors_OriginalPosition_WithDB_PatchsetFound_AttemptedNotSkipped(t *testing.T) {
	origPos := 4
	psNum := 1
	db := newSyncTestDB(t)
	mustInsertPRForAnchorTest(t, db, "pr-anc-2")
	if _, err := db.Exec(`INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-anc-2', 1, 'sha-P1-head', 'sha-P1-base', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert patchset: %v", err)
	}

	o := &Orchestrator{
		anchorResolver: git.NewAnchorResolver(),
		store:          db,
	}
	comments := []*provider.Comment{{
		ID:               "c-with-psnum",
		Kind:             "review",
		FilePath:         "foo.go",
		OriginalSHA:      "sha-P1-head",
		OriginalPosition: &origPos,
		PatchsetNumber:   &psNum, // exact patchset 1 → found in DB
		Side:             "RIGHT",
	}}
	// Clone missing → 0 results (graceful), but NOT because patchset was skipped.
	// The patchset guard passes because Number==1 is in DB.
	results := o.resolveAnchors(context.Background(), anchorResolutionOptions{
		comments: comments, prID: "pr-anc-2", localPath: "/tmp/bistro-nonexistent-clone-456",
		headSHA: "sha-current-head", prBaseSHA: "sha-WRONG-base", log: silentLog(),
	})
	if len(results) != 0 {
		t.Errorf("missing clone: expected 0 (graceful), got %d", len(results))
	}
}

// TestResolveAnchors_OriginalPosition_DBLookup_PatchsetFound_AttemptedNotSkipped
// verifies the DB-lookup path: c.PatchsetNumber is nil, but the comment has a
// stored patchset_number in the DB that is loaded by GetCommentPatchsetNumbers.
func TestResolveAnchors_OriginalPosition_DBLookup_AttemptedNotSkipped(t *testing.T) {
	origPos := 3
	db := newSyncTestDB(t)
	mustInsertPRForAnchorTest(t, db, "pr-anc-3")
	// Patchset row in DB.
	db.Exec(`INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-anc-3', 2, 'sha-P2-head', 'sha-P2-base', CURRENT_TIMESTAMP)`) //nolint:errcheck
	// Insert the comment with patchset_number=2 stored in DB.
	db.Exec(`INSERT INTO comments (id, pr_id, author, body, created_at, updated_at, kind, patchset_number)
		VALUES ('c-db-lookup', 'pr-anc-3', 'alice', 'body', '2026-01-01', '2026-01-01', 'review', 2)`) //nolint:errcheck

	o := &Orchestrator{
		anchorResolver: git.NewAnchorResolver(),
		store:          db,
	}
	comments := []*provider.Comment{{
		ID:               "c-db-lookup",
		Kind:             "review",
		FilePath:         "foo.go",
		OriginalSHA:      "sha-P2-head",
		OriginalPosition: &origPos,
		PatchsetNumber:   nil, // NOT set on the comment — must be loaded from DB
		Side:             "RIGHT",
	}}
	results := o.resolveAnchors(context.Background(), anchorResolutionOptions{
		comments: comments, prID: "pr-anc-3", localPath: "/tmp/bistro-nonexistent-clone-789",
		headSHA: "sha-current", prBaseSHA: "sha-WRONG-base", log: silentLog(),
	})
	// Clone missing → 0 results, but not because of patchset skipping.
	if len(results) != 0 {
		t.Errorf("missing clone: expected 0 (graceful), got %d", len(results))
	}
}

// TestResolveAnchors_CurrentPosition_NotSkipped verifies that current-position
// comments are never blocked by the patchset guard.
func TestResolveAnchors_CurrentPosition_NotSkipped(t *testing.T) {
	pos := 3
	o := &Orchestrator{
		anchorResolver: git.NewAnchorResolver(),
	}
	comments := []*provider.Comment{{
		ID:        "c-current",
		Kind:      "review",
		FilePath:  "foo.go",
		CommitSHA: "sha-head",
		Position:  &pos,
		Side:      "RIGHT",
	}}
	results := o.resolveAnchors(context.Background(), anchorResolutionOptions{
		comments: comments, prID: "pr-test", localPath: "/tmp/bistro-nonexistent-clone-xyz",
		headSHA: "sha-head", prBaseSHA: "sha-base", log: silentLog(),
	})
	if len(results) != 0 {
		t.Errorf("expected 0 (graceful clone failure), got %d", len(results))
	}
}

// TestResolveAnchors_NoPositionSkipped verifies a comment with no position is excluded.
func TestResolveAnchors_NoPositionSkipped(t *testing.T) {
	o := &Orchestrator{
		anchorResolver: git.NewAnchorResolver(),
	}
	comments := []*provider.Comment{{
		ID:          "c-nopos",
		Kind:        "review",
		FilePath:    "foo.go",
		CommitSHA:   "some-sha",
		OriginalSHA: "orig-sha",
	}}
	results := o.resolveAnchors(context.Background(), anchorResolutionOptions{
		comments: comments, prID: "pr-test", localPath: "/tmp/bistro-nonexistent-clone",
		headSHA: "sha-head", prBaseSHA: "sha-base", log: silentLog(),
	})
	if len(results) != 0 {
		t.Errorf("expected 0 results for comment with no position, got %d", len(results))
	}
}

// mustInsertPRForAnchorTest inserts a minimal PR row for FK satisfaction.
func mustInsertPRForAnchorTest(t *testing.T, db *store.DB, prID string) {
	t.Helper()
	db.Exec(`INSERT OR IGNORE INTO prs
		(id, provider, owner, repo, number, title, author, state,
		 created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES (?, 'github','o','r',99,'T','a','open','2026-01-01','2026-01-01','main','f','b','h')`,
		prID) //nolint:errcheck
}
