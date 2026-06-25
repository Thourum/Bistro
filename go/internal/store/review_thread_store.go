package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ReviewThreadRow is a row from the review_threads table, used by the WS handler.
type ReviewThreadRow struct {
	ID         string `json:"id"`
	PRID       string `json:"pr_id"`
	FilePath   string `json:"file_path"`
	Line       int    `json:"line"`
	Resolved   bool   `json:"resolved"`
	Outdated   bool   `json:"outdated"`
	StateKnown bool   `json:"state_known"`
}

// ListReviewThreads returns all review threads for a PR, ordered by file_path, line.
// Only threads for which state_known=true are included in the default read path;
// the WS handler may choose to return all and let the UI filter.
func (db *DB) ListReviewThreads(ctx context.Context, prID string) ([]ReviewThreadRow, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, pr_id, COALESCE(file_path, ''), COALESCE(line, 0),
		        resolved, outdated, state_known
		 FROM review_threads
		 WHERE pr_id = ?
		 ORDER BY file_path, line`,
		prID,
	)
	if err != nil {
		return nil, fmt.Errorf("list review threads (%s): %w", prID, err)
	}
	defer rows.Close()
	var out []ReviewThreadRow
	for rows.Next() {
		var t ReviewThreadRow
		var resolvedInt, outdatedInt, stateKnownInt int
		if err := rows.Scan(&t.ID, &t.PRID, &t.FilePath, &t.Line,
			&resolvedInt, &outdatedInt, &stateKnownInt); err != nil {
			return nil, fmt.Errorf("scan review thread: %w", err)
		}
		t.Resolved = resolvedInt != 0
		t.Outdated = outdatedInt != 0
		t.StateKnown = stateKnownInt != 0
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetReviewThread loads a single thread scoped by both pr_id and id.
// Returns (nil, nil) when no row matches — callers should treat that as
// "not found" and reject the action.
func (db *DB) GetReviewThread(ctx context.Context, prID, threadID string) (*ReviewThreadRow, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, pr_id, COALESCE(file_path, ''), COALESCE(line, 0),
		        resolved, outdated, state_known
		 FROM review_threads
		 WHERE pr_id = ? AND id = ?`,
		prID, threadID,
	)
	var t ReviewThreadRow
	var resolvedInt, outdatedInt, stateKnownInt int
	err := row.Scan(&t.ID, &t.PRID, &t.FilePath, &t.Line,
		&resolvedInt, &outdatedInt, &stateKnownInt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get review thread (%s, %s): %w", prID, threadID, err)
	}
	t.Resolved = resolvedInt != 0
	t.Outdated = outdatedInt != 0
	t.StateKnown = stateKnownInt != 0
	return &t, nil
}

// SetThreadResolved updates the resolved flag for a review thread.
// The update is scoped by both pr_id and id to prevent cross-PR mutations.
// Returns an error when no row was affected (unknown thread or wrong PR).
// Called after a successful resolve/unresolve GraphQL mutation to keep the
// local cache consistent without waiting for the next full sync.
func (db *DB) SetThreadResolved(ctx context.Context, prID, threadID string, resolved bool) error {
	resolvedInt := 0
	if resolved {
		resolvedInt = 1
	}
	result, err := db.ExecContext(ctx,
		`UPDATE review_threads SET resolved = ? WHERE pr_id = ? AND id = ?`,
		resolvedInt, prID, threadID,
	)
	if err != nil {
		return fmt.Errorf("set thread resolved (%s/%s, %v): %w", prID, threadID, resolved, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set thread resolved rows affected (%s/%s): %w", prID, threadID, err)
	}
	if n == 0 {
		return fmt.Errorf("set thread resolved: thread %s not found in pr %s", threadID, prID)
	}
	return nil
}

// ReplyTargetCommentIDForThread returns a replyable review-comment ID for a
// thread scoped by PR. Prefer the root review comment because GitHub threads are
// anchored by the original review comment; fall back to the oldest available
// comment if cached reply metadata is incomplete.
func (db *DB) ReplyTargetCommentIDForThread(ctx context.Context, prID, threadID string) (string, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id
		 FROM comments
		 WHERE pr_id = ? AND thread_id = ? AND kind = 'review'
		 ORDER BY CASE WHEN in_reply_to IS NULL OR in_reply_to = '' THEN 0 ELSE 1 END,
		          created_at ASC,
		          id ASC
		 LIMIT 1`,
		prID, threadID,
	)
	var id string
	err := row.Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reply target comment for thread (%s, %s): %w", prID, threadID, err)
	}
	return id, nil
}
