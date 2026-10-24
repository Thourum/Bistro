package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

func newViewedBaselineTestStores(t *testing.T) (*store.DB, *store.DiffStore) {
	t.Helper()
	db := newReadStateTestDB(t)
	ctx := context.Background()
	const prID = "github:o/r:1"
	for _, statement := range []string{
		`INSERT INTO prs (id, provider, owner, repo, number, title, author, state, created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		 VALUES ('github:o/r:1', 'github', 'o', 'r', 1, 'T', 'a', 'open', datetime('now'), datetime('now'), 'main', 'f', 'tip', 'h1')`,
		`INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at, has_fingerprints)
		 VALUES ('github:o/r:1', 1, 'h1', 'tip', datetime('now'), 1)`,
		`INSERT INTO patchset_files (pr_id, patchset_number, path, patch_id)
		 VALUES ('github:o/r:1', 1, 'a.go', 'id-a'), ('github:o/r:1', 1, 'b.go', 'id-b')`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	diffs := store.NewDiffStore(db.DB)
	if err := diffs.SaveDiffs(prID, []store.DiffFile{
		{FilePath: "a.go", Patch: "pa", PatchID: "id-a"},
		{FilePath: "b.go", Patch: "pb", PatchID: "id-b"},
	}); err != nil {
		t.Fatalf("SaveDiffs: %v", err)
	}
	return db, diffs
}

func markViewedBaselineTestFile(t *testing.T, handler ws.MessageHandler, file store.FileViewed) {
	t.Helper()
	data, err := json.Marshal(map[string]string{"pr_id": "github:o/r:1", "file_path": file.FilePath, "content_hash": file.ContentHash})
	if err != nil {
		t.Fatalf("marshal mark: %v", err)
	}
	response, err := handler(context.Background(), ws.InboundMessage{ID: "m-" + file.FilePath, Type: "MARK_FILE_VIEWED", Data: data})
	if err != nil {
		t.Fatalf("mark %s: %v", file.FilePath, err)
	}
	if ack := extractData(t, response); ack["ok"] != true {
		t.Fatalf("mark %s: ack = %v", file.FilePath, ack)
	}
}

func TestMarkingLastFileViewedRecordsAllViewedBaseline(t *testing.T) {
	db, diffs := newViewedBaselineTestStores(t)
	ctx := context.Background()
	const prID = "github:o/r:1"
	handler := NewMarkFileViewedHandlerWithDeps(MarkFileViewedDeps{
		Viewed: db, CurrentUser: &mockCurrentUserReader{login: "alice", found: true},
		DiffStore: diffs, Baseline: db,
	})
	markViewedBaselineTestFile(t, handler, store.FileViewed{FilePath: "a.go", ContentHash: "id-a"})
	if _, found, err := db.GetLastReviewed(ctx, prID, "alice"); err != nil || found {
		t.Fatalf("baseline while b.go is unviewed: found=%v, error=%v", found, err)
	}
	markViewedBaselineTestFile(t, handler, store.FileViewed{FilePath: "b.go", ContentHash: "id-b"})
	record, found, err := db.GetLastReviewed(ctx, prID, "alice")
	if err != nil || !found {
		t.Fatalf("GetLastReviewed: found=%v, error=%v", found, err)
	}
	if record.Source != store.ReviewSourceAllViewed || record.SHA != "h1" || record.PatchsetNum != 1 || record.ReviewedAt.IsZero() {
		t.Fatalf("baseline = %+v, want timestamped all_viewed h1/P1", record)
	}
}

func TestAllViewedBaselineSkipsStaleCachedDiffs(t *testing.T) {
	for _, test := range []struct {
		name            string
		hasFingerprints bool
		patchsetFiles   map[string]string
	}{
		{name: "fingerprints pending"},
		{name: "changed patch", hasFingerprints: true, patchsetFiles: map[string]string{"a.go": "id-a2", "b.go": "id-b"}},
		{name: "added file", hasFingerprints: true, patchsetFiles: map[string]string{"a.go": "id-a", "b.go": "id-b", "c.go": "id-c"}},
		{name: "removed file", hasFingerprints: true, patchsetFiles: map[string]string{"a.go": "id-a"}},
		{name: "renamed file", hasFingerprints: true, patchsetFiles: map[string]string{"renamed.go": "id-a", "b.go": "id-b"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, diffs := newViewedBaselineTestStores(t)
			ctx := context.Background()
			const prID = "github:o/r:1"
			if _, err := db.ExecContext(ctx, `UPDATE prs SET head_sha = 'h2' WHERE id = ?`, prID); err != nil {
				t.Fatalf("advance head: %v", err)
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at, has_fingerprints)
				 VALUES (?, 2, 'h2', 'tip', datetime('now'), ?)`, prID, test.hasFingerprints); err != nil {
				t.Fatalf("seed P2: %v", err)
			}
			for path, hash := range test.patchsetFiles {
				if _, err := db.ExecContext(ctx, `INSERT INTO patchset_files (pr_id, patchset_number, path, patch_id) VALUES (?, 2, ?, ?)`, prID, path, hash); err != nil {
					t.Fatalf("seed P2 fingerprint: %v", err)
				}
			}
			handler := NewMarkFileViewedHandlerWithDeps(MarkFileViewedDeps{
				Viewed: db, CurrentUser: &mockCurrentUserReader{login: "alice", found: true}, DiffStore: diffs, Baseline: db,
			})
			for _, file := range []store.FileViewed{{FilePath: "a.go", ContentHash: "id-a"}, {FilePath: "b.go", ContentHash: "id-b"}} {
				markViewedBaselineTestFile(t, handler, file)
			}
			if record, found, err := db.GetLastReviewed(ctx, prID, "alice"); err != nil || found {
				t.Fatalf("baseline from stale P1 diffs = %+v, found=%v, error=%v", record, found, err)
			}
		})
	}
}

func TestAllFilesViewedRequiresMatchingCurrentHashes(t *testing.T) {
	const patch = "@@ -1 +1 @@\n-old\n+new\n"
	for _, test := range []struct {
		name   string
		diffs  []store.DiffFile
		viewed []store.FileViewed
		want   bool
	}{
		{name: "empty diff", want: false},
		{
			name:  "missing mark",
			diffs: []store.DiffFile{{FilePath: "a.go", PatchID: "id-a"}},
			want:  false,
		},
		{
			name:   "stale mark",
			diffs:  []store.DiffFile{{FilePath: "a.go", PatchID: "id-a"}},
			viewed: []store.FileViewed{{FilePath: "a.go", ContentHash: "stale"}},
			want:   false,
		},
		{
			name:   "all current files viewed",
			diffs:  []store.DiffFile{{FilePath: "a.go", PatchID: "id-a"}, {FilePath: "b.go", PatchID: "id-b"}},
			viewed: []store.FileViewed{{FilePath: "a.go", ContentHash: "id-a"}, {FilePath: "b.go", ContentHash: "id-b"}, {FilePath: "removed.go", ContentHash: "old"}},
			want:   true,
		},
		{
			name:   "legacy cached patch",
			diffs:  []store.DiffFile{{FilePath: "a.go", Patch: patch}},
			viewed: []store.FileViewed{{FilePath: "a.go", ContentHash: viewedHash(store.DiffFile{Patch: patch})}},
			want:   true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := allFilesViewed(test.diffs, test.viewed); got != test.want {
				t.Errorf("allFilesViewed = %v, want %v", got, test.want)
			}
		})
	}
}
