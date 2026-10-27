package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestReviewDrafts_CreateListUpdateDelete_ScopedToLogin(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t).DB
	drafts := NewReviewDrafts(db)

	single, err := drafts.Create(ctx, "me", ReviewDraft{PRID: "pr-1", Path: "a.go", Side: "RIGHT", Line: 10, Body: "one", CommitSHA: "head"})
	if err != nil {
		t.Fatalf("create single: %v", err)
	}
	if single.ID == "" || single.StartLine != 0 || single.CreatedAt.IsZero() {
		t.Fatalf("single draft = %+v, want generated id, StartLine 0 and created time", single)
	}
	ranged, err := drafts.Create(ctx, "me", ReviewDraft{PRID: "pr-1", Path: "b.go", Side: "LEFT", StartLine: 3, Line: 5, Body: "range", CommitSHA: "old"})
	if err != nil {
		t.Fatalf("create range: %v", err)
	}
	if _, err := drafts.Create(ctx, "other", ReviewDraft{PRID: "pr-1", Path: "a.go", Side: "RIGHT", Line: 1, Body: "theirs", CommitSHA: "head"}); err != nil {
		t.Fatalf("create other: %v", err)
	}
	if _, err := drafts.Create(ctx, "me", ReviewDraft{PRID: "pr-2", Path: "a.go", Side: "RIGHT", Line: 1, Body: "another PR", CommitSHA: "head"}); err != nil {
		t.Fatalf("create another PR: %v", err)
	}
	// Second-resolution defaults and random ids do not imply insertion order.
	mustExec(t, db, `UPDATE review_drafts SET created_at = '2026-01-01T00:00:00Z' WHERE id = ?`, single.ID)
	mustExec(t, db, `UPDATE review_drafts SET created_at = '2026-01-02T00:00:00Z' WHERE id = ?`, ranged.ID)

	mine, err := drafts.List(ctx, "pr-1", "me")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(mine) != 2 || mine[0].ID != single.ID || mine[1].ID != ranged.ID || mine[1].StartLine != 3 || mine[1].Side != "LEFT" || mine[1].CommitSHA != "old" {
		t.Fatalf("list = %+v, want my two drafts with the range intact", mine)
	}

	if _, err := drafts.UpdateBody(ctx, "other", single.ID, "hijack"); !errors.Is(err, ErrDraftNotFound) {
		t.Fatalf("update by another login err = %v, want ErrDraftNotFound", err)
	}
	if _, err := drafts.UpdateBody(ctx, "me", "missing", "edited"); !errors.Is(err, ErrDraftNotFound) {
		t.Fatalf("update missing draft err = %v, want ErrDraftNotFound", err)
	}
	updated, err := drafts.UpdateBody(ctx, "me", single.ID, "edited")
	if err != nil || updated.Body != "edited" || updated.ID != single.ID || updated.CommitSHA != "head" {
		t.Fatalf("update = %+v, %v", updated, err)
	}

	if err := drafts.Delete(ctx, "other", []string{ranged.ID}); err != nil {
		t.Fatalf("delete by other: %v", err)
	}
	if err := drafts.Delete(ctx, "me", []string{single.ID, "missing"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	left, err := drafts.List(ctx, "pr-1", "me")
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(left) != 1 || left[0].ID != ranged.ID {
		t.Fatalf("after delete = %+v, want only the range draft", left)
	}
	if err := drafts.Delete(ctx, "me", []string{ranged.ID}); err != nil {
		t.Fatalf("delete last draft: %v", err)
	}
	empty, err := drafts.List(ctx, "pr-1", "me")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list = %+v, %v, want non-nil empty slice", empty, err)
	}
	if err := drafts.Delete(ctx, "me", nil); err != nil {
		t.Fatalf("delete no drafts: %v", err)
	}
}

func TestReviewDrafts_RejectsInvalidRange(t *testing.T) {
	drafts := NewReviewDrafts(setupTestDB(t).DB)
	for _, test := range []struct {
		name      string
		side      string
		startLine int
		line      int
	}{
		{name: "reversed range", side: "RIGHT", startLine: 9, line: 5},
		{name: "equal endpoints", side: "RIGHT", startLine: 5, line: 5},
		{name: "negative start", side: "RIGHT", startLine: -1, line: 5},
		{name: "zero line", side: "RIGHT", line: 0},
		{name: "negative line", side: "RIGHT", line: -1},
		{name: "invalid side", side: "CENTER", line: 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := drafts.Create(context.Background(), "me", ReviewDraft{PRID: "pr-1", Path: "a.go", Side: test.side, StartLine: test.startLine, Line: test.line, Body: "x", CommitSHA: "head"})
			if err == nil {
				t.Fatal("expected the database constraints to reject the draft")
			}
		})
	}
}

