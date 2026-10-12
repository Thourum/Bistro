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

// newTestDB opens an :memory: DB and runs all migrations. Registered as a
// t.Cleanup so the connection is closed when the test exits.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("newTestDB open: %v", err)
	}
	if err := NewMigrationRunner(nil, db.DB).Run(context.Background()); err != nil {
		t.Fatalf("newTestDB migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// mustExec executes a SQL statement, failing the test on error.
func mustExec(t *testing.T, db interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}, query string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("mustExec(%q): %v", query, err)
	}
}

func migrateEmbeddedSchemaToVersion(t *testing.T, ctx context.Context, db *sql.DB, version int64) {
	t.Helper()

	migrationFS, err := fs.Sub(migrationsFS, migrationsDir)
	if err != nil {
		t.Fatalf("open embedded migrations: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, migrationFS, goose.WithLogger(goose.NopLogger()))
	if err != nil {
		t.Fatalf("create goose provider: %v", err)
	}
	if _, err := provider.UpTo(ctx, version); err != nil {
		t.Fatalf("run migrations through version %d: %v", version, err)
	}
}

// mustInsertPR inserts a minimal PR row for FK satisfaction in tests.
// Uses the prID as the number suffix to avoid UNIQUE(provider,owner,repo,number) collisions.
func mustInsertPR(t *testing.T, db interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}, prID string) {
	t.Helper()
	// Use a hash of prID as the PR number to avoid UNIQUE constraint collisions.
	num := 0
	for _, c := range prID {
		num = num*31 + int(c)
	}
	if num < 0 {
		num = -num
	}
	num = (num % 100000) + 1
	mustExec(t, db,
		`INSERT OR IGNORE INTO prs
			(id, provider, owner, repo, number, title, author, state,
			 created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		 VALUES (?, 'github', 'o', 'r', ?, 'T', 'a', 'open',
		         '2026-01-01', '2026-01-01', 'main', 'f', 'base', 'head')`,
		prID, num,
	)
}

func TestOpenDB(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping db: %v", err)
	}
}

func TestMigrations(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	runner := NewMigrationRunner(log, db.DB)

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	tables := []string{"prs", "comments", "files", "schema_migrations", goose.DefaultTablename}
	for _, table := range tables {
		var count int
		err = db.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&count)
		if err != nil || count == 0 {
			t.Errorf("%s table not created", table)
		}
	}
}

