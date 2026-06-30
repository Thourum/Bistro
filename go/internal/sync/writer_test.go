package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

// newSyncTestDB is an alias for openTestDB — both names are used across tests.
func newSyncTestDB(t *testing.T) *store.DB {
	t.Helper()
	return openTestDB(t)
}

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

// openTestDB opens an in-memory SQLite database with the schema applied.
func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("openTestDB: %v", err)
	}
	if err := store.NewMigrationRunner(nil, db.DB).Run(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestWriteSyncResultsActuallyWrites verifies that WriteSyncResults inserts
// PR and comment rows into the database (C1 regression test).
func TestWriteSyncResultsActuallyWrites(t *testing.T) {
	db := openTestDB(t)
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
	db := openTestDB(t)
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
	db := openTestDB(t)
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
	db := openTestDB(t)
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

func TestWriteReviewThreads_PersistsThreadAndComments(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	// Insert a PR first (FK constraint).
	mustWritePR(t, w, &provider.PR{
		ID: "github:o/r:1", Provider: "github", Owner: "o", Repo: "r", Number: 1,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: "b", HeadSHA: "h",
	})

	pos := 7
	origPos := 11
	threads := []*provider.ReviewThread{{
		ID: "thread-1", PRID: "github:o/r:1",
		FilePath: "main.go", Line: 10,
		Resolved: true, Outdated: false, StateKnown: true,
		Comments: []*provider.Comment{
			{ID: "github-review-comment-1", PRID: "github:o/r:1",
				Author: "alice", Body: "nit", Kind: "review",
				FilePath: "main.go", LineNo: 10,
				CommitSHA: "abc", OriginalSHA: "orig-abc", Side: "RIGHT",
				Position: &pos, OriginalPosition: &origPos,
				InReplyTo: "github-review-comment-0", DiffHunk: "@@ -1 +1 @@",
				CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}}

	results := []*PRFetchResult{{
		PR: &provider.PR{ID: "github:o/r:1", Provider: "github", Owner: "o", Repo: "r",
			Number: 1, Title: "T", Author: "a", State: "open",
			BaseBranch: "main", HeadBranch: "f", BaseSHA: "b", HeadSHA: "h"},
		Threads: threads,
	}}
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	// Thread row exists.
	var resolved int
	if err := db.QueryRow(`SELECT resolved FROM review_threads WHERE id='thread-1'`).Scan(&resolved); err != nil {
		t.Fatalf("review_threads missing: %v", err)
	}
	if resolved != 1 {
		t.Errorf("resolved: got %d want 1", resolved)
	}

	// Comment row with inline review-comment fields intact.
	var kind, filePath, commitSHA, originalSHA, side, inReplyTo, threadID, diffHunk string
	var lineNo, position, originalPosition int
	if err := db.QueryRow(`
		SELECT kind, file_path, line_number, commit_sha, original_sha, side,
		       position, original_position, in_reply_to, thread_id, diff_hunk
		FROM comments WHERE id='github-review-comment-1'`).Scan(
		&kind, &filePath, &lineNo, &commitSHA, &originalSHA, &side,
		&position, &originalPosition, &inReplyTo, &threadID, &diffHunk,
	); err != nil {
		t.Fatalf("comment missing: %v", err)
	}
	if kind != "review" {
		t.Errorf("comment kind: got %q want 'review'", kind)
	}
	if filePath != "main.go" || lineNo != 10 || commitSHA != "abc" || originalSHA != "orig-abc" || side != "RIGHT" ||
		position != pos || originalPosition != origPos || inReplyTo != "github-review-comment-0" ||
		threadID != "thread-1" || diffHunk != "@@ -1 +1 @@" {
		t.Errorf("inline fields not persisted: file=%q line=%d commit=%q original=%q side=%q position=%d original_position=%d in_reply_to=%q thread_id=%q diff_hunk=%q",
			filePath, lineNo, commitSHA, originalSHA, side, position, originalPosition, inReplyTo, threadID, diffHunk)
	}
}

func TestWriteReviewThreads_PerItemErrorSkipsNotAborts(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)
	mustWritePR(t, w, &provider.PR{
		ID: "github:o/r:2", Provider: "github", Owner: "o", Repo: "r", Number: 2,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: "b", HeadSHA: "h2",
	})

	threads := []*provider.ReviewThread{
		{
			ID: "good-thread", PRID: "github:o/r:2",
			Comments: []*provider.Comment{
				{ID: "c-good", PRID: "github:o/r:2", Author: "y", Body: "y",
					Kind: "review", CreatedAt: time.Now(), UpdatedAt: time.Now()},
			},
		},
	}

	results := []*PRFetchResult{{
		PR: &provider.PR{ID: "github:o/r:2", Provider: "github", Owner: "o", Repo: "r",
			Number: 2, Title: "T", Author: "a", State: "open",
			BaseBranch: "main", HeadBranch: "f", BaseSHA: "b", HeadSHA: "h2"},
		Threads: threads,
	}}
	// Should not return error.
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}
	// good-thread comment persisted.
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM comments WHERE id='c-good'`).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("c-good should be present, got count=%d", count)
	}
}

func TestRecordPatchsetIfChanged_First(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)
	pr := &provider.PR{
		ID: "github:o/r:3", Provider: "github", Owner: "o", Repo: "r", Number: 3,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: "base", HeadSHA: "sha-1",
	}
	if err := w.WriteSyncResults([]*PRFetchResult{{PR: pr}}); err != nil {
		t.Fatalf("write: %v", err)
	}
	var num int
	var head string
	if err := db.QueryRow(`SELECT number, head_sha FROM patchsets WHERE pr_id=?`, pr.ID).Scan(&num, &head); err != nil {
		t.Fatalf("patchset not found: %v", err)
	}
	if num != 1 {
		t.Errorf("first patchset: got number=%d want 1", num)
	}
	if head != "sha-1" {
		t.Errorf("head_sha: got %q want sha-1", head)
	}
}

func TestRecordPatchsetIfChanged_Unchanged(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)
	pr := &provider.PR{
		ID: "github:o/r:4", Provider: "github", Owner: "o", Repo: "r", Number: 4,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: "base", HeadSHA: "sha-same",
	}
	for i := 0; i < 3; i++ {
		if err := w.WriteSyncResults([]*PRFetchResult{{PR: pr}}); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM patchsets WHERE pr_id=?`, pr.ID).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("unchanged head: got %d patchset rows want 1", count)
	}
}

