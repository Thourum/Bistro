package sync

import (
	"context"
	"testing"

	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestRollbackPRSavepointReportsMissingSavepoint(t *testing.T) {
	db := storetest.Open(t)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck // Cleanup after the explicit rollback below.

	if _, err := tx.ExecContext(ctx, `INSERT INTO repos (id, provider, owner, repo) VALUES (?, ?, ?, ?)`,
		"github:o/r", "github", "o", "r"); err != nil {
		t.Fatalf("write partial transaction: %v", err)
	}
	if err := rollbackPRSavepoint(ctx, tx); err == nil {
		t.Fatal("missing savepoint must return an error, not permit a partial commit")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("roll back outer transaction: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM repos WHERE id = ?`, "github:o/r").Scan(&count); err != nil {
		t.Fatalf("count rolled-back rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("partial write was committed after savepoint failure: %d rows", count)
	}
}

// ── Critical atomicity / savepoint tests ─────────────────────────────────────

// TestWriteDiffs_InsertFailure_SavepointRollsBackAll verifies that when writeDiffs
// fails mid-insert (duplicate file_path in DiffFiles triggers a UNIQUE constraint),
// the savepoint rolls back the DELETE that already ran, preserving the old cache.
// It also asserts the PR row and patchset did NOT advance, since all three critical
// writes are inside the same savepoint boundary.
func TestWriteDiffs_InsertFailure_SavepointRollsBackAll(t *testing.T) {
	db := storetest.Open(t)
	w := NewWriter(nil, db)

	// Seed a PR and its diff cache.
	pr := makePR("github:o/r:30", 30, "head-30", "base-30")
	mustWritePR(t, w, pr)
	db.Exec(`INSERT INTO pr_diffs (pr_id, file_path, patch, additions, deletions) VALUES (?, 'cached.go', 'old-patch', 3, 1)`, pr.ID) //nolint:errcheck

	// Verify precondition: 1 stale row.
	if n := countDiffFiles(t, db, pr.ID); n != 1 {
		t.Fatalf("precondition: want 1 stale row, got %d", n)
	}

	// Duplicate file_path triggers UNIQUE constraint violation on the second INSERT,
	// AFTER the DELETE has already run. Without savepoints this would commit a cleared
	// cache. With savepoints, the whole PR's critical writes roll back.
	badDiffs := []store.DiffFile{
		{FilePath: "dup.go", Patch: "patch-a", Additions: 1},
		{FilePath: "dup.go", Patch: "patch-b", Additions: 2}, // duplicate: will fail
	}
	newPR := makePR("github:o/r:30", 30, "head-30b", "base-30")
	results := []*PRFetchResult{{
		PR:           newPR,
		DiffFiles:    badDiffs,
		DiffComputed: true,
	}}

	// Must not return a fatal batch error.
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults must not be fatal: %v", err)
	}

	// Old diff cache must be intact — savepoint rolled back the DELETE.
	if n := countDiffFiles(t, db, pr.ID); n != 1 {
		t.Errorf("old diff cache should be preserved after INSERT failure, got %d rows", n)
	}
	var savedPatch string
	db.QueryRow(`SELECT patch FROM pr_diffs WHERE pr_id = ?`, pr.ID).Scan(&savedPatch) //nolint:errcheck
	if savedPatch != "old-patch" {
		t.Errorf("preserved diff should still have old-patch content, got %q", savedPatch)
	}

	// PR head_sha must NOT have advanced — savepoint also rolled back the PR upsert.
	var headSHA string
	db.QueryRow(`SELECT head_sha FROM prs WHERE id = ?`, pr.ID).Scan(&headSHA) //nolint:errcheck
	if headSHA != "head-30" {
		t.Errorf("PR head_sha must not advance when writeDiffs failed: got %q, want head-30", headSHA)
	}

	// Patchset must also not have been added for the failed head.
	var psCount int
	db.QueryRow(`SELECT COUNT(*) FROM patchsets WHERE pr_id = ?`, pr.ID).Scan(&psCount) //nolint:errcheck
	// P1 was recorded by the initial mustWritePR, so count is 1 (not 2).
	if psCount != 1 {
		t.Errorf("no new patchset should be added when writeDiffs failed, patchset count=%d", psCount)
	}
}

// TestWritePR_PatchsetFailure_SavepointRollsBackPRRow verifies that when
// recordPatchsetIfChanged fails, the savepoint rolls back the PR row write too,
// so the prs table is not advanced without a matching patchset.
func TestWritePR_PatchsetFailure_SavepointRollsBackPRRow(t *testing.T) {
	db := storetest.Open(t)
	w := NewWriter(nil, db)

	// Drop the patchsets table to force recordPatchsetIfChanged to fail.
	if _, err := db.Exec("DROP TABLE patchsets"); err != nil {
		t.Fatalf("drop patchsets: %v", err)
	}

	pr := makePR("github:o/r:31", 31, "head-31", "base-31")
	results := []*PRFetchResult{{PR: pr}}

	// Must not return a fatal batch error (one PR failed, batch continues).
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults must not be fatal: %v", err)
	}

	// PR row must NOT exist — savepoint rolled back the writePRRow.
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM prs WHERE id = ?`, pr.ID).Scan(&count) //nolint:errcheck
	if count != 0 {
		t.Errorf("PR row must not exist when patchset recording failed, got count=%d", count)
	}
}

// TestWriteSavepoint_OtherPRsUnaffected verifies that a critical failure for
// one PR does not roll back other PRs in the same batch.
func TestWriteSavepoint_OtherPRsUnaffected(t *testing.T) {
	db := storetest.Open(t)
	w := NewWriter(nil, db)

	goodPR := makePR("github:o/r:40", 40, "head-40", "base-40")
	badDiffs := []store.DiffFile{
		{FilePath: "dup.go", Patch: "a"},
		{FilePath: "dup.go", Patch: "b"}, // duplicate → INSERT failure
	}
	failPR := makePR("github:o/r:41", 41, "head-41", "base-41")

	results := []*PRFetchResult{
		{PR: goodPR},
		{PR: failPR, DiffFiles: badDiffs, DiffComputed: true},
	}
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults must not be fatal: %v", err)
	}

	// Good PR must have been committed.
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM prs WHERE id = ?`, goodPR.ID).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("good PR must be committed despite other PR's failure, count=%d", count)
	}

	// Failed PR must not have advanced.
	db.QueryRow(`SELECT COUNT(*) FROM prs WHERE id = ?`, failPR.ID).Scan(&count) //nolint:errcheck
	if count != 0 {
		t.Errorf("failed PR must be rolled back, count=%d", count)
	}
}
