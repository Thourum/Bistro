package sync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	gosync "sync"
	"time"

	"log/slog"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

// Orchestrator coordinates sync operations
type Orchestrator struct {
	log            *slog.Logger
	provider       provider.Provider
	store          *store.DB
	git            *git.Client
	emitter        *events.EventEmitter
	rateLimitMu    gosync.RWMutex
	rateLimitState *provider.RateLimitState
	queue          *store.ActionQueue
	diffStore      *store.DiffStore
}

// NewOrchestrator creates a new sync orchestrator. If log is nil, slog.Default()
// is used — downstream code can rely on o.log being non-nil.
func NewOrchestrator(
	log *slog.Logger,
	prov provider.Provider,
	db *store.DB,
	gitClient *git.Client,
) *Orchestrator {
	if log == nil {
		log = slog.Default()
	}
	return &Orchestrator{
		log:      log,
		provider: prov,
		store:    db,
		git:      gitClient,
		emitter:  events.NewEventEmitter(),
	}
}

// shortID returns a random hex identifier for correlating a single sync cycle
// or action batch across log records.
func shortID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("t%d", time.Now().UnixNano()&0xffffffff)
	}
	return hex.EncodeToString(b[:])
}

// WithDiffStore attaches a DiffStore so diffs are persisted during sync.
// This is optional; if not set, diffs are generated but not stored.
func (o *Orchestrator) WithDiffStore(ds *store.DiffStore) *Orchestrator {
	o.diffStore = ds
	return o
}

// IsRateLimited reports whether the orchestrator is currently rate limited.
func (o *Orchestrator) IsRateLimited() bool {
	o.rateLimitMu.RLock()
	defer o.rateLimitMu.RUnlock()
	if o.rateLimitState == nil {
		return false
	}
	return o.rateLimitState.IsLimited()
}