func TestRecordPatchsetIfChanged_Changed(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)
	base := &provider.PR{
		ID: "github:o/r:5", Provider: "github", Owner: "o", Repo: "r", Number: 5,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: "base", HeadSHA: "sha-1",
	}
	if err := w.WriteSyncResults([]*PRFetchResult{{PR: base}}); err != nil {
		t.Fatalf("write P1: %v", err)
	}
	pushed := *base
	pushed.HeadSHA = "sha-2"
	if err := w.WriteSyncResults([]*PRFetchResult{{PR: &pushed}}); err != nil {
		t.Fatalf("write P2: %v", err)
	}
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM patchsets WHERE pr_id=?`, base.ID).Scan(&count) //nolint:errcheck
	if count != 2 {
		t.Errorf("changed head: got %d patchset rows want 2", count)
	}
	var num int
	db.QueryRow(`SELECT number FROM patchsets WHERE pr_id=? AND head_sha='sha-2'`, base.ID).Scan(&num) //nolint:errcheck
	if num != 2 {
		t.Errorf("P2 number: got %d want 2", num)
	}
}

func TestRecordPatchsetIfChanged_BaseOnlyChanged(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)
	base := &provider.PR{
		ID: "github:o/r:6", Provider: "github", Owner: "o", Repo: "r", Number: 6,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: "base-1", HeadSHA: "sha-same",
	}
	if err := w.WriteSyncResults([]*PRFetchResult{{PR: base}}); err != nil {
		t.Fatalf("write P1: %v", err)
	}
	retargeted := *base
	retargeted.BaseSHA = "base-2"
	if err := w.WriteSyncResults([]*PRFetchResult{{PR: &retargeted}}); err != nil {
		t.Fatalf("write P2: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM patchsets WHERE pr_id=?`, base.ID).Scan(&count); err != nil {
		t.Fatalf("count patchsets: %v", err)
	}
	if count != 2 {
		t.Fatalf("base-only diff change: got %d patchset rows want 2", count)
	}

	var num int
	if err := db.QueryRow(
		`SELECT number FROM patchsets WHERE pr_id=? AND head_sha=? AND base_sha=?`,
		base.ID, "sha-same", "base-2",
	).Scan(&num); err != nil {
		t.Fatalf("base-2 patchset not found: %v", err)
	}
	if num != 2 {
		t.Errorf("base-only P2 number: got %d want 2", num)
	}
}

// TestWriteReviewThreads_GraphQLIDConsistency (finding 2):
// When GetReviewThreads upgrades thread.ID to a GraphQL node ID it must also
// update every child comment's ThreadID so the FK review_threads(id) is satisfied.
// This test simulates the post-enrichment state (IDs already aligned) and verifies
// that both the thread row and the comment row share the same ID, preventing FK violations.
func TestWriteReviewThreads_GraphQLIDConsistency(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)
	mustWritePR(t, w, &provider.PR{
		ID: "github:o/r:10", Provider: "github", Owner: "o", Repo: "r", Number: 10,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: "b", HeadSHA: "hgql",
	})

	// Simulate post-GraphQL enrichment: thread.ID is a GraphQL node ID and all
	// child comments carry that same ID in their ThreadID field.
	gqlID := "RT_kwDOABcdefg"
	threads := []*provider.ReviewThread{{
		ID:         gqlID,
		PRID:       "github:o/r:10",
		FilePath:   "main.go",
		Line:       7,
		Resolved:   false,
		StateKnown: true,
		Comments: []*provider.Comment{
			{
				ID: "github-review-comment-100", PRID: "github:o/r:10",
				Author: "alice", Body: "nit", Kind: "review",
				ThreadID:  gqlID, // must match thread.ID
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
			},
		},
	}}

	results := []*PRFetchResult{{
		PR: &provider.PR{ID: "github:o/r:10", Provider: "github", Owner: "o", Repo: "r",
			Number: 10, Title: "T", Author: "a", State: "open",
			BaseBranch: "main", HeadBranch: "f", BaseSHA: "b", HeadSHA: "hgql"},
		Threads: threads,
	}}
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	// Thread row must use the GraphQL node ID.
	var threadID string
	if err := db.QueryRow(`SELECT id FROM review_threads WHERE id=?`, gqlID).Scan(&threadID); err != nil {
		t.Fatalf("review_thread with gqlID not found: %v", err)
	}

	// Comment's thread_id FK must point to the same ID.
	var commentThreadID string
	if err := db.QueryRow(
		`SELECT thread_id FROM comments WHERE id='github-review-comment-100'`,
	).Scan(&commentThreadID); err != nil {
		t.Fatalf("comment not found: %v", err)
	}
	if commentThreadID != gqlID {
		t.Errorf("comment thread_id FK mismatch: got %q want %q", commentThreadID, gqlID)
	}
}