func TestMigrationsIdempotent(t *testing.T) {
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
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("second Run() failed (not idempotent): %v", err)
	}
}

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

	var appliedCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT version_id) FROM `+goose.DefaultTablename+` WHERE version_id > 0 AND is_applied = 1`).Scan(&appliedCount); err != nil {
		t.Fatalf("query applied goose versions: %v", err)
	}
	if appliedCount != 33 {
		t.Errorf("applied goose version count = %d, want 33", appliedCount)
	}

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
	if len(versions) != 33 {
		t.Fatalf("embedded migration count = %d, want 33 through version 34 excluding version 13", len(versions))
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

	if err := NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var appliedCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+goose.DefaultTablename+` WHERE version_id > 0 AND is_applied = 1`).Scan(&appliedCount); err != nil {
		t.Fatalf("query applied goose versions: %v", err)
	}
	if appliedCount != 33 {
		t.Errorf("applied goose version count = %d, want 33", appliedCount)
	}

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

// TestRateLimitStateProviderKeyMigration verifies that migration 011 correctly
// rebuilds rate_limit_state so that:
//
//	(a) two different provider ids coexist as distinct rows without overwriting each other, and
//	(b) the pre-existing 'github' row's data survives the migration intact.
func TestRateLimitStateProviderKeyMigration(t *testing.T) {
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

	// Seed a 'github' row (simulates data that existed before migration 011).
	const seed = `INSERT INTO rate_limit_state (id, provider, reset_at, remaining_requests, limit_requests)
	              VALUES ('github', 'github', '2099-01-01T00:00:00Z', 42, 5000)`
	if _, err := db.ExecContext(ctx, seed); err != nil {
		t.Fatalf("seed github row: %v", err)
	}

	// Insert a second provider to confirm two rows can coexist.
	const gitlab = `INSERT INTO rate_limit_state (id, provider, reset_at, remaining_requests, limit_requests)
	                VALUES ('gitlab', 'gitlab', '2099-06-01T00:00:00Z', 99, 2000)`
	if _, err := db.ExecContext(ctx, gitlab); err != nil {
		t.Fatalf("insert gitlab row: %v", err)
	}

	// (a) Both rows must be present as distinct rows.
	var rowCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM rate_limit_state").Scan(&rowCount); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rowCount != 2 {
		t.Errorf("expected 2 distinct rows in rate_limit_state, got %d", rowCount)
	}

	// (b) The original github row's data survived unchanged.
	var remaining int
	if err := db.QueryRowContext(ctx,
		"SELECT remaining_requests FROM rate_limit_state WHERE id = 'github'",
	).Scan(&remaining); err != nil {
		t.Fatalf("query github row: %v", err)
	}
	if remaining != 42 {
		t.Errorf("github row remaining_requests: want 42, got %d", remaining)
	}

	// Confirm the gitlab row is independent.
	var gitlabRemaining int
	if err := db.QueryRowContext(ctx,
		"SELECT remaining_requests FROM rate_limit_state WHERE id = 'gitlab'",
	).Scan(&gitlabRemaining); err != nil {
		t.Fatalf("query gitlab row: %v", err)
	}
	if gitlabRemaining != 99 {
		t.Errorf("gitlab row remaining_requests: want 99, got %d", gitlabRemaining)
	}
}

// tableColumns returns a set of column names for the given table.
func tableColumns(t *testing.T, db interface {
	Query(string, ...interface{}) (*sql.Rows, error)
}, table string) map[string]bool {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		t.Fatalf("pragma_table_info(%s): %v", table, err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		cols[n] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("tableColumns rows.Err: %v", err)
	}
	return cols
}

func assertColumnExists(t *testing.T, db interface {
	Query(string, ...interface{}) (*sql.Rows, error)
}, table string, column string) {
	t.Helper()
	if !tableColumns(t, db, table)[column] {
		t.Fatalf("%s missing column %q", table, column)
	}
}

func TestMigration012_Schema(t *testing.T) {
	db := newTestDB(t)

	// --- review_threads table exists; FK enforced ---
	if _, err := db.Exec(`INSERT INTO review_threads (id, pr_id) VALUES ('rt1','nonexistent-pr')`); err == nil {
		t.Error("expected FK violation inserting review_thread with nonexistent pr_id")
	}

	// --- comments new columns exist ---
	cols := tableColumns(t, db, "comments")
	for _, want := range []string{
		"kind", "commit_sha", "original_sha", "side",
		"position", "original_position", "in_reply_to", "thread_id", "patchset_number",
	} {
		if !cols[want] {
			t.Errorf("comments missing column %q", want)
		}
	}

	// --- existing comments get default kind='issue' ---
	mustInsertPR(t, db, "p1")
	mustExec(t, db, `INSERT INTO comments (id, pr_id, author, body, created_at, updated_at)
		VALUES ('c1','p1','alice','hi','2026-01-01','2026-01-01')`)
	var kind string
	if err := db.QueryRow(`SELECT kind FROM comments WHERE id='c1'`).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if kind != "issue" {
		t.Errorf("default kind: got %q want 'issue'", kind)
	}

	// --- thread_id FK: bogus thread rejected ---
	if _, err := db.Exec(`UPDATE comments SET thread_id='bogus-thread' WHERE id='c1'`); err == nil {
		t.Error("expected FK violation on thread_id referencing nonexistent review_thread")
	}

	// --- deleting thread NULLs comments.thread_id (ON DELETE SET NULL) ---
	mustInsertPR(t, db, "p2")
	mustExec(t, db, `INSERT INTO review_threads (id, pr_id) VALUES ('rt1','p2')`)
	mustExec(t, db, `INSERT INTO comments (id, pr_id, author, body, created_at, updated_at, kind, thread_id)
		VALUES ('c2','p2','bob','review','2026-01-01','2026-01-01','review','rt1')`)
	mustExec(t, db, `DELETE FROM review_threads WHERE id='rt1'`)
	var tid *string
	if err := db.QueryRow(`SELECT thread_id FROM comments WHERE id='c2'`).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	if tid != nil {
		t.Errorf("thread_id should be NULL after thread deleted, got %q", *tid)
	}

	// --- patchsets table + UNIQUE constraint ---
	mustExec(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('p1', 1, 'sha-a', 'base', CURRENT_TIMESTAMP)`)
	mustExec(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('p1', 2, 'sha-a', 'base-2', CURRENT_TIMESTAMP)`)
	if _, err := db.Exec(`INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('p1', 3, 'sha-a', 'base', CURRENT_TIMESTAMP)`); err == nil {
		t.Error("expected UNIQUE violation on (pr_id, base_sha, head_sha)")
	}

	// --- current_user table ---
	mustExec(t, db, `INSERT INTO current_user (provider, login) VALUES ('github','alice')`)
	mustExec(t, db, `INSERT OR REPLACE INTO current_user (provider, login) VALUES ('github','alice2')`)
	var u string
	db.QueryRow(`SELECT login FROM current_user WHERE provider='github'`).Scan(&u) //nolint:errcheck
	if u != "alice2" {
		t.Errorf("current_user upsert: got %q want 'alice2'", u)
	}
}

func TestMigration014AddsLastReviewedAndFilesViewed(t *testing.T) {
	db := newTestDB(t)

	lastReviewedCols := tableColumns(t, db, "last_reviewed_sha")
	for _, want := range []string{"pr_id", "account_provider", "account_login", "login", "sha", "updated_at"} {
		if !lastReviewedCols[want] {
			t.Errorf("last_reviewed_sha missing column %q", want)
		}
	}

	filesViewedCols := tableColumns(t, db, "files_viewed")
	for _, want := range []string{"pr_id", "file_path", "account_provider", "account_login", "login", "content_hash", "viewed_at"} {
		if !filesViewedCols[want] {
			t.Errorf("files_viewed missing column %q", want)
		}
	}

	mustInsertPR(t, db, "p014")
	mustExec(t, db, `INSERT INTO last_reviewed_sha (pr_id, login, sha) VALUES ('p014', 'alice', 'sha1')`)
	mustExec(t, db, `INSERT OR REPLACE INTO last_reviewed_sha (pr_id, login, sha) VALUES ('p014', 'alice', 'sha2')`)
	var sha string
	if err := db.QueryRow(`SELECT sha FROM last_reviewed_sha WHERE pr_id='p014' AND login='alice'`).Scan(&sha); err != nil {
		t.Fatalf("last_reviewed_sha query: %v", err)
	}
	if sha != "sha2" {
		t.Errorf("last_reviewed_sha upsert: got %q want sha2", sha)
	}

	mustExec(t, db, `INSERT INTO files_viewed (pr_id, file_path, login, content_hash) VALUES ('p014', 'a.go', 'alice', 'hash1')`)
	if _, err := db.Exec(`INSERT INTO files_viewed (pr_id, file_path, login, content_hash) VALUES ('p014', 'a.go', 'alice', 'hash2')`); err == nil {
		t.Error("expected files_viewed PRIMARY KEY violation for duplicate pr/file/login")
	}
}

func TestMigration015AddsAnchorColumns(t *testing.T) {
	db := newTestDB(t)

	cols := tableColumns(t, db, "comments")
	for _, want := range []string{"anchored_line", "anchor_head_sha", "outdated", "outdated_near_line"} {
		if !cols[want] {
			t.Errorf("comments missing anchor column %q", want)
		}
	}

	mustInsertPR(t, db, "p015")
	mustExec(t, db, `INSERT INTO comments (id, pr_id, author, body, created_at, updated_at, kind)
		VALUES ('c015','p015','bob','review','2026-01-01','2026-01-01','review')`)
	var outdated int
	if err := db.QueryRow(`SELECT outdated FROM comments WHERE id='c015'`).Scan(&outdated); err != nil {
		t.Fatalf("query outdated: %v", err)
	}
	if outdated != 0 {
		t.Errorf("outdated default: got %d want 0", outdated)
	}
}

func TestMigration016_UpgradesOldPatchsetsHeadOnlyIdentity(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	ctx := context.Background()

	mustExec(t, db, `CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		description TEXT NOT NULL,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	for version := 1; version <= 12; version++ {
		mustExec(t, db, `INSERT INTO schema_migrations (version, description) VALUES (?, ?)`, version, "pre-applied")
	}
	mustExec(t, db, `CREATE TABLE repos (
		id       TEXT PRIMARY KEY,
		provider TEXT NOT NULL,
		owner    TEXT NOT NULL,
		repo     TEXT NOT NULL,
		watched  INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
	)`)
	mustExec(t, db, `CREATE TABLE prs (id TEXT PRIMARY KEY)`)
	mustExec(t, db, `INSERT INTO prs (id) VALUES ('p-old')`)
	mustExec(t, db, `CREATE TABLE pr_diffs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		pr_id TEXT NOT NULL,
		file_path TEXT NOT NULL,
		patch TEXT NOT NULL,
		additions INTEGER NOT NULL DEFAULT 0,
		deletions INTEGER NOT NULL DEFAULT 0,
		UNIQUE(pr_id, file_path),
		FOREIGN KEY(pr_id) REFERENCES prs(id) ON DELETE CASCADE
	)`)
	mustExec(t, db, `CREATE TABLE comments (
		id TEXT PRIMARY KEY,
		pr_id TEXT NOT NULL,
		author TEXT NOT NULL,
		body TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		line_number INTEGER,
		file_path TEXT,
		kind TEXT NOT NULL DEFAULT 'issue',
		commit_sha TEXT,
		original_sha TEXT,
		side TEXT,
		position INTEGER,
		original_position INTEGER,
		in_reply_to TEXT,
		thread_id TEXT,
		patchset_number INTEGER,
		FOREIGN KEY(pr_id) REFERENCES prs(id) ON DELETE CASCADE
	)`)
	mustExec(t, db, `CREATE TABLE patchsets (
		pr_id TEXT NOT NULL,
		number INTEGER NOT NULL,
		head_sha TEXT NOT NULL,
		base_sha TEXT NOT NULL,
		pushed_at TIMESTAMP NOT NULL,
		PRIMARY KEY (pr_id, number),
		UNIQUE (pr_id, head_sha),
		FOREIGN KEY(pr_id) REFERENCES prs(id) ON DELETE CASCADE
	)`)
	mustExec(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('p-old', 1, 'head-same', 'base-1', CURRENT_TIMESTAMP)`)

	if err := NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("run migration 016 upgrade: %v", err)
	}

	mustExec(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('p-old', 2, 'head-same', 'base-2', CURRENT_TIMESTAMP)`)
	if _, err := db.Exec(`INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('p-old', 3, 'head-same', 'base-2', CURRENT_TIMESTAMP)`); err == nil {
		t.Error("expected upgraded UNIQUE violation on (pr_id, base_sha, head_sha)")
	}
}

