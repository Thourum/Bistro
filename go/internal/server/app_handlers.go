package server

import (
	"context"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/scan"
	"github.com/itterno/bistro/internal/server/handler"
	"github.com/itterno/bistro/internal/store"
)

func (a *App) registerAuthHandlers(ctx context.Context, stores runStores) {
	authDeps := handler.AuthDeps{
		Log:         a.log,
		Tokens:      a.tokenManager,
		Cache:       a.providerCache,
		Broadcast:   a.wsServer.Broadcast,
		SyncTrigger: a.syncTrigger,
		SetProvider: func(prov provider.Provider, name string) {
			a.provMu.Lock()
			defer a.provMu.Unlock()
			if prov == nil {
				a.teardownProviderLocked(name)
				return
			}
			if a.db != nil {
				a.attachOrchestratorLocked(ctx, prov, name)
				a.log.Info("orchestrator initialized after auth")
			}
		},
		GetClientID:        func() string { return a.githubClientID },
		GetClientSecret:    func() string { return a.githubClientSecret },
		CallbackTimeout:    a.authCallbackTimeout,
		CurrentUserWriter:  stores.currentUser,
		CurrentUserClearer: stores.currentUser,
	}

	authCheck := handler.NewAuthCheckHandler(authDeps)

	start, cancel := handler.NewAuthFlowHandlers(authDeps)
	a.wsServer.RegisterHandler("AUTH_START", start)
	a.wsServer.RegisterHandler("AUTH_CANCEL", cancel)
	a.wsServer.RegisterHandler("AUTH_CHECK", authCheck)
	a.wsServer.RegisterHandler("AUTH_DISCONNECT", handler.NewAuthDisconnectHandler(authDeps, authCheck))
}

func (a *App) registerReviewHandlers(ctx context.Context, stores runStores) func() {
	requestQueue, waitQueue := handler.NewRequestReviewQueueHandler(handler.ReviewQueueDeps{
		Context: ctx, GetProvider: a.currentProviderSnapshot, DB: a.db.DB,
		Broadcast: a.wsServer.Broadcast, Log: a.log,
	})
	a.wsServer.RegisterHandler("REQUEST_REVIEW_QUEUE", requestQueue)
	// REQUEST_DIFF: enriched with per-file content_hash and viewed flag.
	a.wsServer.RegisterHandler("REQUEST_DIFF", handler.NewRequestDiffHandlerWithDeps(handler.DiffHandlerDeps{
		DiffStore:   stores.diff,
		ViewedStore: a.db,
		CurrentUser: stores.currentUser,
		Log:         a.log,
	}))
	a.wsServer.RegisterHandler("REQUEST_PR_LIST", handler.NewRequestPRListHandlerWithExpertise(a.db.DB, a.expertiseEngine, stores.currentUser, a.log))
	a.wsServer.RegisterHandler("REQUEST_COMMENTS",
		handler.NewRequestCommentsHandlerWithLLMKick(a.db.DB, a.log, a.classifier, a.llmKick))
	dismissDeps := handler.DismissDeps{
		DB:          a.db.DB,
		Dismissals:  store.NewDismissals(a.db.DB),
		CurrentUser: stores.currentUser,
		GetApplier:  a.currentApplier,
		Scope:       a.classificationState.DismissalScope,
		Log:         a.log,
	}
	a.wsServer.RegisterHandler("DISMISS_COMMENTS", handler.NewDismissCommentsHandler(dismissDeps))
	a.wsServer.RegisterHandler("UNDISMISS_COMMENTS", handler.NewUndismissCommentsHandler(dismissDeps))
	a.wsServer.RegisterHandler("SET_CLASSIFICATION_SETTINGS",
		handler.NewSetClassificationSettingsHandler(a.classificationState, a.log))
	a.wsServer.RegisterHandler("SET_BYOK_KEY",
		handler.NewSetByokKeyHandler(a.tokenManager, a.llmTier.ResetAuth, a.log))
	a.wsServer.RegisterHandler("DELETE_BYOK_KEY",
		handler.NewDeleteByokKeyHandler(a.tokenManager, a.llmTier.ResetAuth, a.log))
	a.wsServer.RegisterHandler("REQUEST_REPO_LIST",
		handler.NewRequestRepoListHandler(a.getProvider, a.db.DB, a.log))
	a.wsServer.RegisterHandler("TOGGLE_WATCH_REPO",
		handler.NewToggleWatchHandler(a.db.DB, handler.WatchSyncTrigger{Queue: a.syncTrigger, Pending: &a.pendingWatchSync}, a.log))
	a.wsServer.RegisterHandler("FORCE_SYNC", handler.NewForceSyncHandler(a.syncTrigger, a.log))
	a.wsServer.RegisterHandler("REQUEST_PR_SYNC", handler.NewRequestPRSyncHandler(a.currentPRSyncer, a.log))
	a.wsServer.RegisterHandler("SET_SYNC_SETTINGS", handler.NewSetSyncSettingsHandler(a.syncSettings, a.log))
	return waitQueue
}