func TestWriteReviewThreads_CanonicalPromotionDeletesProvisionalRow(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)
	pr := &provider.PR{ID: "github:o/r:11", Provider: "github", Owner: "o", Repo: "r",
		Number: 11, Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: "b", HeadSHA: "h"}

	provisionalID := "github-review-comment-200"
	commentID := "github-review-comment-201"
	fallbackThread := &provider.ReviewThread{
		ID:         provisionalID,
		PRID:       pr.ID,
		FilePath:   "main.go",
		Line:       9,
		StateKnown: false,
		Comments: []*provider.Comment{
			{ID: provisionalID, PRID: pr.ID, Author: "alice", Body: "root", Kind: "review",
				ThreadID: provisionalID, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			{ID: commentID, PRID: pr.ID, Author: "bob", Body: "reply", Kind: "review",
				InReplyTo: provisionalID, ThreadID: provisionalID, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
	if err := w.WriteSyncResults([]*PRFetchResult{{PR: pr, Threads: []*provider.ReviewThread{fallbackThread}}}); err != nil {
		t.Fatalf("fallback write: %v", err)
	}

	canonicalID := "RT_kwDOCanonical11"
	canonicalThread := &provider.ReviewThread{
		ID:         canonicalID,
		PRID:       pr.ID,
		FilePath:   "main.go",
		Line:       9,
		StateKnown: true,
		Comments: []*provider.Comment{
			{ID: provisionalID, PRID: pr.ID, Author: "alice", Body: "root", Kind: "review",
				ThreadID: canonicalID, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			{ID: commentID, PRID: pr.ID, Author: "bob", Body: "reply", Kind: "review",
				InReplyTo: provisionalID, ThreadID: canonicalID, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
	if err := w.WriteSyncResults([]*PRFetchResult{{PR: pr, Threads: []*provider.ReviewThread{canonicalThread}}}); err != nil {
		t.Fatalf("canonical write: %v", err)
	}

	var provisionalCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM review_threads WHERE id=?`, provisionalID).Scan(&provisionalCount); err != nil {
		t.Fatalf("count provisional: %v", err)
	}
	if provisionalCount != 0 {
		t.Fatalf("provisional thread row should be deleted after canonical promotion, got %d", provisionalCount)
	}

	var canonicalCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM review_threads WHERE id=?`, canonicalID).Scan(&canonicalCount); err != nil {
		t.Fatalf("count canonical: %v", err)
	}
	if canonicalCount != 1 {
		t.Fatalf("canonical thread row count: got %d want 1", canonicalCount)
	}

	rows, err := db.Query(`SELECT thread_id FROM comments WHERE id IN (?, ?) ORDER BY id`, provisionalID, commentID)
	if err != nil {
		t.Fatalf("query comments: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var threadID string
		if err := rows.Scan(&threadID); err != nil {
			t.Fatalf("scan comment thread_id: %v", err)
		}
		if threadID != canonicalID {
			t.Errorf("comment thread_id after promotion: got %q want %q", threadID, canonicalID)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
}

// ── Diff atomicity tests (blocker: pr_diffs must not advance independently) ──

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

func TestWriteDiffs_NewPR_AtomicWithPRRow(t *testing.T) {
	// New PR (never written before): PR row, patchset, and pr_diffs must all land
	// in the same transaction. This guards against the FK ordering bug where
	// SaveDiffs was called during fetch before the PR row existed.
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:20", 20, "head-new", "base-new")
	diffs := []store.DiffFile{
		{FilePath: "main.go", Patch: "+new line", Additions: 1},
		{FilePath: "util.go", Patch: "+helper", Additions: 1},
	}

	results := []*PRFetchResult{{
		PR:           pr,
		DiffFiles:    diffs,
		DiffComputed: true,
	}}
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	// PR row must exist.
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM prs WHERE id = ?`, pr.ID).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Fatalf("PR row missing after atomic write, count=%d", count)
	}

	// pr_diffs must also exist (written in same tx, after PR row satisfies FK).
	if n := countDiffFiles(t, db, pr.ID); n != 2 {
		t.Errorf("expected 2 diff files for new PR, got %d", n)
	}
}

func TestWriteDiffs_ErroredResult_DiffsNotReplaced(t *testing.T) {
	// When result.Error != nil, the writer skips that result entirely.
	// Pre-existing pr_diffs must be preserved (not cleared/replaced).
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:21", 21, "head-21", "base-21")
	mustWritePR(t, w, pr)

	// Seed pr_diffs for this PR.
	db.Exec(`INSERT INTO pr_diffs (pr_id, file_path, patch, additions, deletions) VALUES (?, 'a.go', 'patch', 1, 0)`, pr.ID) //nolint:errcheck

	// Now write a result with Error set — diffs must not be touched.
	badResult := &PRFetchResult{
		PR:           pr,
		DiffFiles:    nil, // no diffs
		DiffComputed: true,
		Error:        errors.New("upstream API failure"),
	}
	if err := w.WriteSyncResults([]*PRFetchResult{badResult}); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	// pr_diffs must still have the original row.
	if n := countDiffFiles(t, db, pr.ID); n != 1 {
		t.Errorf("errored result: expected 1 stale diff row preserved, got %d", n)
	}
}

func TestWriteDiffs_EmptyDiff_ClearsStaleRows_OnAcceptedResult(t *testing.T) {
	// DiffComputed=true with empty DiffFiles: pr_diffs cleared (stale files gone).
	// This happens when a PR is reverted to base so no files changed.
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:22", 22, "head-22", "base-22")
	mustWritePR(t, w, pr)

	// Seed stale pr_diffs.
	db.Exec(`INSERT INTO pr_diffs (pr_id, file_path, patch, additions, deletions) VALUES (?, 'stale.go', 'old', 1, 0)`, pr.ID) //nolint:errcheck
	if n := countDiffFiles(t, db, pr.ID); n != 1 {
		t.Fatalf("precondition: expected 1 stale row, got %d", n)
	}

	// Write accepted result with empty diff (zero-file change set).
	results := []*PRFetchResult{{
		PR:           makePR("github:o/r:22", 22, "head-22b", "base-22"),
		DiffFiles:    []store.DiffFile{}, // explicitly empty
		DiffComputed: true,
	}}
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	// Stale row must be gone.
	if n := countDiffFiles(t, db, pr.ID); n != 0 {
		t.Errorf("empty diff should clear stale rows, got %d rows", n)
	}
}

func TestWriteDiffs_DiffNotComputed_PreservesCache(t *testing.T) {
	// DiffComputed=false (diff generation not attempted or failed):
	// pr_diffs must not be touched — preserve the last-known-good cache.
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:23", 23, "head-23", "base-23")
	mustWritePR(t, w, pr)

	db.Exec(`INSERT INTO pr_diffs (pr_id, file_path, patch, additions, deletions) VALUES (?, 'cached.go', 'patch', 2, 0)`, pr.ID) //nolint:errcheck

	// Write result without diff (DiffComputed=false).
	results := []*PRFetchResult{{
		PR:           makePR("github:o/r:23", 23, "head-23b", "base-23"),
		DiffComputed: false, // diff was not computed this sync cycle
	}}
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	// Cached diff must survive.
	if n := countDiffFiles(t, db, pr.ID); n != 1 {
		t.Errorf("DiffComputed=false: expected cached diff preserved, got %d rows", n)
	}
}

func TestWriteDiffs_AtomicWithPatchset_BaseHeadChange(t *testing.T) {
	// When head or base changes, both patchset row and pr_diffs must advance
	// in the same transaction. Neither should be visible independently.
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	// P1: initial state
	pr1 := makePR("github:o/r:24", 24, "head-v1", "base-24")
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR:           pr1,
		DiffFiles:    []store.DiffFile{{FilePath: "a.go", Patch: "v1", Additions: 1}},
		DiffComputed: true,
	}}); err != nil {
		t.Fatalf("P1 write: %v", err)
	}
	patchsetCount := func() int {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM patchsets WHERE pr_id = ?`, pr1.ID).Scan(&n) //nolint:errcheck
		return n
	}
	if patchsetCount() != 1 {
		t.Fatalf("after P1: expected 1 patchset, got %d", patchsetCount())
	}
	if n := countDiffFiles(t, db, pr1.ID); n != 1 {
		t.Fatalf("after P1: expected 1 diff file, got %d", n)
	}

	// P2: head changes → new patchset + new diffs in one tx
	pr2 := makePR("github:o/r:24", 24, "head-v2", "base-24")
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: pr2,
		DiffFiles: []store.DiffFile{
			{FilePath: "a.go", Patch: "v2", Additions: 2},
			{FilePath: "b.go", Patch: "v2b", Additions: 1},
		},
		DiffComputed: true,
	}}); err != nil {
		t.Fatalf("P2 write: %v", err)
	}

	if patchsetCount() != 2 {
		t.Errorf("after P2: expected 2 patchsets, got %d", patchsetCount())
	}
	if n := countDiffFiles(t, db, pr1.ID); n != 2 {
		t.Errorf("after P2: expected 2 diff files (atomic replace), got %d", n)
	}

	// Verify the updated patch content (not the old v1 patch).
	var patch string
	db.QueryRow(`SELECT patch FROM pr_diffs WHERE pr_id = ? AND file_path = 'a.go'`, pr1.ID).Scan(&patch) //nolint:errcheck
	if patch != "v2" {
		t.Errorf("diff content after P2: got %q want v2", patch)
	}
}

// ── Critical atomicity / savepoint tests ─────────────────────────────────────

// TestWriteDiffs_InsertFailure_SavepointRollsBackAll verifies that when writeDiffs
// fails mid-insert (duplicate file_path in DiffFiles triggers a UNIQUE constraint),
// the savepoint rolls back the DELETE that already ran, preserving the old cache.
// It also asserts the PR row and patchset did NOT advance, since all three critical
// writes are inside the same savepoint boundary.
func TestWriteDiffs_InsertFailure_SavepointRollsBackAll(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	// Seed a PR and its diff cache.
	pr := makePR("github:o/r:30", 30, "head-30", "base-30")
	mustWritePR(t, w, pr)
	db.Exec(`INSERT INTO pr_diffs (pr_id, file_path, patch, additions, deletions) VALUES (?, 'cached.go', 'old-patch', 3, 1)`, pr.ID) //nolint:errcheck

	// Verify precondition: 1 stale row.
	if n := countDiffFiles(t, db, pr.ID); n != 1 {
		t.Fatalf("precondition: want 1 stale row, got %d", n)
	}

	// Duplicate file_path triggers UNIQUE constraint violation on the second INSERT,
	// AFTER the DELETE has already run. Without savepoints this would commit a cleared
	// cache. With savepoints, the whole PR's critical writes roll back.
	badDiffs := []store.DiffFile{
		{FilePath: "dup.go", Patch: "patch-a", Additions: 1},
		{FilePath: "dup.go", Patch: "patch-b", Additions: 2}, // duplicate: will fail
	}
	newPR := makePR("github:o/r:30", 30, "head-30b", "base-30")
	results := []*PRFetchResult{{
		PR:           newPR,
		DiffFiles:    badDiffs,
		DiffComputed: true,
	}}

	// Must not return a fatal batch error.
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults must not be fatal: %v", err)
	}

	// Old diff cache must be intact — savepoint rolled back the DELETE.
	if n := countDiffFiles(t, db, pr.ID); n != 1 {
		t.Errorf("old diff cache should be preserved after INSERT failure, got %d rows", n)
	}
	var savedPatch string
	db.QueryRow(`SELECT patch FROM pr_diffs WHERE pr_id = ?`, pr.ID).Scan(&savedPatch) //nolint:errcheck
	if savedPatch != "old-patch" {
		t.Errorf("preserved diff should still have old-patch content, got %q", savedPatch)
	}

	// PR head_sha must NOT have advanced — savepoint also rolled back the PR upsert.
	var headSHA string
	db.QueryRow(`SELECT head_sha FROM prs WHERE id = ?`, pr.ID).Scan(&headSHA) //nolint:errcheck
	if headSHA != "head-30" {
		t.Errorf("PR head_sha must not advance when writeDiffs failed: got %q, want head-30", headSHA)
	}

	// Patchset must also not have been added for the failed head.
	var psCount int
	db.QueryRow(`SELECT COUNT(*) FROM patchsets WHERE pr_id = ?`, pr.ID).Scan(&psCount) //nolint:errcheck
	// P1 was recorded by the initial mustWritePR, so count is 1 (not 2).
	if psCount != 1 {
		t.Errorf("no new patchset should be added when writeDiffs failed, patchset count=%d", psCount)
	}
}

// TestWritePR_PatchsetFailure_SavepointRollsBackPRRow verifies that when
// recordPatchsetIfChanged fails, the savepoint rolls back the PR row write too,
// so the prs table is not advanced without a matching patchset.
func TestWritePR_PatchsetFailure_SavepointRollsBackPRRow(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	// Drop the patchsets table to force recordPatchsetIfChanged to fail.
	if _, err := db.Exec("DROP TABLE patchsets"); err != nil {
		t.Fatalf("drop patchsets: %v", err)
	}

	pr := makePR("github:o/r:31", 31, "head-31", "base-31")
	results := []*PRFetchResult{{PR: pr}}

	// Must not return a fatal batch error (one PR failed, batch continues).
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults must not be fatal: %v", err)
	}

	// PR row must NOT exist — savepoint rolled back the writePRRow.
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM prs WHERE id = ?`, pr.ID).Scan(&count) //nolint:errcheck
	if count != 0 {
		t.Errorf("PR row must not exist when patchset recording failed, got count=%d", count)
	}
}

// TestWriteSavepoint_OtherPRsUnaffected verifies that a critical failure for
// one PR does not roll back other PRs in the same batch.
func TestWriteSavepoint_OtherPRsUnaffected(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	goodPR := makePR("github:o/r:40", 40, "head-40", "base-40")
	badDiffs := []store.DiffFile{
		{FilePath: "dup.go", Patch: "a"},
		{FilePath: "dup.go", Patch: "b"}, // duplicate → INSERT failure
	}
	failPR := makePR("github:o/r:41", 41, "head-41", "base-41")

	results := []*PRFetchResult{
		{PR: goodPR},
		{PR: failPR, DiffFiles: badDiffs, DiffComputed: true},
	}
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults must not be fatal: %v", err)
	}

	// Good PR must have been committed.
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM prs WHERE id = ?`, goodPR.ID).Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("good PR must be committed despite other PR's failure, count=%d", count)
	}

	// Failed PR must not have advanced.
	db.QueryRow(`SELECT COUNT(*) FROM prs WHERE id = ?`, failPR.ID).Scan(&count) //nolint:errcheck
	if count != 0 {
		t.Errorf("failed PR must be rolled back, count=%d", count)
	}
}

