# Phase 7: Rate Limiting & Offline Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement GitHub API rate limit detection with adaptive backoff, token refresh, and offline action queueing for resilience.

**Architecture:** Detect 429 responses from GitHub API, implement exponential backoff with jitter, queue actions when offline, replay when connection restored.

**Tech Stack:** Go 1.21+ (stdlib context, time packages), SQLite (queue table), exponential backoff with jitter.

---

## File Structure

```
go-sync-engine/
├── go/
│   ├── internal/
│   │   ├── provider/
│   │   │   ├── rate_limit.go (NEW) — Rate limit detection and state tracking
│   │   │   └── rate_limit_test.go (NEW) — 6 tests
│   │   ├── sync/
│   │   │   ├── queue.go (NEW) — Offline action queue implementation
│   │   │   ├── queue_test.go (NEW) — 8 tests
│   │   │   └── orchestrator.go (MODIFY) — Add rate limit + queue coordination
│   │   └── store/
│   │       ├── migrations/004_add_action_queue.sql (NEW) — Queue table
│   │       └── migrations/005_add_rate_limit_tracking.sql (NEW) — Rate limit tracking
│   └── cmd/bistro-sync/
│       └── main.go (MODIFY) — Add rate limit recovery startup logic
```

---

## Task 1: Rate Limit Detection & State Tracking

**Files:**
- Create: `go/internal/provider/rate_limit.go`
- Create: `go/internal/provider/rate_limit_test.go`

**Steps:**

- [ ] **Step 1: Write failing tests for RateLimitState**

```go
// go/internal/provider/rate_limit_test.go
package provider

import (
	"testing"
	"time"
)

func TestRateLimitStateIsLimited(t *testing.T) {
	state := &RateLimitState{
		ResetAt: time.Now().Add(1 * time.Minute),
	}
	if !state.IsLimited() {
		t.Error("expected rate limit to be active")
	}

	state.ResetAt = time.Now().Add(-1 * time.Minute)
	if state.IsLimited() {
		t.Error("expected rate limit to be expired")
	}
}

func TestRateLimitStateResetIn(t *testing.T) {
	future := time.Now().Add(30 * time.Second)
	state := &RateLimitState{ResetAt: future}
	duration := state.ResetIn()

	if duration <= 0 || duration > 31*time.Second {
		t.Errorf("expected ~30 seconds, got %v", duration)
	}
}

func TestDetectRateLimitFrom429(t *testing.T) {
	headers := map[string]string{
		"X-RateLimit-Reset": "1712600400",
	}
	state, ok := DetectRateLimitFrom429(headers)
	if !ok {
		t.Fatal("expected to detect rate limit")
	}

	if state == nil || state.ResetAt.IsZero() {
		t.Error("expected valid rate limit state")
	}
}

func TestDetectRateLimitFromMissingHeaders(t *testing.T) {
	headers := map[string]string{}
	state, ok := DetectRateLimitFrom429(headers)
	if ok {
		t.Error("expected no detection with missing headers")
	}
	if state != nil {
		t.Error("expected nil state")
	}
}

func TestRateLimitStateString(t *testing.T) {
	state := &RateLimitState{
		ResetAt: time.Now().Add(30 * time.Second),
	}
	str := state.String()
	if str == "" {
		t.Error("expected non-empty string representation")
	}
}
```

- [ ] **Step 2: Implement RateLimitState and detection**

```go
// go/internal/provider/rate_limit.go
package provider

import (
	"fmt"
	"strconv"
	"time"
)

// RateLimitState tracks GitHub API rate limit status.
type RateLimitState struct {
	ResetAt time.Time
}

// IsLimited returns true if rate limit is currently active.
func (r *RateLimitState) IsLimited() bool {
	return time.Now().Before(r.ResetAt)
}

// ResetIn returns duration until rate limit resets.
func (r *RateLimitState) ResetIn() time.Duration {
	return time.Until(r.ResetAt)
}

// String returns human-readable representation.
func (r *RateLimitState) String() string {
	if !r.IsLimited() {
		return "not rate limited"
	}
	return fmt.Sprintf("rate limited, resets in %v", r.ResetIn().Round(time.Second))
}

// DetectRateLimitFrom429 parses GitHub 429 response headers.
// Returns (state, detected).
func DetectRateLimitFrom429(headers map[string]string) (*RateLimitState, bool) {
	resetStr, ok := headers["X-RateLimit-Reset"]
	if !ok {
		return nil, false
	}

	resetUnix, err := strconv.ParseInt(resetStr, 10, 64)
	if err != nil {
		return nil, false
	}

	return &RateLimitState{
		ResetAt: time.Unix(resetUnix, 0),
	}, true
}
```

