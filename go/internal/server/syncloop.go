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
	a.wsServer.Broadcast(map[string]interface{}{
		"type": "SYNC_ERROR",
		"data": map[string]interface{}{
			"error_code": "SYNC_FAILED",
			"message":    fmt.Sprintf("sync failed for %s: %v", repoID, err),
		},
	})
}

// applyPendingSyncSettings drains the syncSettings channel and applies the
// latest coalesced value. Returns true if any settings were consumed.
// This must only be called from the sync loop goroutine.
func (a *App) applyPendingSyncSettings(ticker *time.Ticker, autoSync *bool, interval *time.Duration) bool {
	var latest *syncSettingsMsg
	for {
		select {
		case msg := <-a.syncSettings:
			m := msg
			latest = &m
		default:
			goto done
		}
	}
done:
	if latest == nil {
		return false
	}
	a.log.Info("sync settings updated",
		"autoSync", latest.AutoSync, "interval", latest.Interval.String())
	*autoSync = latest.AutoSync
	*interval = latest.Interval
	if *autoSync && *interval > 0 {
		ticker.Reset(*interval)
	} else {
		ticker.Stop()
	}
	return true
}

// runSyncLoop runs the periodic sync goroutine. It exits when ctx is cancelled.
func (a *App) runSyncLoop(ctx context.Context) {
	ticker := time.NewTicker(a.syncInterval)
	autoSync := a.autoSync
	interval := a.syncInterval
	if !autoSync {
		ticker.Stop()
	}
	defer ticker.Stop()

	a.log.Debug("sync loop started", "interval", a.syncInterval.String(), "autoSync", autoSync)

	for {
		// Prioritise settings changes over ticker ticks so that a disable/
		// interval-change applied while a sync was running takes effect before
		// the next automatic tick fires.
		select {
		case msg := <-a.syncSettings:
			// Fast-path: at least one settings message is ready. Drain the rest
			// so we always act on the latest value.
			latest := msg
		drainLoop:
			for {
				select {
				case m := <-a.syncSettings:
					latest = m
				default:
					break drainLoop
				}
			}
			a.log.Info("sync settings updated",
				"autoSync", latest.AutoSync, "interval", latest.Interval.String())
			autoSync = latest.AutoSync
			interval = latest.Interval
			if autoSync && interval > 0 {
				ticker.Reset(interval)
			} else {
				ticker.Stop()
			}
			continue
		default:
		}

		select {
		case <-ctx.Done():
			a.log.Debug("sync loop stopping", "reason", ctx.Err())
			return
		case msg := <-a.syncSettings:
			latest := msg
		drainLoop2:
			for {
				select {
				case m := <-a.syncSettings:
					latest = m
				default:
					break drainLoop2
				}
			}
			a.log.Info("sync settings updated",
				"autoSync", latest.AutoSync, "interval", latest.Interval.String())
			autoSync = latest.AutoSync
			interval = latest.Interval
			if autoSync && interval > 0 {
				ticker.Reset(interval)
			} else {
				ticker.Stop()
			}
		case <-ticker.C:
			// Drain any pending settings that arrived during the last sync pass
			// before deciding whether to run.
			a.applyPendingSyncSettings(ticker, &autoSync, &interval)
			if !autoSync {
				// ticker fired but autoSync was disabled; skip
				continue
			}
			a.log.Debug("sync tick fired", "next_tick_in", interval.String())
			a.syncWatchedRepos(ctx)
			// After a (potentially long) sync pass, re-check for settings
			// updates that arrived while we were busy.
			a.applyPendingSyncSettings(ticker, &autoSync, &interval)
		case repoID := <-a.syncTrigger:
			a.log.Debug("sync trigger received", "repo_id", repoID)
			if repoID == "" {
				a.syncWatchedRepos(ctx)
			} else {
				a.syncSingleRepo(ctx, repoID)
			}
			// Re-check settings after manual sync too.
			a.applyPendingSyncSettings(ticker, &autoSync, &interval)
		}
	}
}

// syncWatchedRepos syncs all watched repos. No-ops if not yet authenticated.
func (a *App) syncWatchedRepos(ctx context.Context) {
	a.provMu.RLock()
	orch := a.orchestrator
	prov := a.prov
	a.provMu.RUnlock()

	if prov == nil || orch == nil {
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
		result, syncErr := orch.Sync(ctx, req)
		if syncErr != nil {
			failed++
			a.log.Error("syncWatchedRepos: sync failed",
				"repo", r.id,
				"duration_ms", time.Since(repoStart).Milliseconds(),
				"err", syncErr,
			)
			a.broadcastSyncError(r.id, syncErr)
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

	a.wsServer.Broadcast(map[string]interface{}{
		"type": "SYNC_COMPLETE",
		"data": map[string]interface{}{},
	})
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
			a.log.Error("listWatchedRepos: scan failed", "err", err)
			continue
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

// syncSingleRepo syncs a single repo identified by its repo_id (e.g. "github:owner/repo").
func (a *App) syncSingleRepo(ctx context.Context, repoID string) {
	a.provMu.RLock()
	orch := a.orchestrator
	prov := a.prov
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
	result, err := orch.Sync(ctx, req)
	if err != nil {
		a.log.Error("syncSingleRepo: sync failed",
			"repo_id", repoID,
			"duration_ms", time.Since(repoStart).Milliseconds(),
			"err", err,
		)
		a.broadcastSyncError(repoID, err)
		return
	}
	a.log.Info("syncSingleRepo: synced",
		"repo_id", repoID,
		"prs", result.PRCount,
		"duration_ms", time.Since(repoStart).Milliseconds(),
	)

	a.wsServer.Broadcast(map[string]interface{}{
		"type": "SYNC_COMPLETE",
		"data": map[string]interface{}{"repo_id": repoID},
	})
}
