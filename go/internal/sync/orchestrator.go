package sync

import (
	"context"
	"fmt"
	"time"

	"log/slog"

	"github.com/samuelasvanyi/bistro/internal/git"
	"github.com/samuelasvanyi/bistro/internal/provider"
	"github.com/samuelasvanyi/bistro/internal/store"
)

// Orchestrator coordinates sync operations
type Orchestrator struct {
	log      *slog.Logger
	provider provider.Provider
	store    *store.DB
	git      *git.Client
	emitter  *EventEmitter
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