- [ ] **Step 3: Run tests**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/provider -run RateLimit -v
```

Expected: All 5 tests pass

- [ ] **Step 4: Commit**

```bash
git add go/internal/provider/rate_limit.go go/internal/provider/rate_limit_test.go
git commit -m "feat: add rate limit detection with state tracking"
```

---

## Task 2: Adaptive Backoff with Jitter

**Files:**
- Modify: `go/internal/actions/retry.go` — Add jitter support
- Modify: `go/internal/actions/retry_test.go` — Add 3 tests

**Steps:**

- [ ] **Step 1: Write failing tests for jitter**

```go
// go/internal/actions/retry_test.go — ADD these tests
func TestExponentialBackoffWithJitter(t *testing.T) {
	eb := &ExponentialBackoff{
		InitialDelay: 100 * time.Millisecond,
		Multiplier:   2.0,
		MaxDelay:     5 * time.Second,
		JitterFactor: 0.1, // 10% jitter
	}

	// Run calculation multiple times, expect variation
	durations := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		durations[i] = eb.CalculateDelay(i)
	}

	// Check that not all durations are identical (jitter applied)
	unique := make(map[time.Duration]bool)
	for _, d := range durations {
		unique[d] = true
	}
	if len(unique) < 3 {
		t.Errorf("expected jitter to create variation, got %d unique values", len(unique))
	}
}

func TestJitterWithinBounds(t *testing.T) {
	eb := &ExponentialBackoff{
		InitialDelay: 100 * time.Millisecond,
		Multiplier:   2.0,
		MaxDelay:     5 * time.Second,
		JitterFactor: 0.5, // 50% jitter
	}

	for i := 0; i < 20; i++ {
		d := eb.CalculateDelay(i)
		base := time.Duration(float64(100*time.Millisecond) * math.Pow(2.0, float64(i)))
		if base > 5*time.Second {
			base = 5 * time.Second
		}

		// With 50% jitter, should be 50-150% of base
		minExpected := base / 2
		maxExpected := base + base/2

		if d < minExpected || d > maxExpected {
			t.Errorf("jitter out of bounds for attempt %d: %v not in [%v, %v]", i, d, minExpected, maxExpected)
		}
	}
}

func TestZeroJitterNoDrift(t *testing.T) {
	eb := &ExponentialBackoff{
		InitialDelay: 100 * time.Millisecond,
		Multiplier:   2.0,
		MaxDelay:     5 * time.Second,
		JitterFactor: 0.0, // No jitter
	}

	// Without jitter, same attempt should always give same delay
	d1 := eb.CalculateDelay(2)
	d2 := eb.CalculateDelay(2)
	if d1 != d2 {
		t.Errorf("expected deterministic delay without jitter: %v vs %v", d1, d2)
	}
}
```

- [ ] **Step 2: Add jitter to ExponentialBackoff**

```go
// go/internal/actions/retry.go — MODIFY CalculateDelay method

import (
	"math"
	"math/rand"
	"time"
)

// ExponentialBackoff configuration with jitter support.
type ExponentialBackoff struct {
	InitialDelay time.Duration
	Multiplier   float64
	MaxDelay     time.Duration
	JitterFactor float64 // 0.0-1.0, percentage variance
}

