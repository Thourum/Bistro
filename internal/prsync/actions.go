package prsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/itterno/bistro/internal/store/actionqueue"
	"log/slog"
	"time"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
)

// failedActionResult keeps failure response fields consistent across preflight,
// queueing and executor errors.
func failedActionResult(req *actions.ActionRequest, message string) *actions.ActionResult {
	return &actions.ActionResult{
		PRID: req.PRID, Action: req.Action, Success: false,
		Error: message, Timestamp: time.Now(),
	}
}

// validateThreadPreflight checks thread ownership and known state before either
// immediate execution or queue replay mutates the provider.
func (o *Orchestrator) validateThreadPreflight(ctx context.Context, req *actions.ActionRequest) *actions.ActionResult {
	if req.Action != actions.ResolveThreadCommand && req.Action != actions.UnresolveThreadCommand {
		return nil
	}
	if o.store == nil {
		return nil // No store on the test/noop path.
	}
	thread, err := o.store.GetReviewThread(ctx, req.PRID, req.Body)
	if err != nil {
		return failedActionResult(req, fmt.Sprintf("thread lookup failed: %v", err))
	}
	if thread == nil {
		return failedActionResult(req, fmt.Sprintf("thread %s not found in pr %s", req.Body, req.PRID))
	}
	if !thread.StateKnown {
		return failedActionResult(req, fmt.Sprintf("thread %s state is unknown; sync required before mutation", req.Body))
	}
	return nil
}

// prepareReplyCommentTarget resolves the GitHub review-comment ID without
// changing the caller's request; the UI supplies a thread ID instead.
func (o *Orchestrator) prepareReplyCommentTarget(ctx context.Context, req *actions.ActionRequest) (*actions.ActionRequest, *actions.ActionResult) {
	if req.Action != actions.ReplyCommentCommand || req.InReplyTo != "" || req.ThreadID == "" || o.store == nil {
		return req, nil
	}
	targetID, err := o.store.ReplyTargetCommentIDForThread(ctx, req.PRID, req.ThreadID)
	if err != nil {
		return nil, failedActionResult(req, fmt.Sprintf("reply target lookup failed: %v", err))
	}
	if targetID == "" {
		return nil, failedActionResult(req, fmt.Sprintf("thread %s has no cached review comment target in pr %s", req.ThreadID, req.PRID))
	}
	resolved := *req
	resolved.InReplyTo = targetID
	return &resolved, nil
}

// ApplyAction executes an action immediately or queues it when rate limited.
func (o *Orchestrator) ApplyAction(ctx context.Context, req *actions.ActionRequest) *actions.ActionResult {
	log := logging.FromContext(ctx).With("pr_id", req.PRID, "action", string(req.Action))
	ctx = logging.WithLogger(ctx, log)

	// A captured orchestrator must reject actions after its provider disconnects.
	if o.liveness != nil && o.liveness.Err() != nil {
		return failedActionResult(req, "provider disconnected: action rejected")
	}
	if result := o.validateThreadPreflight(ctx, req); result != nil {
		return result
	}
	// Review submissions with drafts bypass the queue: drafts are already
	// persisted, so a rate-limited or offline submit fails with them intact.
	if result, handled := o.applyReviewWithDrafts(ctx, req); handled {
		return result
	}
	resolvedReq, failure := o.prepareReplyCommentTarget(ctx, req)
	if failure != nil {
		return failure
	}
	if o.IsRateLimited() {
		return o.queueRateLimitedAction(ctx, resolvedReq)
	}
	return o.executeAction(ctx, resolvedReq)
}

func (o *Orchestrator) queueRateLimitedAction(ctx context.Context, req *actions.ActionRequest) *actions.ActionResult {
	log := logging.FromContext(ctx)
	log.Info("rate limited, queueing action")
	// The queue stores Body but not Line; replaying an inline comment would post
	// a general PR comment instead.
	if req.Action == actions.PostCommentCommand && req.Line > 0 {
		return failedActionResult(req, "line-level post_comment cannot be queued for replay (Line context would be lost); retry after rate limit clears")
	}
	payload := req.Body
	if req.Action == actions.ReplyCommentCommand {
		encoded, err := json.Marshal(actions.ReplyCommentPayload{Body: req.Body, InReplyTo: req.InReplyTo, ThreadID: req.ThreadID})
		if err != nil {
			return failedActionResult(req, fmt.Sprintf("failed to encode reply_comment payload: %v", err))
		}
		payload = string(encoded)
	}
	accountLogin := ""
	queueID := queuedActionID(req)
	if o.reviewLogin != nil {
		login, ok, err := o.reviewLogin.Get(ctx, provider.GitHub)
		if err != nil {
			return failedActionResult(req, fmt.Sprintf("rate limited: could not resolve current GitHub account for queueing: %v", err))
		}
		if !ok || login == "" {
			return failedActionResult(req, "rate limited: no current GitHub account, action not queued")
		}
		accountLogin = login
		queueID = accountScopedQueuedActionID(login, queueID)
	}
	queuedAction := &actionqueue.Action{
		ID: queueID, ActionType: string(req.Action), PRID: req.PRID,
		Payload: payload, AccountProvider: provider.GitHub, AccountLogin: accountLogin,
	}
	if o.queue == nil {
		return failedActionResult(req, "rate limited: no action queue configured, action dropped")
	}
	if err := o.queue.Enqueue(ctx, queuedAction); err != nil {
		log.Error("failed to queue action", "error", err)
		return failedActionResult(req, err.Error())
	}
	log.Debug("action queued", "queue_id", queuedAction.ID)
	result := failedActionResult(req, "rate limited: action queued for later replay")
	result.Queued = true
	return result
}

