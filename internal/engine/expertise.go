package engine

import (
	"context"

	"github.com/itterno/bistro/internal/expertise"
	"github.com/itterno/bistro/internal/services"
)

func (a *App) broadcastExpertiseEvent(eventType string, payload any) {
	switch eventType {
	case expertise.ExpertiseIndexProgressEvent:
		if progress, ok := payload.(expertise.IndexProgressPayload); ok {
			a.emitter.Emit(services.EventExpertiseProgress, services.ExpertiseProgress(progress))
		}
	case expertise.ExpertiseReadyEvent:
		if ready, ok := payload.(expertise.IndexReadyPayload); ok {
			a.emitter.Emit(services.EventExpertiseReady, services.ExpertiseReady(ready))
		}
	case expertise.ExpertiseErrorEvent:
		if failure, ok := payload.(expertise.ErrorPayload); ok {
			a.emitter.Emit(services.EventExpertiseError, services.ExpertiseError(failure))
		}
	}
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
	a.emitter.Emit(services.EventExpertiseNudge, services.ExpertiseNudge{RepoID: repoID, ReviewedCount: count})
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
		settings, err := services.NewExpertiseSQLSettingsStore(a.db.DB, stores).ExpertiseSettings(identityCtx)
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
	indexer.OnPanic = func(recovered any, stack []byte) {
		if a.panics != nil {
			a.panics.ReportPanic("expertise", recovered, stack)
		}
	}
	updater := expertise.NewUpdater(a.log, stores, expertiseGit, identity)
	a.expertiseEngine = expertise.NewEngine(a.log, stores, expertiseGit, indexer, updater)
	a.startLoop(ctx, "expertise-restart", func(ctx context.Context) {
		if err := a.expertiseEngine.RestartInterrupted(ctx, a.resolveRepoPath); err != nil {
			a.log.Warn("expertise restart interrupted indexing failed", "error", err)
		}
	})
}
