package sync

import (
	"context"
	"testing"
)

func TestRollbackPRSavepointReportsMissingSavepoint(t *testing.T) {
	db := newSyncTestDB(t)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck // Cleanup after the explicit rollback below.

	if _, err := tx.ExecContext(ctx, `INSERT INTO repos (id, provider, owner, repo) VALUES (?, ?, ?, ?)`,
		"github:o/r", "github", "o", "r"); err != nil {
		t.Fatalf("write partial transaction: %v", err)
	}
	if err := rollbackPRSavepoint(ctx, tx); err == nil {
		t.Fatal("missing savepoint must return an error, not permit a partial commit")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("roll back outer transaction: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM repos WHERE id = ?`, "github:o/r").Scan(&count); err != nil {
		t.Fatalf("count rolled-back rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("partial write was committed after savepoint failure: %d rows", count)
	}
}
