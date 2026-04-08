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

// ActionExecutor coordinates action execution with provider, store, and events.
type ActionExecutor struct {
	log      *slog.Logger
	provider provider.Provider
	store    *store.DB
	emitter  *sync.EventEmitter
}

// NewActionExecutor creates a new action executor.
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

// Execute performs a single action (no retries).
// Errors are returned inside ActionResult rather than as the error return value.
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

	return result, nil
}

// ExecuteWithRetry performs an action with automatic retries using exponential backoff.
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

	result := RetryWithBackoff(ctx, backoff, maxAttempts, func() (*ActionResult, error) {
		return ae.Execute(ctx, req)
	})

	return result, nil
}

// ExecuteMultiple performs multiple actions sequentially and returns all results.
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

// emitEvent broadcasts an action_complete event to all subscribers.
func (ae *ActionExecutor) emitEvent(req *ActionRequest, result *ActionResult) {
	ae.emitter.Emit(&sync.SyncEvent{
		Type: "action_complete",
		Data: map[string]interface{}{
			"pr":      req.PRID,
			"action":  req.Action,
			"success": result.Success,
			"error":   result.Error,
		},
		Timestamp: time.Now(),
	})
}