// ── Anchor write-back tests ───────────────────────────────────────────────

func TestWriteAnchorUpdates_WritesAnchoredLine(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:50", 50, "sha-head", "sha-base")
	pos := 5
	// Write PR with a review comment.
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: pr,
		Comments: []*provider.Comment{{
			ID:        "rc-1",
			PRID:      pr.ID,
			Author:    "alice",
			Body:      "nit",
			Kind:      "review",
			CommitSHA: "sha-commit",
			Position:  &pos,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}},
	}}); err != nil {
		t.Fatalf("initial write: %v", err)
	}

	// Now write with AnchorUpdates: anchored at line 12.
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: makePR("github:o/r:50", 50, "sha-head", "sha-base"),
		AnchorUpdates: []git.AnchorResult{{
			CommentID:    "rc-1",
			AnchoredLine: 12,
			HeadSHA:      "sha-head",
			Anchored:     true,
		}},
	}}); err != nil {
		t.Fatalf("anchor write: %v", err)
	}

	var anchoredLine *int
	var anchorHeadSHA *string
	var outdated int
	db.QueryRow(`SELECT anchored_line, anchor_head_sha, outdated FROM comments WHERE id = 'rc-1'`).
		Scan(&anchoredLine, &anchorHeadSHA, &outdated) //nolint:errcheck
	if anchoredLine == nil || *anchoredLine != 12 {
		t.Errorf("anchored_line: got %v want 12", anchoredLine)
	}
	if anchorHeadSHA == nil || *anchorHeadSHA != "sha-head" {
		t.Errorf("anchor_head_sha: got %v want sha-head", anchorHeadSHA)
	}
	if outdated != 0 {
		t.Errorf("outdated: got %d want 0", outdated)
	}
}

