package sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"log/slog"

	"github.com/samuelasvanyi/bistro/internal/actions"
	"github.com/samuelasvanyi/bistro/internal/git"
	"github.com/samuelasvanyi/bistro/internal/provider"
	"github.com/samuelasvanyi/bistro/internal/store"
)

// Orchestrator coordinates sync operations
type Orchestrator struct {
	log            *slog.Logger
	provider       provider.Provider
	store          *store.DB
	git            *git.Client
	emitter        *EventEmitter
	rateLimitState *provider.RateLimitState
	queue          *ActionQueue
}

// NewOrchestrator creates a new sync orchestrator
func NewOrchestrator(
	log *slog.Logger,
	prov provider.Provider,
	db *store.DB,
	gitClient *git.Client,
) *Orchestrator {
	return &Orchestrator{
		log:      log,
		provider: prov,
		store:    db,
		git:      gitClient,
		emitter:  NewEventEmitter(),
	}
}

// IsRateLimited reports whether the orchestrator is currently rate limited.
func (o *Orchestrator) IsRateLimited() bool {
	if o.rateLimitState == nil {
		return false
	}
	return o.rateLimitState.IsLimited()
}

// ApplyAction executes an action immediately or queues it when rate limited.
func (o *Orchestrator) ApplyAction(ctx context.Context, req *actions.ActionRequest) *actions.ActionResult {
	if o.IsRateLimited() {
		if o.log != nil {
			o.log.Info("rate limited, queueing action", "pr_id", req.PRID)
		}

		queuedAction := &QueuedAction{
			ID:         req.PRID + "-" + string(req.Action),
			ActionType: string(req.Action),
			PRID:       req.PRID,
			Payload:    req.Body,
		}
		if o.queue != nil {
			if err := o.queue.Enqueue(ctx, queuedAction); err != nil {
				if o.log != nil {
					o.log.Error("failed to queue action", "error", err)
				}
				return &actions.ActionResult{
					PRID:      req.PRID,
					Action:    req.Action,
					Success:   false,
					Error:     err.Error(),
					Timestamp: time.Now(),
				}
			}
		}
		return &actions.ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     "rate limited: action queued for later replay",
			Timestamp: time.Now(),
		}
	}

	return o.executeAction(ctx, req)
}

// executeAction runs an action via the executor immediately.
func (o *Orchestrator) executeAction(ctx context.Context, req *actions.ActionRequest) *actions.ActionResult {
	executor := actions.NewActionExecutor(o.log, o.provider, o.store, o.emitter)
	result, err := executor.ExecuteWithRetry(ctx, req, 3)
	if err != nil {
		if o.log != nil {
			o.log.Error("action execution error", "pr", req.PRID, "error", err)
		}
		return &actions.ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     err.Error(),
			Timestamp: time.Now(),
		}
	}
	if result == nil {
		return &actions.ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     "executor returned nil result",
			Timestamp: time.Now(),
		}
	}
	return result
}

// ReplayQueuedActions replays pending actions after a rate limit clears.
func (o *Orchestrator) ReplayQueuedActions(ctx context.Context) error {
	if o.queue == nil {
		return nil
	}

	pending, err := o.queue.GetPending(ctx, 10)
	if err != nil {
		return err
	}

	for _, action := range pending {
		if o.log != nil {
			o.log.Info("replaying queued action", "id", action.ID)
		}

		req := &actions.ActionRequest{
			PRID:   action.PRID,
			Action: actions.ActionCommand(action.ActionType),
			Body:   action.Payload,
		}

		result := o.executeAction(ctx, req)
		if result.Success {
			if err := o.queue.MarkCompleted(ctx, action.ID); err != nil && o.log != nil {
				o.log.Error("failed to mark action completed", "id", action.ID, "error", err)
			}
		} else {
			errMsg := result.Error
			if errMsg == "" {
				errMsg = "unknown error"
			}
			if err := o.queue.MarkFailed(ctx, action.ID, errMsg, action.RetryCount+1); err != nil && o.log != nil {
				o.log.Error("failed to mark action failed", "id", action.ID, "error", err)
			}
		}
	}

	return nil
}

// CleanupQueue removes old completed and failed actions from the queue.
func (o *Orchestrator) CleanupQueue(ctx context.Context) error {
	if o.queue == nil {
		return nil
	}

	deleted, err := o.queue.DeleteCompleted(ctx, 7*24*time.Hour)
	if err != nil {
		return err
	}
	if o.log != nil {
		o.log.Debug("cleaned up queue", "deleted", deleted)
	}

	return o.queue.DeleteFailed(ctx, 24*time.Hour)
}

// isRateLimitError reports whether err signals an HTTP 429 rate limit.
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	if pe, ok := err.(*provider.ProviderError); ok {
		return pe.Type == provider.ErrorTypeRateLimit
	}
	msg := err.Error()
	return strings.Contains(msg, "429") || strings.Contains(msg, "rate limit")
}

// GetEventChannel returns a read-only channel for subscribing to sync events.
// The channel is buffered; the caller should drain it promptly to avoid dropped events.
func (o *Orchestrator) GetEventChannel() <-chan *SyncEvent {
	return o.emitter.Subscribe()
}

