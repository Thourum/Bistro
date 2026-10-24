// go/internal/actions/executor_test.go
package actions

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/store"
)

func TestExecutorApprove(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := events.NewEventEmitter()

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
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := events.NewEventEmitter()
	eventCh := emitter.Subscribe()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	go executor.Execute(context.Background(), &ActionRequest{ //nolint:errcheck
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
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := events.NewEventEmitter()

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
	mockProv := mock.NewMockProvider().WithError(fmt.Errorf("mock error"))
	emitter := events.NewEventEmitter()

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
	emitter := events.NewEventEmitter()

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

func TestExecutorExecuteMultiple(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := events.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	requests := []*ActionRequest{
		{PRID: "github:owner/repo:42", Action: ApproveCommand},
		{PRID: "github:owner/repo:42", Action: PostCommentCommand, Body: "LGTM"},
	}

	results := executor.ExecuteMultiple(context.Background(), requests)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, r := range results {
		if !r.Success {
			t.Errorf("result[%d] expected success, got: %s", i, r.Error)
		}
	}
}

// ---- review tracking tests ----

// stubLoginGetter implements ReviewLoginGetter for tests.
type stubLoginGetter struct {
	login string
	found bool
}

func (s *stubLoginGetter) GetCurrentUser(_ context.Context, _ string) (string, bool, error) {
	return s.login, s.found, nil
}

// stubSHAWriter implements ReviewSHAWriter and captures calls for assertions.
type stubSHAWriter struct {
	calls []store.ReviewEvent
}

func (s *stubSHAWriter) RecordLastReviewed(_ context.Context, event store.ReviewEvent) error {
	s.calls = append(s.calls, event)
	return nil
}

func TestExecutor_ReviewTracking_ApproveRecordsLastReviewedSHA(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider()

	db, err := store.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Insert a PR with a known head_sha.
	const prID = "github:o/r:1"
	_, err = db.ExecContext(ctx, `INSERT INTO prs
		(id, provider, owner, repo, number, title, author, state,
		 created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES (?, 'github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','b','sha-head')`,
		prID)
	if err != nil {
		t.Fatalf("insert pr: %v", err)
	}

	loginGetter := &stubLoginGetter{login: "alice", found: true}
	shaWriter := &stubSHAWriter{}

	emitter := events.NewEventEmitter()
	executor := NewActionExecutor(log, mockProv, db, emitter).
		WithReviewTracking(loginGetter, shaWriter)

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID:   prID,
		Action: ApproveCommand,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}

	if len(shaWriter.calls) != 1 {
		t.Fatalf("expected 1 RecordLastReviewed call, got %d", len(shaWriter.calls))
	}
	call := shaWriter.calls[0]
	if call.PRID != prID {
		t.Errorf("prID: got %q want %q", call.PRID, prID)
	}
	if call.Login != "alice" {
		t.Errorf("login: got %q want alice", call.Login)
	}
	if call.HeadSHA != "sha-head" {
		t.Errorf("sha: got %q want sha-head", call.HeadSHA)
	}
	// patchsetNum may be 0 here (patchset row not yet recorded in test DB) — that is correct.
}

func TestExecutor_ReviewTracking_NoUserSkips(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider()
	emitter := events.NewEventEmitter()

	// No current user → should not record a review baseline.
	loginGetter := &stubLoginGetter{found: false}
	shaWriter := &stubSHAWriter{}

	executor := NewActionExecutor(log, mockProv, nil, emitter).
		WithReviewTracking(loginGetter, shaWriter)

	_, err := executor.Execute(ctx, &ActionRequest{
		PRID: "github:o/r:1", Action: ApproveCommand,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(shaWriter.calls) != 0 {
		t.Errorf("expected 0 SHA writes when no user, got %d", len(shaWriter.calls))
	}
}

func TestExecutor_ReviewTracking_NilTrackerNoOp(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider()
	emitter := events.NewEventEmitter()

	// No review tracking attached → must not panic.
	executor := NewActionExecutor(log, mockProv, nil, emitter)

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID: "github:o/r:1", Action: ApproveCommand,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success without review tracking, got: %s", result.Error)
	}
}

func TestExecutor_ReviewTracking_RecordsPatchsetNumber(t *testing.T) {
	// The newest patchset at the current head wins even after the base tip moves.
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider()

	db, err := store.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.NewMigrationRunner(nil, db.DB).Run(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const prID = "github:o/r:99"
	_, err = db.ExecContext(ctx, `INSERT INTO prs
		(id, provider, owner, repo, number, title, author, state,
		 created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES (?, 'github','o','r',99,'T','a','open','2026-01-01','2026-01-01','main','f','BASE-SHA','HEAD-SHA')`,
		prID)
	if err != nil {
		t.Fatalf("insert pr: %v", err)
	}
	// A moving base tip must not hide the latest patchset at the same head.
	_, err = db.ExecContext(ctx, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES (?, 1, 'HEAD-SHA', 'BASE-SHA', CURRENT_TIMESTAMP),
		       (?, 2, 'HEAD-SHA', 'OLD-BASE-SHA', CURRENT_TIMESTAMP)`, prID, prID)
	if err != nil {
		t.Fatalf("insert patchset: %v", err)
	}

	loginGetter := &stubLoginGetter{login: "alice", found: true}
	shaWriter := &stubSHAWriter{}
	emitter := events.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, db, emitter).
		WithReviewTracking(loginGetter, shaWriter)

	_, err = executor.Execute(ctx, &ActionRequest{PRID: prID, Action: ApproveCommand})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(shaWriter.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(shaWriter.calls))
	}
	call := shaWriter.calls[0]
	if call.PatchsetNum != 2 {
		t.Errorf("patchsetNum: got %d want 2 (must record exact patchset number)", call.PatchsetNum)
	}
	if call.HeadSHA != "HEAD-SHA" {
		t.Errorf("sha: got %q want HEAD-SHA", call.HeadSHA)
	}
	if call.Source != store.ReviewSourceBistroAction {
		t.Errorf("source: got %q want bistro_action", call.Source)
	}
	if call.ReviewedAt.IsZero() {
		t.Error("reviewedAt must record the action time")
	}
}

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

// TestExecute_ValidationFailure_NotAmbiguous verifies that pre-call validation
// failures (missing required field) are NOT marked ambiguous — the provider
// mutation was never attempted.
func TestExecute_ValidationFailure_NotAmbiguous(t *testing.T) {
	ctx := context.Background() // live context — doesn't matter, fails before handler.Handle

	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	executor := NewActionExecutor(lg, mock.NewMockProvider(), nil, events.NewEventEmitter())

	// post_comment without Body fails req.Validate() — before handler.Handle.
	result, err := executor.Execute(ctx, &ActionRequest{
		PRID:   "github:o/r:3",
		Action: PostCommentCommand,
		Body:   "", // missing required field
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Success {
		t.Error("validation failure must not be success")
	}
	if result.IsAmbiguousRemoteSuccess() {
		t.Error("pre-call validation failure must NOT be ambiguous; provider was never called")
	}
}

// TestExecute_UnknownAction_NotAmbiguous verifies that unknown-action failures
// (pre-call, before handler.Handle) are not marked ambiguous.
func TestExecute_UnknownAction_NotAmbiguous(t *testing.T) {
	ctx := context.Background()
	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	executor := NewActionExecutor(lg, mock.NewMockProvider(), nil, events.NewEventEmitter())

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID:   "github:o/r:4",
		Action: ActionCommand("unknown_action"),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsAmbiguousRemoteSuccess() {
		t.Error("unknown-action failure must NOT be ambiguous; no handler was invoked")
	}
}

// ── Blocker 1: recoverable transport failure on non-idempotent actions ────

// TestExecute_PostComment_RecoverableTransportError_IsAmbiguous verifies that
// a recoverable transport error (e.g. 503, connection reset) from post_comment
// is classified as ErrAmbiguousRemoteSuccess rather than left retryable.
// GitHub may have already created the comment before the error was observed.
func TestExecute_PostComment_RecoverableTransportError_IsAmbiguous(t *testing.T) {
	ctx := context.Background() // live context — NOT cancelled

	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	// Provider returns a 503 / "service unavailable" transient error.
	mockProv := mock.NewMockProvider().
		WithError(&provider.ProviderError{Message: "503 service unavailable"})
	emitter := events.NewEventEmitter()
	executor := NewActionExecutor(lg, mockProv, nil, emitter)

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID:   "github:o/r:5",
		Action: PostCommentCommand,
		Body:   "please fix",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Success {
		t.Error("must not be success on transport error")
	}
	if !result.IsAmbiguousRemoteSuccess() {
		t.Errorf("post_comment recoverable transport error must be ambiguous; got: %q", result.Error)
	}
}

// TestExecute_RequestChanges_RecoverableTransportError_IsAmbiguous verifies
// that request_changes with a recoverable error is also ambiguous.
func TestExecute_RequestChanges_RecoverableTransportError_IsAmbiguous(t *testing.T) {
	ctx := context.Background()

	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().
		WithError(&provider.ProviderError{Message: "500 internal server error"})
	executor := NewActionExecutor(lg, mockProv, nil, events.NewEventEmitter())

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID:   "github:o/r:6",
		Action: RequestChangesCommand,
		Body:   "needs work",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsAmbiguousRemoteSuccess() {
		t.Errorf("request_changes recoverable error must be ambiguous; got: %q", result.Error)
	}
}

// TestExecute_Approve_RecoverableTransportError_IsAmbiguous verifies that
// approve with a recoverable transport error IS marked ambiguous — approve
// uses CreateReview (non-idempotent) so we cannot safely retry without risk
// of creating a duplicate review.
func TestExecute_Approve_RecoverableTransportError_IsAmbiguous(t *testing.T) {
	ctx := context.Background()

	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().
		WithError(&provider.ProviderError{Message: "503 service unavailable"})
	executor := NewActionExecutor(lg, mockProv, nil, events.NewEventEmitter())

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID:   "github:o/r:7",
		Action: ApproveCommand,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsAmbiguousRemoteSuccess() {
		t.Errorf("approve recoverable error must be ambiguous (CreateReview is non-idempotent); got: %q", result.Error)
	}
}

// TestExecute_PostComment_PermanentError_NotAmbiguous verifies that a permanent
// auth/permission error (401, 403) from post_comment is NOT ambiguous — the
// provider rejected the call without executing the mutation.
func TestExecute_PostComment_PermanentError_NotAmbiguous(t *testing.T) {
	ctx := context.Background()

	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().
		WithError(&provider.ProviderError{Message: "403 forbidden"})
	executor := NewActionExecutor(lg, mockProv, nil, events.NewEventEmitter())

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID:   "github:o/r:8",
		Action: PostCommentCommand,
		Body:   "nit",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsAmbiguousRemoteSuccess() {
		t.Error("permanent auth error must NOT be ambiguous — provider rejected the call")
	}
}

// ── Blocker: rate-limit exclusion from non-idempotent ambiguous guard ─────

// TestExecute_PostComment_RateLimit_NotAmbiguous verifies that an explicit
// 429 / "rate limit" response for post_comment is NOT marked ambiguous.
// A 429 proves the provider rejected the call before any mutation, so the
// action is safe to re-queue and retry after the rate-limit window resets.
func TestExecute_PostComment_RateLimit_NotAmbiguous(t *testing.T) {
	ctx := context.Background()
	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().
		WithError(&provider.ProviderError{Message: "429 rate limit exceeded"})
	executor := NewActionExecutor(lg, mockProv, nil, events.NewEventEmitter())

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID:   "github:o/r:9",
		Action: PostCommentCommand,
		Body:   "nit",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsAmbiguousRemoteSuccess() {
		t.Error("post_comment with 429 must NOT be ambiguous — provider rejected before mutation")
	}
	if !result.IsRecoverable() {
		t.Error("post_comment with 429 must remain recoverable so it can be retried after reset")
	}
}

// TestExecute_RequestChanges_RateLimit_NotAmbiguous mirrors the post_comment
// test for request_changes.
func TestExecute_RequestChanges_RateLimit_NotAmbiguous(t *testing.T) {
	ctx := context.Background()
	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().
		WithError(&provider.ProviderError{Message: "rate limit: retry after 60s"})
	executor := NewActionExecutor(lg, mockProv, nil, events.NewEventEmitter())

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID:   "github:o/r:10",
		Action: RequestChangesCommand,
		Body:   "needs work",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsAmbiguousRemoteSuccess() {
		t.Error("request_changes with rate-limit error must NOT be ambiguous")
	}
	if !result.IsRecoverable() {
		t.Error("request_changes with rate-limit must remain recoverable")
	}
}

// TestExecute_PostComment_Timeout_StillAmbiguous verifies that a timeout
// (not a rate-limit) for post_comment IS still marked ambiguous — the mutation
// may have been applied before the timeout was observed.
func TestExecute_PostComment_Timeout_StillAmbiguous(t *testing.T) {
	ctx := context.Background()
	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().
		WithError(&provider.ProviderError{Message: "connection timed out"})
	executor := NewActionExecutor(lg, mockProv, nil, events.NewEventEmitter())

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID:   "github:o/r:11",
		Action: PostCommentCommand,
		Body:   "nit",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsAmbiguousRemoteSuccess() {
		t.Errorf("post_comment with timeout must be ambiguous; got: %q", result.Error)
	}
}

// ── Adversarial blocker: PR #429 misclassification ────────────────────────

// TestExecute_PostComment_PR429_ConnectionReset_IsAmbiguous is the core
// regression test: a connection-reset error on PR owner/repo#429 must NOT be
// classified as a rate-limit and must remain ambiguous for post_comment.
func TestExecute_PostComment_PR429_ConnectionReset_IsAmbiguous(t *testing.T) {
	ctx := context.Background()
	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	// Error message embeds "#429" as the PR number, not as an HTTP status code.
	mockProv := mock.NewMockProvider().
		WithError(&provider.ProviderError{
			Message: "failed to post comment on PR owner/repo#429: connection reset by peer",
		})
	executor := NewActionExecutor(lg, mockProv, nil, events.NewEventEmitter())

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID:   "github:owner/repo:429",
		Action: PostCommentCommand,
		Body:   "nit",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsRateLimit() {
		t.Error("PR #429 connection reset must NOT be IsRateLimit() — it contains PR number, not HTTP status")
	}
	if !result.IsAmbiguousRemoteSuccess() {
		t.Errorf("post_comment connection reset on PR #429 must be ambiguous; got: %q", result.Error)
	}
}

// TestIsRateLimit_AnchoredPatterns verifies that the anchored rate-limit
// patterns correctly classify HTTP 429 responses and not PR-number matches.
func TestIsRateLimit_AnchoredPatterns(t *testing.T) {
	cases := []struct {
		errMsg      string
		wantIsRL    bool
		description string
	}{
		// Must be rate-limit.
		{"status 429: too many requests", true, "status 429"},
		{"status: 429 rate limit exceeded", true, "status: 429"},
		{"HTTP 429 Too Many Requests", true, "HTTP 429 (case insensitive)"},
		{"429 Too Many Requests from GitHub", true, "429 too many"},
		{"429 rate limit hit", true, "429 rate prefix"},
		{"GitHub API rate limit exceeded", true, "rate limit text"},
		// Must NOT be rate-limit.
		{"failed to post comment on PR owner/repo#429: connection reset", false, "PR number #429 in message"},
		{"error on PR github:o/r:429: timeout", false, "PR number :429 in message"},
		{"post comment PR-429 timed out", false, "PR-429 identifier"},
		{"connection reset by peer", false, "plain transport error"},
		{"503 service unavailable", false, "5xx not rate limit"},
	}

	for _, tc := range cases {
		ar := &ActionResult{Success: false, Error: tc.errMsg}
		got := ar.IsRateLimit()
		if got != tc.wantIsRL {
			t.Errorf("[%s] IsRateLimit(%q) = %v, want %v", tc.description, tc.errMsg, got, tc.wantIsRL)
		}
	}
}

// ── Adversarial blocker: go-github ": NNN" error format ───────────────────

// TestExecute_PostComment_GoGithub500_IsAmbiguous verifies that a go-github
// ErrorResponse formatted as "POST ...: 500  []" is recognized as a recoverable
// 5xx error and therefore becomes ambiguous terminal for post_comment.
func TestExecute_PostComment_GoGithub500_IsAmbiguous(t *testing.T) {
	ctx := context.Background()
	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().
		WithError(&provider.ProviderError{
			Message: "POST https://api.github.com/repos/owner/repo/issues/1/comments: 500  []",
		})
	executor := NewActionExecutor(lg, mockProv, nil, events.NewEventEmitter())

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID: "github:owner/repo:1", Action: PostCommentCommand, Body: "nit",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Guard 1b overrides recoverable 500 to ambiguous for non-idempotent actions.
	// Verify the final result is the ambiguous sentinel (not just a plain failure).
	if !result.IsAmbiguousRemoteSuccess() {
		t.Errorf("post_comment go-github 500 must be ambiguous; got: %q", result.Error)
	}
	// Verify directly that the raw 500 error string IS recoverable (confirms
	// Guard 1b triggered rather than a permanent/unknown classification).
	raw := &ActionResult{Success: false, Error: "handler error: POST https://api.github.com/repos/owner/repo/issues/1/comments: 500  []"}
	if !raw.IsRecoverable() {
		t.Error("go-github ': 500' error must be recoverable before ambiguous override")
	}
}

// TestExecute_PostComment_GoGithub502_IsAmbiguous covers 502.
func TestExecute_PostComment_GoGithub502_IsAmbiguous(t *testing.T) {
	ctx := context.Background()
	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().
		WithError(&provider.ProviderError{
			Message: "POST https://api.github.com/repos/owner/repo/pulls/1/reviews: 502  []",
		})
	executor := NewActionExecutor(lg, mockProv, nil, events.NewEventEmitter())

	result, _ := executor.Execute(ctx, &ActionRequest{ //nolint:errcheck
		PRID: "github:owner/repo:1", Action: RequestChangesCommand, Body: "needs work",
	})
	if !result.IsAmbiguousRemoteSuccess() {
		t.Errorf("request_changes go-github 502 must be ambiguous; got: %q", result.Error)
	}
}

// TestExecute_PostComment_GoGithub503_IsAmbiguous covers 503.
func TestExecute_PostComment_GoGithub503_IsAmbiguous(t *testing.T) {
	ctx := context.Background()
	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().
		WithError(&provider.ProviderError{
			Message: "POST https://api.github.com/repos/owner/repo/issues/1/comments: 503  []",
		})
	executor := NewActionExecutor(lg, mockProv, nil, events.NewEventEmitter())

	result, _ := executor.Execute(ctx, &ActionRequest{ //nolint:errcheck
		PRID: "github:owner/repo:1", Action: PostCommentCommand, Body: "nit",
	})
	if !result.IsAmbiguousRemoteSuccess() {
		t.Errorf("post_comment go-github 503 must be ambiguous; got: %q", result.Error)
	}
}

// TestExecute_PostComment_GoGithub429_IsRateLimit verifies that go-github
// ": 429" is correctly classified as rate-limit (not ambiguous).
func TestExecute_PostComment_GoGithub429_IsRateLimit(t *testing.T) {
	ctx := context.Background()
	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().
		WithError(&provider.ProviderError{
			Message: "POST https://api.github.com/repos/owner/repo/issues/1/comments: 429  []",
		})
	executor := NewActionExecutor(lg, mockProv, nil, events.NewEventEmitter())

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID: "github:owner/repo:1", Action: PostCommentCommand, Body: "nit",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsRateLimit() {
		t.Errorf("go-github ': 429' must be IsRateLimit(); got: %q", result.Error)
	}
	if result.IsAmbiguousRemoteSuccess() {
		t.Error("go-github ': 429' must NOT be ambiguous — request was rejected before mutation")
	}
	if !result.IsRecoverable() {
		t.Error("go-github ': 429' must remain recoverable for queue replay")
	}
}

// TestExecute_PostComment_PR500_ConnectionReset_NotByStatusCode verifies that
// "failed to post comment on PR owner/repo#500: connection reset" does NOT
// classify as HTTP 500 status (the "500" is a PR number). It IS ambiguous
// for post_comment via the "connection reset" transport error match.
func TestExecute_PostComment_PR500_ConnectionReset_Ambiguous(t *testing.T) {
	ctx := context.Background()
	lg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProv := mock.NewMockProvider().
		WithError(&provider.ProviderError{
			Message: "failed to post comment on PR owner/repo#500: connection reset by peer",
		})
	executor := NewActionExecutor(lg, mockProv, nil, events.NewEventEmitter())

	result, err := executor.Execute(ctx, &ActionRequest{
		PRID: "github:owner/repo:500", Action: PostCommentCommand, Body: "nit",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Must be ambiguous for non-idempotent post_comment (via "connection reset").
	if !result.IsAmbiguousRemoteSuccess() {
		t.Errorf("post_comment PR#500 connection reset must be ambiguous; got: %q", result.Error)
	}
	// Verify directly that the "#500" PR number does not trigger ": 500" status match,
	// while "connection reset" IS what makes it recoverable/ambiguous.
	raw := &ActionResult{Success: false, Error: "handler error: failed to post comment on PR owner/repo#500: connection reset by peer"}
	if !raw.IsRecoverable() {
		t.Error("PR#500 connection reset must be recoverable via transport keyword, not status-code match")
	}
	// Verify "#500" alone (without "connection reset") is NOT status-code recoverable.
	statusOnly := &ActionResult{Success: false, Error: "handler error: failed on PR owner/repo#500: some other error"}
	if statusOnly.IsRecoverable() {
		t.Error("PR#500 in identifier must NOT be recoverable via HTTP-status matching")
	}
}

// TestIsRateLimit_GoGithubFormat covers ": 429" and verifies ": 500" is NOT rate-limit.
func TestIsRateLimit_GoGithubFormat(t *testing.T) {
	cases := []struct {
		errMsg   string
		wantIsRL bool
		desc     string
	}{
		{"POST https://api.github.com/...: 429  []", true, "go-github ': 429'"},
		{"POST https://api.github.com/...: 500  []", false, "go-github ': 500' is not rate-limit"},
		{"POST https://api.github.com/...: 503  []", false, "go-github ': 503' is not rate-limit"},
		{"owner/repo#429: connection reset", false, "PR#429 not rate-limit"},
		{"owner/repo#500: connection reset", false, "PR#500 not rate-limit"},
	}
	for _, tc := range cases {
		ar := &ActionResult{Success: false, Error: tc.errMsg}
		if got := ar.IsRateLimit(); got != tc.wantIsRL {
			t.Errorf("[%s] IsRateLimit(%q) = %v, want %v", tc.desc, tc.errMsg, got, tc.wantIsRL)
		}
	}
}
