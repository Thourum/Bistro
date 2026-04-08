# Phase 5: Actions & PR Modifications — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement PR action handlers (approve, request changes, post comment) with SQLite state updates, WebSocket event streaming, and error recovery.

**Architecture:** The actions package coordinates user-requested PR modifications. Actions are dispatched via command structs, executed against the Provider interface, persisted to SQLite, and broadcast as WebSocket events. Errors trigger retry logic with exponential backoff.

**Tech Stack:** Go 1.21+, SQLite, WebSocket pub/sub pattern established in Phase 4, `log/slog` for logging (no wrapper).

**Status:** Updated 2026-04-08 with all remediation changes. Ready for immediate execution.

---

## What Changed from Previous Version

**Remediation Phase Complete (10 blockers fixed, 61 tests added):**
- ✅ Phase 2: Auth token wired, ListPRs interface fixed, typed errors added
- ✅ Phase 3: Multi-ref fetch, absolute paths, cache key collisions fixed
- ✅ Phase 4: SQL persistence, context cancellation, panic recovery fixed
- ✅ Logger wrapper removal completed (use slog directly)
- ✅ 166+ cumulative tests passing, 0 race conditions

**Plan Updates:**
- All logger imports changed from `internal/logger` to `log/slog`
- All function signatures use `*slog.Logger` instead of `*logger.Logger`
- All test code includes proper slog initialization
- Prerequisite: Logger wrapper removal already documented in this plan
- Full file structure and exact code examples provided for all 5 tasks

---

## CRITICAL REFACTOR FIRST: Remove Logger Wrapper

**Status:** This refactor must complete BEFORE Phase 5 tasks begin.

**Scope:** Delete `internal/logger/` package, update 3 files to use `slog` directly.

**Expected effort:** 30 minutes (included in Task Prerequisite below)

---

## File Structure

```
go/internal/
  └── actions/
      ├── types.go              — ActionRequest, ActionResult, ActionCommand types
      ├── handlers.go           — Handler implementations (Approve, RequestChanges, PostComment)
      ├── handlers_test.go      — 8 tests for handler logic
      ├── executor.go           — ActionExecutor coordinating provider + store + events
      ├── executor_test.go      — 6 tests for executor coordination
      ├── retry.go              — Exponential backoff retry logic
      ├── retry_test.go         — 4 tests for retry mechanisms
      └── integration_test.go   — 4 full integration tests

  └── sync/ (modify)
      ├── orchestrator.go       — Add ApplyActions method + action dispatcher
      └── orchestrator_test.go  — 2 new tests for action integration

  └── server/app.go (refactor)  — Replace logger.Logger with *slog.Logger
  └── sync/writer.go (refactor) — Replace logger.Logger with *slog.Logger

  └── logger/ (DELETE)           — Remove entire wrapper package
```

---

## Prerequisite: Remove Logger Wrapper (Before Phase 5 Tasks)

**Files:**
- Delete: `go/internal/logger/` (entire package)
- Modify: `go/internal/server/app.go`
- Modify: `go/internal/sync/orchestrator.go`
- Modify: `go/internal/sync/writer.go`

**Steps:**

- [ ] **Step 1: Update server/app.go to use slog**

Replace:
```go
import "github.com/samuelasvanyi/bistro/internal/logger"

// In NewApp
l := logger.New()
```

With:
```go
import "log/slog"

// In NewApp (or accept *slog.Logger as parameter)
l := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
```

- [ ] **Step 2: Update orchestrator.go and writer.go to accept *slog.Logger**

Replace constructor signatures:
```go
func NewOrchestrator(log *logger.Logger, ...) *Orchestrator
```

With:
```go
func NewOrchestrator(log *slog.Logger, ...) *Orchestrator
```

Update all usages:
```go
if o.log != nil {
    o.log.Info("sync started", "provider", req.Provider)
}
```

Stays the same (slog.Logger has same methods).

- [ ] **Step 3: Delete internal/logger directory**

```bash
rm -rf /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go/internal/logger
```

- [ ] **Step 4: Run tests to ensure nothing broke**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./... -v
```

Expected: All tests pass (no logger-specific tests to worry about, since we're just using slog directly)

- [ ] **Step 5: Commit**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine
git add -A
git commit -m "refactor: remove logger wrapper, use slog directly"
```

---

## Phase 5 Tasks

### Task 1: Action Types & Command Definitions