func (a *App) registerDiffAndCheckoutHandlers(stores runStores) {
	// Phase 2.1+2.2 handlers.
	a.wsServer.RegisterHandler("GET_PATCHSETS",
		handler.NewListPatchsetsHandler(a.db, a.log))
	a.wsServer.RegisterHandler("REQUEST_TRIAGE", handler.NewTriageHandler(handler.TriageHandlerDeps{
		Store: a.db, CurrentUser: stores.currentUser, Log: a.log,
	}))
	a.wsServer.RegisterHandler("REQUEST_INTERDIFF", handler.NewInterdiffHandler(handler.InterdiffHandlerDeps{
		Patchsets: a.db,
		Git:       a.gitClient,
		RepoForPR: prRepoLocator{db: a.db, locations: stores.repoLocation}.RepoForPR,
		Log:       a.log,
	}))
	a.wsServer.RegisterHandler("MARK_FILE_VIEWED", handler.NewMarkFileViewedHandlerWithDeps(handler.MarkFileViewedDeps{
		Viewed: a.db, CurrentUser: stores.currentUser, DiffStore: stores.diff, Baseline: a.db, Log: a.log,
	}))
	a.wsServer.RegisterHandler("MARK_PRS_READ",
		handler.NewMarkPRsReadHandler(a.db, stores.currentUser, a.wsServer, a.log))
	// Phase 2.4 handlers.
	a.wsServer.RegisterHandler("GET_REVIEW_THREADS",
		handler.NewGetReviewThreadsHandler(a.db, a.log))
	a.wsServer.RegisterHandler("SUBMIT_ACTION",
		handler.NewSubmitActionHandler(a.currentApplier, a.log))
	draftDeps := handler.DraftDeps{
		Drafts:      store.NewReviewDrafts(a.db.DB),
		CurrentUser: stores.currentUser,
		Broadcaster: a.wsServer,
		Log:         a.log,
	}
	a.wsServer.RegisterHandler("DRAFT_SAVE", handler.NewDraftSaveHandler(draftDeps))
	a.wsServer.RegisterHandler("DRAFT_DELETE", handler.NewDraftDeleteHandler(draftDeps))
	a.wsServer.RegisterHandler("DRAFTS_LIST", handler.NewDraftsListHandler(draftDeps))
	// Phase 5 :open in editor — checkout inspection and checkout.
	a.wsServer.RegisterHandler("CHECK_PR_CHECKOUT",
		handler.NewCheckPRCheckoutHandler(a.db, stores.repoLocation, handler.GitCheckoutInspector{}, a.log))
	a.wsServer.RegisterHandler("CHECKOUT_PR_HEAD",
		handler.NewCheckoutPRHeadHandler(a.db, stores.repoLocation, a.gitClient, a.log))
}

func (a *App) registerExpertiseHandlers(stores runStores) {
	expertiseRepoStore := handler.NewExpertiseSQLRepoStore(a.db.DB)
	expertiseDataStore := handler.NewExpertiseSQLDataStore(a.db.DB, stores.expertise)
	expertiseSettingsStore := handler.NewExpertiseSQLSettingsStore(a.db.DB, stores.expertise)
	expertiseDeps := handler.ExpertiseHandlerDeps{
		Engine:      a.expertiseEngine,
		Repos:       expertiseRepoStore,
		Data:        expertiseDataStore,
		Settings:    expertiseSettingsStore,
		CurrentUser: stores.currentUser,
		Log:         a.log,
	}
	a.wsServer.RegisterHandler("EXPERTISE_ENABLE", handler.NewExpertiseEnableHandler(expertiseDeps))
	a.wsServer.RegisterHandler("EXPERTISE_DISABLE", handler.NewExpertiseDisableHandler(expertiseDeps))
	a.wsServer.RegisterHandler("GET_EXPERTISE", handler.NewGetExpertiseHandler(expertiseDeps))
	a.wsServer.RegisterHandler("EXPERTISE_NUDGE_DISMISS", handler.NewExpertiseNudgeDismissHandler(expertiseDeps))
	a.wsServer.RegisterHandler("GET_EXPERTISE_SETTINGS", handler.NewGetExpertiseSettingsHandler(expertiseDeps))
	a.wsServer.RegisterHandler("SET_EXPERTISE_SETTINGS", handler.NewSetExpertiseSettingsHandler(expertiseDeps))
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
