package server

import (
	"context"
	"encoding/json"

	"github.com/itterno/bistro/internal/expertise"
	"github.com/itterno/bistro/internal/server/handler"
)

func (a *App) broadcastExpertiseEvent(eventType string, payload any) {
	a.wsServer.Broadcast(wsEventMsg{Type: eventType, Data: expertisePayloadToMap(payload)})
}

func expertisePayloadToMap(payload any) map[string]any {
	if data, ok := payload.(map[string]any); ok {
		return data
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return map[string]any{"payload": payload}
	}
	data := map[string]any{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return map[string]any{"payload": payload}
	}
	return data
}

func (a *App) broadcastExpertiseNudge(ctx context.Context, prID string) {
	if a.expertiseEngine == nil || a.db == nil || a.currentUserStore == nil {
		return
	}
	var repoID string
	if err := a.db.QueryRowContext(ctx, `SELECT repo_id FROM prs WHERE id = ?`, prID).Scan(&repoID); err != nil {
		a.log.Warn("expertise nudge skipped: PR repo lookup failed", "pr_id", prID, "error", err)
		return
	}
	login, ok, err := a.currentUserStore.Get(ctx, "github")
	if err != nil || !ok || login == "" {
		if err != nil {
			a.log.Warn("expertise nudge skipped: current user lookup failed", "pr_id", prID, "error", err)
		}
		return
	}
	shouldNudge, err := a.expertiseEngine.ShouldNudge(ctx, repoID, expertise.Account{Provider: "github", Login: login})
	if err != nil {
		a.log.Warn("expertise nudge check failed", "repo_id", repoID, "error", err)
		return
	}
	if !shouldNudge {
		return
	}
	count, err := expertise.NewStore(a.db.DB).ReviewedPRCount(ctx, repoID, "github", login, login)
	if err != nil {
		a.log.Warn("expertise nudge count failed", "repo_id", repoID, "error", err)
		return
	}
	a.wsServer.Broadcast(wsEventMsg{Type: "EXPERTISE_NUDGE", Data: map[string]any{"repo_id": repoID, "reviewed_count": count}})
}

func (a *App) currentExpertiseIdentity(stores *expertise.Store) func(context.Context) expertise.Identity {
	return func(identityCtx context.Context) expertise.Identity {
		login, ok, err := a.currentUserStore.Get(identityCtx, "github")
		if err != nil || !ok {
			if err != nil {
				a.log.Warn("expertise identity current user lookup failed", "error", err)
			}
			return expertise.Identity{}
		}
		settings, err := handler.NewExpertiseSQLSettingsStore(a.db.DB, stores).ExpertiseSettings(identityCtx)
		if err != nil {
			a.log.Warn("expertise identity settings lookup failed", "error", err)
		}
		return expertise.Identity{Login: login, AliasEmails: settings.AliasEmails}
	}
}

func (a *App) prepareExpertise(ctx context.Context, stores *expertise.Store) {
	expertiseGit := expertise.NewGitCLI(a.log)
	identity := a.currentExpertiseIdentity(stores)
	indexer := expertise.NewIndexer(a.log, stores, expertiseGit, identity).WithEmitter(a.broadcastExpertiseEvent)
	updater := expertise.NewUpdater(a.log, stores, expertiseGit, identity)
	a.expertiseEngine = expertise.NewEngine(a.log, stores, expertiseGit, indexer, updater)
	go func() {
		if err := a.expertiseEngine.RestartInterrupted(ctx, a.resolveRepoPath); err != nil {
			a.log.Warn("expertise restart interrupted indexing failed", "error", err)
		}
	}()
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
