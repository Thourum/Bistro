package engine

import (
	"github.com/itterno/bistro/internal/store/dismissal"
	"github.com/itterno/bistro/internal/store/reviewdraft"

	"github.com/itterno/bistro/internal/engine/handler"
)

func (a *App) registerReviewHandlers(stores runStores) {
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

}

func (a *App) registerDiffAndCheckoutHandlers(stores runStores) {
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
