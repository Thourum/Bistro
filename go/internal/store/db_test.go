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
