package engine

import (
	"context"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/services"
)

// Services returns the service set prepared by Run (Start from Task 9).
// The instances are retained so all calls share each domain's lifetime state.
func (a *App) Services() *services.Set { return a.serviceSet }

func (a *App) buildServices(ctx context.Context) *services.Set {
	return &services.Set{Auth: &services.Auth{
		Log: a.log, Tokens: a.tokenManager, Cache: a.providerCache,
		Events: a.emitter, Panics: a.panics, SyncTrigger: a.syncTrigger,
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
		CurrentUserWriter:  a.currentUserStore,
		CurrentUserClearer: a.currentUserStore,
	}}
}
