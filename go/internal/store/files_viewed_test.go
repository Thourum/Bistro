package store

import (
	"context"
	"testing"
)

func TestMarkIsFileViewed_ContentHashSemantics(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-fv-1")

	const (
		prID  = "pr-fv-1"
		path  = "go/internal/foo.go"
		login = "alice"
		hash1 = "abc123"
		hash2 = "def456"
	)

	// Not viewed initially
	ok, err := db.IsFileViewed(ctx, prID, path, login, hash1)
	if err != nil {
		t.Fatalf("is viewed: %v", err)
	}
	if ok {
		t.Fatal("should not be viewed yet")
	}

	// Mark viewed with hash1
	if err := db.MarkFileViewed(ctx, prID, path, login, hash1); err != nil {
		t.Fatalf("mark: %v", err)
	}

	// Same hash → viewed
	ok, _ = db.IsFileViewed(ctx, prID, path, login, hash1)
	if !ok {
		t.Fatal("same hash should be viewed")
	}

	// Different hash (content changed) → not viewed
	ok, _ = db.IsFileViewed(ctx, prID, path, login, hash2)
	if ok {
		t.Fatal("different hash should not be viewed (content changed)")
	}

	// Re-mark with hash2 → now hash2 is viewed, hash1 is not
	if err := db.MarkFileViewed(ctx, prID, path, login, hash2); err != nil {
		t.Fatalf("re-mark hash2: %v", err)
	}
	ok, _ = db.IsFileViewed(ctx, prID, path, login, hash2)
	if !ok {
		t.Fatal("hash2 should now be viewed after re-mark")
	}
	ok, _ = db.IsFileViewed(ctx, prID, path, login, hash1)
	if ok {
		t.Fatal("hash1 should no longer be viewed after upsert to hash2")
	}
}

func TestListViewedFiles_LoginIsolation(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-lv-1")

	_ = db.MarkFileViewed(ctx, "pr-lv-1", "a.go", "alice", "h1")
	_ = db.MarkFileViewed(ctx, "pr-lv-1", "b.go", "alice", "h2")
	_ = db.MarkFileViewed(ctx, "pr-lv-1", "c.go", "bob", "h3")

	files, err := db.ListViewedFiles(ctx, "pr-lv-1", "alice")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files for alice, got %d", len(files))
	}
	// Ordered by file_path
	if files[0].FilePath != "a.go" || files[1].FilePath != "b.go" {
		t.Errorf("order: got %v", files)
	}

	bobFiles, _ := db.ListViewedFiles(ctx, "pr-lv-1", "bob")
	if len(bobFiles) != 1 || bobFiles[0].FilePath != "c.go" {
		t.Errorf("bob: got %v", bobFiles)
	}
}

func TestListViewedFiles_Empty(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-lv-2")
	files, err := db.ListViewedFiles(ctx, "pr-lv-2", "nobody")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected empty, got %v", files)
	}
}

func TestMarkFileViewed_PRIsolation(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-lv-3")
	mustInsertPR(t, db, "pr-lv-4")

	_ = db.MarkFileViewed(ctx, "pr-lv-3", "x.go", "alice", "hx")
	files, _ := db.ListViewedFiles(ctx, "pr-lv-4", "alice")
	if len(files) != 0 {
		t.Fatal("pr-lv-4 should have no viewed files")
	}
}
