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
