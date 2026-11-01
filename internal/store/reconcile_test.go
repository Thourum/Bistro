package store

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestMigrationRunnerReconcilesPartialReadStateSchema(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	migrateEmbeddedSchemaToVersion(t, ctx, db.DB, 33)
	mustExec(t, db, `CREATE TABLE pr_read_state (pr_id TEXT NOT NULL PRIMARY KEY)`)
	mustExec(t, db, `DROP TABLE `+goose.DefaultTablename)

	reconciledVersion, err := reconciledSchemaVersion(ctx, db.DB, reconcileBaselineVersion)
	if err != nil {
		t.Fatalf("reconciledSchemaVersion() error = %v", err)
	}
	if reconciledVersion != preReadStateSchemaVersion {
		t.Errorf("reconciled schema version = %d, want %d", reconciledVersion, preReadStateSchemaVersion)
	}
}

// Migration 33 uses a bare ALTER TABLE ADD COLUMN, which SQLite cannot guard
// with IF NOT EXISTS. Idempotency is therefore provided by the runner: goose
// version tracking plus schema reconciliation must keep the statement from
// running twice, including when goose metadata is lost.
func TestMigration033IsIdempotentAcrossRunsAndLostGooseMetadata(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	runner := NewMigrationRunner(nil, db.DB)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	mustExec(t, db, `DROP TABLE `+goose.DefaultTablename)
	if err := NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("Run() after losing goose metadata error = %v, want mentioned column not re-added", err)
	}

	assertColumnExists(t, db.DB, "prs", "mentioned")
	assertAllEmbeddedVersionsApplied(t, ctx, db.DB)

	var mentionedColumnCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('prs') WHERE name = 'mentioned'`).Scan(&mentionedColumnCount); err != nil {
		t.Fatalf("count prs.mentioned columns: %v", err)
	}
	if mentionedColumnCount != 1 {
		t.Errorf("prs.mentioned column count = %d, want 1", mentionedColumnCount)
	}
}

func TestMigrationRunnerRepairsReadStateWithoutCompositeKey(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	migrateEmbeddedSchemaToVersion(t, ctx, db.DB, 33)
	mustExec(t, db, `CREATE TABLE pr_read_state (
		pr_id TEXT NOT NULL,
		login TEXT NOT NULL,
		last_read_at TIMESTAMP NOT NULL
	)`)
	mustExec(t, db, `INSERT INTO pr_read_state (pr_id, login, last_read_at)
		VALUES ('pr-1', 'alice', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `DROP TABLE `+goose.DefaultTablename)

	if err := NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if exists, err := readStateConflictTargetExists(ctx, db.DB); err != nil {
		t.Fatalf("check repaired conflict target: %v", err)
	} else if !exists {
		t.Fatal("pr_read_state was not repaired with composite conflict target")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pr_read_state WHERE pr_id = 'pr-1' AND login = 'alice'`).Scan(&count); err != nil {
		t.Fatalf("check preserved read state: %v", err)
	}
	if count != 1 {
		t.Fatalf("preserved read-state rows = %d, want 1", count)
	}
	assertAllEmbeddedVersionsApplied(t, ctx, db.DB)
}

func TestMigrationRunnerReconcilesSchemaWithoutReadStateTable(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	migrateEmbeddedSchemaToVersion(t, ctx, db.DB, 33)
	mustExec(t, db, `DROP TABLE `+goose.DefaultTablename)

	if err := NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if exists, err := tableExists(ctx, db.DB, "pr_read_state"); err != nil {
		t.Fatalf("check pr_read_state table: %v", err)
	} else if !exists {
		t.Fatal("pr_read_state table does not exist")
	}
	assertAllEmbeddedVersionsApplied(t, ctx, db.DB)
}

func TestMigrationRunnerReconcilesSchemaWithoutMentionedColumn(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	migrateEmbeddedSchemaToVersion(t, ctx, db.DB, 32)
	mustExec(t, db, `DROP TABLE `+goose.DefaultTablename)

	runner := NewMigrationRunner(nil, db.DB)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertColumnExists(t, db.DB, "prs", "mentioned")
	assertAllEmbeddedVersionsApplied(t, ctx, db.DB)

	if err := runner.Run(ctx); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	assertAllEmbeddedVersionsApplied(t, ctx, db.DB)
}

func TestMigrationRunnerReconcilesSchemaWithoutHydratedColumn(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	migrateEmbeddedSchemaToVersion(t, ctx, db.DB, 31)
	mustExec(t, db, `DROP TABLE `+goose.DefaultTablename)

	runner := NewMigrationRunner(nil, db.DB)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertColumnExists(t, db.DB, "prs", "hydrated")
	assertAllEmbeddedVersionsApplied(t, ctx, db.DB)

	if err := runner.Run(ctx); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	assertAllEmbeddedVersionsApplied(t, ctx, db.DB)
}

func TestMigrationRunnerReconcilesSchemaWithHydratedColumn(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	migrateEmbeddedSchemaToVersion(t, ctx, db.DB, reconcileBaselineVersion)
	mustExec(t, db, `DROP TABLE `+goose.DefaultTablename)

	if err := NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertAllEmbeddedVersionsApplied(t, ctx, db.DB)
}

func TestMigrationRunnerDoesNotReconcilePre31Schema(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	migrateEmbeddedSchemaToVersion(t, ctx, db.DB, 30)
	mustExec(t, db, `DROP TABLE `+goose.DefaultTablename)

	runner := NewMigrationRunner(nil, db.DB)
	if err := runner.reconcileGooseFromCurrentSchema(ctx, reconcileBaselineVersion); err != nil {
		t.Fatalf("reconcileGooseFromCurrentSchema() error = %v", err)
	}
	assertNoEmbeddedVersionsApplied(t, ctx, db.DB)
}

func TestCurrentSchemaExistsRejectsPre31Schema(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	mustExec(t, db, `CREATE TABLE prs (id TEXT PRIMARY KEY, repo_id TEXT)`)
	mustExec(t, db, `CREATE TABLE pr_diffs (pr_id TEXT, file_path TEXT, diff_status TEXT)`)
	mustExec(t, db, `CREATE TABLE repos (id TEXT PRIMARY KEY, local_path TEXT)`)
	mustExec(t, db, `CREATE TABLE comments (id TEXT PRIMARY KEY, author_type TEXT, dedup_group_id TEXT)`)
	mustExec(t, db, `CREATE TABLE comment_classification_state (comment_id TEXT PRIMARY KEY)`)

	exists, err := currentSchemaExists(ctx, db.DB)
	if err != nil {
		t.Fatalf("currentSchemaExists: %v", err)
	}
	if exists {
		t.Fatal("currentSchemaExists = true, want false before file_expertise exists")
	}
}

func TestReconcilePreservesMigrationsNewerThanBaseline(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	runner := NewMigrationRunner(log, db.DB)

	ctx := context.Background()
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("first Run() failed: %v", err)
	}
	mustExec(t, db, `DELETE FROM `+goose.DefaultTablename)

	// Reconcile against a baseline older than the newest embedded migrations,
	// simulating a binary whose embedded set is newer than what
	// currentSchemaExists recognizes. Versions past the baseline must stay
	// unmarked so goose still executes them.
	const staleBaseline int64 = 25
	if err := runner.reconcileGooseFromCurrentSchema(ctx, staleBaseline); err != nil {
		t.Fatalf("reconcile with stale baseline failed: %v", err)
	}

	var newerMarked int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM `+goose.DefaultTablename+` WHERE version_id > ? AND is_applied = 1`,
		staleBaseline,
	).Scan(&newerMarked); err != nil {
		t.Fatalf("query goose versions past baseline: %v", err)
	}
	if newerMarked != 0 {
		t.Errorf("reconcile marked %d migrations newer than baseline %d as applied, want 0", newerMarked, staleBaseline)
	}

	var baselineMarked int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT version_id) FROM `+goose.DefaultTablename+` WHERE version_id > 0 AND version_id <= ? AND is_applied = 1`,
		staleBaseline,
	).Scan(&baselineMarked); err != nil {
		t.Fatalf("query goose versions within baseline: %v", err)
	}
	if baselineMarked == 0 {
		t.Error("reconcile marked no migrations within baseline as applied, want > 0")
	}
}