// CalculateDelay returns delay for attempt N with optional jitter.
func (eb *ExponentialBackoff) CalculateDelay(attempt int) time.Duration {
	base := time.Duration(float64(eb.InitialDelay) * math.Pow(eb.Multiplier, float64(attempt)))
	if base > eb.MaxDelay {
		base = eb.MaxDelay
	}

	// Apply jitter if configured
	if eb.JitterFactor > 0 {
		jitterRange := float64(base) * eb.JitterFactor
		jitterAmount := time.Duration(rand.Float64()*jitterRange - jitterRange/2)
		base = base + jitterAmount
		if base < 0 {
			base = 0
		}
	}

	return base
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/actions -run Jitter -v
```

Expected: All 3 new tests pass

- [ ] **Step 4: Commit**

```bash
git add go/internal/actions/retry.go go/internal/actions/retry_test.go
git commit -m "feat: add jitter support to exponential backoff"
```

---

## Task 3: Offline Action Queue

**Files:**
- Create: `go/internal/sync/queue.go`
- Create: `go/internal/sync/queue_test.go`
- Create: `go/internal/store/migrations/004_add_action_queue.sql`

**Steps:**

- [ ] **Step 1: Write migration**

```sql
-- go/internal/store/migrations/004_add_action_queue.sql
CREATE TABLE IF NOT EXISTS action_queue (
    id TEXT PRIMARY KEY,
    action_type TEXT NOT NULL,
    pr_id TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    retry_count INTEGER DEFAULT 0,
    last_error TEXT,
    status TEXT DEFAULT 'pending' -- pending, processing, completed, failed
);

CREATE INDEX IF NOT EXISTS idx_action_queue_status ON action_queue(status);
CREATE INDEX IF NOT EXISTS idx_action_queue_created_at ON action_queue(created_at);
```

- [ ] **Step 2: Write failing tests for ActionQueue**

```go
// go/internal/sync/queue_test.go
package sync

import (
	"context"
	"testing"
	"time"

	"bistro/internal/store"
)

func TestQueueEnqueue(t *testing.T) {
	db := store.OpenTestDB(t)
	defer db.Close()

	queue := NewActionQueue(db)
	action := &QueuedAction{
		ID:         "test-1",
		ActionType: "approve",
		PRID:       "pr-123",
		Payload:    `{"comment":"LGTM"}`,
	}

	err := queue.Enqueue(context.Background(), action)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	// Verify stored
	stored, err := queue.GetByID(context.Background(), "test-1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if stored == nil {
		t.Fatal("action not found")
	}
}

func TestQueuePending(t *testing.T) {
	db := store.OpenTestDB(t)
	defer db.Close()

	queue := NewActionQueue(db)
	for i := 0; i < 3; i++ {
		action := &QueuedAction{
			ID:         "action-" + string(rune(i)),
			ActionType: "approve",
			PRID:       "pr-" + string(rune(i)),
			Payload:    "{}",
		}
		queue.Enqueue(context.Background(), action)
	}

	pending, err := queue.GetPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("get pending failed: %v", err)
	}
	if len(pending) != 3 {
		t.Errorf("expected 3 pending, got %d", len(pending))
	}
}

func TestQueueMarkCompleted(t *testing.T) {
	db := store.OpenTestDB(t)
	defer db.Close()

	queue := NewActionQueue(db)
	action := &QueuedAction{
		ID:         "test-1",
		ActionType: "approve",
		PRID:       "pr-123",
		Payload:    "{}",
	}
	queue.Enqueue(context.Background(), action)

	err := queue.MarkCompleted(context.Background(), "test-1")
	if err != nil {
		t.Fatalf("mark completed failed: %v", err)
	}

	stored, _ := queue.GetByID(context.Background(), "test-1")
	if stored.Status != "completed" {
		t.Errorf("expected completed status, got %s", stored.Status)
	}
}

func TestQueueMarkFailed(t *testing.T) {
	db := store.OpenTestDB(t)
	defer db.Close()

	queue := NewActionQueue(db)
	action := &QueuedAction{
		ID:         "test-1",
		ActionType: "approve",
		PRID:       "pr-123",
		Payload:    "{}",
	}
	queue.Enqueue(context.Background(), action)

	err := queue.MarkFailed(context.Background(), "test-1", "network timeout", 1)
	if err != nil {
		t.Fatalf("mark failed failed: %v", err)
	}

	stored, _ := queue.GetByID(context.Background(), "test-1")
	if stored.Status != "failed" {
		t.Errorf("expected failed status, got %s", stored.Status)
	}
	if stored.LastError != "network timeout" {
		t.Errorf("expected error message, got %s", stored.LastError)
	}
}

