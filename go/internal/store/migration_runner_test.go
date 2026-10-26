package store

import (
	"context"
	"database/sql"
	"io/fs"
	"log/slog"
	"os"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestMigrationsRerunAfterGooseVersionMetadataLoss(t *testing.T) {
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

	if err := runner.Run(ctx); err != nil {
		t.Fatalf("rerun after goose metadata loss failed: %v", err)
	}

	assertColumnExists(t, db.DB, "prs", "repo_id")
	assertColumnExists(t, db.DB, "pr_diffs", "diff_status")
	for _, columnName := range []string{
		"local_path",
		"source",
		"last_used_at",
		"location_status",
		"location_error_code",
		"location_error",
	} {
		assertColumnExists(t, db.DB, "repos", columnName)
	}
	for _, columnName := range []string{
		"author_type",
		"bot_vendor",
		"severity",
		"classification_source",
	} {
		assertColumnExists(t, db.DB, "comments", columnName)
	}
	if exists, err := tableExists(ctx, db.DB, "comment_classification_state"); err != nil {
		t.Fatalf("check comment_classification_state table: %v", err)
	} else if !exists {
		t.Fatal("comment_classification_state table does not exist")
	}

	assertAllEmbeddedVersionsApplied(t, ctx, db.DB)

	if exists, err := tableExists(ctx, db.DB, "pr_read_state"); err != nil {
		t.Fatalf("check pr_read_state table: %v", err)
	} else if !exists {
		t.Fatal("pr_read_state table does not exist")
	}
}

func TestMigration034AddsPRReadState(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	for _, columnName := range []string{"pr_id", "login", "last_read_at"} {
		assertColumnExists(t, db.DB, "pr_read_state", columnName)
	}

	mustExec(t, db, `INSERT INTO pr_read_state (pr_id, login, last_read_at) VALUES ('pr-1', 'alice', '2026-01-01T00:00:00Z')`)
	mustExec(t, db, `INSERT INTO pr_read_state (pr_id, login, last_read_at) VALUES ('pr-1', 'alice', '2026-01-02T00:00:00Z')
		ON CONFLICT(pr_id, login) DO UPDATE SET last_read_at = excluded.last_read_at`)

	var lastReadAt string
	if err := db.QueryRowContext(ctx, `SELECT last_read_at FROM pr_read_state WHERE pr_id = 'pr-1' AND login = 'alice'`).Scan(&lastReadAt); err != nil {
		t.Fatalf("query read state: %v", err)
	}
	if lastReadAt != "2026-01-02T00:00:00Z" {
		t.Errorf("last_read_at = %q, want updated value", lastReadAt)
	}
}

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

func assertAllEmbeddedVersionsApplied(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	versions, err := schemaMigrationVersions()
	if err != nil {
		t.Fatalf("schemaMigrationVersions() error = %v", err)
	}
	// Version 13 was removed before goose adoption; all other versions remain embedded.
	const removedMigrationCount = 1
	wantCount := int(reconcileBaselineVersion) - removedMigrationCount
	if len(versions) != wantCount {
		t.Fatalf("embedded migration count = %d, want %d through version %d excluding version 13", len(versions), wantCount, reconcileBaselineVersion)
	}
	for _, version := range versions {
		var appliedCount int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+goose.DefaultTablename+` WHERE version_id = ? AND is_applied = 1`, version).Scan(&appliedCount); err != nil {
			t.Fatalf("query applied migration version %d: %v", version, err)
		}
		if appliedCount != 1 {
			t.Errorf("applied migration version %d count = %d, want 1", version, appliedCount)
		}
	}
}

func assertNoEmbeddedVersionsApplied(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	var appliedCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+goose.DefaultTablename+` WHERE version_id > 0 AND is_applied = 1`).Scan(&appliedCount); err != nil {
		t.Fatalf("query applied migration versions: %v", err)
	}
	if appliedCount != 0 {
		t.Errorf("applied migration version count = %d, want 0", appliedCount)
	}
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

func TestMigrationRunnerHasGooseMigrationFiles(t *testing.T) {
	migrationFiles, err := fs.Glob(migrationsFS, "migrations/[0-9][0-9][0-9][0-9][0-9]_*.sql")
	if err != nil {
		t.Fatalf("glob goose migration files: %v", err)
	}
	if len(migrationFiles) == 0 {
		t.Error("expected at least one goose migration file")
	}
}

func TestMigrationRunnerRecordsVersions(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	runner := NewMigrationRunner(log, db.DB)

	ctx := context.Background()
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var migCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+goose.DefaultTablename+` WHERE version_id > 0 AND is_applied = 1`).Scan(&migCount); err != nil {
		t.Fatalf("failed to query %s: %v", goose.DefaultTablename, err)
	}
	if migCount == 0 {
		t.Errorf("expected migrations to be recorded in %s", goose.DefaultTablename)
	}
}

func TestMigrationRunnerSeedsGooseFromLegacySchemaMigrations(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	mustExec(t, db, `CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		description TEXT NOT NULL,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	for version := 0; version <= 29; version++ {
		if version == 13 {
			continue
		}
		mustExec(t, db, `INSERT INTO schema_migrations (version, description) VALUES (?, ?)`, version, "pre-applied")
	}
	mustExec(t, db, `CREATE TABLE prs (id TEXT PRIMARY KEY)`)
	mustExec(t, db, `CREATE TABLE comments (id TEXT PRIMARY KEY)`)
	mustExec(t, db, `CREATE TABLE files (id TEXT PRIMARY KEY)`)
	mustExec(t, db, `CREATE TABLE patchsets (
		pr_id TEXT NOT NULL, number INTEGER NOT NULL, head_sha TEXT NOT NULL,
		base_sha TEXT NOT NULL, pushed_at TIMESTAMP NOT NULL, PRIMARY KEY (pr_id, number)
	)`)
	mustExec(t, db, `CREATE TABLE pr_diffs (pr_id TEXT NOT NULL, file_path TEXT NOT NULL)`)
	mustExec(t, db, `CREATE TABLE last_reviewed_sha (
		pr_id TEXT NOT NULL, login TEXT NOT NULL, sha TEXT NOT NULL, updated_at TIMESTAMP NOT NULL
	)`)

	if err := NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	assertAllEmbeddedVersionsApplied(t, ctx, db.DB)

	var prsSQL string
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='prs'`).Scan(&prsSQL); err != nil {
		t.Fatalf("query prs schema: %v", err)
	}
	// Migrations past the legacy seed (00032 adds hydrated and 00033 adds
	// mentioned) still apply, but the table must not have been dropped and
	// re-created.
	if prsSQL != "CREATE TABLE prs (id TEXT PRIMARY KEY, hydrated INTEGER NOT NULL DEFAULT 1, mentioned INTEGER NOT NULL DEFAULT 0)" {
		t.Errorf("prs table was re-migrated; schema = %q", prsSQL)
	}
}
