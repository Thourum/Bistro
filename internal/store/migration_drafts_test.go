package store

import (
	"context"
	"slices"
	"testing"

	"github.com/itterno/bistro/internal/store/reviewdraft"
	"github.com/pressly/goose/v3"
)

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
	drafts := reviewdraft.New(db.DB)
	draft, err := drafts.Create(ctx, "me", reviewdraft.Draft{PRID: "pr-1", Path: "a.go", Side: "RIGHT", Line: 10, Body: "one", CommitSHA: "head"})
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
				remaining, err := reviewdraft.New(db.DB).List(ctx, want.PRID, "Viewer")
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

func prepareMigration36Draft(t *testing.T, branch string) (*DB, reviewdraft.Draft) {
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
	draft, err := reviewdraft.New(db.DB).Create(ctx, "Viewer", reviewdraft.Draft{
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
	remaining, err := reviewdraft.New(db.DB).List(ctx, draft.PRID, "Viewer")
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
	remaining, err = reviewdraft.New(db.DB).List(ctx, draft.PRID, "Viewer")
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
	remaining, err := reviewdraft.New(db.DB).List(ctx, draft.PRID, "Viewer")
	if err != nil || len(remaining) != 1 || remaining[0] != draft {
		t.Fatalf("draft after failed migration 37 = %+v, %v, want %+v", remaining, err, draft)
	}
	mustExec(t, db, `DROP TRIGGER fail_draft_fold`)
	if err := NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("retry draft migration: %v", err)
	}
	draft.PRID = "github:owner/repo:1"
	remaining, err = reviewdraft.New(db.DB).List(ctx, draft.PRID, "Viewer")
	if err != nil || len(remaining) != 1 || remaining[0] != draft {
		t.Fatalf("draft after retry = %+v, %v, want %+v", remaining, err, draft)
	}
	assertAllEmbeddedVersionsApplied(t, ctx, db.DB)
}

func TestReviewDrafts_Migration37PreservesAccountsAndEvictedDrafts(t *testing.T) {
	ctx := context.Background()
	db, legacy := prepareMigration36Draft(t, "feature")
	drafts := reviewdraft.New(db.DB)
	expected := map[string][]reviewdraft.Draft{}
	for _, account := range []string{"Viewer", "OtherViewer"} {
		for _, prID := range []string{"github:owner/repo:1", "github:Evicted/Repo:2", "gitea:Case/Sensitive:3"} {
			draft, err := drafts.Create(ctx, account, reviewdraft.Draft{
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
	db := newTestDB(t).DB
	mustInsertPR(t, db, "pr-1")
	drafts := reviewdraft.New(db)
	draft, err := drafts.Create(ctx, "me", reviewdraft.Draft{PRID: "pr-1", Path: "a.go", Side: "RIGHT", Line: 10, Body: "one", CommitSHA: "head"})
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
