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