func TestMigration020_AddsDiffHunkColumn(t *testing.T) {
	db := newTestDB(t)

	cols := tableColumns(t, db, "comments")
	if !cols["diff_hunk"] {
		t.Error("comments missing column \"diff_hunk\"")
	}

	// Verify NULL default for existing rows (diff_hunk not set).
	mustInsertPR(t, db, "p020")
	mustExec(t, db, `INSERT INTO comments (id, pr_id, author, body, created_at, updated_at, kind)
		VALUES ('c020','p020','alice','hello','2026-01-01','2026-01-01','issue')`)
	var hunk *string
	if err := db.QueryRow(`SELECT diff_hunk FROM comments WHERE id='c020'`).Scan(&hunk); err != nil {
		t.Fatalf("query diff_hunk: %v", err)
	}
	if hunk != nil {
		t.Errorf("diff_hunk default: got %q want NULL", *hunk)
	}

	// Verify it can be set for review comments.
	mustExec(t, db, `UPDATE comments SET diff_hunk = '@@ -1,3 +1,4 @@' WHERE id='c020'`)
	if err := db.QueryRow(`SELECT diff_hunk FROM comments WHERE id='c020'`).Scan(&hunk); err != nil {
		t.Fatalf("query diff_hunk after update: %v", err)
	}
	if hunk == nil || *hunk != "@@ -1,3 +1,4 @@" {
		t.Errorf("diff_hunk after update: got %v want '@@ -1,3 +1,4 @@'", hunk)
	}
}

