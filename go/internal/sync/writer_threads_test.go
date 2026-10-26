package sync

import (
	"testing"
	"time"

	"github.com/itterno/bistro/internal/provider"
)

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
