package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/itterno/bistro/internal/store/actionqueue"
	"time"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
)

// replayNeedsRemoteReconciliation identifies actions that may not be safe to
// repeat after an unknown remote outcome.
func replayNeedsRemoteReconciliation(action actions.ActionCommand) bool {
	switch action {
	case actions.ApproveCommand, actions.RequestChangesCommand, actions.PostCommentCommand, actions.ReplyCommentCommand:
		return true
	}
	return false
}

func replayTimestampMatches(remote time.Time, queuedAt time.Time) bool {
	return !remote.IsZero() && !remote.Before(queuedAt.Add(-replayReconciliationClockSkew))
}

func (o *Orchestrator) reconcileQueuedActionSuccess(ctx context.Context, action *actionqueue.Action, req *actions.ActionRequest) (bool, error) {
	if o.provider == nil {
		return false, nil
	}
	switch req.Action {
	case actions.ApproveCommand, actions.RequestChangesCommand:
		return o.reconcileQueuedReview(ctx, action, req)
	case actions.PostCommentCommand:
		return o.reconcileQueuedComment(ctx, action, req)
	case actions.ReplyCommentCommand:
		return o.reconcileQueuedReply(ctx, action, req)
	case actions.ResolveThreadCommand, actions.UnresolveThreadCommand:
		return o.reconcileQueuedThread(ctx, req)
	}
	return false, nil
}

func (o *Orchestrator) reconcileQueuedReview(ctx context.Context, action *actionqueue.Action, req *actions.ActionRequest) (bool, error) {
	username, err := o.provider.GetUsername(ctx)
	if err != nil {
		return false, fmt.Errorf("get username for review reconciliation: %w", err)
	}
	reviews, err := o.provider.ListReviews(ctx, req.PRID)
	if err != nil {
		return false, fmt.Errorf("list reviews for %s: %w", req.PRID, err)
	}
	wantState := "approved"
	if req.Action == actions.RequestChangesCommand {
		wantState = "changes_requested"
	}
	for _, review := range reviews {
		if review.Author == username && review.State == wantState && replayTimestampMatches(review.SubmittedAt, action.CreatedAt) {
			return true, nil
		}
	}
	return false, nil
}

func (o *Orchestrator) reconcileQueuedComment(ctx context.Context, action *actionqueue.Action, req *actions.ActionRequest) (bool, error) {
	username, err := o.provider.GetUsername(ctx)
	if err != nil {
		return false, fmt.Errorf("get username for comment reconciliation: %w", err)
	}
	comments, err := o.provider.GetComments(ctx, req.PRID)
	if err != nil {
		return false, fmt.Errorf("get comments for %s: %w", req.PRID, err)
	}
	for _, comment := range comments {
		if comment.Author == username && comment.Body == req.Body && replayTimestampMatches(comment.CreatedAt, action.CreatedAt) {
			return true, nil
		}
	}
	return false, nil
}

func (o *Orchestrator) reconcileQueuedReply(ctx context.Context, action *actionqueue.Action, req *actions.ActionRequest) (bool, error) {
	threadProvider, ok := o.provider.(provider.ThreadProvider)
	if !ok {
		return false, errors.New("provider does not support review-thread reconciliation")
	}
	username, err := o.provider.GetUsername(ctx)
	if err != nil {
		return false, fmt.Errorf("get username for reply reconciliation: %w", err)
	}
	threads, err := threadProvider.GetReviewThreads(ctx, req.PRID)
	if err != nil {
		return false, fmt.Errorf("get review threads for %s: %w", req.PRID, err)
	}
	for _, thread := range threads {
		for _, comment := range thread.Comments {
			if comment.Author != username || comment.Body != req.Body || !replayTimestampMatches(comment.CreatedAt, action.CreatedAt) {
				continue
			}
			if req.InReplyTo != "" && comment.InReplyTo == req.InReplyTo {
				return true, nil
			}
			if req.ThreadID != "" && thread.ID == req.ThreadID {
				return true, nil
			}
		}
	}
	return false, nil
}