func TestReviewDrafts_ListOrdersByCreatedAtAndID(t *testing.T) {
	db := setupTestDB(t).DB
	const earlier = "2026-01-01T00:00:00Z"
	const later = "2026-01-02T00:00:00Z"
	for _, row := range []struct{ id, createdAt string }{{"b", later}, {"a", later}, {"c", earlier}} {
		mustExec(t, db, `INSERT INTO review_drafts (id, pr_id, login, path, side, line, body, commit_sha, created_at)
			VALUES (?, 'pr-1', 'me', 'a.go', 'RIGHT', 1, 'x', 'head', ?)`, row.id, row.createdAt)
	}
	drafts, err := NewReviewDrafts(db).List(context.Background(), "pr-1", "me")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(drafts) != 3 || drafts[0].ID != "c" || drafts[1].ID != "a" || drafts[2].ID != "b" {
		t.Fatalf("list = %+v, want ids c, a, b", drafts)
	}
}

func TestReviewDrafts_DeleteRollsBackOnFailure(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t).DB
	drafts := NewReviewDrafts(db)
	for _, id := range []string{"first", "second"} {
		mustExec(t, db, `INSERT INTO review_drafts (id, pr_id, login, path, side, line, body, commit_sha)
			VALUES (?, 'pr-1', 'me', 'a.go', 'RIGHT', 1, 'x', 'head')`, id)
	}
	mustExec(t, db, `CREATE TRIGGER fail_second_delete BEFORE DELETE ON review_drafts
		WHEN OLD.id = 'second' BEGIN SELECT RAISE(ABORT, 'blocked delete'); END`)
	if err := drafts.Delete(ctx, "me", []string{"first", "second"}); err == nil {
		t.Fatal("expected delete to fail")
	}
	remaining, err := drafts.List(ctx, "pr-1", "me")
	if err != nil {
		t.Fatalf("list after failed delete: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("after failed delete = %+v, want both drafts preserved", remaining)
	}
}

func TestReviewDrafts_MigrationUpgradeAndMetadataLossPreserveDraft(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	migrateEmbeddedSchemaToVersion(t, ctx, db.DB, 34)
	runner := NewMigrationRunner(nil, db.DB)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	drafts := NewReviewDrafts(db.DB)
	draft, err := drafts.Create(ctx, "me", ReviewDraft{PRID: "pr-1", Path: "a.go", Side: "RIGHT", Line: 10, Body: "one", CommitSHA: "head"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	mustExec(t, db, `DROP TABLE `+goose.DefaultTablename)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("recover metadata: %v", err)
	}
	remaining, err := drafts.List(ctx, "pr-1", "me")
	if err != nil || len(remaining) != 1 || remaining[0] != draft {
		t.Fatalf("after migration replay = %+v, %v, want original draft %+v", remaining, err, draft)
	}
}

func TestReviewDrafts_Migration36BranchUpgrades(t *testing.T) {
	for _, branch := range []string{"feature", "master", "feature-lost-metadata"} {
		t.Run(branch, func(t *testing.T) {
			ctx := context.Background()
			db, draft := prepareMigration36Draft(t, branch)
			want := draft
			want.PRID = "github:owner/repo:1"
			for _, metadata := range []string{"retained", "lost", "replayed"} {
				if metadata == "lost" {
					mustExec(t, db, `DROP TABLE `+goose.DefaultTablename)
				}
				if err := NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
					t.Fatalf("%s metadata: migrate: %v", metadata, err)
				}
				remaining, err := NewReviewDrafts(db.DB).List(ctx, want.PRID, "Viewer")
				if err != nil || len(remaining) != 1 || remaining[0] != want {
					t.Fatalf("%s metadata: drafts = %+v, %v, want %+v", metadata, remaining, err, want)
				}
				var repoID string
				if err := db.QueryRowContext(ctx, `SELECT id FROM repos`).Scan(&repoID); err != nil || repoID != "github:owner/repo" {
					t.Fatalf("%s metadata: repo = %q, %v, want canonical identity", metadata, repoID, err)
				}
				var foreignKeyViolations int
				if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&foreignKeyViolations); err != nil || foreignKeyViolations != 0 {
					t.Fatalf("%s metadata: FK violations = %d, %v", metadata, foreignKeyViolations, err)
				}
				assertAllEmbeddedVersionsApplied(t, ctx, db.DB)
			}
		})
	}
}

