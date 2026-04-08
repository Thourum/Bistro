// go/internal/actions/executor.go
package actions

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/samuelasvanyi/bistro/internal/events"
	"github.com/samuelasvanyi/bistro/internal/provider"
	"github.com/samuelasvanyi/bistro/internal/store"
)

// ActionExecutor coordinates action execution with provider, store, and events.
type ActionExecutor struct {
	log      *slog.Logger
	provider provider.Provider
	store    *store.DB
	emitter  *events.EventEmitter
}

// NewActionExecutor creates a new action executor.
func NewActionExecutor(
	log *slog.Logger,
	prov provider.Provider,
	db *store.DB,
	emitter *events.EventEmitter,
) *ActionExecutor {
	return &ActionExecutor{
		log:      log,
		provider: prov,
		store:    db,
		emitter:  emitter,
	}
}

// Execute performs a single action (no retries).
// Errors are returned inside ActionResult rather than as the error return value.
func (ae *ActionExecutor) Execute(ctx context.Context, req *ActionRequest) (*ActionResult, error) {
	if req == nil {
		return &ActionResult{
			Success:   false,
			Error:     "nil request",
			Timestamp: time.Now(),
		}, nil
	}
	if err := req.Validate(); err != nil {
		return &ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     fmt.Sprintf("validation failed: %v", err),
			Timestamp: time.Now(),
		}, nil
	}

	handler, ok := ae.handlerFor(req.Action)
	if !ok {
		return &ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     fmt.Sprintf("unknown action: %s", req.Action),
			Timestamp: time.Now(),
		}, nil
	}

	result, err := handler.Handle(ctx, req)
	if err != nil {
		result = &ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     fmt.Sprintf("handler error: %v", err),
			Timestamp: time.Now(),
		}
	}

	ae.logResult(req, result)
	ae.emitEvent(req, result)

	if result.Success {
		ae.updatePRReviewStatus(ctx, req)
	}

	return result, nil
}

// ExecuteWithRetry performs an action with automatic retries using exponential backoff.
func (ae *ActionExecutor) ExecuteWithRetry(ctx context.Context, req *ActionRequest, maxAttempts int) (*ActionResult, error) {
	if req == nil {
		return &ActionResult{
			Success:   false,
			Error:     "nil request",
			Timestamp: time.Now(),
		}, nil
	}
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
			results[i] = &ActionResult{
				Success:   false,
				Error:     "nil request",
				Timestamp: time.Now(),
			}
			continue
		}
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

// handlerFor returns the appropriate Handler for the given action, or false if unknown.
func (ae *ActionExecutor) handlerFor(action ActionCommand) (Handler, bool) {
	switch action {
	case ApproveCommand:
		return NewApproveHandler(ae.provider), true
	case RequestChangesCommand:
		return NewRequestChangesHandler(ae.provider), true
	case PostCommentCommand:
		return NewPostCommentHandler(ae.provider), true
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
	ae.emitter.Emit(&events.Event{
		Type: "action_complete",
		Data: map[string]interface{}{
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
			Data: map[string]interface{}{
				"pr_id": req.PRID,
			},
			Timestamp: time.Now(),
		})
	case RequestChangesCommand:
		ae.emitter.Emit(&events.Event{
			Type: "changes_requested",
			Data: map[string]interface{}{
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
			if ae.log != nil {
				ae.log.Error("failed to update review_status to approved", "pr_id", req.PRID, "error", err)
			}
		}
	case RequestChangesCommand:
		const query = `
			UPDATE prs
			SET review_status = 'changes_requested', changes_requested_by = ?
			WHERE id = ?
		`
		if _, err := ae.store.ExecContext(ctx, query, "", req.PRID); err != nil {
			if ae.log != nil {
				ae.log.Error("failed to update review_status to changes_requested", "pr_id", req.PRID, "error", err)
			}
		}
	}
}
