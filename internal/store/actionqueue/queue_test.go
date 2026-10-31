package actionqueue

import (
	"context"
	"github.com/itterno/bistro/internal/store/storetest"
	"testing"
	"time"
)

// TestQueueEnqueue verifies that a new action can be enqueued and retrieved by ID.
func TestQueueEnqueue(t *testing.T) {
	db := storetest.Open(t)
	q := New(db.DB)
	ctx := context.Background()

	action := &Action{
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
	db := storetest.Open(t)
	q := New(db.DB)
	ctx := context.Background()

	actions := []*Action{
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
	db := storetest.Open(t)
	q := New(db.DB)
	ctx := context.Background()

	action := &Action{
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
	db := storetest.Open(t)
	q := New(db.DB)
	ctx := context.Background()

	enqueue := func(id string) {
		t.Helper()
		if err := q.Enqueue(ctx, &Action{
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
	db := storetest.Open(t)
	q := New(db.DB)
	ctx := context.Background()

	// Enqueue two actions; fail one past the retry limit.
	eligible := &Action{ID: "eligible", ActionType: "approve", PRID: "pr-1", Payload: "{}"}
	exhausted := &Action{ID: "exhausted", ActionType: "approve", PRID: "pr-2", Payload: "{}"}

	for _, a := range []*Action{eligible, exhausted} {
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
	db := storetest.Open(t)
	q := New(db.DB)
	ctx := context.Background()

	action := &Action{
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
	db := storetest.Open(t)
	q := New(db.DB)
	ctx := context.Background()

	action := &Action{
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

// TestEnqueueIdempotent verifies that re-enqueuing the same ID replaces the prior row,
// resets status to 'pending' and retry_count to 0, and updates the payload.
func TestEnqueueIdempotent(t *testing.T) {
	db := storetest.Open(t)
	q := New(db.DB)
	ctx := context.Background()

	first := &Action{
		ID:         "pr-42-approve",
		ActionType: "approve",
		PRID:       "pr-42",
		Payload:    `{"comment":"first attempt"}`,
	}
	if err := q.Enqueue(ctx, first); err != nil {
		t.Fatalf("first Enqueue error: %v", err)
	}

	// Simulate a failed attempt so the row has a non-zero retry_count and last_error.
	if err := q.MarkFailed(ctx, first.ID, "rate limited", 1); err != nil {
		t.Fatalf("MarkFailed error: %v", err)
	}

	// Re-enqueue with a different payload — must succeed and reset the row.
	second := &Action{
		ID:         "pr-42-approve",
		ActionType: "approve",
		PRID:       "pr-42",
		Payload:    `{"comment":"second attempt"}`,
	}
	if err := q.Enqueue(ctx, second); err != nil {
		t.Fatalf("second Enqueue (re-enqueue) error: %v", err)
	}

	// Exactly one row for this ID.
	var count int
	if err := db.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM action_queue WHERE id = ?`, first.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count query error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 row, got %d", count)
	}

	got, err := q.GetByID(ctx, first.ID)
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if got.Status != "pending" {
		t.Errorf("expected status 'pending' after re-enqueue, got %q", got.Status)
	}
	if got.RetryCount != 0 {
		t.Errorf("expected retry_count 0 after re-enqueue, got %d", got.RetryCount)
	}
	if got.Payload != second.Payload {
		t.Errorf("expected payload %q after re-enqueue, got %q", second.Payload, got.Payload)
	}
}

// TestQueueGetByIDNotFound verifies that GetByID returns an error for unknown IDs.
func TestQueueGetByIDNotFound(t *testing.T) {
	db := storetest.Open(t)
	q := New(db.DB)
	ctx := context.Background()

	_, err := q.GetByID(ctx, "nonexistent-id")
	if err == nil {
		t.Error("expected error for nonexistent ID, got nil")
	}
}

// TestQueuePendingLimit verifies that GetPending respects the limit parameter.
func TestQueuePendingLimit(t *testing.T) {
	db := storetest.Open(t)
	q := New(db.DB)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		a := &Action{
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

func TestQueueGetPendingForAccountIsolation(t *testing.T) {
	db := storetest.Open(t)
	q := New(db.DB)
	ctx := context.Background()

	for _, action := range []*Action{
		{ID: "alice-action", ActionType: "approve", PRID: "pr-1", Payload: "{}", AccountProvider: "github", AccountLogin: "alice"},
		{ID: "bob-action", ActionType: "approve", PRID: "pr-1", Payload: "{}", AccountProvider: "github", AccountLogin: "bob"},
		{ID: "legacy-unowned", ActionType: "approve", PRID: "pr-1", Payload: "{}", AccountProvider: "github"},
	} {
		if err := q.Enqueue(ctx, action); err != nil {
			t.Fatalf("Enqueue %s: %v", action.ID, err)
		}
	}

	pending, err := q.GetPendingForAccount(ctx, "github", "alice", 10)
	if err != nil {
		t.Fatalf("GetPendingForAccount: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "alice-action" {
		t.Fatalf("alice pending = %+v, want only alice-action", pending)
	}
}

func TestQueueProcessingLifecycle(t *testing.T) {
	db := storetest.Open(t)
	q := New(db.DB)
	ctx := context.Background()

	const actionID = "processing-action"
	if err := q.Enqueue(ctx, &Action{ID: actionID, ActionType: "post_comment", PRID: "pr-1", Payload: "body"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := q.MarkProcessing(ctx, actionID); err != nil {
		t.Fatalf("MarkProcessing: %v", err)
	}

	pending, err := q.GetPending(ctx, 10)
	if err != nil {
		t.Fatalf("GetPending: %v", err)
	}
	for _, action := range pending {
		if action.ID == actionID {
			t.Fatal("processing action must be hidden from pending replay until startup reset")
		}
	}

	reset, err := q.ResetProcessing(ctx)
	if err != nil {
		t.Fatalf("ResetProcessing: %v", err)
	}
	if reset != 1 {
		t.Fatalf("ResetProcessing reset %d rows, want 1", reset)
	}

	got, err := q.GetByID(ctx, actionID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Status != "pending" {
		t.Fatalf("status = %q, want pending", got.Status)
	}
}
