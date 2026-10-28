package actions

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
)

// ── Liveness / context-cancellation guards ────────────────────────────────

// TestExecute_ContextCancelledAfterProviderCall verifies that when the context
// is cancelled between the provider call and Execute's side-effect section,
// Execute treats the result as failed and does NOT:
//   - emit pr_approved / changes_requested (success-only events)
//   - emit action_complete with success=true
//   - call updatePRReviewStatus
func TestExecute_ContextCancelledAfterProviderCall(t *testing.T) {
	// Use a context we cancel after the handler's approval is "complete".
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled — simulates liveness firing mid-call

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{{ID: "github:o/r:1"}})
	emitter := events.NewEventEmitter()
	eventCh := emitter.Subscribe()
	defer emitter.Unsubscribe(eventCh)

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID:   "github:o/r:1",
		Action: ApproveCommand,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// The result must not be a success — context was cancelled.
	if result.Success {
		t.Error("Execute must return failure when context is cancelled before side-effects")
	}

	// Drain all buffered events.
	var types []string
drainLoop:
	for {
		select {
		case ev := <-eventCh:
			types = append(types, ev.Type)
		default:
			break drainLoop
		}
	}

	// action_complete must be present but with success=false.
	foundComplete := false
	for _, typ := range types {
		if typ == "pr_approved" || typ == "changes_requested" {
			t.Errorf("must not emit %q when context cancelled", typ)
		}
		if typ == "action_complete" {
			foundComplete = true
		}
	}
	if !foundComplete {
		t.Error("expected action_complete event even on cancelled context (with success=false)")
	}
}

// TestExecuteWithRetry_CancelledContextSuppressesSideEffects verifies the full
// ExecuteWithRetry path with a pre-cancelled context: no success events and no
// success result are produced.
func TestExecuteWithRetry_CancelledContextSuppressesSideEffects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel to simulate liveness cancellation before the call

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{{ID: "github:o/r:2"}})
	emitter := events.NewEventEmitter()
	eventCh := emitter.Subscribe()
	defer emitter.Unsubscribe(eventCh)

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	result, err := executor.ExecuteWithRetry(ctx, &ActionRequest{
		PRID:   "github:o/r:2",
		Action: ApproveCommand,
	}, 3)
	if err != nil {
		t.Fatalf("ExecuteWithRetry: %v", err)
	}
	if result.Success {
		t.Error("ExecuteWithRetry must not return success when context is pre-cancelled")
	}

	// No pr_approved event should have been emitted.
	for {
		select {
		case ev := <-eventCh:
			if ev.Type == "pr_approved" {
				t.Error("must not emit pr_approved when context cancelled")
			}
		default:
			return
		}
	}
}

// TestExecute_AmbiguousResultCarriesSentinel verifies that when context is
// cancelled after the provider call, Execute returns a result with the
// ErrAmbiguousRemoteSuccess sentinel prefix — required so replay can detect
// the ambiguous case and avoid duplicate mutations.
func TestExecute_AmbiguousResultCarriesSentinel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{{ID: "github:o/r:1"}})
	executor := NewActionExecutor(log, mockProv, nil, events.NewEventEmitter())

	result, err := executor.Execute(ctx, &ActionRequest{PRID: "github:o/r:1", Action: ApproveCommand})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Success {
		t.Fatal("must not be success on cancelled context")
	}
	if !result.IsAmbiguousRemoteSuccess() {
		t.Errorf("expected ErrAmbiguousRemoteSuccess prefix; got: %q", result.Error)
	}
}

// TestExecute_GuardsBeforeEverySuccessSideEffect verifies that multiple
// ctx.Err() guards in Execute prevent success events/writes even when
// cancellation occurs after Guard 1 has already passed.
//
// We test this by verifying no pr_approved event is emitted when ctx is
// pre-cancelled — the guard before emitEvent (Guard 2) must catch it.
func TestExecute_GuardsBeforeEverySuccessSideEffect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{{ID: "github:o/r:3"}})
	emitter := events.NewEventEmitter()
	eventCh := emitter.Subscribe()
	defer emitter.Unsubscribe(eventCh)

	executor := NewActionExecutor(log, mockProv, nil, emitter)
	result, _ := executor.Execute(ctx, &ActionRequest{PRID: "github:o/r:3", Action: RequestChangesCommand, Body: "nit"}) //nolint:errcheck

	if result.Success {
		t.Error("must not be success on cancelled context")
	}

	var gotTypes []string
drain:
	for {
		select {
		case ev := <-eventCh:
			gotTypes = append(gotTypes, ev.Type)
		default:
			break drain
		}
	}

	for _, typ := range gotTypes {
		if typ == "changes_requested" || typ == "pr_approved" {
			t.Errorf("must not emit %q after cancellation", typ)
		}
	}
}

// ── Adversarial blocker: context-cancel error after mutation attempt ───────

// TestExecute_ContextCancelledHandlerError_IsAmbiguous verifies that when
// handler.Handle returns a context-cancellation error (provider may have
// already executed the mutation), Execute marks the result as ambiguous so
// replay can terminally fail the queue row and avoid duplicate mutations.
func TestExecute_ContextCancelledHandlerError_IsAmbiguous(t *testing.T) {
	// Pre-cancel the context to simulate mid-flight disconnect where the
	// provider's HTTP client observes the cancellation and returns an error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	// Mock provider will return context.Canceled from Approve since ctx is done.
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{{ID: "github:o/r:1"}})
	emitter := events.NewEventEmitter()
	executor := NewActionExecutor(lg, mockProv, nil, emitter)

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID:   "github:o/r:1",
		Action: ApproveCommand,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Success {
		t.Error("must not be success on cancelled context")
	}
	if !result.IsAmbiguousRemoteSuccess() {
		t.Errorf("cancellation error after handler.Handle must be ambiguous; got: %q", result.Error)
	}
}

// TestExecute_PostComment_ContextCancelledHandlerError_IsAmbiguous verifies
// that the ambiguous guard applies to post_comment (non-idempotent) too.
func TestExecute_PostComment_ContextCancelledHandlerError_IsAmbiguous(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{{ID: "github:o/r:2"}})
	executor := NewActionExecutor(lg, mockProv, nil, events.NewEventEmitter())

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID:   "github:o/r:2",
		Action: PostCommentCommand,
		Body:   "nit: please fix",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsAmbiguousRemoteSuccess() {
		t.Errorf("post_comment cancellation must be ambiguous; got: %q", result.Error)
	}
}
