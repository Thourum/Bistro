// go/internal/actions/executor.go
package actions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

// ReviewLoginGetter resolves the authenticated login for a provider.
// Satisfied by *currentuser.Store.
type ReviewLoginGetter interface {
	Get(ctx context.Context, providerName string) (string, bool, error)
}

// ReviewSHAWriter records the since-last-review baseline.
// Satisfied by *store.DB.
type ReviewSHAWriter interface {
	RecordLastReviewed(ctx context.Context, event store.ReviewEvent) error
}

// ActionExecutor coordinates action execution with provider, store, and events.
type ActionExecutor struct {
	log             *slog.Logger
	provider        provider.Provider
	store           *store.DB
	emitter         *events.EventEmitter
	reviewLogin     ReviewLoginGetter // optional; if nil, last-reviewed SHA is not recorded
	reviewSHAWriter ReviewSHAWriter   // optional; if nil, last-reviewed SHA is not recorded
}

// NewActionExecutor creates a new action executor. If log is nil, slog.Default
// is used so downstream code never needs to nil-check.
func NewActionExecutor(
	log *slog.Logger,
	prov provider.Provider,
	db *store.DB,
	emitter *events.EventEmitter,
) *ActionExecutor {
	if log == nil {
		log = slog.Default()
	}
	return &ActionExecutor{
		log:      log,
		provider: prov,
		store:    db,
		emitter:  emitter,
	}
}

// WithReviewTracking attaches the login resolver and SHA writer used to record
// which SHA the current user last reviewed after a successful action. Both must
// be non-nil together; passing nil for either disables tracking silently.
func (ae *ActionExecutor) WithReviewTracking(loginGetter ReviewLoginGetter, shaWriter ReviewSHAWriter) *ActionExecutor {
	if loginGetter != nil && shaWriter != nil {
		ae.reviewLogin = loginGetter
		ae.reviewSHAWriter = shaWriter
	}
	return ae
}

func failedResult(req *ActionRequest, message string) *ActionResult {
	result := &ActionResult{Error: message, Timestamp: time.Now()}
	if req != nil {
		result.PRID = req.PRID
		result.Action = req.Action
	}
	return result
}

// Execute performs a single action (no retries).
// Errors are returned inside ActionResult rather than as the error return value.
func (ae *ActionExecutor) Execute(ctx context.Context, req *ActionRequest) (*ActionResult, error) {
	if req == nil {
		return failedResult(nil, "nil request"), nil
	}
	if err := req.Validate(); err != nil {
		return failedResult(req, fmt.Sprintf("validation failed: %v", err)), nil
	}

	handler, ok := ae.handlerFor(req.Action)
	if !ok {
		return failedResult(req, fmt.Sprintf("unknown action: %s", req.Action)), nil
	}

	result, err := handler.Handle(ctx, req)
	if err != nil {
		result = failedResult(req, fmt.Sprintf("handler error: %v", err))
	}

	// ambiguousResult builds the sentinel failure used whenever context
	// cancellation is observed after a provider call that may have succeeded.
	// Using ErrAmbiguousRemoteSuccess prefix marks the result as terminal in
	// the queue replay path so it is never retried (blocker 1).
	ambiguousResult := func() *ActionResult {
		return failedResult(req, ErrAmbiguousRemoteSuccess+"provider action may have executed; local effects suppressed")
	}

	// Guard 1: immediately after handler.Handle, which is the provider mutation
	// attempt. If ctx is cancelled at this point, the provider may have already
	// executed the mutation regardless of whether Handle returned success or an
	// error (e.g. context.Canceled from a mid-flight network cancellation).
	// Mark as ambiguous unconditionally so replay terminally fails the row and
	// never retries non-idempotent actions (post_comment, request_changes, etc).
	//
	// Pre-call validation failures (nil request, Validate(), unknown action)
	// return early ABOVE — they never reach this point — so they are correctly
	// NOT marked ambiguous.
	if ctx.Err() != nil {
		result = ambiguousResult()
	}

	// Guard 1b: recoverable transport failure from a non-idempotent action.
	// A timeout, 5xx, or connection-reset from the provider after handler.Handle
	// is called may indicate that GitHub processed the request even though the
	// client received an error. For idempotent actions (approve, resolve_thread)
	// retrying is safe; for post_comment and request_changes it would create
	// duplicate records (comment threads, review submissions). Mark as ambiguous
	// so replay terminally fails the row instead of retrying.
	//
	// Exception: explicit rate-limit (429 / "rate limit") responses are excluded
	// because a 429 proves the provider rejected the request before executing
	// any mutation — the action is safe to re-queue and retry after reset.
	//
	// Auth/permission errors (401, 403, 404, 422) are permanent, not recoverable,
	// so IsRecoverable() returns false for them and this guard never fires.
	if result.IsRecoverable() && !result.IsRateLimit() && req.Action.isNonIdempotent() {
		result = ambiguousResult()
	}

	ae.logResult(req, result)

	// Guard 2: before emitting events (action_complete, pr_approved,
	// changes_requested). Closes the race window between Guard 1 and here.
	if result.Success && ctx.Err() != nil {
		result = ambiguousResult()
	}
	ae.emitEvent(req, result)

	// Guard 3: before local DB writes (updatePRReviewStatus, recordLastReviewedSHA).
	if result.Success && ctx.Err() != nil {
		result = ambiguousResult()
	}
	if result.Success {
		ae.updatePRReviewStatus(ctx, req)
	}

	// Guard 4: final success return. Ensures the caller never sees Success=true
	// if cancellation occurred at any point during the side-effect section.
	if result.Success && ctx.Err() != nil {
		result = ambiguousResult()
	}

	return result, nil
}