func prepareMigration36Draft(t *testing.T, branch string) (*DB, ReviewDraft) {
	t.Helper()
	ctx := context.Background()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const preBranchMigrationVersion = 35
	migrateEmbeddedSchemaToVersion(t, ctx, db.DB, preBranchMigrationVersion)
	const repoID = "github:Owner/Repo"
	const prID = repoID + ":1"
	mustExec(t, db, `INSERT INTO repos (id, provider, owner, repo) VALUES (?, 'github', 'Owner', 'Repo')`, repoID)
	mustExec(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha, repo_id)
		VALUES (?, 'github', 'Owner', 'Repo', 1, 'T', 'a', 'open',
		'2026-01-01', '2026-01-01', 'main', 'f', 'base', 'head', ?)`, prID, repoID)
	if branch == "master" {
		const repoIdentityMigrationVersion = 36
		migrateEmbeddedSchemaToVersion(t, ctx, db.DB, repoIdentityMigrationVersion)
		if err := NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
			t.Fatalf("upgrade master database: %v", err)
		}
	}
	if branch != "master" {
		// The old feature branch created this schema under version 36, not 37.
		draftSQL, err := migrationsFS.ReadFile("migrations/00037_add_review_drafts.sql")
		if err != nil {
			t.Fatalf("read draft migration: %v", err)
		}
		mustExec(t, db, string(draftSQL))
		mustExec(t, db, `INSERT INTO `+goose.DefaultTablename+` (version_id, is_applied) VALUES (36, 1)`)
	}
	draftPRID := prID
	if branch == "master" {
		draftPRID = "github:owner/repo:1"
	}
	draft, err := NewReviewDrafts(db.DB).Create(ctx, "Viewer", ReviewDraft{
		PRID: draftPRID, Path: "Src/File.go", Side: "RIGHT", StartLine: 3, Line: 5, Body: "Keep Casing", CommitSHA: "HeadSHA",
	})
	if err != nil {
		t.Fatalf("create legacy draft: %v", err)
	}
	if branch == "feature-lost-metadata" {
		mustExec(t, db, `DROP TABLE `+goose.DefaultTablename)
	}
	return db, draft
}

func TestReviewDrafts_MigrationRepairPreservesNewerTracking(t *testing.T) {
	ctx := context.Background()
	db, _ := prepareMigration36Draft(t, "feature")
	const newerMigrationVersion = 38
	mustExec(t, db, `INSERT INTO `+goose.DefaultTablename+` (version_id, is_applied) VALUES (?, 1)`, newerMigrationVersion)
	if err := NewMigrationRunner(nil, db.DB).repairLegacyReviewDraftsVersion(ctx); err != nil {
		t.Fatalf("repair legacy tracking: %v", err)
	}
	var trackedCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+goose.DefaultTablename+` WHERE version_id IN (36, ?) AND is_applied = 1`, newerMigrationVersion).Scan(&trackedCount); err != nil || trackedCount != 2 {
		t.Fatalf("tracked versions 36 and %d = %d, %v, want both preserved", newerMigrationVersion, trackedCount, err)
	}
}

func TestReviewDrafts_Migration36FailureCanRetry(t *testing.T) {
	ctx := context.Background()
	db, draft := prepareMigration36Draft(t, "feature")
	mustExec(t, db, `CREATE TRIGGER fail_repo_fold BEFORE UPDATE ON repos
		BEGIN SELECT RAISE(ABORT, 'blocked fold'); END`)
	if err := NewMigrationRunner(nil, db.DB).Run(ctx); err == nil {
		t.Fatal("expected migration failure")
	}
	remaining, err := NewReviewDrafts(db.DB).List(ctx, draft.PRID, "Viewer")
	if err != nil || len(remaining) != 1 || remaining[0] != draft {
		t.Fatalf("draft after failed fold = %+v, %v, want %+v", remaining, err, draft)
	}
	var repoID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM repos`).Scan(&repoID); err != nil || repoID != "github:Owner/Repo" {
		t.Fatalf("failed fold did not roll back: repo = %q, %v", repoID, err)
	}
	mustExec(t, db, `DROP TRIGGER fail_repo_fold`)
	if err := NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("retry fold: %v", err)
	}
	draft.PRID = "github:owner/repo:1"
	remaining, err = NewReviewDrafts(db.DB).List(ctx, draft.PRID, "Viewer")
	if err != nil || len(remaining) != 1 || remaining[0] != draft {
		t.Fatalf("draft after retry = %+v, %v, want %+v", remaining, err, draft)
	}
	assertAllEmbeddedVersionsApplied(t, ctx, db.DB)
}