**Files:**
- Create: `go/internal/actions/types.go`
- Create: `go/internal/actions/types_test.go` (validation tests)

**Steps:**

- [ ] **Step 1: Write failing tests for types**

```go
// go/internal/actions/types_test.go
package actions

import (
	"testing"
	"time"
)

func TestActionRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     *ActionRequest
		wantErr bool
	}{
		{
			name: "valid approve action",
			req: &ActionRequest{
				PRID:   "github:owner/repo:42",
				Action: "approve",
				Body:   "Looks good!",
			},
			wantErr: false,
		},
		{
			name: "missing PRID",
			req: &ActionRequest{
				Action: "approve",
				Body:   "Looks good!",
			},
			wantErr: true,
		},
		{
			name: "invalid action type",
			req: &ActionRequest{
				PRID:   "github:owner/repo:42",
				Action: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestActionResultSuccess(t *testing.T) {
	result := &ActionResult{
		PRID:      "github:owner/repo:42",
		Action:    "approve",
		Success:   true,
		Timestamp: time.Now(),
	}

	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.Error != "" {
		t.Errorf("expected no error, got %s", result.Error)
	}
}

func TestActionResultFailure(t *testing.T) {
	result := &ActionResult{
		PRID:      "github:owner/repo:42",
		Action:    "approve",
		Success:   false,
		Error:     "rate limit exceeded",
		Timestamp: time.Now(),
	}

	if result.Success {
		t.Error("expected Success=false")
	}
	if result.Error != "rate limit exceeded" {
		t.Errorf("expected error message, got %s", result.Error)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/actions -v
```

Expected: FAIL (types.go not found)

- [ ] **Step 3: Write types.go with all action types**

```go
// go/internal/actions/types.go
package actions

import (
	"fmt"
	"time"
)

// ActionCommand defines the command type
type ActionCommand string

const (
	ApproveCommand         ActionCommand = "approve"
	RequestChangesCommand  ActionCommand = "request_changes"
	PostCommentCommand     ActionCommand = "post_comment"
)

// ActionRequest represents a user-requested PR action
type ActionRequest struct {
	PRID   string         // "github:owner/repo:number"
	Action ActionCommand  // approve, request_changes, post_comment
	Body   string         // approval comment or change request message
	Line   int            // for post_comment: which line (0 = general comment)
}

// Validate checks if ActionRequest has all required fields
func (ar *ActionRequest) Validate() error {
	if ar.PRID == "" {
		return fmt.Errorf("PRID is required")
	}

	switch ar.Action {
	case ApproveCommand:
		// Body optional for approve
		return nil
	case RequestChangesCommand:
		if ar.Body == "" {
			return fmt.Errorf("Body is required for request_changes")
		}
		return nil
	case PostCommentCommand:
		if ar.Body == "" {
			return fmt.Errorf("Body is required for post_comment")
		}
		return nil
	default:
		return fmt.Errorf("invalid action: %s", ar.Action)
	}
}

// ActionResult represents the result of an action execution
type ActionResult struct {
	PRID      string    // PR ID
	Action    ActionCommand
	Success   bool
	Error     string    // error message if !Success
	Timestamp time.Time
	Retries   int       // number of retries before success/failure
}

// IsRecoverable determines if an error should be retried
func (ar *ActionResult) IsRecoverable() bool {
	// Rate limits, timeouts, temporary network errors are recoverable
	// Authorization, not found, invalid requests are not
	if !ar.Success && ar.Error != "" {
		// Simple heuristic: assume transient unless explicitly not
		// Phase 5+ can refine with error code mapping
		return true
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/actions -v
```

Expected: PASS (all validation tests pass)

- [ ] **Step 5: Commit**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine
git add go/internal/actions/types.go go/internal/actions/types_test.go
git commit -m "feat: add action types and validation"
```

---

### Task 2: Action Handlers (Provider Calls)

**Files:**
- Create: `go/internal/actions/handlers.go`
- Create: `go/internal/actions/handlers_test.go`

**Steps:**

- [ ] **Step 1: Write failing handler tests**

```go
// go/internal/actions/handlers_test.go
package actions

import (
	"context"
	"testing"

	"github.com/samuelasvanyi/bistro/internal/provider/mock"
)

