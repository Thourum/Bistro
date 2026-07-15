package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	storedb "github.com/itterno/bistro/internal/store/db"
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
	rows, err := storedb.New(db).ListReviewThreads(ctx, prID)
	if err != nil {
		return nil, fmt.Errorf("list review threads (%s): %w", prID, err)
	}

	out := make([]ReviewThreadRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, reviewThreadFromListRow(row))
	}
	return out, nil
}

// GetReviewThread loads a single thread scoped by both pr_id and id.
// Returns (nil, nil) when no row matches — callers should treat that as
// "not found" and reject the action.
func (db *DB) GetReviewThread(ctx context.Context, prID, threadID string) (*ReviewThreadRow, error) {
	row, err := storedb.New(db).GetReviewThread(ctx, storedb.GetReviewThreadParams{
		PrID: prID,
		ID:   threadID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get review thread (%s, %s): %w", prID, threadID, err)
	}

	thread := reviewThreadFromGetRow(row)
	return &thread, nil
}

// SetThreadResolved updates the resolved flag for a review thread.
// The update is scoped by both pr_id and id to prevent cross-PR mutations.
// Returns an error when no row was affected (unknown thread or wrong PR).
// Called after a successful resolve/unresolve GraphQL mutation to keep the
// local cache consistent without waiting for the next full sync.
func (db *DB) SetThreadResolved(ctx context.Context, prID, threadID string, resolved bool) error {
	rowsAffected, err := storedb.New(db).SetReviewThreadResolved(ctx, storedb.SetReviewThreadResolvedParams{
		Resolved: boolToInt(resolved),
		PrID:     prID,
		ID:       threadID,
	})
	if err != nil {
		return fmt.Errorf("set thread resolved (%s/%s, %v): %w", prID, threadID, resolved, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("set thread resolved: thread %s not found in pr %s", threadID, prID)
	}
	return nil
}

// ReplyTargetCommentIDForThread returns a replyable review-comment ID for a
// thread scoped by PR. Prefer the root review comment because GitHub threads are
// anchored by the original review comment; fall back to the oldest available
// comment if cached reply metadata is incomplete.
func (db *DB) ReplyTargetCommentIDForThread(ctx context.Context, prID, threadID string) (string, error) {
	id, err := storedb.New(db).GetReplyTargetCommentIDForThread(ctx, storedb.GetReplyTargetCommentIDForThreadParams{
		PrID:     prID,
		ThreadID: sql.NullString{String: threadID, Valid: true},
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reply target comment for thread (%s, %s): %w", prID, threadID, err)
	}
	return id, nil
}

func reviewThreadFromListRow(row storedb.ListReviewThreadsRow) ReviewThreadRow {
	return ReviewThreadRow{
		ID:         row.ID,
		PRID:       row.PrID,
		FilePath:   row.FilePath,
		Line:       int(row.Line),
		Resolved:   row.Resolved != 0,
		Outdated:   row.Outdated != 0,
		StateKnown: row.StateKnown != 0,
	}
}

func reviewThreadFromGetRow(row storedb.GetReviewThreadRow) ReviewThreadRow {
	return ReviewThreadRow{
		ID:         row.ID,
		PRID:       row.PrID,
		FilePath:   row.FilePath,
		Line:       int(row.Line),
		Resolved:   row.Resolved != 0,
		Outdated:   row.Outdated != 0,
		StateKnown: row.StateKnown != 0,
	}
}
