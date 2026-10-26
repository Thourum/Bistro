package prdiff

import (
	"strings"
	"testing"

	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestDiffStore_SaveAndGet(t *testing.T) {
	db := storetest.Open(t)

	// Seed a PR row to satisfy the foreign-key constraint.
	seedPR(t, db, "pr-123")

	store := New(db.DB)

	diffs := []File{
		{FilePath: "main.go", Patch: "+added line\n-removed line", Additions: 1, Deletions: 1},
		{FilePath: "utils.go", Patch: "+new file", Additions: 1, Deletions: 0},
	}

	if err := store.Save("pr-123", diffs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Get("pr-123")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 diffs, got %d", len(got))
	}

	found := map[string]File{}
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

func TestDiffStore_SaveAndGet_MetadataRoundTrip(t *testing.T) {
	db := storetest.Open(t)
	seedPR(t, db, "pr-meta")
	ds := New(db.DB)

	want := File{
		FilePath:    "new-name.go",
		Status:      "renamed",
		OldPath:     "old-name.go",
		IsBinary:    true,
		IsTruncated: true,
		PatchBytes:  1234,
		HunkCount:   2,
		Patch:       "patch",
		Additions:   3,
		Deletions:   4,
		DiffStatus:  "ok",
	}
	if err := ds.Save("pr-meta", []File{want}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := ds.Get("pr-meta")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d diffs, want 1", len(got))
	}
	if got[0] != want {
		t.Fatalf("metadata roundtrip mismatch\ngot:  %+v\nwant: %+v", got[0], want)
	}
}

func TestDiffStore_GetDiffs_Empty(t *testing.T) {
	db := storetest.Open(t)
	store := New(db.DB)

	got, err := store.Get("pr-nonexistent")
	if err != nil {
		t.Fatalf("Get on empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 diffs, got %d", len(got))
	}
}

func TestDiffStore_Upsert(t *testing.T) {
	db := storetest.Open(t)
	seedPR(t, db, "pr-789")
	store := New(db.DB)

	v1 := []File{{FilePath: "a.go", Patch: "v1", Additions: 1, Deletions: 0}}
	if err := store.Save("pr-789", v1); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	v2 := []File{{FilePath: "a.go", Patch: "v2", Additions: 2, Deletions: 1}}
	if err := store.Save("pr-789", v2); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	got, err := store.Get("pr-789")
	if err != nil {
		t.Fatalf("Get: %v", err)
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
	db := storetest.Open(t)
	seedPR(t, db, "pr-456")
	store := New(db.DB)

	diffs := []File{
		{FilePath: "main.go", Patch: "--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,3 @@\n-old\n+new"},
		{FilePath: "util.go", Patch: "--- a/util.go\n+++ b/util.go\n@@ -1 +1 @@\n-x\n+y"},
	}
	if err := store.Save("pr-456", diffs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	unified, err := store.GetUnified("pr-456")
	if err != nil {
		t.Fatalf("GetUnified: %v", err)
	}

	for _, want := range []string{"diff --git a/main.go b/main.go", "diff --git a/util.go b/util.go"} {
		if !containsString(unified, want) {
			t.Errorf("expected unified diff to contain %q\ngot:\n%s", want, unified)
		}
	}
}

func TestDiffStore_GetUnifiedDiff_PatchWithOwnHeaderNotDuplicated(t *testing.T) {
	db := storetest.Open(t)
	seedPR(t, db, "pr-header")
	store := New(db.DB)

	if err := store.Save("pr-header", []File{{
		FilePath: "main.go",
		Patch:    "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n",
	}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	unified, err := store.GetUnified("pr-header")
	if err != nil {
		t.Fatalf("GetUnified: %v", err)
	}

	if got := strings.Count(unified, "diff --git "); got != 1 {
		t.Errorf("expected exactly one diff --git header, got %d\ngot:\n%s", got, unified)
	}
}

func TestDiffStore_GetUnifiedDiff_NotFound(t *testing.T) {
	db := storetest.Open(t)
	store := New(db.DB)

	_, err := store.GetUnified("pr-missing")
	if err == nil {
		t.Error("expected error for missing pr, got nil")
	}
}

func TestDiffStore_SaveDiffs_Empty(t *testing.T) {
	db := storetest.Open(t)
	store := New(db.DB)

	if err := store.Save("pr-any", nil); err != nil {
		t.Fatalf("Save with nil: %v", err)
	}
	if err := store.Save("pr-any", []File{}); err != nil {
		t.Fatalf("Save with empty slice: %v", err)
	}
}

// ---- Stale-row removal tests (Blocker 2 fix) ----

func TestDiffStore_SaveDiffs_RemovesDeletedFile(t *testing.T) {
	// First diff has two files; second diff has only one.
	// The removed file must not appear in Get after the second Save.
	db := storetest.Open(t)
	seedPR(t, db, "pr-stale")
	ds := New(db.DB)

	// Save initial: main.go + utils.go
	if err := ds.Save("pr-stale", []File{
		{FilePath: "main.go", Patch: "patch-main"},
		{FilePath: "utils.go", Patch: "patch-utils"},
	}); err != nil {
		t.Fatalf("first save: %v", err)
	}

	// Save updated: only main.go (utils.go deleted from PR)
	if err := ds.Save("pr-stale", []File{
		{FilePath: "main.go", Patch: "patch-main-v2"},
	}); err != nil {
		t.Fatalf("second save: %v", err)
	}

	got, err := ds.Get("pr-stale")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 file after deletion, got %d: %v", len(got), got)
	}
	if got[0].FilePath != "main.go" {
		t.Errorf("expected main.go, got %q", got[0].FilePath)
	}
	if got[0].Patch != "patch-main-v2" {
		t.Errorf("expected updated patch, got %q", got[0].Patch)
	}
}

func TestDiffStore_SaveDiffs_EmptyClearsStaleRows(t *testing.T) {
	// Saving an empty diff list must delete all existing rows.
	db := storetest.Open(t)
	seedPR(t, db, "pr-empty")
	ds := New(db.DB)

	if err := ds.Save("pr-empty", []File{
		{FilePath: "a.go", Patch: "patch-a"},
		{FilePath: "b.go", Patch: "patch-b"},
	}); err != nil {
		t.Fatalf("initial save: %v", err)
	}

	// Simulate a sync that found no changed files (e.g. revert to base).
	if err := ds.Save("pr-empty", nil); err != nil {
		t.Fatalf("empty save: %v", err)
	}

	got, err := ds.Get("pr-empty")
	if err != nil {
		t.Fatalf("Get after empty save: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 diffs after empty save, got %d: %v", len(got), got)
	}
}

func TestDiffStore_SaveDiffs_RenameFile(t *testing.T) {
	// Rename: old file gone, new file appears. Get must return only the new name.
	db := storetest.Open(t)
	seedPR(t, db, "pr-rename")
	ds := New(db.DB)

	if err := ds.Save("pr-rename", []File{
		{FilePath: "old_name.go", Patch: "patch-old"},
	}); err != nil {
		t.Fatalf("save old: %v", err)
	}
	if err := ds.Save("pr-rename", []File{
		{FilePath: "new_name.go", Patch: "patch-new"},
	}); err != nil {
		t.Fatalf("save new: %v", err)
	}

	got, _ := ds.Get("pr-rename")
	if len(got) != 1 || got[0].FilePath != "new_name.go" {
		t.Errorf("after rename: got %v", got)
	}
}

func TestDiffStore_DiffStatus_Roundtrip(t *testing.T) {
	db := storetest.Open(t)
	seedPR(t, db, "pr-status")
	store := New(db.DB)

	diffs := []File{
		{FilePath: "ok.go", Patch: "+ok", Additions: 1, Deletions: 0, DiffStatus: "ok"},
		{FilePath: "big.go", Patch: "", Additions: 0, Deletions: 0, DiffStatus: "truncated"},
		{FilePath: "overflow.go", Patch: "", Additions: 0, Deletions: 0, DiffStatus: "too_large"},
	}
	if err := store.Save("pr-status", diffs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Get("pr-status")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 diffs, got %d", len(got))
	}
	byPath := map[string]File{}
	for _, d := range got {
		byPath[d.FilePath] = d
	}
	cases := []struct{ path, want string }{
		{"ok.go", "ok"},
		{"big.go", "truncated"},
		{"overflow.go", "too_large"},
	}
	for _, c := range cases {
		d, ok := byPath[c.path]
		if !ok {
			t.Errorf("missing %s", c.path)
			continue
		}
		if d.DiffStatus != c.want {
			t.Errorf("%s: expected DiffStatus %q, got %q", c.path, c.want, d.DiffStatus)
		}
	}
}

// seedPR inserts a minimal PR row so foreign-key constraints are satisfied.
func seedPR(t *testing.T, db *store.DB, prID string) {
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

func TestDiffPatchIDRoundTrip(t *testing.T) {
	db := storetest.Open(t)
	seedPR(t, db, "pr-patch-id")
	diffs := New(db.DB)
	if err := diffs.Save("pr-patch-id", []File{{FilePath: "a.go", Patch: "+added", PatchID: "fingerprint"}, {FilePath: "legacy.go"}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	files, err := diffs.Get("pr-patch-id")
	if err != nil || len(files) != 2 {
		t.Fatalf("get: files=%+v err=%v", files, err)
	}
	if files[0].PatchID != "fingerprint" || files[1].PatchID != "" {
		t.Fatalf("patch IDs: got %+v", files)
	}
}