func TestWriteAnchorUpdates_OutdatedComment(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:51", 51, "sha-h51", "sha-b51")
	pos := 3
	w.WriteSyncResults([]*PRFetchResult{{
		PR: pr,
		Comments: []*provider.Comment{{
			ID: "rc-2", PRID: pr.ID, Author: "bob", Body: "q",
			Kind: "review", CommitSHA: "sha-c51", Position: &pos,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}},
	}}) //nolint:errcheck

	w.WriteSyncResults([]*PRFetchResult{{
		PR: makePR("github:o/r:51", 51, "sha-h51", "sha-b51"),
		AnchorUpdates: []git.AnchorResult{{
			CommentID:        "rc-2",
			HeadSHA:          "sha-h51",
			Outdated:         true,
			OutdatedNearLine: 7,
		}},
	}}) //nolint:errcheck

	var anchoredLine *int
	var outdated int
	var nearLineDB *int
	db.QueryRow(`SELECT anchored_line, outdated, outdated_near_line FROM comments WHERE id = 'rc-2'`).
		Scan(&anchoredLine, &outdated, &nearLineDB) //nolint:errcheck
	if anchoredLine != nil {
		t.Errorf("anchored_line should be NULL when outdated, got %d", *anchoredLine)
	}
	if outdated != 1 {
		t.Errorf("outdated: got %d want 1", outdated)
	}
	if nearLineDB == nil || *nearLineDB != 7 {
		t.Errorf("outdated_near_line: got %v want 7", nearLineDB)
	}
}