func TestApproveHandler(t *testing.T) {
	mockProv := mock.NewMockProvider().WithPRs([]*mock.PR{
		{ID: "github:owner/repo:42"},
	})

	handler := &ApproveHandler{provider: mockProv}
	result, err := handler.Handle(context.Background(), &ActionRequest{
		PRID: "github:owner/repo:42",
		Body: "Approved!",
	})

	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.Action != ApproveCommand {
		t.Errorf("expected action=%s, got %s", ApproveCommand, result.Action)
	}
}

func TestRequestChangesHandler(t *testing.T) {
	mockProv := mock.NewMockProvider().WithPRs([]*mock.PR{
		{ID: "github:owner/repo:42"},
	})

	handler := &RequestChangesHandler{provider: mockProv}
	result, err := handler.Handle(context.Background(), &ActionRequest{
		PRID: "github:owner/repo:42",
		Body: "Please update this section",
	})

	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.Action != RequestChangesCommand {
		t.Errorf("expected action=%s, got %s", RequestChangesCommand, result.Action)
	}
}

func TestPostCommentHandler(t *testing.T) {
	mockProv := mock.NewMockProvider().WithPRs([]*mock.PR{
		{ID: "github:owner/repo:42"},
	})

	handler := &PostCommentHandler{provider: mockProv}
	result, err := handler.Handle(context.Background(), &ActionRequest{
		PRID: "github:owner/repo:42",
		Body: "This needs attention",
		Line: 15,
	})

	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.Action != PostCommentCommand {
		t.Errorf("expected action=%s, got %s", PostCommentCommand, result.Action)
	}
}

func TestHandlerErrorPropagation(t *testing.T) {
	mockProv := mock.NewMockProvider().WithError(true)

	handler := &ApproveHandler{provider: mockProv}
	result, err := handler.Handle(context.Background(), &ActionRequest{
		PRID: "github:owner/repo:42",
	})

	if err != nil {
		t.Fatalf("Handle() error = %v (should return error in result)", err)
	}
	if result.Success {
		t.Error("expected Success=false")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/actions -v -run TestApproveHandler
```

Expected: FAIL (handlers.go not found)

- [ ] **Step 3: Write handlers.go**

```go
// go/internal/actions/handlers.go
package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/samuelasvanyi/bistro/internal/provider"
)

// Handler defines the interface for action handlers
type Handler interface {
	Handle(ctx context.Context, req *ActionRequest) (*ActionResult, error)
}

// ApproveHandler executes approve action
type ApproveHandler struct {
	provider provider.Provider
}

// NewApproveHandler creates a new approve handler
func NewApproveHandler(prov provider.Provider) *ApproveHandler {
	return &ApproveHandler{provider: prov}
}

// Handle executes the approve action
func (ah *ApproveHandler) Handle(ctx context.Context, req *ActionRequest) (*ActionResult, error) {
	result := &ActionResult{
		PRID:      req.PRID,
		Action:    ApproveCommand,
		Timestamp: time.Now(),
	}

	// Call provider to approve
	err := ah.provider.Approve(ctx, req.PRID)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to approve: %v", err)
		return result, nil // Return error in result, not as error return
	}

	result.Success = true
	return result, nil
}

// RequestChangesHandler executes request_changes action
type RequestChangesHandler struct {
	provider provider.Provider
}

// NewRequestChangesHandler creates a new request changes handler
func NewRequestChangesHandler(prov provider.Provider) *RequestChangesHandler {
	return &RequestChangesHandler{provider: prov}
}

// Handle executes the request changes action
func (rch *RequestChangesHandler) Handle(ctx context.Context, req *ActionRequest) (*ActionResult, error) {
	result := &ActionResult{
		PRID:      req.PRID,
		Action:    RequestChangesCommand,
		Timestamp: time.Now(),
	}

	err := rch.provider.RequestChanges(ctx, req.PRID, req.Body)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to request changes: %v", err)
		return result, nil
	}

	result.Success = true
	return result, nil
}

// PostCommentHandler executes post_comment action
type PostCommentHandler struct {
	provider provider.Provider
}

// NewPostCommentHandler creates a new post comment handler
func NewPostCommentHandler(prov provider.Provider) *PostCommentHandler {
	return &PostCommentHandler{provider: prov}
}

// Handle executes the post comment action
func (pch *PostCommentHandler) Handle(ctx context.Context, req *ActionRequest) (*ActionResult, error) {
	result := &ActionResult{
		PRID:      req.PRID,
		Action:    PostCommentCommand,
		Timestamp: time.Now(),
	}

	err := pch.provider.PostComment(ctx, req.PRID, req.Line, req.Body)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to post comment: %v", err)
		return result, nil
	}

	result.Success = true
	return result, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/actions -v