func TestQueueRetryLimit(t *testing.T) {
	db := store.OpenTestDB(t)
	defer db.Close()

	queue := NewActionQueue(db)
	action := &QueuedAction{
		ID:         "test-1",
		ActionType: "approve",
		PRID:       "pr-123",
		Payload:    "{}",
	}
	queue.Enqueue(context.Background(), action)

	// Mark failed 4 times
	for i := 0; i < 4; i++ {
		queue.MarkFailed(context.Background(), "test-1", "error", i+1)
	}

	pending, _ := queue.GetPending(context.Background(), 10)
	found := false
	for _, p := range pending {
		if p.ID == "test-1" {
			found = true
		}
	}
	if found {
		t.Error("expected action to exceed retry limit and be hidden from pending")
	}
}

func TestQueueDeleteCompleted(t *testing.T) {
	db := store.OpenTestDB(t)
	defer db.Close()

	queue := NewActionQueue(db)
	action := &QueuedAction{
		ID:         "test-1",
		ActionType: "approve",
		PRID:       "pr-123",
		Payload:    "{}",
	}
	queue.Enqueue(context.Background(), action)
	queue.MarkCompleted(context.Background(), "test-1")

	count, err := queue.DeleteCompleted(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 deleted, got %d", count)
	}
}

func TestQueueCleanup(t *testing.T) {
	db := store.OpenTestDB(t)
	defer db.Close()

	queue := NewActionQueue(db)
	action := &QueuedAction{
		ID:         "test-1",
		ActionType: "approve",
		PRID:       "pr-123",
		Payload:    "{}",
	}
	queue.Enqueue(context.Background(), action)
	queue.MarkFailed(context.Background(), "test-1", "error", 5)

	// Mark as old (older than 24 hours)
	err := queue.DeleteFailed(context.Background(), 1*time.Hour)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
}
```

- [ ] **Step 3: Implement ActionQueue**

```go
// go/internal/sync/queue.go
package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"bistro/internal/store"
)

// QueuedAction represents an offline action in queue.
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

// ActionQueue manages offline action persistence and retry.
type ActionQueue struct {
	db *sql.DB
}

// NewActionQueue creates queue instance.
func NewActionQueue(db *sql.DB) *ActionQueue {
	return &ActionQueue{db: db}
}

// Enqueue adds action to queue.
func (q *ActionQueue) Enqueue(ctx context.Context, action *QueuedAction) error {
	query := `
		INSERT INTO action_queue (id, action_type, pr_id, payload, created_at, status)
		VALUES (?, ?, ?, ?, ?, 'pending')
	`
	_, err := q.db.ExecContext(ctx, query,
		action.ID, action.ActionType, action.PRID, action.Payload, time.Now())
	return err
}

