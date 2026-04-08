package store

import (
	"testing"
)

func TestOpenDB(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		t.Fatalf("failed to ping db: %v", err)
	}
}

func TestMigrations(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	err = Migrate(db)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	tables := []string{"prs", "comments", "files"}
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

	if err := Migrate(db); err != nil {
		t.Fatalf("first migration failed: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second migration failed (not idempotent): %v", err)
	}
}