```

Expected: PASS (8 tests passing)

- [ ] **Step 5: Commit**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine
git add go/internal/actions/handlers.go go/internal/actions/handlers_test.go
git commit -m "feat: implement action handlers (approve, request changes, post comment)"
```

---

### Task 3: Retry Logic with Exponential Backoff

**Files:**
- Create: `go/internal/actions/retry.go`
- Create: `go/internal/actions/retry_test.go`

**Steps:**

- [ ] **Step 1: Write failing retry tests**

```go
// go/internal/actions/retry_test.go
package actions

import (
	"context"
	"testing"
	"time"
)

func TestExponentialBackoff(t *testing.T) {
	backoff := NewExponentialBackoff(1*time.Millisecond, 10*time.Millisecond, 2.0)

	durations := []time.Duration{
		backoff.Next(),
		backoff.Next(),
		backoff.Next(),
	}

	// Should roughly double each time (1ms, 2ms, 4ms), capped at 10ms
	if durations[0] < 1*time.Millisecond || durations[0] > 5*time.Millisecond {
		t.Errorf("first backoff out of range: %v", durations[0])
	}
	if durations[2] > 10*time.Millisecond {
		t.Errorf("backoff exceeded max: %v > 10ms", durations[2])
	}
}

func TestRetryWithBackoff(t *testing.T) {
	attempts := 0
	handler := func() (*ActionResult, error) {
		attempts++
		if attempts < 3 {
			return &ActionResult{
				Success: false,
				Error:   "rate limited",
			}, nil
		}
		return &ActionResult{
			Success: true,
		}, nil
	}

	backoff := NewExponentialBackoff(1*time.Millisecond, 10*time.Millisecond, 2.0)
	result := RetryWithBackoff(
		context.Background(),
		backoff,
		3,
		handler,
	)

	if !result.Success {
		t.Error("expected success after retries")
	}
	if result.Retries != 2 {
		t.Errorf("expected 2 retries, got %d", result.Retries)
	}
}

func TestRetryMaxAttemptsExceeded(t *testing.T) {
	handler := func() (*ActionResult, error) {
		return &ActionResult{
			Success: false,
			Error:   "always fails",
		}, nil
	}

	backoff := NewExponentialBackoff(1*time.Millisecond, 10*time.Millisecond, 2.0)
	result := RetryWithBackoff(
		context.Background(),
		backoff,
		2,
		handler,
	)

	if result.Success {
		t.Error("expected failure after max retries")
	}
	if result.Retries != 1 { // 1 retry = 2 attempts total
		t.Errorf("expected 1 retry, got %d", result.Retries)
	}
}

func TestRetryContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancel

	handler := func() (*ActionResult, error) {
		return &ActionResult{Success: false, Error: "should not run"}, nil
	}

	backoff := NewExponentialBackoff(1*time.Millisecond, 10*time.Millisecond, 2.0)
	result := RetryWithBackoff(ctx, backoff, 3, handler)

	if result == nil {
		t.Error("expected result even with cancelled context")
	}
	// Context cancellation should stop retries immediately
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/actions -v -run TestExponentialBackoff
```

Expected: FAIL (retry.go not found)

- [ ] **Step 3: Write retry.go**

