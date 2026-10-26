package actionqueue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/itterno/bistro/internal/store"
	"sync"
	"time"

	storedb "github.com/itterno/bistro/internal/store/db"
)

// SQLite's strftime default uses RFC3339; older actions used a space separator.
const timeLayout = "2006-01-02 15:04:05"

// ErrActionNotFound indicates an action ID is missing from the queue.
var ErrActionNotFound = errors.New("action not found")

// parseCreatedAt tries RFC3339 first, then the legacy space-separated format.
func parseCreatedAt(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse(timeLayout, s)
}

// Action represents an offline action waiting to be replayed against the provider.
type Action struct {
	ID              string    `json:"id"`
	ActionType      string    `json:"action_type"` // approve, request_changes, post_comment
	PRID            string    `json:"pr_id"`
	Payload         string    `json:"payload"`
	AccountProvider string    `json:"account_provider"`
	AccountLogin    string    `json:"account_login"`
	CreatedAt       time.Time `json:"created_at"`
	RetryCount      int       `json:"retry_count"`
	LastError       string    `json:"last_error"`
	Status          string    `json:"status"` // pending, processing, completed, failed
}

// Queue provides persistence for offline actions using SQLite.
type Queue struct {
	queries *storedb.Queries
}

var replayMu sync.Mutex

// New creates a new Queue backed by the given database.
func New(db *sql.DB) *Queue {
	return &Queue{queries: storedb.New(db)}
}

// LockReplay serializes offline action replay in this Bistro process. The Go
// sync engine is the sole SQLite writer, so a process-wide queue lock prevents
// separate orchestrator instances from resetting each other's in-flight rows
// while a non-idempotent GitHub mutation is running.
func (q *Queue) LockReplay() {
	replayMu.Lock()
}

// UnlockReplay releases a replay lock acquired by LockReplay.
func (q *Queue) UnlockReplay() {
	replayMu.Unlock()
}

// Enqueue inserts a new action into the queue with status 'pending'.
func (q *Queue) Enqueue(ctx context.Context, action *Action) error {
	if action.ID == "" {
		return fmt.Errorf("action ID cannot be empty")
	}
	if action.ActionType == "" {
		return fmt.Errorf("action_type cannot be empty")
	}
	if action.PRID == "" {
		return fmt.Errorf("pr_id cannot be empty")
	}

	if action.AccountProvider == "" {
		action.AccountProvider = store.LocalReviewAccountProvider
	}

	if err := q.queries.UpsertQueuedAction(ctx, storedb.UpsertQueuedActionParams{
		ID:              action.ID,
		ActionType:      action.ActionType,
		PrID:            action.PRID,
		Payload:         action.Payload,
		AccountProvider: action.AccountProvider,
		AccountLogin:    action.AccountLogin,
	}); err != nil {
		return fmt.Errorf("enqueue action %s: %w", action.ID, err)
	}
	return nil
}

// GetByID retrieves a single action by its ID, returning ErrActionNotFound if absent.
func (q *Queue) GetByID(ctx context.Context, id string) (*Action, error) {
	row, err := q.queries.GetQueuedActionByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get action %q: %w", id, ErrActionNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get action %s: %w", id, err)
	}

	action, err := queuedActionFromRow(queuedActionDBRow(row))
	if err != nil {
		return nil, fmt.Errorf("parse created_at for action %s: %w", row.ID, err)
	}
	return action, nil
}