func (o *Orchestrator) reconcileQueuedThread(ctx context.Context, req *actions.ActionRequest) (bool, error) {
	threadProvider, ok := o.provider.(provider.ThreadProvider)
	if !ok {
		return false, nil
	}
	threads, err := threadProvider.GetReviewThreads(ctx, req.PRID)
	if err != nil {
		return false, fmt.Errorf("get review threads for %s: %w", req.PRID, err)
	}
	wantResolved := req.Action == actions.ResolveThreadCommand
	for _, thread := range threads {
		if thread.ID == req.Body && thread.StateKnown && thread.Resolved == wantResolved {
			return true, nil
		}
	}
	return false, nil
}

// ReplayQueuedActions replays pending actions after a rate limit clears.
func (o *Orchestrator) ReplayQueuedActions(ctx context.Context) error {
	if o.queue == nil {
		o.log.Debug("skipping queue replay: no queue configured")
		return nil
	}
	// The replay lock prevents another orchestrator sharing this DB from resetting
	// our in-flight processing rows and repeating non-idempotent mutations.
	o.queue.LockReplay()
	defer o.queue.UnlockReplay()

	log := logging.FromContext(ctx).With("replay_batch", shortID())
	ctx = logging.WithLogger(ctx, log)
	reset, err := o.queue.ResetProcessing(ctx)
	if err != nil {
		return fmt.Errorf("reset processing queue rows: %w", err)
	}
	if reset > 0 {
		log.Warn("reset crash-orphaned processing queue rows before replay", "count", reset)
	}
	pending, err := o.pendingReplayActions(ctx)
	if err != nil {
		return err
	}
	log.Info("replaying queued actions", "count", len(pending))
	for _, action := range pending {
		o.replayQueuedAction(ctx, action)
	}
	return nil
}

func (o *Orchestrator) pendingReplayActions(ctx context.Context) ([]*actionqueue.Action, error) {
	if o.reviewLogin == nil {
		pending, err := o.queue.GetPending(ctx, replayBatchSize)
		if err != nil {
			return nil, fmt.Errorf("load pending queue: %w", err)
		}
		return pending, nil
	}
	login, ok, err := o.reviewLogin.GetCurrentUser(ctx, provider.GitHub)
	if err != nil {
		return nil, fmt.Errorf("resolve current account for queue replay: %w", err)
	}
	if !ok || login == "" {
		logging.FromContext(ctx).Info("skipping queue replay: no current GitHub account")
		return nil, nil
	}
	pending, err := o.queue.GetPendingForAccount(ctx, provider.GitHub, login, replayBatchSize)
	if err != nil {
		return nil, fmt.Errorf("load pending queue: %w", err)
	}
	return pending, nil
}

func (o *Orchestrator) replayQueuedAction(ctx context.Context, action *actionqueue.Action) {
	log := logging.FromContext(ctx)
	log.Info("replaying queued action", "id", action.ID, "pr_id", action.PRID,
		"action", action.ActionType, "retry_count", action.RetryCount)
	req, err := decodeQueuedAction(action)
	if err != nil {
		log.Warn("replay: invalid reply_comment payload, marking failed", "id", action.ID, "error", err)
		if markErr := o.queue.MarkFailed(ctx, action.ID, err.Error(), actionqueue.MaxRetryCount); markErr != nil {
			log.Error("failed to mark action failed after payload decode", "id", action.ID, "error", markErr)
		}
		return
	}
	// Queue replay bypasses ApplyAction; re-run its thread preflight.
	if preflight := o.validateThreadPreflight(ctx, req); preflight != nil {
		log.Warn("replay: thread preflight failed, marking failed", "id", action.ID, "reason", preflight.Error)
		if markErr := o.queue.MarkFailed(ctx, action.ID, preflight.Error, actionqueue.MaxRetryCount); markErr != nil {
			log.Error("failed to mark action failed after preflight", "id", action.ID, "error", markErr)
		}
		return
	}
	if !o.shouldExecuteQueuedAction(ctx, action, req) {
		return
	}
	if err := o.queue.MarkProcessing(ctx, action.ID); err != nil {
		log.Error("failed to mark queued action processing; skipping unsafe replay", "id", action.ID, "error", err)
		return
	}
	o.finishQueuedAction(ctx, action, o.executeAction(ctx, req))
}

