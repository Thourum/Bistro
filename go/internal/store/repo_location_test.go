package store

import (
	"context"
	"testing"
)

func TestMigration026AddsLocationColumns(t *testing.T) {
	db := newTestDBMigratedThrough(t, 25)

	// Existing row survives migration 026 and new columns are readable with NULL defaults.
	mustExec(t, db, `INSERT INTO repos (id, provider, owner, repo) VALUES ('github:acme/api','github','acme','api')`)

	if err := NewMigrationRunner(nil, db.DB).Run(context.Background()); err != nil {
		t.Fatalf("run remaining migrations: %v", err)
	}

	var localPath, source, lastUsed, status, code, msg any
	err := db.QueryRow(`SELECT local_path, source, last_used_at, location_status, location_error_code, location_error FROM repos WHERE id='github:acme/api'`).
		Scan(&localPath, &source, &lastUsed, &status, &code, &msg)
	if err != nil {
		t.Fatalf("select new columns: %v", err)
	}
	if localPath != nil || source != nil || lastUsed != nil || status != nil || code != nil || msg != nil {
		t.Fatalf(
			"expected NULL defaults, got local_path=%v source=%v last_used_at=%v location_status=%v location_error_code=%v location_error=%v",
			localPath,
			source,
			lastUsed,
			status,
			code,
			msg,
		)
	}
}

func newTestDBMigratedThrough(t *testing.T, throughVersion int) *DB {
	t.Helper()

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	runner := NewMigrationRunner(nil, db.DB)
	if err := runner.createMigrationTable(ctx); err != nil {
		t.Fatalf("create migration table: %v", err)
	}

	migrations, err := runner.loadMigrationFiles()
	if err != nil {
		t.Fatalf("load migration files: %v", err)
	}
	for _, migration := range migrations {
		if migration.Version > throughVersion {
			continue
		}
		if err := runner.applyMigration(ctx, migration); err != nil {
			t.Fatalf("apply migration %s: %v", migration.Name, err)
		}
	}

	return db
}
