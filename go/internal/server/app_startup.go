package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/itterno/bistro/internal/classify/llm"
	"github.com/itterno/bistro/internal/expertise"
	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/server/handler"
	"github.com/itterno/bistro/internal/store"
)

type runStores struct {
	currentUser  *store.CurrentUserStore
	repoLocation *store.RepoLocationStore
	expertise    *expertise.Store
	diff         *store.DiffStore
}

func (a *App) openDatabase(ctx context.Context) error {
	db, err := store.OpenDB(a.dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	if err := store.NewMigrationRunner(a.log, db.DB).Run(ctx); err != nil {
		return errors.Join(fmt.Errorf("migrate database: %w", err), db.Close())
	}
	a.db = db
	a.log.Info("database initialized")
	return nil
}

func (a *App) prepareStores() runStores {
	if a.repoLocks == nil {
		a.repoLocks = git.NewRepoLocks()
	}
	if a.gitClient == nil {
		a.gitClient = git.NewClient(git.NewCache())
	}
	a.gitClient.SetLocks(a.repoLocks)
	a.gitClient.SetTokenSource(a.githubToken)

	stores := runStores{
		currentUser:  store.NewCurrentUserStore(a.db),
		repoLocation: store.NewRepoLocationStore(a.db.DB),
		expertise:    expertise.NewStore(a.db.DB),
		diff:         store.NewDiffStore(a.db.DB),
	}
	a.anchorResolver = git.NewAnchorResolver()
	a.currentUserStore = stores.currentUser
	a.configureRepoLocationStore(stores.repoLocation)
	a.resolveRepoPath = func(repoID string) (string, bool) {
		location, err := stores.repoLocation.Get(context.Background(), repoID)
		if err != nil {
			a.log.Warn("expertise repo path resolve failed", "repo_id", repoID, "error", err)
			return "", false
		}
		if location == nil || location.Source != "local" || location.LocalPath == "" {
			return "", false
		}
		return location.LocalPath, true
	}
	return stores
}

func (a *App) currentExpertiseIdentity(stores *expertise.Store) func(context.Context) expertise.Identity {
	return func(identityCtx context.Context) expertise.Identity {
		login, ok, err := a.currentUserStore.GetCurrentUser(identityCtx, "github")
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

func (a *App) prepareLLMTier(ctx context.Context) {
	// Tier no-ops outside LLM modes, so it can be wired unconditionally.
	a.llmTier = llm.NewTier(llm.TierConfig{
		DB:     a.db.DB,
		Client: &liveByokClient{state: a.classificationState, tokens: a.tokenManager},
		Mode:   a.classificationState.Mode,
		Settings: func() llm.SettingsSnapshot {
			s := a.classificationState.Snapshot()
			return llm.SettingsSnapshot{Provider: s.Provider, Model: s.Model}
		},
		Emit: func(eventType string, data map[string]any) {
			a.wsServer.Broadcast(wsEventMsg{Type: eventType, Data: data})
		},
		Log: slogPrintf{a.log},
	})
	a.llmKick = func(prID string) { a.llmTier.KickPR(ctx, prID) }
}

func (a *App) initializeStartupProvider(ctx context.Context) {
	a.provMu.Lock()
	if a.prov != nil {
		a.attachOrchestratorLocked(ctx, a.prov, "github")
	}
	hasStartupProvider := a.prov != nil
	a.provMu.Unlock()
	if !hasStartupProvider {
		return
	}
	a.log.Info("orchestrator initialized at startup (token from keychain)")
	if !a.replayOnStart {
		return
	}
	a.log.Info("replaying offline action queue after startup")
	if err := a.ReplayQueuedActions(ctx); err != nil {
		a.log.Error("startup queue replay failed", "error", err)
	}
}