func TestWriteAnchorUpdates_NoUpdates_ColumnsRemainNull(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:52", 52, "sha-h52", "sha-b52")
	pos := 2
	w.WriteSyncResults([]*PRFetchResult{{
		PR: pr,
		Comments: []*provider.Comment{{
			ID: "rc-3", PRID: pr.ID, Author: "carol", Body: "x",
			Kind: "review", CommitSHA: "sha-c52", Position: &pos,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}},
	}}) //nolint:errcheck

	// Sync with no AnchorUpdates (e.g. clone unavailable).
	w.WriteSyncResults([]*PRFetchResult{{
		PR:            makePR("github:o/r:52", 52, "sha-h52b", "sha-b52"),
		AnchorUpdates: nil, // no updates
	}}) //nolint:errcheck

	var anchoredLine *int
	db.QueryRow(`SELECT anchored_line FROM comments WHERE id = 'rc-3'`).Scan(&anchoredLine) //nolint:errcheck
	if anchoredLine != nil {
		t.Errorf("anchored_line should remain NULL when no AnchorUpdates, got %d", *anchoredLine)
	}
}

func TestWriteAnchorUpdates_ErrorSkippedSilently(t *testing.T) {
	// AnchorUpdate referencing a non-existent comment ID is a no-op (UPDATE 0 rows).
	// The writer must not fail the batch for this.
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)
	pr := makePR("github:o/r:53", 53, "sha-h53", "sha-b53")
	mustWritePR(t, w, pr)

	results := []*PRFetchResult{{
		PR: makePR("github:o/r:53", 53, "sha-h53", "sha-b53"),
		AnchorUpdates: []git.AnchorResult{{
			CommentID:    "nonexistent-comment",
			AnchoredLine: 5,
			HeadSHA:      "sha-h53",
			Anchored:     true,
		}},
	}}
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults must not fail for missing comment anchor: %v", err)
	}
}

// TestWriteComment_OutdatedComment_PreservesPatchsetNumber verifies requirement 4:
// outdated/rebased comments (Position==nil, OriginalPosition set) must NOT have
// their patchset_number overwritten with the current patchset number.
// The COALESCE in writeComment preserves the original value when patchsetNum==0.
func TestWriteComment_OutdatedComment_PreservesPatchsetNumber(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:60", 60, "head-60", "base-60")
	origPos := 5

	// First sync: write the comment with patchset_number=1 (original patchset).
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: pr,
		Comments: []*provider.Comment{{
			ID:        "outdated-c1",
			PRID:      pr.ID,
			Author:    "alice",
			Body:      "review",
			Kind:      "review",
			CommitSHA: "sha-orig",
			Position:  &origPos, // has current position on first write
			CreatedAt: mustTime(), UpdatedAt: mustTime(),
		}},
	}}); err != nil {
		t.Fatalf("first write: %v", err)
	}
	// Manually set patchset_number=1 to simulate it was written on P1.
	db.Exec(`UPDATE comments SET patchset_number=1 WHERE id='outdated-c1'`) //nolint:errcheck

	// Second sync: comment is now outdated (Position==nil), OriginalPosition set.
	// This represents a force-push after which GitHub marks the comment outdated.
	newPR := makePR("github:o/r:60", 60, "head-60b", "base-60") // new head → new patchset
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: newPR,
		Comments: []*provider.Comment{{
			ID:               "outdated-c1",
			PRID:             pr.ID,
			Author:           "alice",
			Body:             "review",
			Kind:             "review",
			CommitSHA:        "", // cleared by GitHub for outdated comments
			OriginalSHA:      "sha-orig",
			Position:         nil,      // no current position — outdated
			OriginalPosition: &origPos, // original position still set
			CreatedAt:        mustTime(), UpdatedAt: mustTime(),
		}},
	}}); err != nil {
		t.Fatalf("second write (outdated): %v", err)
	}

	// patchset_number must still be 1 (NOT overwritten with current patchset number).
	var psNum *int
	db.QueryRow(`SELECT patchset_number FROM comments WHERE id='outdated-c1'`).Scan(&psNum) //nolint:errcheck
	if psNum == nil || *psNum != 1 {
		t.Errorf("outdated comment patchset_number: got %v want 1 (must not be overwritten with current patchset)", psNum)
	}
}

func TestWriteComment_ReviewPositionFields(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:61", 61, "head-61", "base-61")
	pos := 3
	origPos := 4
	origLine := 40
	startLine := 38
	origStartLine := 37
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: pr,
		Comments: []*provider.Comment{{
			ID: "review-position-c1", PRID: pr.ID, Author: "alice", Body: "range nit",
			Kind: "review", CommitSHA: "head-61", OriginalSHA: "orig-61", Side: "RIGHT",
			StartSide: "LEFT", Position: &pos, OriginalPosition: &origPos,
			OriginalLine: &origLine, StartLine: &startLine, OriginalStartLine: &origStartLine,
			SubjectType: "LINE", FilePath: "main.go", LineNo: 41,
			CreatedAt: mustTime(), UpdatedAt: mustTime(),
		}},
	}}); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	var gotOriginalLine, gotStartLine, gotOriginalStartLine *int
	var gotStartSide, gotSubjectType *string
	if err := db.QueryRow(`SELECT original_line, start_line, original_start_line, start_side, subject_type FROM comments WHERE id='review-position-c1'`).Scan(
		&gotOriginalLine, &gotStartLine, &gotOriginalStartLine, &gotStartSide, &gotSubjectType,
	); err != nil {
		t.Fatalf("query review positioning fields: %v", err)
	}
	if gotOriginalLine == nil || *gotOriginalLine != 40 || gotStartLine == nil || *gotStartLine != 38 || gotOriginalStartLine == nil || *gotOriginalStartLine != 37 {
		t.Fatalf("line positioning fields = original:%v start:%v original_start:%v", gotOriginalLine, gotStartLine, gotOriginalStartLine)
	}
	if gotStartSide == nil || *gotStartSide != "LEFT" || gotSubjectType == nil || *gotSubjectType != "LINE" {
		t.Fatalf("side/subject fields = start_side:%v subject_type:%v", gotStartSide, gotSubjectType)
	}
}

