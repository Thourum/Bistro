package sync

import (
	"errors"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/storetest"
)

// mustWritePR writes a single PR via WriteSyncResults, failing the test on error.
func mustWritePR(t *testing.T, w *Writer, pr *provider.PR) {
	t.Helper()
	if err := w.WriteSyncResults([]*PRFetchResult{{PR: pr}}); err != nil {
		t.Fatalf("mustWritePR %s: %v", pr.ID, err)
	}
}

func TestWriterCreation(t *testing.T) {
	// Just verify the method exists
	if true {
		t.Skip("requires database")
	}
}

func TestPRFetchResultValidation(t *testing.T) {
	result := &PRFetchResult{
		PR: &provider.PR{
			ID:     "github:org/repo:123",
			Number: 123,
			Title:  "Test PR",
		},
		Comments: []*provider.Comment{
			{ID: "comment-1", Author: "alice", Body: "Good work"},
		},
	}

	if result.PR == nil {
		t.Error("PR should not be nil")
	}
	if len(result.Comments) != 1 {
		t.Error("expected 1 comment")
	}
}

func TestSyncResultAggregation(t *testing.T) {
	results := []*PRFetchResult{
		{
			PR:       &provider.PR{ID: "1", Number: 1},
			Comments: []*provider.Comment{{ID: "c1"}},
		},
		{
			PR:       &provider.PR{ID: "2", Number: 2},
			Comments: []*provider.Comment{{ID: "c2"}, {ID: "c3"}},
		},
	}

	totalComments := 0
	for _, r := range results {
		totalComments += len(r.Comments)
	}

	if totalComments != 3 {
		t.Errorf("expected 3 comments total, got %d", totalComments)
	}
}

func TestWriterSkipsNilResults(t *testing.T) {
	writer := NewWriter(nil, nil)
	results := []*PRFetchResult{nil, nil}
	err := writer.WriteSyncResults(results)
	if err != nil {
		t.Errorf("expected no error for nil results, got %v", err)
	}
}

func TestWriterEmptyResults(t *testing.T) {
	writer := NewWriter(nil, nil)
	err := writer.WriteSyncResults([]*PRFetchResult{})
	if err != nil {
		t.Errorf("expected no error for empty results, got %v", err)
	}
}

// TestWriteSyncResultsActuallyWrites verifies that WriteSyncResults inserts
// PR and comment rows into the database (C1 regression test).
func TestWriteSyncResultsActuallyWrites(t *testing.T) {
	db := storetest.Open(t)
	writer := NewWriter(nil, db)

	now := time.Now().UTC()
	pr := &provider.PR{
		ID:         "github:org/repo:1",
		Provider:   "github",
		Owner:      "org",
		Repo:       "repo",
		Number:     1,
		Title:      "Fix the bug",
		Author:     "alice",
		State:      "open",
		BaseBranch: "main",
		HeadBranch: "fix-bug",
		BaseSHA:    "abc123",
		HeadSHA:    "def456",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	comment := &provider.Comment{
		ID:        "comment-1",
		PRID:      pr.ID,
		Author:    "bob",
		Body:      "LGTM",
		CreatedAt: now,
		UpdatedAt: now,
	}

	results := []*PRFetchResult{
		{PR: pr, Comments: []*provider.Comment{comment}},
	}

	if err := writer.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults error: %v", err)
	}

	// Verify PR was inserted.
	var prID, title string
	row := db.QueryRow("SELECT id, title FROM prs WHERE id = ?", pr.ID)
	if err := row.Scan(&prID, &title); err != nil {
		t.Fatalf("PR not found in database: %v", err)
	}
	if prID != pr.ID {
		t.Errorf("expected pr id %q, got %q", pr.ID, prID)
	}
	if title != pr.Title {
		t.Errorf("expected title %q, got %q", pr.Title, title)
	}

	// Verify comment was inserted.
	var commentID, body string
	row = db.QueryRow("SELECT id, body FROM comments WHERE id = ?", comment.ID)
	if err := row.Scan(&commentID, &body); err != nil {
		t.Fatalf("comment not found in database: %v", err)
	}
	if commentID != comment.ID {
		t.Errorf("expected comment id %q, got %q", comment.ID, commentID)
	}
	if body != comment.Body {
		t.Errorf("expected body %q, got %q", comment.Body, body)
	}
}