// executeAction runs an action via the executor immediately.
func (o *Orchestrator) executeAction(ctx context.Context, req *actions.ActionRequest) *actions.ActionResult {
	log := logging.FromContext(ctx)
	log.Debug("executing action", "pr_id", req.PRID, "action", string(req.Action))

	// Cancel the execution when either the caller or orchestrator disconnects.
	execCtx, execCancel := context.WithCancel(ctx)
	defer execCancel()
	if o.liveness != nil {
		go func() {
			select {
			case <-o.liveness.Done():
				execCancel()
			case <-execCtx.Done():
			}
		}()
	}

	executor := actions.NewActionExecutor(o.log, o.provider, o.store, o.emitter).
		WithReviewTracking(o.reviewLogin, o.reviewSHAWriter)
	start := time.Now()
	attempts := actionRetryCount
	if req.Action == actions.SubmitReviewCommand {
		// A retried CreateReview that GitHub already accepted posts every comment twice.
		attempts = 1
	}
	result, err := executor.ExecuteWithRetry(execCtx, req, attempts)
	dur := time.Since(start).Milliseconds()

	if err != nil {
		log.Error("action execution error", "pr_id", req.PRID, "action", string(req.Action),
			"duration_ms", dur, "error", err)
		return failedActionResult(req, err.Error())
	}
	if result == nil {
		log.Error("action executor returned nil result", "pr_id", req.PRID,
			"action", string(req.Action), "duration_ms", dur)
		return failedActionResult(req, "executor returned nil result")
	}
	log.Debug("action executed", "pr_id", req.PRID, "action", string(req.Action),
		"duration_ms", dur, "success", result.Success)

	if result.Success {
		switch req.Action {
		case actions.ResolveThreadCommand, actions.UnresolveThreadCommand:
			o.handleThreadActionSuccess(ctx, req, log)
		case actions.ReplyCommentCommand:
			o.handleReplyCommentSuccess(req)
		case actions.ApproveCommand, actions.RequestChangesCommand, actions.PostCommentCommand, actions.SubmitReviewCommand:
			if o.postReview != nil {
				o.postReview(ctx, req.PRID)
			}
		}
	}
	return result
}

// handleReplyCommentSuccess emits a refresh hint after a successful reply.
// The created GitHub comment is not synchronously stored in SQLite.
func (o *Orchestrator) handleReplyCommentSuccess(req *actions.ActionRequest) {
	if o.emitter == nil {
		return
	}
	o.emitter.Emit(&events.Event{
		Type: "COMMENT_POSTED",
		Data: map[string]interface{}{
			"pr_id": req.PRID, "thread_id": req.ThreadID, "in_reply_to": req.InReplyTo,
		},
		Timestamp: time.Now(),
	})
}

// handleThreadActionSuccess updates local cache and emits a thread event after a
// successful resolve/unresolve. Cache failures must not undo remote success.
func (o *Orchestrator) handleThreadActionSuccess(ctx context.Context, req *actions.ActionRequest, log *slog.Logger) {
	// A disconnected provider must not emit stale local state to the new session.
	if o.liveness != nil && o.liveness.Err() != nil {
		log.Debug("handleThreadActionSuccess: liveness cancelled, skipping local write/event", "thread_id", req.Body)
		return
	}
	resolved := req.Action == actions.ResolveThreadCommand
	threadID := req.Body
	if o.store != nil {
		if err := o.store.SetThreadResolved(ctx, req.PRID, threadID, resolved); err != nil {
			log.Warn("handleThreadActionSuccess: SetThreadResolved failed",
				"thread_id", threadID, "resolved", resolved, "error", err)
		}
	}
	if o.emitter != nil {
		eventType := "THREAD_RESOLVED"
		if !resolved {
			eventType = "THREAD_REOPENED"
		}
		o.emitter.Emit(&events.Event{
			Type: eventType,
			Data: map[string]interface{}{
				"thread_id": threadID, "pr_id": req.PRID, "resolved": resolved,
			},
			Timestamp: time.Now(),
		})
	}
}

// queuedActionID keeps thread-node IDs distinct and hashes user-authored text.
func queuedActionID(req *actions.ActionRequest) string {
	if req.Body == "" {
		return req.PRID + "-" + string(req.Action)
	}
	switch req.Action {
	case actions.ResolveThreadCommand, actions.UnresolveThreadCommand:
		return req.PRID + "-" + string(req.Action) + "-" + req.Body
	case actions.ReplyCommentCommand:
		h := sha256.Sum256([]byte(req.InReplyTo + "\x00" + req.Body))
		return req.PRID + "-" + string(req.Action) + "-" + hex.EncodeToString(h[:queuedIDHashBytes])
	default:
		h := sha256.Sum256([]byte(req.Body))
		return req.PRID + "-" + string(req.Action) + "-" + hex.EncodeToString(h[:queuedIDHashBytes])
	}
}

func accountScopedQueuedActionID(accountLogin, actionID string) string {
	h := sha256.Sum256([]byte(accountLogin))
	return provider.GitHub + "-" + hex.EncodeToString(h[:queuedIDHashBytes]) + "-" + actionID
}