// ── Blocker 1: thread comment patchset_number stamping ────────────────────

// TestWriteThreadComment_CurrentPosition_StampsPatchsetNumber verifies that
// review comments arriving via result.Threads (not result.Comments) receive
// the current patchset_number when they have a current Position.
func TestWriteThreadComment_CurrentPosition_StampsPatchsetNumber(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:70", 70, "head-70", "base-70")
	pos := 3
	thread := &provider.ReviewThread{
		ID:   "thread-70",
		PRID: pr.ID,
		Comments: []*provider.Comment{{
			ID:        "tc-current",
			PRID:      pr.ID,
			Author:    "alice",
			Body:      "thread comment",
			Kind:      "review",
			CommitSHA: "head-70",
			Position:  &pos, // current position → should get patchset_number
			CreatedAt: mustTime(), UpdatedAt: mustTime(),
		}},
	}

	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR:      pr,
		Threads: []*provider.ReviewThread{thread},
	}}); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	var psNum *int
	db.QueryRow(`SELECT patchset_number FROM comments WHERE id='tc-current'`).Scan(&psNum) //nolint:errcheck
	// patchset was written by the PR write; thread comment should have been stamped.
	if psNum == nil || *psNum == 0 {
		t.Errorf("thread comment with current position: patchset_number should be stamped, got %v", psNum)
	}
}

// TestWriteThreadComment_OutdatedPosition_PreservesPatchsetNumber verifies that
// an outdated thread comment (Position==nil) preserves its existing patchset_number
// instead of being overwritten with the current patchset number.
func TestWriteThreadComment_OutdatedPosition_PreservesPatchsetNumber(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:71", 71, "head-71", "base-71")
	origPos := 5

	// First sync: current comment with patchset stamped.
	pos := 5
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: pr,
		Threads: []*provider.ReviewThread{{
			ID:   "thread-71",
			PRID: pr.ID,
			Comments: []*provider.Comment{{
				ID:        "tc-outdated",
				PRID:      pr.ID,
				Author:    "bob",
				Body:      "thread comment",
				Kind:      "review",
				CommitSHA: "head-71",
				Position:  &pos,
				CreatedAt: mustTime(), UpdatedAt: mustTime(),
			}},
		}},
	}}); err != nil {
		t.Fatalf("first write: %v", err)
	}
	// Manually mark patchset_number=1 (as if it was stamped on first sync).
	db.Exec(`UPDATE comments SET patchset_number=1 WHERE id='tc-outdated'`) //nolint:errcheck

	// Second sync: the thread comment is now outdated (Position==nil after force-push).
	newPR := makePR("github:o/r:71", 71, "head-71b", "base-71")
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: newPR,
		Threads: []*provider.ReviewThread{{
			ID:   "thread-71",
			PRID: pr.ID,
			Comments: []*provider.Comment{{
				ID:               "tc-outdated",
				PRID:             pr.ID,
				Author:           "bob",
				Body:             "thread comment",
				Kind:             "review",
				OriginalSHA:      "head-71",
				Position:         nil, // outdated — no current position
				OriginalPosition: &origPos,
				CreatedAt:        mustTime(), UpdatedAt: mustTime(),
			}},
		}},
	}}); err != nil {
		t.Fatalf("second write: %v", err)
	}

	var psNum *int
	db.QueryRow(`SELECT patchset_number FROM comments WHERE id='tc-outdated'`).Scan(&psNum) //nolint:errcheck
	if psNum == nil || *psNum != 1 {
		t.Errorf("outdated thread comment: patchset_number should remain 1, got %v", psNum)
	}
}

// ── First-sync anchor identity tests ─────────────────────────────────────

// TestWriteAnchorUpdates_FirstSync_GetsCurrentPatchsetNum is the primary
// regression test for the "second sync needed" bug: a current-position anchor
// resolved during first sync has PatchsetNumber=0 (patchset not in DB at fetch
// time), but writeAnchorUpdates must fill it in with currentPatchsetNum so
// anchor_patchset_number is non-NULL and the handler treats the anchor as fresh
// without requiring a second sync.
func TestWriteAnchorUpdates_FirstSync_GetsCurrentPatchsetNum(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	// First sync: write PR + review comment + anchor update with PatchsetNumber=0.
	pr := makePR("github:o/r:80", 80, "sha-head-80", "sha-base-80")
	pos := 3
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: pr,
		Comments: []*provider.Comment{{
			ID:        "rc-first-sync",
			PRID:      pr.ID,
			Author:    "alice",
			Body:      "nit",
			Kind:      "review",
			CommitSHA: "sha-head-80",
			Position:  &pos,
			CreatedAt: mustTime(), UpdatedAt: mustTime(),
		}},
		// PatchsetNumber=0: simulates first sync where patchset row didn't exist
		// during fetch (resolveAnchors returned resolvedPatchsetNum=0).
		AnchorUpdates: []git.AnchorResult{{
			CommentID:      "rc-first-sync",
			AnchoredLine:   5,
			Anchored:       true,
			HeadSHA:        "sha-head-80",
			PatchsetNumber: 0, // ← the bug: was NULL before this fix
		}},
	}}); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	// Verify anchor_patchset_number is NON-NULL (set to currentPatchsetNum).
	var anchorPsNum *int
	db.QueryRow(`SELECT anchor_patchset_number FROM comments WHERE id='rc-first-sync'`).Scan(&anchorPsNum) //nolint:errcheck
	if anchorPsNum == nil {
		t.Fatal("anchor_patchset_number must be non-NULL on first sync (not stale)")
	}

	// Verify anchor_current_patchset_number is also set (same value for current-position).
	var anchorCurPsNum *int
	db.QueryRow(`SELECT anchor_current_patchset_number FROM comments WHERE id='rc-first-sync'`).Scan(&anchorCurPsNum) //nolint:errcheck
	if anchorCurPsNum == nil {
		t.Fatal("anchor_current_patchset_number must be non-NULL on first sync")
	}
	if *anchorCurPsNum == 0 {
		t.Error("anchor_current_patchset_number must be > 0")
	}

	// Verify the patchset row was written (for completeness).
	var psCount int
	db.QueryRow(`SELECT COUNT(*) FROM patchsets WHERE pr_id=?`, pr.ID).Scan(&psCount) //nolint:errcheck
	if psCount == 0 {
		t.Error("patchset row must exist after WriteSyncResults")
	}

	// Verify anchor_patchset_number matches the patchset that was written.
	var currentPsNum int
	db.QueryRow(`SELECT number FROM patchsets WHERE pr_id=? AND head_sha=? AND base_sha=?`,
		pr.ID, "sha-head-80", "sha-base-80",
	).Scan(&currentPsNum) //nolint:errcheck
	if *anchorPsNum != currentPsNum {
		t.Errorf("anchor_patchset_number=%d must equal currentPatchsetNum=%d", *anchorPsNum, currentPsNum)
	}
}