// ApplyAction executes an action immediately or queues it when rate limited.
func (o *Orchestrator) ApplyAction(ctx context.Context, req *actions.ActionRequest) *actions.ActionResult {
	log := logging.FromContext(ctx).With(
		"pr_id", req.PRID,
		"action", string(req.Action),
	)
	ctx = logging.WithLogger(ctx, log)

	if o.IsRateLimited() {
		log.Info("rate limited, queueing action")

		queuedAction := &store.QueuedAction{
			ID:         req.PRID + "-" + string(req.Action),
			ActionType: string(req.Action),
			PRID:       req.PRID,
			Payload:    req.Body,
		}
		if o.queue != nil {
			if err := o.queue.Enqueue(ctx, queuedAction); err != nil {
				log.Error("failed to queue action", "error", err)
				return &actions.ActionResult{
					PRID:      req.PRID,
					Action:    req.Action,
					Success:   false,
					Error:     err.Error(),
					Timestamp: time.Now(),
				}
			}
			log.Debug("action queued", "queue_id", queuedAction.ID)
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
	log := logging.FromContext(ctx)
	log.Debug("executing action", "pr_id", req.PRID, "action", string(req.Action))

	executor := actions.NewActionExecutor(o.log, o.provider, o.store, o.emitter)
	start := time.Now()
	result, err := executor.ExecuteWithRetry(ctx, req, 3)
	dur := time.Since(start).Milliseconds()

	if err != nil {
		log.Error("action execution error",
			"pr_id", req.PRID,
			"action", string(req.Action),
			"duration_ms", dur,
			"error", err,
		)
		return &actions.ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     err.Error(),
			Timestamp: time.Now(),
		}
	}
	if result == nil {
		log.Error("action executor returned nil result",
			"pr_id", req.PRID,
			"action", string(req.Action),
			"duration_ms", dur,
		)
		return &actions.ActionResult{
			PRID:      req.PRID,
			Action:    req.Action,
			Success:   false,
			Error:     "executor returned nil result",
			Timestamp: time.Now(),
		}
	}
	log.Debug("action executed",
		"pr_id", req.PRID,
		"action", string(req.Action),
		"duration_ms", dur,
		"success", result.Success,
	)
	return result
}

// ReplayQueuedActions replays pending actions after a rate limit clears.
func (o *Orchestrator) ReplayQueuedActions(ctx context.Context) error {
	if o.queue == nil {
		o.log.Debug("skipping queue replay: no queue configured")
		return nil
	}

	batchID := shortID()
	log := logging.FromContext(ctx).With("replay_batch", batchID)
	ctx = logging.WithLogger(ctx, log)

	pending, err := o.queue.GetPending(ctx, 10)
	if err != nil {
		log.Error("failed to load pending queue", "error", err)
		return err
	}

	log.Info("replaying queued actions", "count", len(pending))

	for _, action := range pending {
		log.Info("replaying queued action",
			"id", action.ID,
			"pr_id", action.PRID,
			"action", action.ActionType,
			"retry_count", action.RetryCount,
		)

		req := &actions.ActionRequest{
			PRID:   action.PRID,
			Action: actions.ActionCommand(action.ActionType),
			Body:   action.Payload,
		}

		result := o.executeAction(ctx, req)
		if result.Success {
			if err := o.queue.MarkCompleted(ctx, action.ID); err != nil {
				log.Error("failed to mark action completed", "id", action.ID, "error", err)
			}
		} else {
			errMsg := result.Error
			if errMsg == "" {
				errMsg = "unknown error"
			}
			if err := o.queue.MarkFailed(ctx, action.ID, errMsg, action.RetryCount+1); err != nil {
				log.Error("failed to mark action failed", "id", action.ID, "error", err)
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

	log := logging.FromContext(ctx)
	deleted, err := o.queue.DeleteCompleted(ctx, 7*24*time.Hour)
	if err != nil {
		log.Error("queue cleanup (completed) failed", "error", err)
		return err
	}
	log.Debug("cleaned up completed queue entries", "deleted", deleted)

	if err := o.queue.DeleteFailed(ctx, 24*time.Hour); err != nil {
		log.Error("queue cleanup (failed) failed", "error", err)
		return err
	}
	return nil
}

// isRateLimitError reports whether err signals an HTTP 429 rate limit.
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	var pe *provider.ProviderError
	if errors.As(err, &pe) {
		return pe.Type == provider.ErrorTypeRateLimit
	}
	return false
}

// GetEventChannel returns a read-only channel for subscribing to sync events.
// The channel is buffered; the caller should drain it promptly to avoid dropped events.
func (o *Orchestrator) GetEventChannel() <-chan *events.Event {
	return o.emitter.Subscribe()
}

// Sync executes a full sync operation. All log records emitted downstream are
// tagged with a per-cycle sync_id plus the repo slug so a full sync can be
// grep'd out of interleaved output.
func (o *Orchestrator) Sync(ctx context.Context, req *SyncRequest) (*SyncResult, error) {
	if req == nil {
		return nil, fmt.Errorf("sync request cannot be nil")
	}

	if req.Provider == "" || req.Owner == "" || req.Repo == "" {
		return nil, fmt.Errorf("provider, owner, and repo are required")
	}

	syncID := shortID()
	repoSlug := req.Owner + "/" + req.Repo
	log := logging.FromContext(ctx).With(
		"sync_id", syncID,
		"repo", repoSlug,
		"provider", req.Provider,
	)
	ctx = logging.WithLogger(ctx, log)

	startTime := time.Now()
	result := &SyncResult{
		Errors: make([]string, 0),
	}

	log.Info("sync started")
	o.emitter.Emit(&events.Event{
		Type: "sync_started",
		Data: map[string]interface{}{
			"provider": req.Provider,
			"owner":    req.Owner,
			"repo":     req.Repo,
			"sync_id":  syncID,
		},
		Timestamp: time.Now(),
	})

	// Phase 1: Fetch PR list
	var fetchedPRs []*PRFetchResult
	if req.IncludePRs {
		log.Debug("phase list-prs start")
		phaseStart := time.Now()
		o.emitter.Emit(events.NewProgressEvent("fetching_prs", 0, 0))

		prs, err := o.syncPRList(ctx, req)
		phaseDur := time.Since(phaseStart).Milliseconds()
		if err != nil {
			errMsg := fmt.Sprintf("failed to sync PRs: %v", err)
			result.Errors = append(result.Errors, errMsg)
			log.Error("phase list-prs failed", "duration_ms", phaseDur, "error", err)
			o.emitter.Emit(events.NewErrorEvent(errMsg))
		} else {
			result.PRCount = len(prs)
			fetchedPRs = make([]*PRFetchResult, len(prs))
			for i, pr := range prs {
				fetchedPRs[i] = &PRFetchResult{PR: pr}
			}
			log.Debug("phase list-prs done",
				"count", len(prs),
				"duration_ms", phaseDur,
			)
		}
	} else {
		log.Debug("phase list-prs skipped (include_prs=false)")
	}

	// Phase 2: Fetch details concurrently (comments, CI, diffs)
	if len(fetchedPRs) > 0 {
		log.Debug("phase fetch-details start",
			"pr_count", len(fetchedPRs),
			"include_comments", req.IncludeComments,
			"include_ci", req.IncludeCI,
			"include_diff", req.IncludeDiff,
		)
		phaseStart := time.Now()
		o.emitter.Emit(events.NewProgressEvent("fetching_details", 0, len(fetchedPRs)))

		// Extract the PR slice from the Phase 1 results to avoid a second ListPRs call.
		prSlice := make([]*provider.PR, len(fetchedPRs))
		for i, r := range fetchedPRs {
			prSlice[i] = r.PR
		}
		detailResults, err := o.FetchPRDetails(ctx, prSlice, req.IncludeComments, req.IncludeCI, req.IncludeDiff)
		phaseDur := time.Since(phaseStart).Milliseconds()
		if err != nil {
			errMsg := fmt.Sprintf("failed to fetch details: %v", err)
			result.Errors = append(result.Errors, errMsg)
			log.Error("phase fetch-details failed", "duration_ms", phaseDur, "error", err)
			o.emitter.Emit(events.NewErrorEvent(errMsg))
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
			log.Debug("phase fetch-details done",
				"duration_ms", phaseDur,
				"pr_ok", result.UpdatedPRs,
				"pr_errors", len(detailResults)-result.UpdatedPRs,
			)
		}
	}

	// Phase 3: Write to SQLite (batched transactions)
	if len(fetchedPRs) > 0 {
		log.Debug("phase write-db start", "pr_count", len(fetchedPRs))
		phaseStart := time.Now()
		o.emitter.Emit(events.NewProgressEvent("writing_database", 0, len(fetchedPRs)))

		writer := NewWriter(o.log, o.store)
		if err := writer.WriteSyncResultsContext(ctx, fetchedPRs); err != nil {
			errMsg := fmt.Sprintf("failed to write results: %v", err)
			result.Errors = append(result.Errors, errMsg)
			log.Error("phase write-db failed",
				"duration_ms", time.Since(phaseStart).Milliseconds(),
				"error", err,
			)
			o.emitter.Emit(events.NewErrorEvent(errMsg))
		} else {
			log.Debug("phase write-db done",
				"duration_ms", time.Since(phaseStart).Milliseconds(),
			)
		}
	}

	result.Duration = time.Since(startTime)
	log.Info("sync complete",
		"duration_ms", result.Duration.Milliseconds(),
		"prs_total", result.PRCount,
		"prs_updated", result.UpdatedPRs,
		"comments", result.CommentCount,
		"errors", len(result.Errors),
	)

	o.emitter.Emit(events.NewCompleteEvent(
		result.PRCount,
		result.NewPRs,
		result.UpdatedPRs,
		len(result.Errors),
		result.Duration,
	))

	return result, nil
}

// syncPRList fetches and stores the PR list
func (o *Orchestrator) syncPRList(ctx context.Context, req *SyncRequest) ([]*provider.PR, error) {
	log := logging.FromContext(ctx)
	if o.provider == nil {
		log.Error("syncPRList: provider not configured")
		return nil, fmt.Errorf("provider not configured")
	}

	filters := provider.PRFilters{
		Owner: req.Owner,
		Repo:  req.Repo,
		State: req.State,
		Limit: 100,
	}

	log.Debug("calling provider.ListPRs",
		"state", filters.State,
		"limit", filters.Limit,
	)
	start := time.Now()
	prs, err := o.provider.ListPRs(ctx, filters)
	dur := time.Since(start).Milliseconds()
	if err != nil {
		if isRateLimitError(err) {
			state := &provider.RateLimitState{
				ResetAt: time.Now().Add(60 * time.Second),
			}
			o.rateLimitMu.Lock()
			o.rateLimitState = state
			o.rateLimitMu.Unlock()
			log.Warn("rate limited",
				"reset_at", state.ResetAt,
				"duration_ms", dur,
			)
			o.emitter.Emit(&events.Event{
				Type: "rate_limited",
				Data: map[string]interface{}{
					"reset_in": state.ResetIn().Seconds(),
				},
				Timestamp: time.Now(),
			})
			return nil, err
		}
		log.Error("provider.ListPRs failed",
			"duration_ms", dur,
			"state", filters.State,
			"error", err,
		)
		return nil, fmt.Errorf("failed to list PRs: %w", err)
	}

	log.Info("prs fetched", "count", len(prs), "duration_ms", dur)

	return prs, nil
}