func TestReviewDrafts_Migration37FailureCanRetry(t *testing.T) {
	ctx := context.Background()
	db, draft := prepareMigration36Draft(t, "feature")
	mustExec(t, db, `CREATE TRIGGER fail_draft_fold BEFORE UPDATE ON review_drafts
		BEGIN SELECT RAISE(ABORT, 'blocked draft fold'); END`)
	if err := NewMigrationRunner(nil, db.DB).Run(ctx); err == nil {
		t.Fatal("expected draft migration failure")
	}
	var repoID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM repos`).Scan(&repoID); err != nil || repoID != "github:owner/repo" {
		t.Fatalf("migration 36 did not commit: repo = %q, %v", repoID, err)
	}
	remaining, err := NewReviewDrafts(db.DB).List(ctx, draft.PRID, "Viewer")
	if err != nil || len(remaining) != 1 || remaining[0] != draft {
		t.Fatalf("draft after failed migration 37 = %+v, %v, want %+v", remaining, err, draft)
	}
	mustExec(t, db, `DROP TRIGGER fail_draft_fold`)
	if err := NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("retry draft migration: %v", err)
	}
	draft.PRID = "github:owner/repo:1"
	remaining, err = NewReviewDrafts(db.DB).List(ctx, draft.PRID, "Viewer")
	if err != nil || len(remaining) != 1 || remaining[0] != draft {
		t.Fatalf("draft after retry = %+v, %v, want %+v", remaining, err, draft)
	}
	assertAllEmbeddedVersionsApplied(t, ctx, db.DB)
}

func TestReviewDrafts_Migration37PreservesAccountsAndEvictedDrafts(t *testing.T) {
	ctx := context.Background()
	db, legacy := prepareMigration36Draft(t, "feature")
	drafts := NewReviewDrafts(db.DB)
	expected := map[string][]ReviewDraft{}
	for _, account := range []string{"Viewer", "OtherViewer"} {
		for _, prID := range []string{"github:owner/repo:1", "github:Evicted/Repo:2", "gitea:Case/Sensitive:3"} {
			draft, err := drafts.Create(ctx, account, ReviewDraft{
				PRID: prID, Path: "Src/File.go", Side: "LEFT", Line: 2, Body: "Keep Casing; DROP TABLE prs;", CommitSHA: "HeadSHA",
			})
			if err != nil {
				t.Fatalf("create draft: %v", err)
			}
			if prID == "github:Evicted/Repo:2" {
				draft.PRID = "github:evicted/repo:2"
			}
			expected[account] = append(expected[account], draft)
		}
	}
	if err := NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("migrate drafts: %v", err)
	}
	for account, accountDrafts := range expected {
		for _, want := range accountDrafts {
			remaining, err := drafts.List(ctx, want.PRID, account)
			if err != nil || !slices.Contains(remaining, want) {
				t.Fatalf("%s drafts = %+v, %v, want %+v", account, remaining, err, want)
			}
		}
	}
	legacy.PRID = "github:owner/repo:1"
	remaining, err := drafts.List(ctx, legacy.PRID, "Viewer")
	if err != nil || len(remaining) != 2 || !slices.Contains(remaining, legacy) {
		t.Fatalf("canonical and legacy drafts = %+v, %v, want both Viewer drafts", remaining, err)
	}
}

func TestReviewDrafts_SurvivesPRCacheEviction(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t).DB
	mustInsertPR(t, db, "pr-1")
	drafts := NewReviewDrafts(db)
	draft, err := drafts.Create(ctx, "me", ReviewDraft{PRID: "pr-1", Path: "a.go", Side: "RIGHT", Line: 10, Body: "one", CommitSHA: "head"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	mustExec(t, db, `DELETE FROM prs WHERE id = 'pr-1'`)
	remaining, err := drafts.List(ctx, "pr-1", "me")
	if err != nil {
		t.Fatalf("list after cache eviction: %v", err)
	}
	if len(remaining) != 1 || remaining[0] != draft {
		t.Fatalf("after cache eviction = %+v, want original draft %+v", remaining, draft)
	}
}