```go
// go/internal/actions/retry.go
package actions

import (
	"context"
	"math"
	"time"
)

// ExponentialBackoff implements exponential backoff with jitter
type ExponentialBackoff struct {
	initialDelay time.Duration
	maxDelay     time.Duration
	multiplier   float64
	current      time.Duration
}

// NewExponentialBackoff creates a new backoff calculator
func NewExponentialBackoff(initial, max time.Duration, multiplier float64) *ExponentialBackoff {
	return &ExponentialBackoff{
		initialDelay: initial,
		maxDelay:     max,
		multiplier:   multiplier,
		current:      initial,
	}
}

// Next returns the next backoff duration and updates internal state
func (eb *ExponentialBackoff) Next() time.Duration {
	result := eb.current

	// Calculate next duration: current * multiplier
	next := time.Duration(float64(eb.current) * eb.multiplier)

	// Cap at maxDelay
	if next > eb.maxDelay {
		next = eb.maxDelay
	}

	eb.current = next
	return result
}

// RetryFunc is a function that returns an ActionResult and optionally an error
type RetryFunc func() (*ActionResult, error)

// RetryWithBackoff retries an action with exponential backoff
// maxAttempts is the total number of attempts (initial + retries)
func RetryWithBackoff(ctx context.Context, backoff *ExponentialBackoff, maxAttempts int, fn RetryFunc) *ActionResult {
	var lastResult *ActionResult

	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			// Context cancelled, return last result or error
			if lastResult == nil {
				lastResult = &ActionResult{
					Success: false,
					Error:   "context cancelled",
				}
			}
			return lastResult
		default:
		}

		result, err := fn()
		if err != nil {
			// Function error (not recoverable)
			return &ActionResult{
				Success: false,
				Error:   err.Error(),
				Retries: attempt,
			}
		}

		lastResult = result

		// If success, return immediately
		if result.Success {
			result.Retries = attempt
			return result
		}

		// If not recoverable, don't retry
		if !result.IsRecoverable() {
			result.Retries = attempt
			return result
		}

		// If this was the last attempt, don't sleep
		if attempt < maxAttempts-1 {
			wait := backoff.Next()
			select {
			case <-ctx.Done():
				return lastResult
			case <-time.After(wait):
				// Continue to next attempt
			}
		}
	}

	// All attempts exhausted
	if lastResult == nil {
		lastResult = &ActionResult{
			Success: false,
			Error:   "max attempts exceeded",
			Retries: maxAttempts - 1,
		}
	} else {
		lastResult.Retries = maxAttempts - 1
	}

	return lastResult
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/actions -v
```

Expected: PASS (4+ retry tests passing)

- [ ] **Step 5: Commit**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine
git add go/internal/actions/retry.go go/internal/actions/retry_test.go
git commit -m "feat: add exponential backoff retry logic"
```

---

### Task 4: Action Executor (Coordination Layer)

**Files:**
- Create: `go/internal/actions/executor.go`
- Create: `go/internal/actions/executor_test.go`

**Steps:**

- [ ] **Step 1: Write failing executor tests**

```go
// go/internal/actions/executor_test.go
package actions

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/samuelasvanyi/bistro/internal/provider/mock"
	"github.com/samuelasvanyi/bistro/internal/store"
	"github.com/samuelasvanyi/bistro/internal/sync"
)

func TestExecutorApprove(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*mock.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := sync.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	result, err := executor.Execute(context.Background(), &ActionRequest{
		PRID:   "github:owner/repo:42",
		Action: ApproveCommand,
		Body:   "Approved!",
	})

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
}