func TestMigration023_AddsReviewCommentPositionFields(t *testing.T) {
	db := newTestDB(t)

	cols := tableColumns(t, db, "comments")
	for _, want := range []string{"original_line", "start_line", "original_start_line", "start_side", "subject_type"} {
		if !cols[want] {
			t.Errorf("comments missing review-comment position column %q", want)
		}
	}

	// Existing issue comments remain readable and get NULL review positioning fields.
	mustInsertPR(t, db, "p023")
	mustExec(t, db, `INSERT INTO comments (id, pr_id, author, body, created_at, updated_at)
		VALUES ('c023','p023','alice','hello','2026-01-01','2026-01-01')`)
	var originalLine *int
	var subjectType *string
	if err := db.QueryRow(`SELECT original_line, subject_type FROM comments WHERE id='c023'`).Scan(&originalLine, &subjectType); err != nil {
		t.Fatalf("query migration 023 columns: %v", err)
	}
	if originalLine != nil || subjectType != nil {
		t.Fatalf("new review positioning columns default = (%v, %v), want NULLs", originalLine, subjectType)
	}
}

func TestMigration033AddsPRMentioned(t *testing.T) {
	db := newTestDB(t)

	cols := tableColumns(t, db, "prs")
	if !cols["mentioned"] {
		t.Error("prs missing column mentioned")
	}
}
