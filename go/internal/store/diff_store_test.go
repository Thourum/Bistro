package store

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

// setupTestDB opens an in-memory SQLite DB and runs all migrations.
func setupTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	if err := NewMigrationRunner(log, db.DB).Run(context.Background()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}

func TestDiffStore_SaveAndGet(t *testing.T) {
	db := setupTestDB(t)

	// Seed a PR row to satisfy the foreign-key constraint.
	seedPR(t, db, "pr-123")

	store := NewDiffStore(db.DB)

	diffs := []DiffFile{
		{FilePath: "main.go", Patch: "+added line\n-removed line", Additions: 1, Deletions: 1},
		{FilePath: "utils.go", Patch: "+new file", Additions: 1, Deletions: 0},
	}

	if err := store.SaveDiffs("pr-123", diffs); err != nil {
		t.Fatalf("SaveDiffs: %v", err)
	}

	got, err := store.GetDiffs("pr-123")
	if err != nil {
		t.Fatalf("GetDiffs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 diffs, got %d", len(got))
	}

	found := map[string]DiffFile{}
	for _, d := range got {
		found[d.FilePath] = d
	}

	if d, ok := found["main.go"]; !ok {
		t.Error("expected main.go in results")
	} else {
		if d.Patch != "+added line\n-removed line" {
			t.Errorf("unexpected patch: %q", d.Patch)
		}
		if d.Additions != 1 || d.Deletions != 1 {
			t.Errorf("unexpected additions/deletions: %d/%d", d.Additions, d.Deletions)
		}
	}

	if _, ok := found["utils.go"]; !ok {
		t.Error("expected utils.go in results")
	}
}

func TestDiffStore_GetDiffs_Empty(t *testing.T) {
	db := setupTestDB(t)
	store := NewDiffStore(db.DB)

	got, err := store.GetDiffs("pr-nonexistent")
	if err != nil {
		t.Fatalf("GetDiffs on empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 diffs, got %d", len(got))
	}
}

func TestDiffStore_Upsert(t *testing.T) {
	db := setupTestDB(t)
	seedPR(t, db, "pr-789")
	store := NewDiffStore(db.DB)

	v1 := []DiffFile{{FilePath: "a.go", Patch: "v1", Additions: 1, Deletions: 0}}
	if err := store.SaveDiffs("pr-789", v1); err != nil {
		t.Fatalf("first SaveDiffs: %v", err)
	}

	v2 := []DiffFile{{FilePath: "a.go", Patch: "v2", Additions: 2, Deletions: 1}}
	if err := store.SaveDiffs("pr-789", v2); err != nil {
		t.Fatalf("second SaveDiffs: %v", err)
	}

	got, err := store.GetDiffs("pr-789")
	if err != nil {
		t.Fatalf("GetDiffs: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 diff after upsert, got %d", len(got))
	}
	if got[0].Patch != "v2" {
		t.Errorf("expected patch v2 after upsert, got %q", got[0].Patch)
	}
	if got[0].Additions != 2 || got[0].Deletions != 1 {
		t.Errorf("unexpected additions/deletions after upsert: %d/%d", got[0].Additions, got[0].Deletions)
	}
}

func TestDiffStore_GetUnifiedDiff(t *testing.T) {
	db := setupTestDB(t)
	seedPR(t, db, "pr-456")
	store := NewDiffStore(db.DB)

	diffs := []DiffFile{
		{FilePath: "main.go", Patch: "--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,3 @@\n-old\n+new"},
		{FilePath: "util.go", Patch: "--- a/util.go\n+++ b/util.go\n@@ -1 +1 @@\n-x\n+y"},
	}
	if err := store.SaveDiffs("pr-456", diffs); err != nil {
		t.Fatalf("SaveDiffs: %v", err)
	}

	unified, err := store.GetUnifiedDiff("pr-456")
	if err != nil {
		t.Fatalf("GetUnifiedDiff: %v", err)
	}

	for _, want := range []string{"diff --git a/main.go b/main.go", "diff --git a/util.go b/util.go"} {
		if !containsString(unified, want) {
			t.Errorf("expected unified diff to contain %q\ngot:\n%s", want, unified)
		}
	}
}

func TestDiffStore_GetUnifiedDiff_NotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewDiffStore(db.DB)

	_, err := store.GetUnifiedDiff("pr-missing")
	if err == nil {
		t.Error("expected error for missing pr, got nil")
	}
}

func TestDiffStore_SaveDiffs_Empty(t *testing.T) {
	db := setupTestDB(t)
	store := NewDiffStore(db.DB)

	if err := store.SaveDiffs("pr-any", nil); err != nil {
		t.Fatalf("SaveDiffs with nil: %v", err)
	}
	if err := store.SaveDiffs("pr-any", []DiffFile{}); err != nil {
		t.Fatalf("SaveDiffs with empty slice: %v", err)
	}
}

// seedPR inserts a minimal PR row so foreign-key constraints are satisfied.
func seedPR(t *testing.T, db *DB, prID string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT OR IGNORE INTO prs
			(id, provider, owner, repo, number, title, state, author,
			 base_branch, head_branch, base_sha, head_sha,
			 created_at, updated_at)
		VALUES (?, 'github', 'owner', 'repo', 1, 'Test PR', 'open', 'user',
			'main', 'feature', 'abc123', 'def456',
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, prID)
	if err != nil {
		t.Fatalf("seedPR %s: %v", prID, err)
	}
}

// containsString reports whether s contains substr.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