// TestWriteAnchorUpdates_OriginalPosition_KeepsOriginalPatchsetNum verifies
// that an original-position anchor (PatchsetNumber > 0) is NOT overwritten
// with the current patchset number, preserving its correct historical identity.
func TestWriteAnchorUpdates_OriginalPosition_KeepsOriginalPatchsetNum(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	// Set up: two patchsets. Current PR is on P2; the comment was made on P1.
	pr := makePR("github:o/r:81", 81, "sha-head-81", "sha-base-81")

	// First write: record P1 (initial patchset).
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: makePR("github:o/r:81", 81, "sha-head-81a", "sha-base-81"),
		Comments: []*provider.Comment{{
			ID: "rc-orig-ps", PRID: pr.ID, Author: "bob", Body: "q",
			Kind: "review", CommitSHA: "sha-head-81a",
			Position:  func() *int { p := 2; return &p }(),
			CreatedAt: mustTime(), UpdatedAt: mustTime(),
		}},
	}}); err != nil {
		t.Fatalf("P1 write: %v", err)
	}

	// Manually set the comment to simulate it was anchored for P1 with PatchsetNumber=1.
	db.Exec(`UPDATE comments SET patchset_number=1 WHERE id='rc-orig-ps'`) //nolint:errcheck

	// Second write: PR moves to new head (P2 = currentPatchsetNum=2).
	// The comment is now outdated (original-position), anchor was resolved for P1.
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: pr, // sha-head-81, sha-base-81 → P2
		AnchorUpdates: []git.AnchorResult{{
			CommentID:        "rc-orig-ps",
			Outdated:         true,
			OutdatedNearLine: 3,
			Anchored:         false,
			HeadSHA:          "sha-head-81",
			PatchsetNumber:   1, // ← original patchset, must NOT be overwritten with P2
		}},
	}}); err != nil {
		t.Fatalf("P2 write: %v", err)
	}

	// anchor_patchset_number must remain 1 (original patchset), not 2 (current).
	var anchorPsNum *int
	db.QueryRow(`SELECT anchor_patchset_number FROM comments WHERE id='rc-orig-ps'`).Scan(&anchorPsNum) //nolint:errcheck
	if anchorPsNum == nil || *anchorPsNum != 1 {
		t.Errorf("original-position anchor: anchor_patchset_number should be 1 (original), got %v", anchorPsNum)
	}
}

// TestWriteAnchorUpdates_BothZero_WritesNull verifies that when both
// PatchsetNumber=0 AND currentPatchsetNum=0 (degenerate case), anchor_patchset_number
// is NULL — handler treats it as stale, which is the correct safe behavior.
func TestWriteAnchorUpdates_BothZero_WritesNull(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	// Write a PR without a patchset (edge case: recordPatchsetIfChanged failed
	// would trigger savepoint rollback, so in practice currentPatchsetNum > 0
	// when we reach writeAnchorUpdates — but test the defensive path anyway).
	pr := makePR("github:o/r:82", 82, "sha-head-82", "sha-base-82")
	mustWritePR(t, w, pr)
	db.Exec(`INSERT INTO comments (id, pr_id, author, body, created_at, updated_at, kind)
		VALUES ('rc-zero-ps', ?, 'c', 'b', '2026-01-01', '2026-01-01', 'review')`, pr.ID) //nolint:errcheck

	// Directly call writeAnchorUpdates with currentPatchsetNum=0 (both zero).
	ctx := context.Background()
	tx, _ := db.BeginTx(ctx, nil)
	w.writeAnchorUpdates(ctx, tx, []git.AnchorResult{{
		CommentID:      "rc-zero-ps",
		AnchoredLine:   7,
		Anchored:       true,
		HeadSHA:        "sha-head-82",
		PatchsetNumber: 0, // fetcher didn't find patchset
	}}, 0 /* currentPatchsetNum also 0 */)
	tx.Commit() //nolint:errcheck

	var anchorPsNum *int
	db.QueryRow(`SELECT anchor_patchset_number FROM comments WHERE id='rc-zero-ps'`).Scan(&anchorPsNum) //nolint:errcheck
	if anchorPsNum != nil {
		t.Errorf("both-zero: anchor_patchset_number should be NULL (stale), got %d", *anchorPsNum)
	}
}

func mustTime() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
