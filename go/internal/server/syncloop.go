package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/itterno/bistro/internal/sync"
)

// broadcastSyncError surfaces a sync failure to every connected UI instead of
// leaving it buried in the logs.
func (a *App) broadcastSyncError(repoID string, err error) {
	a.wsServer.Broadcast(map[string]any{
		"type": "SYNC_ERROR",
		"data": map[string]any{
			"error_code": "SYNC_FAILED",
			"message":    fmt.Sprintf("sync failed for %s: %v", repoID, err),
		},
	})
}

type syncLoopSettings struct {
	ticker   *time.Ticker
	autoSync bool
	interval time.Duration
}

func (a *App) applySyncSettings(settings *syncLoopSettings, latest syncSettingsMsg) {
	for {
		select {
		case msg := <-a.syncSettings:
			latest = msg
		default:
			a.log.Info("sync settings updated", "autoSync", latest.AutoSync, "interval", latest.Interval.String())
			settings.autoSync = latest.AutoSync
			settings.interval = latest.Interval
			if settings.autoSync && settings.interval > 0 {
				settings.ticker.Reset(settings.interval)
			} else {
				settings.ticker.Stop()
			}
			return
		}
	}
}

// applyPendingSyncSettings drains and applies the most recent settings message.
// It must only be called from the sync loop goroutine.
func (a *App) applyPendingSyncSettings(settings *syncLoopSettings) bool {
	select {
	case msg := <-a.syncSettings:
		a.applySyncSettings(settings, msg)
		return true
	default:
		return false
	}
}

// runSyncLoop runs the periodic sync goroutine. It exits when ctx is cancelled.
func (a *App) runSyncLoop(ctx context.Context) {
	settings := syncLoopSettings{
		ticker:   time.NewTicker(a.syncInterval),
		autoSync: a.autoSync,
		interval: a.syncInterval,
	}
	if !settings.autoSync {
		settings.ticker.Stop()
	}
	defer settings.ticker.Stop()

	a.log.Debug("sync loop started", "interval", settings.interval.String(), "autoSync", settings.autoSync)

	for {
		// Apply pending settings before a tick, including updates that arrived
		// while a sync pass was running.
		if err := ctx.Err(); err != nil {
			a.log.Debug("sync loop stopping", "reason", err)
			return
		}
		if a.applyPendingSyncSettings(&settings) {
			continue
		}
		if a.pendingWatchSync.Swap(false) {
			a.syncPendingWatchedRepos(ctx)
			continue
		}
		select {
		case <-ctx.Done():
			a.log.Debug("sync loop stopping", "reason", ctx.Err())
			return
		case msg := <-a.syncSettings:
			a.applySyncSettings(&settings, msg)
		case <-settings.ticker.C:
			a.applyPendingSyncSettings(&settings)
			if !settings.autoSync {
				continue
			}
			a.log.Debug("sync tick fired", "next_tick_in", settings.interval.String())
			a.syncWatchedRepos(ctx)
			a.applyPendingSyncSettings(&settings)
		case repoID := <-a.syncTrigger:
			a.log.Debug("sync trigger received", "repo_id", repoID)
			if repoID == "" {
				a.syncWatchedRepos(ctx)
			} else {
				a.syncSingleRepo(ctx, repoID)
			}
			a.applyPendingSyncSettings(&settings)
		}
	}
}

// Overflowed watch requests must sync watched repos, not just GitHub's inbox
// discovery lane: a newly watched repo may have no PR involving the user yet.
func (a *App) syncPendingWatchedRepos(ctx context.Context) {
	repos, err := a.listWatchedRepos(ctx)
	if err != nil {
		a.log.Error("read pending watched repos failed", "err", err)
		return
	}
	for _, repo := range repos {
		if ctx.Err() != nil {
			return
		}
		a.syncSingleRepo(ctx, repo.id)
	}
}

