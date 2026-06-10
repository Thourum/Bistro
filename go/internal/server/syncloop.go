package server

import (
	"context"
	"strings"
	"time"

	"github.com/samuelasvanyi/bistro/internal/sync"
)

// runSyncLoop runs the periodic sync goroutine. It exits when ctx is cancelled.
func (a *App) runSyncLoop(ctx context.Context) {
	ticker := time.NewTicker(a.syncInterval)
	defer ticker.Stop()

	a.log.Debug("sync loop started", "interval", a.syncInterval.String())

	for {
		select {
		case <-ctx.Done():
			a.log.Debug("sync loop stopping", "reason", ctx.Err())
			return
		case <-ticker.C:
			a.log.Debug("sync tick fired", "next_tick_in", a.syncInterval.String())
			a.syncWatchedRepos(ctx)
		case repoID := <-a.syncTrigger:
			a.log.Debug("sync trigger received", "repo_id", repoID)
			a.syncSingleRepo(ctx, repoID)
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

	rows, err := a.db.DB.QueryContext(ctx,
		"SELECT id, provider, owner, repo FROM repos WHERE watched = 1",
	)
	if err != nil {
		a.log.Error("syncWatchedRepos: query failed", "err", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, repoProvider, owner, repo string
		if err := rows.Scan(&id, &repoProvider, &owner, &repo); err != nil {
			a.log.Error("syncWatchedRepos: scan failed", "err", err)
			continue
		}
		req := &sync.SyncRequest{
			Provider:        repoProvider,
			Owner:           owner,
			Repo:            repo,
			IncludePRs:      true,
			IncludeComments: true,
			State:           "open",
		}
		result, syncErr := orch.Sync(ctx, req)
		if syncErr != nil {
			a.log.Error("syncWatchedRepos: sync failed", "repo", id, "err", syncErr)
			continue
		}
		a.log.Info("syncWatchedRepos: synced", "repo", id, "prs", result.PRCount)
	}
	if err := rows.Err(); err != nil {
		a.log.Error("syncWatchedRepos: rows error", "err", err)
	}

	a.wsServer.Broadcast(map[string]interface{}{
		"type": "SYNC_COMPLETE",
		"data": map[string]interface{}{},
	})
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
		State:           "open",
	}
	result, err := orch.Sync(ctx, req)
	if err != nil {
		a.log.Error("syncSingleRepo: sync failed", "repo_id", repoID, "err", err)
		return
	}
	a.log.Info("syncSingleRepo: synced", "repo_id", repoID, "prs", result.PRCount)

	a.wsServer.Broadcast(map[string]interface{}{
		"type": "SYNC_COMPLETE",
		"data": map[string]interface{}{"repo_id": repoID},
	})
}
