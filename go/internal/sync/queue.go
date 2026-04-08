package sync

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// QueuedAction represents an offline action waiting to be replayed against the provider.
type QueuedAction struct {
	ID         string    `json:"id"`
	ActionType string    `json:"action_type"` // approve, request_changes, post_comment
	PRID       string    `json:"pr_id"`
	Payload    string    `json:"payload"`
	CreatedAt  time.Time `json:"created_at"`
	RetryCount int       `json:"retry_count"`
	LastError  string    `json:"last_error"`
	Status     string    `json:"status"` // pending, processing, completed, failed
}

// ActionQueue provides persistence for offline actions using SQLite.
type ActionQueue struct {
	db *sql.DB
}

// NewActionQueue creates a new ActionQueue backed by the given database.
func NewActionQueue(db *sql.DB) *ActionQueue {
	return &ActionQueue{db: db}
}

// Enqueue inserts a new action into the queue with status 'pending'.
func (q *ActionQueue) Enqueue(ctx context.Context, action *QueuedAction) error {
	if action.ID == "" {
		return fmt.Errorf("action ID cannot be empty")
	}
	if action.ActionType == "" {
		return fmt.Errorf("action_type cannot be empty")
	}
	if action.PRID == "" {
		return fmt.Errorf("pr_id cannot be empty")
	}

	const query = `
		INSERT INTO action_queue (id, action_type, pr_id, payload, status, retry_count)
		VALUES (?, ?, ?, ?, 'pending', 0)
	`
	_, err := q.db.ExecContext(ctx, query, action.ID, action.ActionType, action.PRID, action.Payload)
	if err != nil {
		return fmt.Errorf("enqueue action %s: %w", action.ID, err)
	}
	return nil
}

// GetByID retrieves a single action by its ID. Returns an error if not found.
func (q *ActionQueue) GetByID(ctx context.Context, id string) (*QueuedAction, error) {
	const query = `
		SELECT id, action_type, pr_id, payload, created_at, retry_count,
		       COALESCE(last_error, ''), status
		FROM action_queue
		WHERE id = ?
	`
	row := q.db.QueryRowContext(ctx, query, id)

	var a QueuedAction
	var createdAt string
	err := row.Scan(
		&a.ID,
		&a.ActionType,
		&a.PRID,
		&a.Payload,
		&createdAt,
		&a.RetryCount,
		&a.LastError,
		&a.Status,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("action %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get action %s: %w", id, err)
	}

	a.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return &a, nil
}

// GetPending returns up to limit pending actions with retry_count < 3, ordered by created_at.
func (q *ActionQueue) GetPending(ctx context.Context, limit int) ([]*QueuedAction, error) {
	const query = `
		SELECT id, action_type, pr_id, payload, created_at, retry_count,
		       COALESCE(last_error, ''), status
		FROM action_queue
		WHERE status = 'pending' AND retry_count < 3
		ORDER BY created_at ASC
		LIMIT ?
	`
	rows, err := q.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("get pending actions: %w", err)
	}
	defer rows.Close()

	var actions []*QueuedAction
	for rows.Next() {
		var a QueuedAction
		var createdAt string
		if err := rows.Scan(
			&a.ID,
			&a.ActionType,
			&a.PRID,
			&a.Payload,
			&createdAt,
			&a.RetryCount,
			&a.LastError,
			&a.Status,
		); err != nil {
			return nil, fmt.Errorf("scan pending action: %w", err)
		}
		a.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		actions = append(actions, &a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending actions: %w", err)
	}

	return actions, nil
}

// MarkCompleted sets the status of an action to 'completed'.
func (q *ActionQueue) MarkCompleted(ctx context.Context, id string) error {
	const query = `UPDATE action_queue SET status = 'completed' WHERE id = ?`
	result, err := q.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("mark completed %s: %w", id, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark completed rows affected %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("action %q not found", id)
	}
	return nil
}

// MarkFailed sets the status of an action to 'failed' and records the error and retry count.
func (q *ActionQueue) MarkFailed(ctx context.Context, id string, errMsg string, retryCount int) error {
	const query = `
		UPDATE action_queue
		SET status = 'failed', last_error = ?, retry_count = ?
		WHERE id = ?
	`
	result, err := q.db.ExecContext(ctx, query, errMsg, retryCount, id)
	if err != nil {
		return fmt.Errorf("mark failed %s: %w", id, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark failed rows affected %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("action %q not found", id)
	}
	return nil
}

// DeleteCompleted removes completed actions older than the given duration.
// Returns the number of rows deleted.
func (q *ActionQueue) DeleteCompleted(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	const query = `DELETE FROM action_queue WHERE status = 'completed' AND created_at <= ?`
	result, err := q.db.ExecContext(ctx, query, cutoff.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, fmt.Errorf("delete completed actions: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete completed rows affected: %w", err)
	}
	return n, nil
}

// DeleteFailed removes failed actions older than the given duration.
func (q *ActionQueue) DeleteFailed(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	const query = `DELETE FROM action_queue WHERE status = 'failed' AND created_at <= ?`
	_, err := q.db.ExecContext(ctx, query, cutoff.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return fmt.Errorf("delete failed actions: %w", err)
	}
	return nil
}