// TestWriteSyncResultsWritesDiffsAfterPR verifies diffs are persisted by the
// writer after the PR row exists, avoiding pr_diffs' foreign-key ordering bug.
func TestWriteSyncResultsWritesDiffsAfterPR(t *testing.T) {
	db := storetest.Open(t)
	writer := NewWriter(nil, db)

	now := time.Now().UTC()
	pr := &provider.PR{
		ID:         "github:org/repo:diffs",
		Provider:   "github",
		Owner:      "org",
		Repo:       "repo",
		Number:     9,
		Title:      "Store diffs",
		Author:     "alice",
		State:      "open",
		BaseBranch: "main",
		HeadBranch: "diffs",
		BaseSHA:    "base123",
		HeadSHA:    "head456",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	results := []*PRFetchResult{
		{
			PR: pr,
			DiffFiles: []store.DiffFile{
				{FilePath: "go/main.go", Patch: "@@ -1 +1 @@\n", Additions: 2, Deletions: 1},
				{FilePath: "go/other.go", Patch: "@@ -2 +2 @@\n", Additions: 1, Deletions: 3},
			},
			DiffComputed: true,
		},
	}

	if err := writer.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults error: %v", err)
	}

	var prCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM prs WHERE id = ?", pr.ID).Scan(&prCount); err != nil {
		t.Fatalf("PR count query: %v", err)
	}
	if prCount != 1 {
		t.Fatalf("expected PR row to be written, got %d", prCount)
	}

	var diffCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM pr_diffs WHERE pr_id = ?", pr.ID).Scan(&diffCount); err != nil {
		t.Fatalf("diff count query: %v", err)
	}
	if diffCount != 2 {
		t.Fatalf("expected 2 diff rows, got %d", diffCount)
	}
}

// TestWriteSyncResultsUpsert verifies that writing the same PR twice updates
// mutable fields rather than erroring on the unique constraint.
func TestWriteSyncResultsUpsert(t *testing.T) {
	db := storetest.Open(t)
	writer := NewWriter(nil, db)

	now := time.Now().UTC()
	pr := &provider.PR{
		ID:         "github:org/repo:2",
		Provider:   "github",
		Owner:      "org",
		Repo:       "repo",
		Number:     2,
		Title:      "Original title",
		Author:     "alice",
		State:      "open",
		BaseBranch: "main",
		HeadBranch: "feature",
		BaseSHA:    "aaa",
		HeadSHA:    "bbb",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	first := []*PRFetchResult{{PR: pr}}
	if err := writer.WriteSyncResults(first); err != nil {
		t.Fatalf("first write error: %v", err)
	}

	updated := *pr
	updated.Title = "Updated title"
	updated.State = "closed"
	second := []*PRFetchResult{{PR: &updated}}
	if err := writer.WriteSyncResults(second); err != nil {
		t.Fatalf("second write error: %v", err)
	}

	var title, state string
	row := db.QueryRow("SELECT title, state FROM prs WHERE id = ?", pr.ID)
	if err := row.Scan(&title, &state); err != nil {
		t.Fatalf("PR not found: %v", err)
	}
	if title != "Updated title" {
		t.Errorf("expected updated title, got %q", title)
	}
	if state != "closed" {
		t.Errorf("expected state closed, got %q", state)
	}
}

// TestWriteSyncResultsSkipsErrorResults verifies that results with a non-nil
// Error field are skipped without aborting the whole batch.
func TestWriteSyncResultsSkipsErrorResults(t *testing.T) {
	db := storetest.Open(t)
	writer := NewWriter(nil, db)

	now := time.Now().UTC()
	goodPR := &provider.PR{
		ID:         "github:org/repo:3",
		Provider:   "github",
		Owner:      "org",
		Repo:       "repo",
		Number:     3,
		Title:      "Good PR",
		Author:     "alice",
		State:      "open",
		BaseBranch: "main",
		HeadBranch: "good",
		BaseSHA:    "ccc",
		HeadSHA:    "ddd",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	results := []*PRFetchResult{
		{PR: nil, Error: errors.New("fetch failed")},
		{PR: goodPR},
	}

	if err := writer.WriteSyncResults(results); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM prs").Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 PR in db, got %d", count)
	}
}

// makePR returns a minimal provider.PR for use in tests.
func makePR(id string, number int, headSHA, baseSHA string) *provider.PR {
	return &provider.PR{
		ID: id, Provider: "github", Owner: "o", Repo: "r", Number: number,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: baseSHA, HeadSHA: headSHA,
	}
}

// countDiffFiles returns the number of pr_diffs rows for the given PR.
func countDiffFiles(t *testing.T, db *store.DB, prID string) int {
	t.Helper()
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM pr_diffs WHERE pr_id = ?`, prID).Scan(&n) //nolint:errcheck
	return n
}

func mustTime() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
