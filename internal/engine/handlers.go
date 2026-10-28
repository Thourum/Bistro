package engine

import (
	"context"
	"github.com/itterno/bistro/internal/store/dismissal"
	"github.com/itterno/bistro/internal/store/reviewdraft"

	"github.com/itterno/bistro/internal/engine/handler"
	"github.com/itterno/bistro/internal/git/diff"
)

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
		Dismissals:  dismissal.New(a.db.DB),
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
		Interdiff: func(req diff.InterdiffRequest) (diff.InterdiffResult, error) {
			return diff.Interdiff(a.repoLocks, req)
		},
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
		Drafts:      reviewdraft.New(a.db.DB),
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
