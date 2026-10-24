package store

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestCurrentPatchsetUsesLatestCurrentHeadNotBaseTip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	const prID = "pr-fingerprints"
	mustInsertPR(t, db, prID)
	mustExec(t, db, `UPDATE prs SET head_sha = 'current', base_sha = 'moving-base' WHERE id = ?`, prID)
	mustExec(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at, merge_base_sha, has_fingerprints)
		VALUES (?, 1, 'current', 'base-1', CURRENT_TIMESTAMP, 'merge-1', 0),
		       (?, 2, 'current', 'base-2', CURRENT_TIMESTAMP, 'merge-2', 1),
		       (?, 3, 'other', 'moving-base', CURRENT_TIMESTAMP, 'merge-3', 1)`, prID, prID, prID)

	current, found, err := db.CurrentPatchset(ctx, prID)
	if err != nil || !found {
		t.Fatalf("current: found=%v err=%v", found, err)
	}
	if current.Number != 2 || current.HeadSHA != "current" || current.BaseSHA != "base-2" || current.MergeBaseSHA != "merge-2" || !current.HasFingerprints {
		t.Fatalf("current: got %+v, want latest matching head P2", current)
	}
	patchsets, err := db.ListPatchsets(ctx, prID)
	if err != nil || len(patchsets) != 3 {
		t.Fatalf("list: patchsets=%+v err=%v", patchsets, err)
	}
	if patchsets[0].HasFingerprints || patchsets[0].MergeBaseSHA != "merge-1" || patchsets[1] != current {
		t.Fatalf("list fingerprint metadata: %+v", patchsets)
	}

	mustExec(t, db, `UPDATE prs SET head_sha = 'unobserved' WHERE id = ?`, prID)
	if _, found, err := db.CurrentPatchset(ctx, prID); err != nil || found {
		t.Fatalf("unobserved head: found=%v err=%v", found, err)
	}
	if _, found, err := db.CurrentPatchset(ctx, "missing"); err != nil || found {
		t.Fatalf("missing PR: found=%v err=%v", found, err)
	}
}

func TestListPatchsetFilesIsolatesPatchsetsAndCascades(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	const prID = "pr-patchset-files"
	mustInsertPR(t, db, prID)
	mustInsertPR(t, db, "other-pr")
	mustExec(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES (?, 1, 'h1', 'b', CURRENT_TIMESTAMP), (?, 2, 'h2', 'b', CURRENT_TIMESTAMP),
		       ('other-pr', 1, 'h1', 'b', CURRENT_TIMESTAMP)`, prID, prID)
	mustExec(t, db, `INSERT INTO patchset_files (pr_id, patchset_number, path, patch_id)
		VALUES (?, 1, 'a.go', 'patch-a'), (?, 1, 'binary.png', ''),
		       (?, 2, 'a.go', 'patch-new'), ('other-pr', 1, 'a.go', 'patch-other')`, prID, prID, prID)
	files, err := db.ListPatchsetFiles(ctx, prID, 1)
	if err != nil || !reflect.DeepEqual(files, map[string]string{"a.go": "patch-a", "binary.png": ""}) {
		t.Fatalf("files: got %v err=%v", files, err)
	}
	mustExec(t, db, `DELETE FROM patchsets WHERE pr_id = ? AND number = 1`, prID)
	files, err = db.ListPatchsetFiles(ctx, prID, 1)
	if err != nil || files == nil || len(files) != 0 {
		t.Fatalf("deleted patchset files: got %v err=%v", files, err)
	}
}