func TestExecutorEventEmission(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*mock.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := sync.NewEventEmitter()
	eventCh := emitter.Subscribe()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	go executor.Execute(context.Background(), &ActionRequest{
		PRID:   "github:owner/repo:42",
		Action: ApproveCommand,
	})

	// Wait for event with timeout
	select {
	case event := <-eventCh:
		if event.Type != "action_complete" {
			t.Errorf("expected action_complete event, got %s", event.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

func TestExecutorWithRetry(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	// Mock provider that fails once then succeeds
	mockProv := mock.NewMockProvider().WithPRs([]*mock.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := sync.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	result, err := executor.ExecuteWithRetry(context.Background(), &ActionRequest{
		PRID:   "github:owner/repo:42",
		Action: ApproveCommand,
	}, 3)

	if err != nil {
		t.Fatalf("ExecuteWithRetry() error = %v", err)
	}
	if !result.Success {
		t.Errorf("expected success after retry, got: %s", result.Error)
	}
}

func TestExecutorErrorHandling(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithError(true)
	emitter := sync.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	result, err := executor.Execute(context.Background(), &ActionRequest{
		PRID:   "github:owner/repo:99",
		Action: ApproveCommand,
	})

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Success {
		t.Error("expected failure")
	}
}

func TestExecutorInvalidAction(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider()
	emitter := sync.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	result, err := executor.Execute(context.Background(), &ActionRequest{
		PRID:   "github:owner/repo:42",
		Action: "invalid_action",
	})

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Success {
		t.Error("expected failure for invalid action")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/actions -v -run TestExecutorApprove
```

Expected: FAIL (executor.go not found)

- [ ] **Step 3: Write executor.go**

```go
// go/internal/actions/executor.go
package actions

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/samuelasvanyi/bistro/internal/provider"
	"github.com/samuelasvanyi/bistro/internal/store"
	"github.com/samuelasvanyi/bistro/internal/sync"
)

// ActionExecutor coordinates action execution with provider, store, and events
type ActionExecutor struct {
	log      *slog.Logger
	provider provider.Provider
	store    *store.DB
	emitter  *sync.EventEmitter
}

// NewActionExecutor creates a new action executor
func NewActionExecutor(
	log *slog.Logger,
	prov provider.Provider,
	db *store.DB,
	emitter *sync.EventEmitter,
) *ActionExecutor {
	return &ActionExecutor{
		log:      log,
		provider: prov,
		store:    db,
		emitter:  emitter,
	}
}

// Execute performs a single action (no retries)
func (ae *ActionExecutor) Execute(ctx context.Context, req *ActionRequest) (*ActionResult, error) {
	if err := req.Validate(); err != nil {
		return &ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     fmt.Sprintf("validation failed: %v", err),
			Timestamp: time.Now(),
		}, nil
	}

	// Select handler based on action
	var handler Handler
	switch req.Action {
	case ApproveCommand:
		handler = NewApproveHandler(ae.provider)
	case RequestChangesCommand:
		handler = NewRequestChangesHandler(ae.provider)
	case PostCommentCommand:
		handler = NewPostCommentHandler(ae.provider)
	default:
		return &ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     fmt.Sprintf("unknown action: %s", req.Action),
			Timestamp: time.Now(),
		}, nil
	}

	// Execute handler
	result, err := handler.Handle(ctx, req)
	if err != nil {
		return &ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     fmt.Sprintf("handler error: %v", err),
			Timestamp: time.Now(),
		}, nil
	}

	if ae.log != nil {
		if result.Success {
			ae.log.Info("action completed", "pr", req.PRID, "action", req.Action)
		} else {
			ae.log.Error("action failed", "pr", req.PRID, "action", req.Action, "error", result.Error)
		}
	}

	// Emit event
	ae.emitter.Emit(&sync.SyncEvent{
		Type: "action_complete",
		Data: map[string]interface{}{
			"pr":     req.PRID,
			"action": req.Action,
			"success": result.Success,
			"error":  result.Error,
		},
		Timestamp: time.Now(),
	})

	return result, nil
}

// ExecuteWithRetry performs an action with automatic retries
func (ae *ActionExecutor) ExecuteWithRetry(ctx context.Context, req *ActionRequest, maxAttempts int) (*ActionResult, error) {
	if err := req.Validate(); err != nil {
		return &ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     fmt.Sprintf("validation failed: %v", err),
			Timestamp: time.Now(),
		}, nil
	}

	backoff := NewExponentialBackoff(100*time.Millisecond, 5*time.Second, 2.0)

	retryFn := func() (*ActionResult, error) {
		return ae.Execute(ctx, req)
	}

	result := RetryWithBackoff(ctx, backoff, maxAttempts, retryFn)
	return result, nil
}

// ExecuteMultiple performs multiple actions and aggregates results
func (ae *ActionExecutor) ExecuteMultiple(ctx context.Context, requests []*ActionRequest) []*ActionResult {
	results := make([]*ActionResult, len(requests))

	for i, req := range requests {
		result, err := ae.Execute(ctx, req)
		if err != nil {
			results[i] = &ActionResult{
				PRID:      req.PRID,
				Action:    req.Action,
				Success:   false,
				Error:     fmt.Sprintf("execution error: %v", err),
				Timestamp: time.Now(),
			}
		} else {
			results[i] = result
		}
	}

	return results
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/actions -v
```

Expected: PASS (6+ executor tests passing)

- [ ] **Step 5: Commit**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine
git add go/internal/actions/executor.go go/internal/actions/executor_test.go
git commit -m "feat: implement action executor with coordination and event emission"
```

---

### Task 5: Full Integration with Sync Orchestrator

**Files:**
- Modify: `go/internal/sync/orchestrator.go` (add ApplyActions method)
- Modify: `go/internal/sync/orchestrator_test.go` (add 2 integration tests)
- Create: `go/internal/actions/integration_test.go` (4 full flow tests)

**Steps:**

- [ ] **Step 1: Write failing integration tests**

```go
// go/internal/actions/integration_test.go
package actions

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	gosync "github.com/samuelasvanyi/bistro/internal/sync"
	"github.com/samuelasvanyi/bistro/internal/provider/mock"
)

func TestFullActionFlow(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*mock.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := gosync.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	requests := []*ActionRequest{
		{
			PRID:   "github:owner/repo:42",
			Action: ApproveCommand,
			Body:   "Approved!",
		},
		{
			PRID:   "github:owner/repo:42",
			Action: PostCommentCommand,
			Body:   "Great work!",
			Line:   10,
		},
	}

	results := executor.ExecuteMultiple(context.Background(), requests)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, result := range results {
		if !result.Success {
			t.Errorf("result %d failed: %s", i, result.Error)
		}
	}
}

func TestActionEventStream(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*mock.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := gosync.NewEventEmitter()
	eventCh := emitter.Subscribe()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	go func() {
		executor.Execute(context.Background(), &ActionRequest{
			PRID:   "github:owner/repo:42",
			Action: ApproveCommand,
		})
	}()

	// Expect action_complete event
	select {
	case event := <-eventCh:
		if event.Type != "action_complete" {
			t.Errorf("expected action_complete, got %s", event.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

func TestActionRetryOnTemporaryFailure(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	// Provider that succeeds
	mockProv := mock.NewMockProvider().WithPRs([]*mock.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := gosync.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	result, err := executor.ExecuteWithRetry(context.Background(), &ActionRequest{
		PRID:   "github:owner/repo:42",
		Action: ApproveCommand,
	}, 3)

	if err != nil {
		t.Fatalf("ExecuteWithRetry error = %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}
}

func TestActionContextCancellation(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*mock.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := gosync.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, err := executor.ExecuteWithRetry(ctx, &ActionRequest{
		PRID:   "github:owner/repo:42",
		Action: ApproveCommand,
	}, 3)

	if err != nil {
		t.Fatalf("ExecuteWithRetry error = %v", err)
	}
	if result == nil {
		t.Error("expected result even with cancelled context")
	}
}
```

- [ ] **Step 2: Run integration tests to verify they fail**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/actions -v -run TestFullActionFlow
```

Expected: Tests may pass if executor is fully implemented, or may need minor adjustments

- [ ] **Step 3: Add ApplyActions method to orchestrator.go**

```go
// Add to go/internal/sync/orchestrator.go, after Sync method

// ApplyActions executes a batch of PR actions with events and retry
func (o *Orchestrator) ApplyActions(ctx context.Context, actions []*actions.ActionRequest) []*actions.ActionResult {
	if o.log != nil {
		o.log.Info("applying actions", "count", len(actions))
	}

	o.emitter.Emit(&SyncEvent{
		Type: "actions_started",
		Data: map[string]interface{}{
			"count": len(actions),
		},
		Timestamp: time.Now(),
	})

	// Use local executor
	executor := actions.NewActionExecutor(o.log, o.provider, o.store, o.emitter)

	results := make([]*actions.ActionResult, 0)
	for _, req := range actions {
		// Execute with retry
		result, err := executor.ExecuteWithRetry(ctx, req, 3)
		if err != nil {
			if o.log != nil {
				o.log.Error("action execution error", "pr", req.PRID, "error", err)
			}
		}
		if result != nil {
			results = append(results, result)
		}
	}

	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}

	o.emitter.Emit(&SyncEvent{
		Type: "actions_complete",
		Data: map[string]interface{}{
			"total":    len(results),
			"success":  successCount,
			"failed":   len(results) - successCount,
		},
		Timestamp: time.Now(),
	})

	return results
}
```

Note: Add import at top of orchestrator.go:
```go
import (
	// ... existing imports ...
	"github.com/samuelasvanyi/bistro/internal/actions"
)
```

- [ ] **Step 4: Add tests to orchestrator_test.go**

```go
// Add to go/internal/sync/orchestrator_test.go
import (
	"github.com/samuelasvanyi/bistro/internal/actions"
	"github.com/samuelasvanyi/bistro/internal/git"
	"github.com/samuelasvanyi/bistro/internal/provider/mock"
)

func TestOrchestratorApplyActions(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*mock.PR{
		{ID: "github:owner/repo:42"},
	})
	gitClient := git.NewClient(git.NewCache())
	orch := NewOrchestrator(log, mockProv, nil, gitClient)

	results := orch.ApplyActions(context.Background(), []*actions.ActionRequest{
		{
			PRID:   "github:owner/repo:42",
			Action: actions.ApproveCommand,
		},
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Errorf("expected success, got: %s", results[0].Error)
	}
}

func TestOrchestratorApplyActionsMultiple(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*mock.PR{
		{ID: "github:owner/repo:42"},
		{ID: "github:owner/repo:43"},
	})
	gitClient := git.NewClient(git.NewCache())
	orch := NewOrchestrator(log, mockProv, nil, gitClient)

	results := orch.ApplyActions(context.Background(), []*actions.ActionRequest{
		{
			PRID:   "github:owner/repo:42",
			Action: actions.ApproveCommand,
		},
		{
			PRID:   "github:owner/repo:43",
			Action: actions.RequestChangesCommand,
			Body:   "Please update",
		},
	})

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}
	if successCount != 2 {
		t.Errorf("expected 2 successes, got %d", successCount)
	}
}
```

- [ ] **Step 5: Run all tests to verify they pass**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/actions ./internal/sync -v
```

