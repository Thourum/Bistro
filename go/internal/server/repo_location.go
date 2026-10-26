package server

import (
	"context"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/scan"
	"github.com/itterno/bistro/internal/server/handler"
	"github.com/itterno/bistro/internal/store"
)

func (a *App) configureRepoLocationStore(repoLocationStore *store.RepoLocationStore) {
	if a.gitClient == nil {
		return
	}
	a.gitClient.SetLocalResolver(newRepoLocationResolver(repoLocationStore, a.log))
}

func (a *App) cloneRepoLocationRepo(ctx context.Context, repoURL, destination string, bare bool) error {
	return a.gitClient.CloneRepoToPath(ctx, repoURL, destination, bare)
}

func (a *App) registerRepoLocationHandlers(ctx context.Context, stores runStores) func() {
	deleteCacheDir := func(path string) error {
		return git.DeleteCacheDir(path)
	}
	cacheDeps := handler.CacheMaintenanceDeps{
		Store:          stores.repoLocation,
		Locks:          a.repoLocks,
		DeleteCacheDir: deleteCacheDir,
		SweepOrphans:   git.SweepOrphans,
		Log:            a.log,
	}
	a.wsServer.RegisterHandler("VALIDATE_DEV_FOLDER", handler.NewValidateDevFolderHandler(a.log))
	startScan, cancelScan, waitScans := handler.NewScanDevFolderHandlers(handler.ScanDevFolderDeps{
		Context: ctx, Locations: stores.repoLocation, SyncTrigger: a.syncTrigger,
		Broadcast: a.wsServer.Broadcast, Scan: scan.Scan, Log: a.log,
	})
	a.wsServer.RegisterHandler("SCAN_DEV_FOLDER", startScan)
	a.wsServer.RegisterHandler("SCAN_CANCEL", cancelScan)
	a.wsServer.RegisterHandler("ADD_REPO_BY_URL", handler.NewAddRepoByURLHandler(handler.AddRepoByURLDeps{
		DB:        a.db.DB,
		Store:     stores.repoLocation,
		Clone:     a.cloneRepoLocationRepo,
		CacheRoot: git.CacheRoot,
		Log:       a.log,
	}))
	a.wsServer.RegisterHandler("REMOVE_REPO_PREFLIGHT", handler.NewRemoveRepoPreflightHandler(a.db.DB, a.log))
	a.wsServer.RegisterHandler("GET_CACHE_STATS", handler.NewGetCacheStatsHandler(cacheDeps))
	a.wsServer.RegisterHandler("PRUNE_CACHE", handler.NewPruneCacheHandler(cacheDeps))
	a.wsServer.RegisterHandler("CLEAR_CACHE", handler.NewClearCacheHandler(cacheDeps))
	a.wsServer.RegisterHandler("DELETE_CACHE_REPO", handler.NewDeleteCacheRepoHandler(cacheDeps))
	a.wsServer.RegisterHandler("RECOVER_REPO_PATH", handler.NewRecoverRepoPathHandler(stores.repoLocation, a.log))
	a.wsServer.RegisterHandler("RECOVER_REPO_TO_CACHE", handler.NewRecoverRepoToCacheHandler(handler.RecoverRepoToCacheDeps{
		DB:        a.db.DB,
		Store:     stores.repoLocation,
		Clone:     a.cloneRepoLocationRepo,
		CacheRoot: git.CacheRoot,
		Log:       a.log,
	}))
	a.wsServer.RegisterHandler("REMOVE_REPO", handler.NewRemoveRepoHandler(handler.RemoveRepoDeps{
		DB:             a.db.DB,
		Store:          stores.repoLocation,
		Locks:          a.repoLocks,
		DeleteCacheDir: deleteCacheDir,
		Log:            a.log,
	}))
	return waitScans
}
