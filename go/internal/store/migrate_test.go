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

	assertAllEmbeddedVersionsApplied(t, ctx, db.DB)

	if exists, err := tableExists(ctx, db.DB, "pr_read_state"); err != nil {
		t.Fatalf("check pr_read_state table: %v", err)
	} else if !exists {
		t.Fatal("pr_read_state table does not exist")
	}
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
