// Package storetest provides migrated in-memory databases for tests outside store.
package storetest

import (
	"context"
	"testing"

	"github.com/itterno/bistro/internal/store"
)

// Open opens an in-memory SQLite database with the schema applied and closes it
// when the test completes.
func Open(t testing.TB) *store.DB {
	t.Helper()
	db, err := store.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Cleanup only releases the ephemeral database; there is no data to preserve.
	t.Cleanup(func() { _ = db.Close() })
	if err := store.NewMigrationRunner(nil, db.DB).Run(context.Background()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}