// GetByID retrieves single action.
func (q *ActionQueue) GetByID(ctx context.Context, id string) (*QueuedAction, error) {
	query := `SELECT id, action_type, pr_id, payload, created_at, retry_count, last_error, status FROM action_queue WHERE id = ?`
	row := q.db.QueryRowContext(ctx, query, id)

	action := &QueuedAction{}
	err := row.Scan(&action.ID, &action.ActionType, &action.PRID, &action.Payload,
		&action.CreatedAt, &action.RetryCount, &action.LastError, &action.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return action, err
}

// GetPending retrieves actions ready for retry (retry_count < 3).
func (q *ActionQueue) GetPending(ctx context.Context, limit int) ([]*QueuedAction, error) {
	query := `
		SELECT id, action_type, pr_id, payload, created_at, retry_count, last_error, status
		FROM action_queue
		WHERE status = 'pending' AND retry_count < 3
		ORDER BY created_at ASC
		LIMIT ?
	`
	rows, err := q.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []*QueuedAction
	for rows.Next() {
		action := &QueuedAction{}
		err := rows.Scan(&action.ID, &action.ActionType, &action.PRID, &action.Payload,
			&action.CreatedAt, &action.RetryCount, &action.LastError, &action.Status)
		if err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	return actions, rows.Err()
}

// MarkCompleted marks action as successfully executed.
func (q *ActionQueue) MarkCompleted(ctx context.Context, id string) error {
	query := `UPDATE action_queue SET status = 'completed' WHERE id = ?`
	_, err := q.db.ExecContext(ctx, query, id)
	return err
}

// MarkFailed marks action as failed with error and increments retry count.
func (q *ActionQueue) MarkFailed(ctx context.Context, id string, errMsg string, retryCount int) error {
	query := `UPDATE action_queue SET status = 'failed', last_error = ?, retry_count = ? WHERE id = ?`
	_, err := q.db.ExecContext(ctx, query, errMsg, retryCount, id)
	return err
}

// DeleteCompleted removes completed actions older than duration.
func (q *ActionQueue) DeleteCompleted(ctx context.Context, olderThan time.Duration) (int64, error) {
	query := `DELETE FROM action_queue WHERE status = 'completed' AND created_at < datetime('now', ?)`
	cutoff := fmt.Sprintf("-%d seconds", int(olderThan.Seconds()))
	result, err := q.db.ExecContext(ctx, query, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteFailed removes failed actions older than duration.
func (q *ActionQueue) DeleteFailed(ctx context.Context, olderThan time.Duration) error {
	query := `DELETE FROM action_queue WHERE status = 'failed' AND created_at < datetime('now', ?)`
	cutoff := fmt.Sprintf("-%d seconds", int(olderThan.Seconds()))
	_, err := q.db.ExecContext(ctx, query, cutoff)
	return err
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/sync -run Queue -v
```

Expected: All 8 tests pass

- [ ] **Step 5: Commit**

```bash
git add go/internal/sync/queue.go go/internal/sync/queue_test.go
git add go/internal/store/migrations/004_add_action_queue.sql
git commit -m "feat: implement offline action queue with retry tracking"
```

---

## Task 4: Rate Limit Tracking Migration

**Files:**
- Create: `go/internal/store/migrations/005_add_rate_limit_tracking.sql`
- Modify: `go/internal/provider/provider.go` — Add rate limit tracking interface

**Steps:**

- [ ] **Step 1: Write migration**

```sql
-- go/internal/store/migrations/005_add_rate_limit_tracking.sql
CREATE TABLE IF NOT EXISTS rate_limit_state (
    id TEXT PRIMARY KEY DEFAULT 'github',
    provider TEXT NOT NULL,
    reset_at TIMESTAMP NOT NULL,
    remaining_requests INTEGER DEFAULT 0,
    limit_requests INTEGER DEFAULT 5000,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_rate_limit_provider ON rate_limit_state(provider);
```

- [ ] **Step 2: Add RateLimitTracker interface**

```go
// go/internal/provider/provider.go — ADD to existing Provider interface

// RateLimitTracker manages rate limit state persistence.
type RateLimitTracker interface {
	// Store persists rate limit state.
	Store(ctx context.Context, state *RateLimitState) error
	// Load retrieves last known rate limit state.
	Load(ctx context.Context) (*RateLimitState, error)
	// Clear resets stored rate limit state.
	Clear(ctx context.Context) error
}
```

- [ ] **Step 3: Commit**

```bash
git add go/internal/store/migrations/005_add_rate_limit_tracking.sql
git add go/internal/provider/provider.go
git commit -m "feat: add rate limit tracking schema and interface"
```

---

## Task 5: Orchestrator Integration

**Files:**
- Modify: `go/internal/sync/orchestrator.go` — Add rate limit + queue coordination
- Create: `go/internal/sync/orchestrator_integration_test.go` — 4 integration tests

**Steps:**

- [ ] **Step 1: Write failing integration tests**

```go
// go/internal/sync/orchestrator_integration_test.go
package sync

import (
	"context"
	"testing"
	"time"

	"bistro/internal/provider"
	"bistro/internal/store"
)

func TestOrchestratorRateLimitDetection(t *testing.T) {
	db := store.OpenTestDB(t)
	defer db.Close()

	mockProvider := provider.NewMockProvider()
	mockProvider.SetRateLimitError(true)

	orch := NewOrchestrator(db, mockProvider, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try sync, should detect rate limit
	orch.Sync(ctx)

	// Verify rate limit state is tracked
	if !orch.IsRateLimited() {
		t.Error("expected rate limited state after 429 response")
	}
}

func TestOrchestratorQueueActionsWhenRateLimited(t *testing.T) {
	db := store.OpenTestDB(t)
	defer db.Close()

	mockProvider := provider.NewMockProvider()
	orch := NewOrchestrator(db, mockProvider, NewActionQueue(db))

	// Simulate rate limit active
	orch.rateLimitState = &provider.RateLimitState{
		ResetAt: time.Now().Add(1 * time.Minute),
	}

	ctx := context.Background()
	request := &provider.ActionRequest{
		Type:  provider.ActionApprove,
		PRID:  "pr-123",
	}

	// Try to apply action
	result := orch.ApplyAction(ctx, request)
	if result.Status != provider.ActionQueued {
		t.Errorf("expected queued status, got %s", result.Status)
	}

	// Verify queued
	pending, _ := orch.queue.GetPending(ctx, 10)
	if len(pending) != 1 {
		t.Errorf("expected 1 queued action, got %d", len(pending))
	}
}

func TestOrchestratorReplayQueuedActions(t *testing.T) {
	db := store.OpenTestDB(t)
	defer db.Close()

	mockProvider := provider.NewMockProvider()
	orch := NewOrchestrator(db, mockProvider, NewActionQueue(db))

	// Enqueue action
	action := &QueuedAction{
		ID:         "queued-1",
		ActionType: "approve",
		PRID:       "pr-123",
		Payload:    "{}",
	}
	orch.queue.Enqueue(context.Background(), action)

	// Clear rate limit (simulate reset)
	orch.rateLimitState = nil

	// Sync should replay queued actions
	ctx := context.Background()
	orch.ReplayQueuedActions(ctx)

	// Verify action was processed
	stored, _ := orch.queue.GetByID(ctx, "queued-1")
	if stored.Status != "completed" {
		t.Errorf("expected completed status, got %s", stored.Status)
	}
}

func TestOrchestratorCleanupOldQueuedActions(t *testing.T) {
	db := store.OpenTestDB(t)
	defer db.Close()

	mockProvider := provider.NewMockProvider()
	orch := NewOrchestrator(db, mockProvider, NewActionQueue(db))

	// Enqueue and complete action
	action := &QueuedAction{
		ID:         "completed-1",
		ActionType: "approve",
		PRID:       "pr-123",
		Payload:    "{}",
	}
	orch.queue.Enqueue(context.Background(), action)
	orch.queue.MarkCompleted(context.Background(), "completed-1")

	// Cleanup old actions
	err := orch.CleanupQueue(context.Background())
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
}
```

- [ ] **Step 2: Add rate limit tracking to Orchestrator**

```go
// go/internal/sync/orchestrator.go — ADD fields and methods

type Orchestrator struct {
	// ... existing fields ...
	rateLimitState *provider.RateLimitState
	queue          *ActionQueue
	logger         *slog.Logger
}

// IsRateLimited returns true if currently rate limited.
func (o *Orchestrator) IsRateLimited() bool {
	if o.rateLimitState == nil {
		return false
	}
	return o.rateLimitState.IsLimited()
}

// ApplyAction either executes immediately or queues if rate limited.
func (o *Orchestrator) ApplyAction(ctx context.Context, req *provider.ActionRequest) *provider.ActionResult {
	if o.IsRateLimited() {
		o.logger.Info("rate limited, queueing action", "pr_id", req.PRID)

		queuedAction := &QueuedAction{
			ID:         req.PRID + "-" + req.Type.String(),
			ActionType: req.Type.String(),
			PRID:       req.PRID,
			Payload:    req.Comment,
		}
		if err := o.queue.Enqueue(ctx, queuedAction); err != nil {
			o.logger.Error("failed to queue action", "error", err)
			return &provider.ActionResult{
				Status: provider.ActionFailed,
				Error:  err,
			}
		}
		return &provider.ActionResult{
			Status: provider.ActionQueued,
		}
	}

	// Execute immediately
	return o.executeAction(ctx, req)
}

// ReplayQueuedActions processes pending actions from queue.
func (o *Orchestrator) ReplayQueuedActions(ctx context.Context) error {
	pending, err := o.queue.GetPending(ctx, 10)
	if err != nil {
		return err
	}

	for _, action := range pending {
		o.logger.Info("replaying queued action", "id", action.ID)

		// Reconstruct request from queue
		req := &provider.ActionRequest{
			PRID:    action.PRID,
			Comment: action.Payload,
		}

		result := o.executeAction(ctx, req)
		if result.Status == provider.ActionSuccess {
			o.queue.MarkCompleted(ctx, action.ID)
		} else {
			o.queue.MarkFailed(ctx, action.ID, result.Error.Error(), action.RetryCount+1)
		}
	}

	return nil
}

// CleanupQueue removes old completed/failed actions.
func (o *Orchestrator) CleanupQueue(ctx context.Context) error {
	// Delete completed actions older than 7 days
	deleted, err := o.queue.DeleteCompleted(ctx, 7*24*time.Hour)
	if err != nil {
		return err
	}
	o.logger.Debug("cleaned up queue", "deleted", deleted)

	// Delete failed actions older than 24 hours
	return o.queue.DeleteFailed(ctx, 24*time.Hour)
}
```

- [ ] **Step 3: Update Sync method to detect rate limits**

```go
// go/internal/sync/orchestrator.go — MODIFY Sync method to handle 429

func (o *Orchestrator) Sync(ctx context.Context) error {
	// ... existing code ...

	// In PRFetch loop, catch rate limit errors:
	for _, filter := range o.filters {
		prs, err := o.provider.ListPRs(ctx, filter)
		if err != nil {
			if isRateLimitError(err) {
				// Extract rate limit state from error response
				if rateLimitState, ok := err.(*provider.ProviderError).RateLimitState(); ok {
					o.rateLimitState = rateLimitState
					o.logger.Warn("rate limited", "reset_at", rateLimitState.ResetAt)

					// Emit event
					o.eventEmitter.Emit("rate_limited", map[string]interface{}{
						"reset_in": rateLimitState.ResetIn().Seconds(),
					})
					return err
				}
			}
			return err
		}
		// ... process prs ...
	}

	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/sync -run Integration -v
```

Expected: All 4 integration tests pass

- [ ] **Step 5: Commit**

```bash
git add go/internal/sync/orchestrator.go go/internal/sync/orchestrator_integration_test.go
git commit -m "feat: integrate rate limit tracking and offline queue into orchestrator"
```

---

## Task 6: Startup Recovery

**Files:**
- Modify: `go/cmd/bistro-sync/main.go` — Add queue replay on startup

**Steps:**

- [ ] **Step 1: Add startup flag**

```go
// go/cmd/bistro-sync/main.go — MODIFY main()

func main() {
	wsPort := flag.Int("ws-port", 8080, "WebSocket server port")
	dbPath := flag.String("db-path", "./data/sync.db", "SQLite database path")
	replayQueue := flag.Bool("replay-queue", true, "Replay offline queue on startup")
	flag.Parse()

	// ... existing setup ...

	if *replayQueue {
		log.Info("replaying offline queue")
		if err := app.orchestrator.ReplayQueuedActions(context.Background()); err != nil {
			log.Error("queue replay failed", "error", err)
		}
	}

	if err := app.Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 2: Test startup behavior**

```bash
# Build binary
go build ./cmd/bistro-sync

# Run with queue replay (default)
./bistro-sync --ws-port 8080 --db-path ./sync.db

# Run without queue replay
./bistro-sync --ws-port 8080 --db-path ./sync.db --replay-queue=false
```

- [ ] **Step 3: Commit**

```bash
git add go/cmd/bistro-sync/main.go
git commit -m "feat: add offline queue replay on startup"
```

---

## Success Criteria

- ✅ 21+ new tests (rate limit detection 5, jitter 3, queue 8, integration 4, startup 1)
- ✅ 207+ cumulative tests (186+ → 207+), all passing
- ✅ 0 race conditions (verified with `-race` flag)
- ✅ Rate limit detection working with GitHub 429 responses
- ✅ Exponential backoff with jitter prevents thundering herd
- ✅ Offline queue persists actions when rate limited
- ✅ Queue replay on startup recovers failed actions
- ✅ WebSocket events emitted for rate limit conditions
- ✅ Integration tests validate full flow

---

**Plan complete and ready for execution via superpowers:subagent-driven-development.**