// GetPending returns up to limit pending actions with retry_count below MaxRetryCount, ordered by created_at.
func (q *Queue) GetPending(ctx context.Context, limit int) ([]*Action, error) {
	rows, err := q.queries.ListPendingQueuedActions(ctx, storedb.ListPendingQueuedActionsParams{
		RetryCount: sql.NullInt64{Int64: MaxRetryCount, Valid: true},
		Limit:      int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("get pending actions: %w", err)
	}

	actions := make([]*Action, 0, len(rows))
	for _, row := range rows {
		action, err := queuedActionFromRow(queuedActionDBRow(row))
		if err != nil {
			return nil, fmt.Errorf("parse created_at for action %s: %w", row.ID, err)
		}
		actions = append(actions, action)
	}

	return actions, nil
}

// GetPendingForAccount returns pending actions owned by the authenticated account.
// Unowned legacy rows are intentionally excluded; migration 022 backfills them
// from current_user when possible, and excluding unknown ownership prevents stale
// queued mutations from replaying under a different GitHub login.
func (q *Queue) GetPendingForAccount(ctx context.Context, providerName, accountLogin string, limit int) ([]*Action, error) {
	if providerName == "" {
		providerName = store.LocalReviewAccountProvider
	}
	rows, err := q.queries.ListPendingQueuedActionsForAccount(ctx, storedb.ListPendingQueuedActionsForAccountParams{
		RetryCount:      sql.NullInt64{Int64: MaxRetryCount, Valid: true},
		AccountProvider: providerName,
		AccountLogin:    accountLogin,
		Limit:           int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("get pending actions for account: %w", err)
	}

	actions := make([]*Action, 0, len(rows))
	for _, row := range rows {
		action, err := queuedActionFromRow(queuedActionDBRow(row))
		if err != nil {
			return nil, fmt.Errorf("parse created_at for action %s: %w", row.ID, err)
		}
		actions = append(actions, action)
	}

	return actions, nil
}

// ResetProcessing moves any crash-orphaned in-flight actions back to pending so
// replay can reconcile their GitHub-synced state before deciding whether to
// execute again. The action_queue row is local-only retry metadata; GitHub
// remains authoritative for whether the remote mutation already happened.
func (q *Queue) ResetProcessing(ctx context.Context) (int64, error) {
	rows, err := q.queries.ResetProcessingQueuedActions(ctx)
	if err != nil {
		return 0, fmt.Errorf("reset processing actions: %w", err)
	}
	return rows, nil
}

// MarkProcessing claims a pending action for replay before the provider call.
// If Bistro crashes after GitHub succeeds but before MarkCompleted, the next
// startup resets this local-only state and reconciles against GitHub before any
// duplicate non-idempotent mutation is attempted.
func (q *Queue) MarkProcessing(ctx context.Context, id string) error {
	rows, err := q.queries.MarkQueuedActionProcessing(ctx, id)
	if err != nil {
		return fmt.Errorf("mark processing %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("action %q not found or not pending", id)
	}
	return nil
}

// MarkCompleted sets the status of an action to 'completed'.
func (q *Queue) MarkCompleted(ctx context.Context, id string) error {
	rows, err := q.queries.MarkQueuedActionCompleted(ctx, id)
	if err != nil {
		return fmt.Errorf("mark completed %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("action %q not found", id)
	}
	return nil
}

// MaxRetryCount is the maximum number of attempts before an action is
// permanently marked as failed.
const MaxRetryCount = 3

// MarkFailed increments the retry count and records the error.
// When retry_count < MaxRetryCount the action stays 'pending' so it will be
// picked up by the next replay pass.  Once retries are exhausted the status
// is set to 'failed' and the action will not be retried again.
func (q *Queue) MarkFailed(ctx context.Context, id string, errMsg string, retryCount int) error {
	status := "pending"
	if retryCount >= MaxRetryCount {
		status = "failed"
	}

	rows, err := q.queries.MarkQueuedActionFailed(ctx, storedb.MarkQueuedActionFailedParams{
		Status:     sql.NullString{String: status, Valid: true},
		LastError:  sql.NullString{String: errMsg, Valid: true},
		RetryCount: sql.NullInt64{Int64: int64(retryCount), Valid: true},
		ID:         id,
	})
	if err != nil {
		return fmt.Errorf("mark failed %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("action %q not found", id)
	}
	return nil
}

// DeleteCompleted removes completed actions older than the given duration.
// Returns the number of rows deleted.
//
// Note: the schema has no updated_at column, so created_at is used as the
// cutoff timestamp. This means the retention window is measured from when
// the action was first created, not when it completed. Actions that spend a
// long time in the queue before completing may be deleted sooner than
// expected relative to their completion time.
func (q *Queue) DeleteCompleted(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := cutoffTime(olderThan)
	rows, err := q.queries.DeleteCompletedQueuedActionsBefore(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete completed actions: %w", err)
	}
	return rows, nil
}

// DeleteFailed removes failed actions older than the given duration.
//
// Note: the schema has no updated_at column, so created_at is used as the
// cutoff timestamp. This means the retention window is measured from when
// the action was first created, not when it reached the failed state.
func (q *Queue) DeleteFailed(ctx context.Context, olderThan time.Duration) error {
	cutoff := cutoffTime(olderThan)
	if err := q.queries.DeleteFailedQueuedActionsBefore(ctx, cutoff); err != nil {
		return fmt.Errorf("delete failed actions: %w", err)
	}
	return nil
}

func cutoffTime(olderThan time.Duration) string {
	return time.Now().Add(-olderThan).UTC().Format(timeLayout)
}

// All three sqlc queries select the same columns in the same order; named
// conversion avoids duplicating the mapping without changing generated code.
// Keep these fields in sync with the action-queue SELECT statements.
type queuedActionDBRow storedb.GetQueuedActionByIDRow

func queuedActionFromRow(row queuedActionDBRow) (*Action, error) {
	createdAt, err := parseCreatedAt(row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &Action{
		ID:              row.ID,
		ActionType:      row.ActionType,
		PRID:            row.PrID,
		Payload:         row.Payload,
		AccountProvider: row.AccountProvider,
		AccountLogin:    row.AccountLogin,
		CreatedAt:       createdAt,
		RetryCount:      int(row.RetryCount.Int64),
		LastError:       row.LastError,
		Status:          row.Status.String,
	}, nil
}
