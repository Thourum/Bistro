package server

import (
	"context"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/server/handler"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/sync"
)

// teardownProviderLocked clears the active provider, orchestrator, and all
// associated per-provider state (event bridge, provCtx, activeProviderName).
// When name is non-empty, the teardown only proceeds if it matches the current
// activeProviderName — this prevents a disconnect for "github" from clearing a
// freshly-reconnected "github2" session, for example.
// Passing name="" unconditionally tears down (used by the external SetProvider(nil)).
// Must be called with a.provMu held for writing.
func (a *App) teardownProviderLocked(name string) {
	if name != "" && a.activeProviderName != name {
		return // named disconnect doesn't match current provider — no-op
	}
	a.prov = nil
	a.activeProviderName = ""
	a.orchestrator = nil
	// Cancel the per-provider context — stops sync runs, event bridge, and
	// in-flight actions that captured a reference before disconnect.
	if a.provCtxCancel != nil {
		a.provCtxCancel()
	}
	// Re-initialise to a non-cancelled context for the next connect.
	a.provCtx, a.provCtxCancel = context.WithCancel(context.Background())
	if a.bridgeCancel != nil {
		a.bridgeCancel()
		a.bridgeCancel = nil
	}
}

// attachOrchestratorLocked creates, wires, and stores a new orchestrator for
// the given provider, then starts the event bridge. Cancels any previous
// per-provider context first so all stale goroutines stop.
// runCtx is the context passed to Run(); it provides the lifetime ceiling for
// the new per-provider context. Must be called with a.provMu held for writing.
func (a *App) attachOrchestratorLocked(runCtx context.Context, prov provider.Provider, name string) {
	// Cancel previous per-provider context (stops stale syncs/bridge/actions).
	if a.provCtxCancel != nil {
		a.provCtxCancel()
	}
	a.provCtx, a.provCtxCancel = context.WithCancel(runCtx)

	diffStore := store.NewDiffStore(a.db.DB)
	a.orchestrator = sync.NewOrchestrator(a.log, prov, a.db, a.gitClient).
		WithDiffStore(diffStore).
		WithReviewTracking(a.currentUserStore, a.db).
		WithAnchorResolver(a.anchorResolver).
		WithActionQueue(store.NewActionQueue(a.db.DB)).
		WithLiveness(a.provCtx).
		WithClassifier(a.classifier).
		WithCurrentUser(a.currentUserStore).
		WithExpertise(a.expertiseEngine).
		WithRepoPathResolver(a.resolveRepoPath).
		WithPostCommit(func(prIDs []string) {
			if a.llmKick == nil {
				return
			}
			for _, prID := range prIDs {
				a.llmKick(prID)
			}
		}).
		WithPostReview(func(reviewCtx context.Context, prID string) {
			a.broadcastExpertiseNudge(reviewCtx, prID)
		})
	a.prov = prov
	a.activeProviderName = name
	a.startEventBridge(a.provCtx)
}

// SetProvider configures the GitHub provider used by the sync orchestrator.
// If the database and git client are already available, it also (re)creates the
// orchestrator so the sync loop can start working immediately after auth.
func (a *App) SetProvider(prov provider.Provider) {
	a.provMu.Lock()
	defer a.provMu.Unlock()

	if prov == nil {
		a.teardownProviderLocked("") // unconditional — external nil means disconnect
		return
	}

	// Record the provider name so AUTH_DISCONNECT can match it later.
	// This covers the keychain-startup path (main.go calls SetProvider before
	// Run) which otherwise leaves activeProviderName empty.
	a.prov = prov
	a.activeProviderName = "github"

	if a.db != nil {
		a.attachOrchestratorLocked(context.Background(), prov, "github")
	}
}

// getProvider safely returns the current provider under a read lock.
func (a *App) getProvider() provider.Provider {
	a.provMu.RLock()
	defer a.provMu.RUnlock()
	return a.prov
}

// currentProviderSnapshot pairs a provider with its account generation lifetime.
func (a *App) currentProviderSnapshot() (provider.Provider, context.Context) {
	a.provMu.RLock()
	defer a.provMu.RUnlock()
	return a.prov, a.provCtx
}

// currentApplier returns the live orchestrator as an ActionApplier under a
// read lock, or nil when no provider is authenticated. Satisfies
// handler.ApplierGetter so SUBMIT_ACTION always routes to the current
// orchestrator even after an auth/provider replacement.
// Both prov and orchestrator must be non-nil; either being nil (e.g. after
// AUTH_DISCONNECT) causes a nil return so the handler rejects cleanly.
func (a *App) currentApplier() handler.ActionApplier {
	a.provMu.RLock()
	defer a.provMu.RUnlock()
	if a.prov == nil || a.orchestrator == nil {
		return nil
	}
	return a.orchestrator
}

// currentPRSyncer returns the live orchestrator as a PRSyncer under a read
// lock, or nil when no provider is authenticated. Satisfies
// handler.PRSyncerGetter (same lifecycle rules as currentApplier).
func (a *App) currentPRSyncer() handler.PRSyncer {
	a.provMu.RLock()
	defer a.provMu.RUnlock()
	if a.prov == nil || a.orchestrator == nil {
		return nil
	}
	return a.orchestrator
}

func (a *App) initializeStartupProvider(ctx context.Context) {
	a.provMu.Lock()
	if a.prov != nil {
		a.attachOrchestratorLocked(ctx, a.prov, "github")
	}
	hasStartupProvider := a.prov != nil
	a.provMu.Unlock()
	if !hasStartupProvider {
		return
	}
	a.log.Info("orchestrator initialized at startup (token from keychain)")
	if !a.replayOnStart {
		return
	}
	a.log.Info("replaying offline action queue after startup")
	if err := a.ReplayQueuedActions(ctx); err != nil {
		a.log.Error("startup queue replay failed", "error", err)
	}
}

// SetReplayOnStart configures whether Run() should replay the offline action
// queue after the production orchestrator is attached (DB open, queue wired).
// This replaces pre-Run calls to ReplayQueuedActions which are always no-ops
// because the orchestrator does not exist before Run opens the database.
func (a *App) SetReplayOnStart(replay bool) {
	a.replayOnStart = replay
}

// ReplayQueuedActions replays any pending offline queue actions.
// It is a no-op when the orchestrator has not been set up.
// Prefer SetReplayOnStart so replay happens inside Run() after the database and
// queue are initialised; calling this before Run() is always a no-op.
func (a *App) ReplayQueuedActions(ctx context.Context) error {
	a.provMu.RLock()
	orchestrator := a.orchestrator
	a.provMu.RUnlock()
	if orchestrator == nil {
		return nil
	}
	return orchestrator.ReplayQueuedActions(ctx)
}