// ExecuteWithRetry performs an action with automatic retries using exponential backoff.
func (ae *ActionExecutor) ExecuteWithRetry(ctx context.Context, req *ActionRequest, maxAttempts int) (*ActionResult, error) {
	if req == nil {
		return failedResult(nil, "nil request"), nil
	}
	if err := req.Validate(); err != nil {
		return failedResult(req, fmt.Sprintf("validation failed: %v", err)), nil
	}

	backoff := NewExponentialBackoff(100*time.Millisecond, 5*time.Second, 2.0)

	result := RetryWithBackoff(ctx, backoff, maxAttempts, func() (*ActionResult, error) {
		return ae.Execute(ctx, req)
	})

	return result, nil
}

// ExecuteMultiple performs multiple actions sequentially and returns all results.
func (ae *ActionExecutor) ExecuteMultiple(ctx context.Context, requests []*ActionRequest) []*ActionResult {
	results := make([]*ActionResult, len(requests))

	for i, req := range requests {
		if req == nil {
			results[i] = failedResult(nil, "nil request")
			continue
		}
		result, err := ae.Execute(ctx, req)
		if err != nil {
			results[i] = failedResult(req, fmt.Sprintf("execution error: %v", err))
		} else {
			results[i] = result
		}
	}

	return results
}

// handlerFor returns the appropriate Handler for the given action, or false if unknown.
func (ae *ActionExecutor) handlerFor(action ActionCommand) (Handler, bool) {
	switch action {
	case ApproveCommand:
		return NewApproveHandler(ae.provider), true
	case RequestChangesCommand:
		return NewRequestChangesHandler(ae.provider), true
	case PostCommentCommand:
		return NewPostCommentHandler(ae.provider), true
	case ReplyCommentCommand:
		return NewReplyCommentHandler(ae.provider), true
	case ResolveThreadCommand:
		return NewResolveThreadHandler(ae.provider), true
	case UnresolveThreadCommand:
		return NewUnresolveThreadHandler(ae.provider), true
	default:
		return nil, false
	}
}

// logResult logs the action outcome at the appropriate level.
func (ae *ActionExecutor) logResult(req *ActionRequest, result *ActionResult) {
	if ae.log == nil {
		return
	}
	if result.Success {
		ae.log.Info("action completed", "pr", req.PRID, "action", req.Action)
	} else {
		ae.log.Error("action failed", "pr", req.PRID, "action", req.Action, "error", result.Error)
	}
}

// emitEvent broadcasts an action_complete event and, on success, an
// action-specific event (pr_approved or changes_requested) to all subscribers.
func (ae *ActionExecutor) emitEvent(req *ActionRequest, result *ActionResult) {
	if ae.emitter == nil {
		return
	}
	ae.emitter.Emit(&events.Event{
		Type: "action_complete",
		Data: map[string]any{
			"pr":      req.PRID,
			"action":  req.Action,
			"success": result.Success,
			"error":   result.Error,
		},
		Timestamp: time.Now(),
	})

	if !result.Success {
		return
	}

	switch req.Action {
	case ApproveCommand:
		ae.emitter.Emit(&events.Event{
			Type: "pr_approved",
			Data: map[string]any{
				"pr_id": req.PRID,
			},
			Timestamp: time.Now(),
		})
	case RequestChangesCommand:
		ae.emitter.Emit(&events.Event{
			Type: "changes_requested",
			Data: map[string]any{
				"pr_id": req.PRID,
			},
			Timestamp: time.Now(),
		})
	}
}