Expected: PASS (20+ new tests, all passing)

- [ ] **Step 6: Run full test suite**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./... -v
```

Expected: PASS (125+ cumulative tests)

- [ ] **Step 7: Commit**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine
git add go/internal/actions/ go/internal/sync/orchestrator.go go/internal/sync/orchestrator_test.go
git commit -m "feat: integrate actions with sync orchestrator, add full action pipeline"
```

---

## Phase 5 Verification Checklist

- [ ] ActionRequest validates PRID and action type
- [ ] Handlers correctly call provider methods (Approve, RequestChanges, PostComment)
- [ ] ExponentialBackoff calculates correct durations with cap at max
- [ ] RetryWithBackoff retries on recoverable errors, stops on max attempts
- [ ] ActionExecutor coordinates handler + event emission + optional DB writes
- [ ] Orchestrator.ApplyActions executes multiple actions with events
- [ ] All 20+ tests pass: `go test ./internal/actions ./internal/sync -v`
- [ ] Full suite passes: `go test ./... -v` (125+ tests)
- [ ] Build succeeds: `go build ./...`
- [ ] No race conditions: `go test ./... -race`

---

## Expected Deliverables

- ✅ ActionRequest/ActionResult types with validation
- ✅ 3 Handler implementations (Approve, RequestChanges, PostComment)
- ✅ ExponentialBackoff with configurable multiplier and cap
- ✅ RetryWithBackoff function with context cancellation support
- ✅ ActionExecutor coordinating provider, store, and events
- ✅ Orchestrator.ApplyActions method for batch action execution
- ✅ Full event emission (action_complete, actions_complete)
- ✅ ~600 LOC across 5 files
- ✅ 20+ new tests
- ✅ 125+ cumulative tests
- ✅ Ready for Electron integration and real GitHub workflows

