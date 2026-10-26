package sync

import (
	"errors"
	"testing"

	"github.com/itterno/bistro/internal/store/prdiff"
	"github.com/itterno/bistro/internal/store/storetest"
)

// ── Diff atomicity tests (blocker: pr_diffs must not advance independently) ──

func TestWriteDiffs_NewPR_AtomicWithPRRow(t *testing.T) {
	// New PR (never written before): PR row, patchset, and pr_diffs must all land
	// in the same transaction. This guards against the FK ordering bug where
	// Save was called during fetch before the PR row existed.
	db := storetest.Open(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:20", 20, "head-new", "base-new")
	diffs := []prdiff.File{
		{FilePath: "main.go", Patch: "+new line", Additions: 1},
		{FilePath: "util.go", Patch: "+helper", Additions: 1},
	}

	results := []*PRFetchResult{{
		PR:           pr,
		DiffFiles:    diffs,
		DiffComputed: true,
	}}
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	// PR row must exist.
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM prs WHERE id = ?`, pr.ID).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Fatalf("PR row missing after atomic write, count=%d", count)
	}

	// pr_diffs must also exist (written in same tx, after PR row satisfies FK).
	if n := countDiffFiles(t, db, pr.ID); n != 2 {
		t.Errorf("expected 2 diff files for new PR, got %d", n)
	}
}

func TestWriteDiffs_ErroredResult_DiffsNotReplaced(t *testing.T) {
	// When result.Error != nil, the writer skips that result entirely.
	// Pre-existing pr_diffs must be preserved (not cleared/replaced).
	db := storetest.Open(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:21", 21, "head-21", "base-21")
	mustWritePR(t, w, pr)

	// Seed pr_diffs for this PR.
	db.Exec(`INSERT INTO pr_diffs (pr_id, file_path, patch, additions, deletions) VALUES (?, 'a.go', 'patch', 1, 0)`, pr.ID) //nolint:errcheck

	// Now write a result with Error set — diffs must not be touched.
	badResult := &PRFetchResult{
		PR:           pr,
		DiffFiles:    nil, // no diffs
		DiffComputed: true,
		Error:        errors.New("upstream API failure"),
	}
	if err := w.WriteSyncResults([]*PRFetchResult{badResult}); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	// pr_diffs must still have the original row.
	if n := countDiffFiles(t, db, pr.ID); n != 1 {
		t.Errorf("errored result: expected 1 stale diff row preserved, got %d", n)
	}
}

func TestWriteDiffs_EmptyDiff_ClearsStaleRows_OnAcceptedResult(t *testing.T) {
	// DiffComputed=true with empty DiffFiles: pr_diffs cleared (stale files gone).
	// This happens when a PR is reverted to base so no files changed.
	db := storetest.Open(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:22", 22, "head-22", "base-22")
	mustWritePR(t, w, pr)

	// Seed stale pr_diffs.
	db.Exec(`INSERT INTO pr_diffs (pr_id, file_path, patch, additions, deletions) VALUES (?, 'stale.go', 'old', 1, 0)`, pr.ID) //nolint:errcheck
	if n := countDiffFiles(t, db, pr.ID); n != 1 {
		t.Fatalf("precondition: expected 1 stale row, got %d", n)
	}

	// Write accepted result with empty diff (zero-file change set).
	results := []*PRFetchResult{{
		PR:           makePR("github:o/r:22", 22, "head-22b", "base-22"),
		DiffFiles:    []prdiff.File{}, // explicitly empty
		DiffComputed: true,
	}}
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	// Stale row must be gone.
	if n := countDiffFiles(t, db, pr.ID); n != 0 {
		t.Errorf("empty diff should clear stale rows, got %d rows", n)
	}
}

func TestWriteDiffs_DiffNotComputed_PreservesCache(t *testing.T) {
	// DiffComputed=false (diff generation not attempted or failed):
	// pr_diffs must not be touched — preserve the last-known-good cache.
	db := storetest.Open(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:23", 23, "head-23", "base-23")
	mustWritePR(t, w, pr)

	db.Exec(`INSERT INTO pr_diffs (pr_id, file_path, patch, additions, deletions) VALUES (?, 'cached.go', 'patch', 2, 0)`, pr.ID) //nolint:errcheck

	// Write result without diff (DiffComputed=false).
	results := []*PRFetchResult{{
		PR:           makePR("github:o/r:23", 23, "head-23b", "base-23"),
		DiffComputed: false, // diff was not computed this sync cycle
	}}
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	// Cached diff must survive.
	if n := countDiffFiles(t, db, pr.ID); n != 1 {
		t.Errorf("DiffComputed=false: expected cached diff preserved, got %d rows", n)
	}
}

func TestWriteDiffs_AtomicWithPatchset_BaseHeadChange(t *testing.T) {
	// When head or base changes, both patchset row and pr_diffs must advance
	// in the same transaction. Neither should be visible independently.
	db := storetest.Open(t)
	w := NewWriter(nil, db)

	// P1: initial state
	pr1 := makePR("github:o/r:24", 24, "head-v1", "base-24")
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR:           pr1,
		DiffFiles:    []prdiff.File{{FilePath: "a.go", Patch: "v1", Additions: 1}},
		DiffComputed: true,
	}}); err != nil {
		t.Fatalf("P1 write: %v", err)
	}
	patchsetCount := func() int {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM patchsets WHERE pr_id = ?`, pr1.ID).Scan(&n) //nolint:errcheck
		return n
	}
	if patchsetCount() != 1 {
		t.Fatalf("after P1: expected 1 patchset, got %d", patchsetCount())
	}
	if n := countDiffFiles(t, db, pr1.ID); n != 1 {
		t.Fatalf("after P1: expected 1 diff file, got %d", n)
	}

	// P2: head changes → new patchset + new diffs in one tx
	pr2 := makePR("github:o/r:24", 24, "head-v2", "base-24")
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: pr2,
		DiffFiles: []prdiff.File{
			{FilePath: "a.go", Patch: "v2", Additions: 2},
			{FilePath: "b.go", Patch: "v2b", Additions: 1},
		},
		DiffComputed: true,
	}}); err != nil {
		t.Fatalf("P2 write: %v", err)
	}

	if patchsetCount() != 2 {
		t.Errorf("after P2: expected 2 patchsets, got %d", patchsetCount())
	}
	if n := countDiffFiles(t, db, pr1.ID); n != 2 {
		t.Errorf("after P2: expected 2 diff files (atomic replace), got %d", n)
	}

	// Verify the updated patch content (not the old v1 patch).
	var patch string
	db.QueryRow(`SELECT patch FROM pr_diffs WHERE pr_id = ? AND file_path = 'a.go'`, pr1.ID).Scan(&patch) //nolint:errcheck
	if patch != "v2" {
		t.Errorf("diff content after P2: got %q want v2", patch)
	}
}