// Sync executes a full sync operation
func (o *Orchestrator) Sync(ctx context.Context, req *SyncRequest) (*SyncResult, error) {
	if req == nil {
		return nil, fmt.Errorf("sync request cannot be nil")
	}

	if req.Provider == "" || req.Owner == "" || req.Repo == "" {
		return nil, fmt.Errorf("provider, owner, and repo are required")
	}

	startTime := time.Now()
	result := &SyncResult{
		Errors: make([]string, 0),
	}

	if o.log != nil {
		o.log.Info("sync started", "provider", req.Provider, "owner", req.Owner, "repo", req.Repo)
	}
	o.emitter.Emit(&SyncEvent{
		Type: "sync_started",
		Data: map[string]interface{}{
			"provider": req.Provider,
			"owner":    req.Owner,
			"repo":     req.Repo,
		},
		Timestamp: time.Now(),
	})

	// Phase 1: Fetch PR list
	var fetchedPRs []*PRFetchResult
	if req.IncludePRs {
		o.emitter.Emit(NewProgressEvent("fetching_prs", 0, 0))

		prs, err := o.syncPRList(ctx, req)
		if err != nil {
			errMsg := fmt.Sprintf("failed to sync PRs: %v", err)
			result.Errors = append(result.Errors, errMsg)
			if o.log != nil {
				o.log.Error("sync failed", "err", err)
			}
			o.emitter.Emit(NewErrorEvent(errMsg))
		} else {
			result.PRCount = len(prs)
			fetchedPRs = make([]*PRFetchResult, len(prs))
			for i, pr := range prs {
				fetchedPRs[i] = &PRFetchResult{PR: pr}
			}
		}
	}

	// Phase 2: Fetch details concurrently (comments, CI, diffs)
	if len(fetchedPRs) > 0 {
		o.emitter.Emit(NewProgressEvent("fetching_details", 0, len(fetchedPRs)))

		detailResults, err := o.SyncPRsWithDetails(ctx, req)
		if err != nil {
			errMsg := fmt.Sprintf("failed to fetch details: %v", err)
			result.Errors = append(result.Errors, errMsg)
			if o.log != nil {
				o.log.Error("detail fetch failed", "err", err)
			}
			o.emitter.Emit(NewErrorEvent(errMsg))
		} else {
			fetchedPRs = detailResults
			for _, r := range detailResults {
				if r.Error == nil {
					result.UpdatedPRs++
					result.CommentCount += len(r.Comments)
				} else {
					result.Errors = append(result.Errors, r.Error.Error())
				}
			}
		}
	}

	// Phase 3: Write to SQLite (batched transactions)
	if len(fetchedPRs) > 0 {
		o.emitter.Emit(NewProgressEvent("writing_database", 0, len(fetchedPRs)))

		writer := NewWriter(o.log, o.store)
		if err := writer.WriteSyncResults(fetchedPRs); err != nil {
			errMsg := fmt.Sprintf("failed to write results: %v", err)
			result.Errors = append(result.Errors, errMsg)
			if o.log != nil {
				o.log.Error("write failed", "err", err)
			}
			o.emitter.Emit(NewErrorEvent(errMsg))
		}
	}

	result.Duration = time.Since(startTime)
	if o.log != nil {
		o.log.Info("sync complete",
			"duration", result.Duration,
			"prs", result.PRCount,
			"errors", len(result.Errors))
	}

	o.emitter.Emit(NewCompleteEvent(
		result.PRCount,
		result.NewPRs,
		result.UpdatedPRs,
		len(result.Errors),
		result.Duration,
	))

	return result, nil
}

// ApplyActions executes a batch of PR actions with events and retry.
func (o *Orchestrator) ApplyActions(ctx context.Context, requests []*actions.ActionRequest) []*actions.ActionResult {
	if o.log != nil {
		o.log.Info("applying actions", "count", len(requests))
	}

	o.emitter.Emit(&SyncEvent{
		Type: "actions_started",
		Data: map[string]interface{}{
			"count": len(requests),
		},
		Timestamp: time.Now(),
	})

	executor := actions.NewActionExecutor(o.log, o.provider, o.store, o.emitter)

	results := make([]*actions.ActionResult, 0, len(requests))
	for _, req := range requests {
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
			"total":   len(results),
			"success": successCount,
			"failed":  len(results) - successCount,
		},
		Timestamp: time.Now(),
	})

	return results
}

// syncPRList fetches and stores the PR list
func (o *Orchestrator) syncPRList(ctx context.Context, req *SyncRequest) ([]*provider.PR, error) {
	if o.provider == nil {
		return nil, fmt.Errorf("provider not configured")
	}

	filters := provider.PRFilters{
		State: req.State,
		Limit: 100,
	}

	prs, err := o.provider.ListPRs(ctx, filters)
	if err != nil {
		if isRateLimitError(err) {
			if pe, ok := err.(*provider.ProviderError); ok {
				_ = pe // ProviderError does not carry RateLimitState directly; use type to set state
			}
			// Record a rate limit state so subsequent calls are gated
			o.rateLimitState = &provider.RateLimitState{
				ResetAt: time.Now().Add(60 * time.Second),
			}
			if o.log != nil {
				o.log.Warn("rate limited", "reset_at", o.rateLimitState.ResetAt)
			}
			o.emitter.Emit(&SyncEvent{
				Type: "rate_limited",
				Data: map[string]interface{}{
					"reset_in": o.rateLimitState.ResetIn().Seconds(),
				},
				Timestamp: time.Now(),
			})
			return nil, err
		}
		return nil, fmt.Errorf("failed to list PRs: %w", err)
	}

	if o.log != nil {
		o.log.Info("prs fetched", "count", len(prs))
	}

	return prs, nil
}

// SyncWithProgress executes sync with progress callback
func (o *Orchestrator) SyncWithProgress(
	ctx context.Context,
	req *SyncRequest,
	onProgress func(*SyncProgress),
) (*SyncResult, error) {
	if onProgress == nil {
		return o.Sync(ctx, req)
	}

	onProgress(&SyncProgress{
		Stage: "starting",
	})

	result, err := o.Sync(ctx, req)

	if err == nil {
		onProgress(&SyncProgress{
			Stage:   "complete",
			Total:   result.PRCount,
			Current: result.PRCount,
		})
	}

	return result, err
}
