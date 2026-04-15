package sync

import (
	"context"
	"testing"
	"time"

	"github.com/samuelasvanyi/bistro/internal/actions"
	"github.com/samuelasvanyi/bistro/internal/events"
	"github.com/samuelasvanyi/bistro/internal/provider"
	"github.com/samuelasvanyi/bistro/internal/provider/mock"
	"github.com/samuelasvanyi/bistro/internal/store"
)

// TestOrchestratorRateLimitDetection verifies that a 429 ProviderError from
// ListPRs causes the orchestrator to record a rate-limit state and emit a
// "rate_limited" event.
func TestOrchestratorRateLimitDetection(t *testing.T) {
	rateLimitErr := provider.NewRateLimitError("API rate limit exceeded")
	mockProv := mock.NewMockProvider().WithError(rateLimitErr)

	orch := NewOrchestrator(nil, mockProv, nil, nil)
	eventCh := orch.GetEventChannel()

	req := &SyncRequest{
		Provider:   "github",
		Owner:      "org",
		Repo:       "repo",
		IncludePRs: true,
		State:      "open",
	}

	// Collect events in the background until we see rate_limited or timeout.
	received := make(chan *events.Event, 32)
	collectorDone := make(chan struct{})
	go func() {
		defer close(collectorDone)
		timeout := time.After(2 * time.Second)
		for {
			select {
			case ev, ok := <-eventCh:
				if !ok {
					return
				}
				received <- ev
				if ev.Type == "rate_limited" {
					return
				}
			case <-timeout:
				return
			}
		}
	}()

	_, _ = orch.Sync(context.Background(), req)
	<-collectorDone
	close(received)

	// After sync the orchestrator must consider itself rate-limited.
	if !orch.IsRateLimited() {
		t.Error("expected orchestrator to be rate limited after 429 response")
	}

	// A "rate_limited" event must have been emitted.
	found := false
	for ev := range received {
		if ev.Type == "rate_limited" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected rate_limited event to be emitted")
	}
}

// TestOrchestratorQueueActionsWhenRateLimited verifies that ApplyAction
// enqueues the action (not execute it) when the orchestrator is rate limited.
func TestOrchestratorQueueActionsWhenRateLimited(t *testing.T) {
	storeDB := openTestDB(t)

	mockProv := mock.NewMockProvider()
	orch := NewOrchestrator(nil, mockProv, nil, nil)

	// Force rate-limited state (resets in 10 minutes).
	orch.rateLimitState = &provider.RateLimitState{
		ResetAt: time.Now().Add(10 * time.Minute),
	}
	orch.queue = store.NewActionQueue(storeDB.DB)

	req := &actions.ActionRequest{
		PRID:   "github:org/repo:1",
		Action: actions.ApproveCommand,
	}

	result := orch.ApplyAction(context.Background(), req)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Action should NOT have succeeded — it was queued, not executed.
	if result.Success {
		t.Error("expected action to be not-successful (queued) when rate limited")
	}

	// Verify the action is persisted in the queue as pending.
	pending, err := orch.queue.GetPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending queued action, got %d", len(pending))
	}
	if pending[0].PRID != req.PRID {
		t.Errorf("queued PRID mismatch: got %q, want %q", pending[0].PRID, req.PRID)
	}
	if pending[0].ActionType != string(req.Action) {
		t.Errorf("queued ActionType mismatch: got %q, want %q", pending[0].ActionType, req.Action)
	}
}

// TestOrchestratorReplayQueuedActions verifies that ReplayQueuedActions
// processes all pending items and transitions them out of 'pending' status.
func TestOrchestratorReplayQueuedActions(t *testing.T) {
	storeDB := openTestDB(t)

	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:org/repo:1", Number: 1, Title: "Test PR"},
	})
	orch := NewOrchestrator(nil, mockProv, nil, nil)
	orch.queue = store.NewActionQueue(storeDB.DB)

	// Pre-populate one pending action.
	action := &store.QueuedAction{
		ID:         "github:org/repo:1-approve",
		ActionType: string(actions.ApproveCommand),
		PRID:       "github:org/repo:1",
		Payload:    "",
	}
	if err := orch.queue.Enqueue(context.Background(), action); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Rate limit is clear — replay should execute the action.
	if err := orch.ReplayQueuedActions(context.Background()); err != nil {
		t.Fatalf("ReplayQueuedActions: %v", err)
	}

	// After replay the action must no longer be pending.
	pending, err := orch.queue.GetPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPending after replay: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending actions after replay, got %d", len(pending))
	}
}

// TestOrchestratorCleanupOldQueuedActions verifies that CleanupQueue deletes
// completed actions older than 7 days and failed actions older than 24 hours,
// while leaving recent pending actions intact.
func TestOrchestratorCleanupOldQueuedActions(t *testing.T) {
	storeDB := openTestDB(t)

	orch := NewOrchestrator(nil, mock.NewMockProvider(), nil, nil)
	orch.queue = store.NewActionQueue(storeDB.DB)

	ctx := context.Background()
	db := storeDB.DB

	// Insert a completed action created 8 days ago.
	oldCompleted := time.Now().Add(-8 * 24 * time.Hour).UTC().Format("2006-01-02 15:04:05")
	if _, err := db.ExecContext(ctx,
		`INSERT INTO action_queue (id, action_type, pr_id, payload, status, retry_count, created_at)
		 VALUES (?, ?, ?, ?, 'completed', 0, ?)`,
		"old-completed-1", "approve", "github:org/repo:1", "", oldCompleted,
	); err != nil {
		t.Fatalf("insert old completed action: %v", err)
	}

	// Insert a failed action created 25 hours ago.
	oldFailed := time.Now().Add(-25 * time.Hour).UTC().Format("2006-01-02 15:04:05")
	if _, err := db.ExecContext(ctx,
		`INSERT INTO action_queue (id, action_type, pr_id, payload, status, retry_count, created_at)
		 VALUES (?, ?, ?, ?, 'failed', 3, ?)`,
		"old-failed-1", "approve", "github:org/repo:2", "", oldFailed,
	); err != nil {
		t.Fatalf("insert old failed action: %v", err)
	}

	// Insert a recent pending action — must NOT be deleted.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO action_queue (id, action_type, pr_id, payload, status, retry_count)
		 VALUES (?, ?, ?, ?, 'pending', 0)`,
		"recent-pending-1", "approve", "github:org/repo:3", "",
	); err != nil {
		t.Fatalf("insert recent pending action: %v", err)
	}

	if err := orch.CleanupQueue(ctx); err != nil {
		t.Fatalf("CleanupQueue: %v", err)
	}

	// Old completed and old failed rows must be gone.
	var staleCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM action_queue WHERE id IN ('old-completed-1', 'old-failed-1')`,
	).Scan(&staleCount); err != nil {
		t.Fatalf("count stale rows: %v", err)
	}
	if staleCount != 0 {
		t.Errorf("expected stale actions to be deleted, %d still remain", staleCount)
	}

	// Recent pending action must still be present.
	var pendingCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM action_queue WHERE id = 'recent-pending-1'`,
	).Scan(&pendingCount); err != nil {
		t.Fatalf("count recent pending: %v", err)
	}
	if pendingCount != 1 {
		t.Errorf("expected recent pending action to remain, got count %d", pendingCount)
	}
}
