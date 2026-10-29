package engine

import (
	"github.com/itterno/bistro/internal/engine/handler"
)

func (a *App) registerReviewHandlers(stores runStores) {
	a.wsServer.RegisterHandler("SET_CLASSIFICATION_SETTINGS",
		handler.NewSetClassificationSettingsHandler(a.classificationState, a.log))
	a.wsServer.RegisterHandler("SET_BYOK_KEY",
		handler.NewSetByokKeyHandler(a.tokenManager, a.llmTier.ResetAuth, a.log))
	a.wsServer.RegisterHandler("DELETE_BYOK_KEY",
		handler.NewDeleteByokKeyHandler(a.tokenManager, a.llmTier.ResetAuth, a.log))

}

func (a *App) registerDiffAndCheckoutHandlers(stores runStores) {
	// Phase 5 :open in editor — checkout inspection and checkout.
	a.wsServer.RegisterHandler("CHECK_PR_CHECKOUT",
		handler.NewCheckPRCheckoutHandler(a.db, stores.repoLocation, handler.GitCheckoutInspector{}, a.log))
	a.wsServer.RegisterHandler("CHECKOUT_PR_HEAD",
		handler.NewCheckoutPRHeadHandler(a.db, stores.repoLocation, a.gitClient, a.log))
}
