package sync

import (
	"context"
	"testing"
	"time"
)

// TestQueueEnqueue verifies that a new action can be enqueued and retrieved by ID.
func TestQueueEnqueue(t *testing.T) {
	db := openTestDB(t)
	q := NewActionQueue(db.DB)
	ctx := context.Background()

	action := &QueuedAction{
		ID:         "action-1",
		ActionType: "approve",
		PRID:       "github:org/repo:1",
		Payload:    `{"comment":"LGTM"}`,
	}

	if err := q.Enqueue(ctx, action); err != nil {
		t.Fatalf("Enqueue error: %v", err)
	}

	got, err := q.GetByID(ctx, "action-1")
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if got.ID != action.ID {
		t.Errorf("expected ID %q, got %q", action.ID, got.ID)
	}
	if got.ActionType != action.ActionType {
		t.Errorf("expected ActionType %q, got %q", action.ActionType, got.ActionType)
	}
	if got.PRID != action.PRID {
		t.Errorf("expected PRID %q, got %q", action.PRID, got.PRID)
	}
	if got.Status != "pending" {
		t.Errorf("expected status 'pending', got %q", got.Status)
	}
	if got.RetryCount != 0 {
		t.Errorf("expected RetryCount 0, got %d", got.RetryCount)
	}
}

// TestQueuePending verifies that GetPending returns only pending actions with retry_count < 3.
func TestQueuePending(t *testing.T) {
	db := openTestDB(t)
	q := NewActionQueue(db.DB)
	ctx := context.Background()

	actions := []*QueuedAction{
		{ID: "a1", ActionType: "approve", PRID: "pr-1", Payload: "{}"},
		{ID: "a2", ActionType: "comment", PRID: "pr-2", Payload: "{}"},
		{ID: "a3", ActionType: "approve", PRID: "pr-3", Payload: "{}"},
	}
	for _, a := range actions {
		if err := q.Enqueue(ctx, a); err != nil {
			t.Fatalf("Enqueue error: %v", err)
		}
	}

	pending, err := q.GetPending(ctx, 10)
	if err != nil {
		t.Fatalf("GetPending error: %v", err)
	}
	if len(pending) != 3 {
		t.Errorf("expected 3 pending actions, got %d", len(pending))
	}
}

// TestQueueMarkCompleted verifies that MarkCompleted sets the status to 'completed'.
func TestQueueMarkCompleted(t *testing.T) {
	db := openTestDB(t)
	q := NewActionQueue(db.DB)
	ctx := context.Background()

	action := &QueuedAction{
		ID:         "action-complete",
		ActionType: "approve",
		PRID:       "pr-1",
		Payload:    "{}",
	}
	if err := q.Enqueue(ctx, action); err != nil {
		t.Fatalf("Enqueue error: %v", err)
	}

	if err := q.MarkCompleted(ctx, "action-complete"); err != nil {
		t.Fatalf("MarkCompleted error: %v", err)
	}

	got, err := q.GetByID(ctx, "action-complete")
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if got.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", got.Status)
	}
}

// TestQueueMarkFailed verifies that MarkFailed keeps the action pending while
// retries remain, and only moves it to 'failed' once retries are exhausted.
func TestQueueMarkFailed(t *testing.T) {
	db := openTestDB(t)
	q := NewActionQueue(db.DB)
	ctx := context.Background()

	enqueue := func(id string) {
		t.Helper()
		if err := q.Enqueue(ctx, &QueuedAction{
			ID:         id,
			ActionType: "request_changes",
			PRID:       "pr-2",
			Payload:    "{}",
		}); err != nil {
			t.Fatalf("Enqueue error: %v", err)
		}
	}

	// retryCount < MaxRetryCount → action stays pending for another attempt.
	enqueue("action-retry")
	if err := q.MarkFailed(ctx, "action-retry", "network timeout", 1); err != nil {
		t.Fatalf("MarkFailed error: %v", err)
	}
	got, err := q.GetByID(ctx, "action-retry")
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if got.Status != "pending" {
		t.Errorf("expected status 'pending' when retries remain, got %q", got.Status)
	}
	if got.LastError != "network timeout" {
		t.Errorf("expected LastError 'network timeout', got %q", got.LastError)
	}
	if got.RetryCount != 1 {
		t.Errorf("expected RetryCount 1, got %d", got.RetryCount)
	}

	// retryCount >= MaxRetryCount → action is permanently failed.
	enqueue("action-exhausted")
	if err := q.MarkFailed(ctx, "action-exhausted", "too many failures", MaxRetryCount); err != nil {
		t.Fatalf("MarkFailed error: %v", err)
	}
	got, err = q.GetByID(ctx, "action-exhausted")
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if got.Status != "failed" {
		t.Errorf("expected status 'failed' when retries exhausted, got %q", got.Status)
	}
}