func decodeQueuedAction(action *actionqueue.Action) (*actions.ActionRequest, error) {
	req := &actions.ActionRequest{PRID: action.PRID, Action: actions.ActionCommand(action.ActionType), Body: action.Payload}
	if req.Action != actions.ReplyCommentCommand {
		return req, nil
	}
	var payload actions.ReplyCommentPayload
	if err := json.Unmarshal([]byte(action.Payload), &payload); err != nil {
		return nil, err
	}
	req.Body, req.InReplyTo, req.ThreadID = payload.Body, payload.InReplyTo, payload.ThreadID
	return req, nil
}

func (o *Orchestrator) shouldExecuteQueuedAction(ctx context.Context, action *actionqueue.Action, req *actions.ActionRequest) bool {
	log := logging.FromContext(ctx)
	reconciled, reconcileErr := o.reconcileQueuedActionSuccess(ctx, action, req)
	if reconcileErr != nil {
		log.Warn("replay: remote reconciliation failed", "id", action.ID, "error", reconcileErr)
		if replayNeedsRemoteReconciliation(req.Action) {
			errMsg := fmt.Sprintf("remote reconciliation failed before non-idempotent replay: %v", reconcileErr)
			if err := o.queue.MarkFailed(ctx, action.ID, errMsg, action.RetryCount+1); err != nil {
				log.Error("failed to defer action after reconciliation error", "id", action.ID, "error", err)
			}
			return false
		}
		return true
	}
	if reconciled {
		log.Info("replay: queued action already present on GitHub, marking completed", "id", action.ID)
		if err := o.queue.MarkCompleted(ctx, action.ID); err != nil {
			log.Error("failed to mark reconciled action completed", "id", action.ID, "error", err)
		}
		return false
	}
	if replayNeedsRemoteReconciliation(req.Action) {
		log.Debug("replay: no prior GitHub success found, executing queued action", "id", action.ID)
	}
	return true
}

func (o *Orchestrator) finishQueuedAction(ctx context.Context, action *actionqueue.Action, result *actions.ActionResult) {
	log := logging.FromContext(ctx)
	if result.Success {
		// Preserve the replay context for completion; changing this changes the
		// pending/processing retry behavior and needs separate correctness review.
		if err := o.queue.MarkCompleted(ctx, action.ID); err != nil {
			log.Error("failed to mark action completed", "id", action.ID, "error", err)
		}
		return
	}
	errMsg := result.Error
	if errMsg == "" {
		errMsg = "unknown error"
	}
	retryCount := action.RetryCount + 1
	markCtx := ctx
	// An ambiguous provider result may mean GitHub already applied the mutation.
	// A detached terminal write avoids retrying it after caller cancellation.
	if result.IsAmbiguousRemoteSuccess() {
		log.Warn("replay: ambiguous remote success, marking terminal to prevent duplicate mutation",
			"id", action.ID, "action", action.ActionType)
		retryCount = actionqueue.MaxRetryCount
		markCtx = context.Background()
	}
	if err := o.queue.MarkFailed(markCtx, action.ID, errMsg, retryCount); err != nil {
		log.Error("failed to mark action failed", "id", action.ID, "error", err)
	}
}

// CleanupQueue removes old completed and failed actions from the queue.
func (o *Orchestrator) CleanupQueue(ctx context.Context) error {
	if o.queue == nil {
		return nil
	}
	deleted, err := o.queue.DeleteCompleted(ctx, completedQueueRetention)
	if err != nil {
		return fmt.Errorf("clean completed queue entries: %w", err)
	}
	logging.FromContext(ctx).Debug("cleaned up completed queue entries", "deleted", deleted)
	if err := o.queue.DeleteFailed(ctx, failedQueueRetention); err != nil {
		return fmt.Errorf("clean failed queue entries: %w", err)
	}
	return nil
}