func TestDiffPatchIDRoundTrip(t *testing.T) {
	db := newTestDB(t)
	mustInsertPR(t, db, "pr-patch-id")
	diffs := NewDiffStore(db.DB)
	if err := diffs.SaveDiffs("pr-patch-id", []DiffFile{{FilePath: "a.go", Patch: "+added", PatchID: "fingerprint"}, {FilePath: "legacy.go"}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	files, err := diffs.GetDiffs("pr-patch-id")
	if err != nil || len(files) != 2 {
		t.Fatalf("get: files=%+v err=%v", files, err)
	}
	if files[0].PatchID != "fingerprint" || files[1].PatchID != "" {
		t.Fatalf("patch IDs: got %+v", files)
	}
}

func TestRecordLastReviewedTransactionAndEqualTimestamps(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-review-tx")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck
	event := ReviewEvent{PRID: "pr-review-tx", Login: "alice", HeadSHA: "first", PatchsetNum: 1,
		ReviewedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.FixedZone("offset", int(time.Hour/time.Second))), Source: ReviewSourceGitHubReview}
	if err := RecordLastReviewed(ctx, tx, event); err != nil {
		t.Fatalf("record in transaction: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	event.HeadSHA = "equal"
	event.Source = ReviewSourceAllViewed
	if err := db.RecordLastReviewed(ctx, event); err != nil {
		t.Fatalf("record equal timestamp: %v", err)
	}
	record, found, err := db.GetLastReviewed(ctx, event.PRID, event.Login)
	if err != nil || !found || record.SHA != "first" || record.PatchsetNum != 1 || record.Source != ReviewSourceGitHubReview || !record.ReviewedAt.Equal(event.ReviewedAt) {
		t.Fatalf("equal timestamp must not replace first: record=%+v found=%v err=%v", record, found, err)
	}
}

func TestReconciledSchemaVersionRequiresCompleteFingerprintSchema(t *testing.T) {
	const fingerprintMigrationVersion int64 = 35
	const previousMigrationVersion int64 = 34
	for _, schemaChange := range []string{
		"",
		"ALTER TABLE patchsets DROP COLUMN merge_base_sha",
		"ALTER TABLE patchsets DROP COLUMN has_fingerprints",
		"DROP TABLE patchset_files",
		"ALTER TABLE pr_diffs DROP COLUMN patch_id",
		"ALTER TABLE last_reviewed_sha DROP COLUMN reviewed_at",
		"ALTER TABLE last_reviewed_sha DROP COLUMN source",
	} {
		t.Run(schemaChange, func(t *testing.T) {
			db := newTestDB(t)
			wantVersion := fingerprintMigrationVersion
			if schemaChange != "" {
				mustExec(t, db, schemaChange)
				wantVersion = previousMigrationVersion
			}
			version, err := reconciledSchemaVersion(context.Background(), db.DB, fingerprintMigrationVersion)
			if err != nil || version != wantVersion {
				t.Fatalf("reconciled version = %d err=%v, want %d", version, err, wantVersion)
			}
		})
	}
}

func TestMigration035BackfillsBaselineAndLegacyFingerprints(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	const previousMigrationVersion = 34
	migrateEmbeddedSchemaToVersion(t, ctx, db.DB, previousMigrationVersion)
	mustInsertPR(t, db, "pr-before-fingerprints")
	mustExec(t, db, `INSERT INTO last_reviewed_sha (pr_id, account_login, login, sha, updated_at)
		VALUES ('pr-before-fingerprints', 'alice', 'alice', 'old-head', '2026-10-01T12:00:00Z')`)
	mustExec(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-before-fingerprints', 1, 'old-head', 'base', CURRENT_TIMESTAMP)`)
	mustExec(t, db, `INSERT INTO pr_diffs (pr_id, file_path, patch)
		VALUES ('pr-before-fingerprints', 'a.go', '+old')`)
	if err := NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	record, found, err := db.GetLastReviewed(ctx, "pr-before-fingerprints", "alice")
	wantTime := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if err != nil || !found || !record.ReviewedAt.Equal(wantTime) || record.Source != ReviewSourceBistroAction {
		t.Fatalf("backfill: record=%+v found=%v err=%v", record, found, err)
	}
	patchsets, err := db.ListPatchsets(ctx, "pr-before-fingerprints")
	if err != nil || len(patchsets) != 1 || patchsets[0].HasFingerprints || patchsets[0].MergeBaseSHA != "" {
		t.Fatalf("legacy patchsets: %+v err=%v", patchsets, err)
	}
	files, err := NewDiffStore(db.DB).GetDiffs("pr-before-fingerprints")
	if err != nil || len(files) != 1 || files[0].PatchID != "" {
		t.Fatalf("legacy diff fingerprint: %+v err=%v", files, err)
	}
}