// syncWatchedRepos syncs all watched repos. No-ops if not yet authenticated.
func (a *App) syncWatchedRepos(ctx context.Context) {
	a.provMu.RLock()
	orch := a.orchestrator
	prov := a.prov
	orchCtx := a.provCtx // per-provider context; cancelled on disconnect/replace
	a.provMu.RUnlock()

	if prov == nil || orch == nil {
		return
	}

	if orch.SupportsInboxDiscovery() {
		result, err := orch.DiscoverAndSyncInbox(orchCtx)
		if err != nil {
			a.log.Error("syncWatchedRepos: inbox discovery failed", "err", err)
			if orchCtx.Err() == nil {
				a.broadcastSyncError("github:inbox", err)
			}
			return
		} else if result != nil && result.PRCount > 0 {
			a.log.Info("syncWatchedRepos: inbox discovery hydrated",
				"prs", result.PRCount,
				"updated", result.UpdatedPRs,
				"comments", result.CommentCount,
			)
		}
		if orchCtx.Err() == nil {
			a.wsServer.Broadcast(map[string]any{
				"type": "SYNC_COMPLETE",
				"data": map[string]any{"scope": "inbox"},
			})
		}
		return
	}

	// Read the full repo list and close the result set BEFORE syncing.
	// Keeping rows open across orch.Sync deadlocks the connection pool:
	// the open result set holds a connection while the sync's write
	// transaction waits for one.
	repos, err := a.listWatchedRepos(ctx)
	if err != nil {
		a.log.Error("syncWatchedRepos: query failed", "err", err)
		return
	}

	passStart := time.Now()
	var synced, failed int
	for _, r := range repos {
		// Abort the whole pass if the provider was disconnected/replaced.
		if orchCtx.Err() != nil {
			a.log.Debug("syncWatchedRepos: provider changed, aborting pass")
			return
		}
		req := &sync.SyncRequest{
			Provider:        r.provider,
			Owner:           r.owner,
			Repo:            r.repo,
			IncludePRs:      true,
			IncludeComments: true,
			IncludeDiff:     true,
			State:           "open",
		}
		repoStart := time.Now()
		result, syncErr := orch.Sync(orchCtx, req)
		if syncErr != nil {
			failed++
			a.log.Error("syncWatchedRepos: sync failed",
				"repo", r.id,
				"duration_ms", time.Since(repoStart).Milliseconds(),
				"err", syncErr,
			)
			if orchCtx.Err() == nil {
				a.broadcastSyncError(r.id, syncErr)
			}
			continue
		}
		synced++
		a.log.Info("syncWatchedRepos: synced",
			"repo", r.id,
			"prs", result.PRCount,
			"duration_ms", time.Since(repoStart).Milliseconds(),
		)
	}

	a.log.Info("sync pass complete",
		"synced", synced,
		"failed", failed,
		"duration_ms", time.Since(passStart).Milliseconds(),
	)

	// Only broadcast SYNC_COMPLETE if still on the same provider.
	if orchCtx.Err() == nil {
		a.wsServer.Broadcast(map[string]any{
			"type": "SYNC_COMPLETE",
			"data": map[string]any{},
		})
	}
}

type watchedRepo struct {
	id       string
	provider string
	owner    string
	repo     string
}

// listWatchedRepos reads all watched repos, fully draining and closing the
// result set so no connection is held while syncs run.
func (a *App) listWatchedRepos(ctx context.Context) ([]watchedRepo, error) {
	rows, err := a.db.DB.QueryContext(ctx,
		"SELECT id, provider, owner, repo FROM repos WHERE watched = 1",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []watchedRepo
	for rows.Next() {
		var r watchedRepo
		if err := rows.Scan(&r.id, &r.provider, &r.owner, &r.repo); err != nil {
			return nil, fmt.Errorf("scan watched repo: %w", err)
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read watched repos: %w", err)
	}
	return repos, nil
}

// syncSingleRepo syncs a single repo identified by its repo_id (e.g. "github:owner/repo").
func (a *App) syncSingleRepo(ctx context.Context, repoID string) {
	a.provMu.RLock()
	orch := a.orchestrator
	prov := a.prov
	orchCtx := a.provCtx // per-provider context; cancelled on disconnect/replace
	a.provMu.RUnlock()

	if prov == nil || orch == nil {
		return
	}

	providerName, ownerRepo, found := strings.Cut(repoID, ":")
	if !found {
		a.log.Error("syncSingleRepo: invalid repo_id format", "repo_id", repoID)
		return
	}
	owner, repo, found := strings.Cut(ownerRepo, "/")
	if !found {
		a.log.Error("syncSingleRepo: invalid repo_id format (no slash)", "repo_id", repoID)
		return
	}

	req := &sync.SyncRequest{
		Provider:        providerName,
		Owner:           owner,
		Repo:            repo,
		IncludePRs:      true,
		IncludeComments: true,
		IncludeDiff:     true,
		State:           "open",
	}
	repoStart := time.Now()
	result, err := orch.Sync(orchCtx, req)
	if err != nil {
		a.log.Error("syncSingleRepo: sync failed",
			"repo_id", repoID,
			"duration_ms", time.Since(repoStart).Milliseconds(),
			"err", err,
		)
		if orchCtx.Err() == nil {
			a.broadcastSyncError(repoID, err)
		}
		return
	}
	a.log.Info("syncSingleRepo: synced",
		"repo_id", repoID,
		"prs", result.PRCount,
		"duration_ms", time.Since(repoStart).Milliseconds(),
	)

	// Only broadcast if still on the same provider.
	if orchCtx.Err() == nil {
		a.wsServer.Broadcast(map[string]any{
			"type": "SYNC_COMPLETE",
			"data": map[string]any{"repo_id": repoID},
		})
	}
}
