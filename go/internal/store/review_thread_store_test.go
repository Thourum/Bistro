package store

import (
	"context"
	"testing"
)

func TestListReviewThreads_Empty(t *testing.T) {
	db := newTestDB(t)
	mustInsertPR(t, db, "pr-rt-1")
	rows, err := db.ListReviewThreads(context.Background(), "pr-rt-1")
	if err != nil {
		t.Fatalf("ListReviewThreads: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestListReviewThreads_ReturnsRows(t *testing.T) {
	db := newTestDB(t)
	mustInsertPR(t, db, "pr-rt-2")
	mustExec(t, db, `INSERT INTO review_threads (id, pr_id, file_path, line, resolved, outdated, state_known)
		VALUES ('rt-1', 'pr-rt-2', 'main.go', 10, 0, 0, 1)`)
	mustExec(t, db, `INSERT INTO review_threads (id, pr_id, file_path, line, resolved, outdated, state_known)
		VALUES ('rt-2', 'pr-rt-2', 'util.go', 5, 1, 0, 1)`)

	rows, err := db.ListReviewThreads(context.Background(), "pr-rt-2")
	if err != nil {
		t.Fatalf("ListReviewThreads: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// Ordered by file_path, line: main.go < util.go
	if rows[0].ID != "rt-1" || rows[0].FilePath != "main.go" {
		t.Errorf("unexpected first row: %+v", rows[0])
	}
	if rows[1].ID != "rt-2" || rows[1].Resolved != true {
		t.Errorf("unexpected second row: %+v", rows[1])
	}
}

func TestSetThreadResolved_ResolveAndUnresolve(t *testing.T) {
	db := newTestDB(t)
	mustInsertPR(t, db, "pr-rt-3")
	mustExec(t, db, `INSERT INTO review_threads (id, pr_id, resolved) VALUES ('rt-3', 'pr-rt-3', 0)`)

	ctx := context.Background()

	// Initially unresolved.
	rows, _ := db.ListReviewThreads(ctx, "pr-rt-3")
	if len(rows) != 1 || rows[0].Resolved {
		t.Fatalf("expected unresolved thread, got: %+v", rows)
	}

	// Resolve it (now requires pr_id too).
	if err := db.SetThreadResolved(ctx, "pr-rt-3", "rt-3", true); err != nil {
		t.Fatalf("SetThreadResolved true: %v", err)
	}
	rows, _ = db.ListReviewThreads(ctx, "pr-rt-3")
	if !rows[0].Resolved {
		t.Error("expected Resolved=true after SetThreadResolved")
	}

	// Unresolve it.
	if err := db.SetThreadResolved(ctx, "pr-rt-3", "rt-3", false); err != nil {
		t.Fatalf("SetThreadResolved false: %v", err)
	}
	rows, _ = db.ListReviewThreads(ctx, "pr-rt-3")
	if rows[0].Resolved {
		t.Error("expected Resolved=false after SetThreadResolved(false)")
	}
}

// TestSetThreadResolved_ErrorOnMissingThread verifies that updating a
// non-existent thread (or wrong pr_id) returns an error (no rows affected).
func TestSetThreadResolved_ErrorOnMissingThread(t *testing.T) {
	db := newTestDB(t)
	mustInsertPR(t, db, "pr-rt-nm")
	// No thread row inserted — should return error.
	err := db.SetThreadResolved(context.Background(), "pr-rt-nm", "nonexistent-thread", true)
	if err == nil {
		t.Error("SetThreadResolved on missing thread should return an error")
	}
}

// TestSetThreadResolved_WrongPR verifies that scoping by pr_id prevents
// cross-PR mutations (thread exists but belongs to a different PR).
func TestSetThreadResolved_WrongPR(t *testing.T) {
	db := newTestDB(t)
	mustInsertPR(t, db, "pr-A")
	mustInsertPR(t, db, "pr-B")
	mustExec(t, db, `INSERT INTO review_threads (id, pr_id, resolved) VALUES ('rt-x', 'pr-A', 0)`)

	// Try to update with wrong PR — should fail (0 rows affected).
	err := db.SetThreadResolved(context.Background(), "pr-B", "rt-x", true)
	if err == nil {
		t.Error("SetThreadResolved with wrong pr_id should return an error")
	}

	// Thread on pr-A must remain unresolved.
	rows, _ := db.ListReviewThreads(context.Background(), "pr-A")
	if len(rows) == 0 || rows[0].Resolved {
		t.Error("thread in pr-A must remain unresolved after wrong-PR update attempt")
	}
}

// ── GetReviewThread ───────────────────────────────────────────────────────

func TestGetReviewThread_Found(t *testing.T) {
	db := newTestDB(t)
	mustInsertPR(t, db, "pr-g1")
	mustExec(t, db, `INSERT INTO review_threads (id, pr_id, file_path, line, resolved, outdated, state_known)
		VALUES ('rt-g1', 'pr-g1', 'foo.go', 3, 0, 0, 1)`)

	row, err := db.GetReviewThread(context.Background(), "pr-g1", "rt-g1")
	if err != nil {
		t.Fatalf("GetReviewThread: %v", err)
	}
	if row == nil {
		t.Fatal("expected non-nil row")
	}
	if row.ID != "rt-g1" || row.PRID != "pr-g1" || !row.StateKnown {
		t.Errorf("unexpected row: %+v", row)
	}
}

func TestGetReviewThread_NotFound(t *testing.T) {
	db := newTestDB(t)
	mustInsertPR(t, db, "pr-g2")

	row, err := db.GetReviewThread(context.Background(), "pr-g2", "does-not-exist")
	if err != nil {
		t.Fatalf("GetReviewThread: %v", err)
	}
	if row != nil {
		t.Errorf("expected nil for missing thread, got: %+v", row)
	}
}

// TestGetReviewThread_WrongPR verifies that a thread is not found when the
// pr_id does not match, even if the thread id exists under a different PR.
func TestGetReviewThread_WrongPR(t *testing.T) {
	db := newTestDB(t)
	mustInsertPR(t, db, "pr-g3")
	mustInsertPR(t, db, "pr-g4")
	mustExec(t, db, `INSERT INTO review_threads (id, pr_id, resolved) VALUES ('rt-shared', 'pr-g3', 0)`)

	row, err := db.GetReviewThread(context.Background(), "pr-g4", "rt-shared")
	if err != nil {
		t.Fatalf("GetReviewThread: %v", err)
	}
	if row != nil {
		t.Errorf("expected nil when pr_id does not match, got: %+v", row)
	}
}

func TestReplyTargetCommentIDForThread_PrefersRootComment(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-reply")
	mustExec(t, db, `INSERT INTO review_threads (id, pr_id, resolved) VALUES ('PRRT_reply', 'pr-reply', 0)`)
	mustExec(t, db, `INSERT INTO comments (id, pr_id, author, body, created_at, updated_at, kind, thread_id, in_reply_to)
		VALUES ('github-review-comment-2', 'pr-reply', 'a', 'child', '2024-01-02', '2024-01-02', 'review', 'PRRT_reply', 'github-review-comment-1')`)
	mustExec(t, db, `INSERT INTO comments (id, pr_id, author, body, created_at, updated_at, kind, thread_id)
		VALUES ('github-review-comment-1', 'pr-reply', 'a', 'root', '2024-01-01', '2024-01-01', 'review', 'PRRT_reply')`)

	id, err := db.ReplyTargetCommentIDForThread(ctx, "pr-reply", "PRRT_reply")
	if err != nil {
		t.Fatalf("ReplyTargetCommentIDForThread: %v", err)
	}
	if id != "github-review-comment-1" {
		t.Fatalf("target id = %q, want root comment", id)
	}
}