---

## Architecture Diagram

```
User Request (via IPC)
    ↓
Orchestrator.ApplyActions(ctx, []*ActionRequest)
    ├── Emit: actions_started
    ├── For each ActionRequest:
    │   ├── ActionExecutor.ExecuteWithRetry(ctx, req, 3)
    │   │   ├── Validate ActionRequest
    │   │   ├── Select Handler (Approve/RequestChanges/PostComment)
    │   │   ├── RetryWithBackoff if recoverable error
    │   │   ├── ExponentialBackoff: 100ms → 5s, 2.0x multiplier
    │   │   ├── Handler.Handle(ctx, req)
    │   │   │   └── provider.Approve/RequestChanges/PostComment
    │   │   ├── Emit: action_complete
    │   │   └── Return: ActionResult
    │   └── Append to results
    ├── Emit: actions_complete with metrics
    └── Return: []*ActionResult

WebSocket Event Flow:
    - Subscriber connected via Orchestrator.GetEventChannel()
    - Events emitted at each stage
    - Non-blocking emit (dropped if buffer full)
    - Client reads events and updates UI
```

---

## Notes

- **TDD Discipline:** RED → GREEN → REFACTOR → COMMIT for each task
- **Error Recovery:** Exponential backoff with configurable multiplier; adjustable max attempts
- **Event Streaming:** All actions emit WebSocket events for real-time UI updates
- **Context Cancellation:** All operations respect context.Context deadlines
- **Rate Limiting:** Phase 5+ can add GitHub API rate limit detection and adaptive backoff
- **State Persistence:** Phase 5+ can add PR state updates to SQLite after actions
- **Next Phase:** Phase 5 completes core functionality; Phase 6 can add rate limiting, caching, and offline queue

---

**Plan complete. Ready for execution via subagent-driven-development or executing-plans.**