// updatePRReviewStatus updates review_status, approved_by, or changes_requested_by
// in the prs table after a successful approve or request_changes action.
// The actor username is derived from the provider when available; the PRID is
// always recorded regardless.  Errors are logged and not propagated — a DB
// update failure must not abort an already-successful provider action.
func (ae *ActionExecutor) updatePRReviewStatus(ctx context.Context, req *ActionRequest) {
	if ae.store == nil {
		return
	}

	switch req.Action {
	case ApproveCommand:
		const query = `
			UPDATE prs
			SET review_status = 'approved', approved_by = ?
			WHERE id = ?
		`
		if _, err := ae.store.ExecContext(ctx, query, "", req.PRID); err != nil {
			ae.log.Error("failed to update review_status to approved", "pr_id", req.PRID, "error", err)
		}
	case RequestChangesCommand:
		const query = `
			UPDATE prs
			SET review_status = 'changes_requested', changes_requested_by = ?
			WHERE id = ?
		`
		if _, err := ae.store.ExecContext(ctx, query, "", req.PRID); err != nil {
			ae.log.Error("failed to update review_status to changes_requested", "pr_id", req.PRID, "error", err)
		}
	}

	// Record which SHA the current user reviewed, for the since-last-review
	// feature (phase 2.2). Review submissions and new top-level comments count
	// as a review. Replies to existing threads and thread resolve/unresolve do
	// not constitute a "review" for SHA tracking.
	switch req.Action {
	case ApproveCommand, RequestChangesCommand, PostCommentCommand:
		ae.recordLastReviewedSHA(ctx, req.PRID)
	}
}

// recordLastReviewedSHA fetches the PR's current patchset identity and writes a
// last_reviewed_sha row for the authenticated login. Best-effort: any failure
// is logged and swallowed so the caller's action result is unaffected.
func (ae *ActionExecutor) recordLastReviewedSHA(ctx context.Context, prID string) {
	if ae.reviewLogin == nil || ae.reviewSHAWriter == nil || ae.store == nil {
		return
	}

	login, ok, err := ae.reviewLogin.Get(ctx, "github")
	if err != nil {
		ae.log.Warn("recordLastReviewedSHA: could not fetch current user", "pr_id", prID, "error", err)
		return
	}
	if !ok || login == "" {
		ae.log.Debug("recordLastReviewedSHA: no current user, skipping", "pr_id", prID)
		return
	}

	var headSHA string
	if err := ae.store.QueryRowContext(ctx,
		`SELECT head_sha FROM prs WHERE id = ?`, prID,
	).Scan(&headSHA); err != nil {
		ae.log.Warn("recordLastReviewedSHA: could not fetch PR SHAs",
			"pr_id", prID, "login", login, "error", err)
		return
	}

	// The base tip moves without changing the PR's patchset identity.
	// Best-effort: patchsetNum stays 0 (unknown) if the patchset row doesn't exist yet.
	var patchsetNum int
	if err := ae.store.QueryRowContext(ctx,
		`SELECT number FROM patchsets WHERE pr_id = ? AND head_sha = ? ORDER BY number DESC LIMIT 1`,
		prID, headSHA,
	).Scan(&patchsetNum); err != nil && !errors.Is(err, sql.ErrNoRows) {
		ae.log.Warn("recordLastReviewedSHA: could not fetch patchset number", "pr_id", prID, "error", err)
	}

	event := store.ReviewEvent{
		PRID: prID, Login: login, HeadSHA: headSHA, PatchsetNum: patchsetNum,
		ReviewedAt: time.Now(), Source: store.ReviewSourceBistroAction,
	}
	if err := ae.reviewSHAWriter.RecordLastReviewed(ctx, event); err != nil {
		ae.log.Warn("recordLastReviewedSHA: RecordLastReviewed failed",
			"pr_id", prID, "login", login, "patchset_num", patchsetNum, "error", err)
	}
}
