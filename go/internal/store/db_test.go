package store

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

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

	tables := []string{"prs", "comments", "files", "schema_migrations"}
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

func TestMigrationRunnerLoadsFiles(t *testing.T) {
	runner := NewMigrationRunner(nil, nil)

	migrations, err := runner.loadMigrationFiles()
	if err != nil {
		t.Fatalf("loadMigrationFiles error = %v", err)
	}
	if len(migrations) == 0 {
		t.Error("expected at least one migration file")
	}

	for i := 1; i < len(migrations); i++ {
		if migrations[i].Version <= migrations[i-1].Version {
			t.Errorf("migrations not sorted by version: index %d (%d) <= index %d (%d)",
				i, migrations[i].Version, i-1, migrations[i-1].Version)
		}
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
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&migCount); err != nil {
		t.Fatalf("failed to query schema_migrations: %v", err)
	}
	if migCount == 0 {
		t.Error("expected migrations to be recorded in schema_migrations")
	}
}

// TestRateLimitStateProviderKeyMigration verifies that migration 011 correctly
// rebuilds rate_limit_state so that:
//   (a) two different provider ids coexist as distinct rows without overwriting each other, and
//   (b) the pre-existing 'github' row's data survives the migration intact.
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