// TestQueueRetryLimit verifies that actions with retry_count >= 3 are excluded from GetPending.
func TestQueueRetryLimit(t *testing.T) {
	db := openTestDB(t)
	q := NewActionQueue(db.DB)
	ctx := context.Background()

	// Enqueue two actions; fail one past the retry limit.
	eligible := &QueuedAction{ID: "eligible", ActionType: "approve", PRID: "pr-1", Payload: "{}"}
	exhausted := &QueuedAction{ID: "exhausted", ActionType: "approve", PRID: "pr-2", Payload: "{}"}

	for _, a := range []*QueuedAction{eligible, exhausted} {
		if err := q.Enqueue(ctx, a); err != nil {
			t.Fatalf("Enqueue error: %v", err)
		}
	}

	// Mark exhausted as failed with retry_count = 3 (at limit).
	if err := q.MarkFailed(ctx, "exhausted", "too many failures", 3); err != nil {
		t.Fatalf("MarkFailed error: %v", err)
	}

	pending, err := q.GetPending(ctx, 10)
	if err != nil {
		t.Fatalf("GetPending error: %v", err)
	}

	for _, p := range pending {
		if p.ID == "exhausted" {
			t.Error("exhausted action (retry_count=3) should not appear in GetPending")
		}
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 eligible pending action, got %d", len(pending))
	}
}

// TestQueueDeleteCompleted verifies that DeleteCompleted removes old completed actions.
func TestQueueDeleteCompleted(t *testing.T) {
	db := openTestDB(t)
	q := NewActionQueue(db.DB)
	ctx := context.Background()

	action := &QueuedAction{
		ID:         "old-completed",
		ActionType: "approve",
		PRID:       "pr-1",
		Payload:    "{}",
	}
	if err := q.Enqueue(ctx, action); err != nil {
		t.Fatalf("Enqueue error: %v", err)
	}
	if err := q.MarkCompleted(ctx, "old-completed"); err != nil {
		t.Fatalf("MarkCompleted error: %v", err)
	}

	// Pass 0 duration so everything older than now is deleted.
	rowsDeleted, err := q.DeleteCompleted(ctx, 0)
	if err != nil {
		t.Fatalf("DeleteCompleted error: %v", err)
	}
	if rowsDeleted < 1 {
		t.Errorf("expected at least 1 deleted row, got %d", rowsDeleted)
	}

	_, err = q.GetByID(ctx, "old-completed")
	if err == nil {
		t.Error("expected error after deletion, but GetByID succeeded")
	}
}

// TestQueueCleanup verifies that DeleteFailed removes old failed actions.
func TestQueueCleanup(t *testing.T) {
	db := openTestDB(t)
	q := NewActionQueue(db.DB)
	ctx := context.Background()

	action := &QueuedAction{
		ID:         "old-failed",
		ActionType: "post_comment",
		PRID:       "pr-3",
		Payload:    "{}",
	}
	if err := q.Enqueue(ctx, action); err != nil {
		t.Fatalf("Enqueue error: %v", err)
	}
	if err := q.MarkFailed(ctx, "old-failed", "API error", 3); err != nil {
		t.Fatalf("MarkFailed error: %v", err)
	}

	// Pass 0 duration so everything older than now is deleted.
	if err := q.DeleteFailed(ctx, 0); err != nil {
		t.Fatalf("DeleteFailed error: %v", err)
	}

	_, err := q.GetByID(ctx, "old-failed")
	if err == nil {
		t.Error("expected error after deletion, but GetByID succeeded")
	}
}

// TestQueueGetByIDNotFound verifies that GetByID returns an error for unknown IDs.
func TestQueueGetByIDNotFound(t *testing.T) {
	db := openTestDB(t)
	q := NewActionQueue(db.DB)
	ctx := context.Background()

	_, err := q.GetByID(ctx, "nonexistent-id")
	if err == nil {
		t.Error("expected error for nonexistent ID, got nil")
	}
}

// TestQueuePendingLimit verifies that GetPending respects the limit parameter.
func TestQueuePendingLimit(t *testing.T) {
	db := openTestDB(t)
	q := NewActionQueue(db.DB)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		a := &QueuedAction{
			ID:         "action-limit-" + time.Now().Format("15:04:05.000000000") + string(rune('0'+i)),
			ActionType: "approve",
			PRID:       "pr-limit",
			Payload:    "{}",
		}
		if err := q.Enqueue(ctx, a); err != nil {
			t.Fatalf("Enqueue error: %v", err)
		}
	}

	pending, err := q.GetPending(ctx, 2)
	if err != nil {
		t.Fatalf("GetPending error: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("expected 2 results with limit=2, got %d", len(pending))
	}
}
