package engine

import (
	"context"

	"github.com/itterno/bistro/internal/expertise"
	"github.com/itterno/bistro/internal/git/clonecache"
	"github.com/itterno/bistro/internal/git/diff"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/scan"
	"github.com/itterno/bistro/internal/services"
	"github.com/itterno/bistro/internal/store/dismissal"
	"github.com/itterno/bistro/internal/store/prdiff"
	"github.com/itterno/bistro/internal/store/repolocation"
	"github.com/itterno/bistro/internal/store/reviewdraft"
)

// Services returns the service set prepared by Start.
// The instances are retained so all calls share each domain's lifetime state.
func (a *App) Services() *services.Set { return a.serviceSet }

func (a *App) buildServices(ctx context.Context) *services.Set {
	diffStore := prdiff.New(a.db.DB)
	locations := repolocation.New(a.db.DB)
	expertiseStore := expertise.NewStore(a.db.DB)
	return &services.Set{
		Expertise:      &services.Expertise{Engine: a.expertiseEngine, Repos: services.NewExpertiseSQLRepoStore(a.db.DB), Data: services.NewExpertiseSQLDataStore(a.db.DB, expertiseStore), Settings: services.NewExpertiseSQLSettingsStore(a.db.DB, expertiseStore), CurrentUser: a.currentUserStore, Panics: a.panics, Log: a.log},
		Classification: &services.Classification{State: a.classificationState, Keys: a.tokenManager, OnKeyChanged: a.llmTier.ResetAuth, Panics: a.panics, Log: a.log},
		Editor:         &services.Editor{PRs: a.db, Repos: locations, Inspector: services.GitCheckoutInspector{}, Checker: a.gitClient, Panics: a.panics, Log: a.log},
		RepoLocation:   &services.RepoLocation{DB: a.db.DB, Store: locations, Clone: a.cloneRepoLocationRepo, CacheRoot: clonecache.Root, Locks: a.repoLocks, DeleteCacheDir: func(path string) error { return clonecache.DeleteDir(path) }, SweepOrphans: clonecache.SweepOrphans, Panics: a.panics, Log: a.log},
		DevFolderScan:  &services.DevFolderScan{Locations: locations, SyncTrigger: a.syncTrigger, ScanFunc: scan.Scan, Panics: a.panics, Log: a.log},
		Comments:       &services.Comments{DB: a.db.DB, Provider: a.classifier, Kick: a.llmKick, Panics: a.panics, Log: a.log},
		Dismiss:        &services.Dismiss{DB: a.db.DB, Dismissals: dismissal.New(a.db.DB), CurrentUser: a.currentUserStore, GetApplier: a.currentApplier, Scope: a.classificationState.DismissalScope, Panics: a.panics, Log: a.log},
		Threads:        &services.Threads{DB: a.db, GetApplier: a.currentApplier, Panics: a.panics, Log: a.log},
		Drafts:         &services.Drafts{Drafts: reviewdraft.New(a.db.DB), CurrentUser: a.currentUserStore, Events: a.emitter, Panics: a.panics, Log: a.log},
		Diff:           &services.Diff{DiffStore: diffStore, ViewedStore: a.db, CurrentUser: a.currentUserStore, Panics: a.panics, Log: a.log},
		Patchsets:      &services.Patchsets{Store: a.db, Panics: a.panics, Log: a.log},
		Triage:         &services.Triage{Store: a.db, CurrentUser: a.currentUserStore, Panics: a.panics, Log: a.log},
		Interdiff:      &services.Interdiff{Patchsets: a.db, Interdiff: func(req diff.InterdiffRequest) (diff.InterdiffResult, error) { return diff.Interdiff(a.repoLocks, req) }, RepoForPR: prRepoLocator{db: a.db, locations: repolocation.New(a.db.DB)}.RepoForPR, Panics: a.panics, Log: a.log},
		Viewed:         &services.Viewed{Viewed: a.db, CurrentUser: a.currentUserStore, DiffStore: diffStore, Baseline: a.db, Panics: a.panics, Log: a.log},
		ReadState:      &services.ReadState{DB: a.db, CurrentUser: a.currentUserStore, Events: a.emitter, Panics: a.panics, Log: a.log},
		PR:             &services.PR{DB: a.db.DB, CurrentUser: a.currentUserStore, TouchesYou: a.expertiseEngine, GetSyncer: a.currentPRSyncer, Panics: a.panics, Log: a.log},
		Sync:           &services.Sync{Trigger: a.syncTrigger, Panics: a.panics, Log: a.log},
		Repos:          &services.Repos{DB: a.db.DB, GetProvider: a.getProvider, SyncTrigger: services.WatchSyncTrigger{Queue: a.syncTrigger, Pending: &a.pendingWatchSync}, Panics: a.panics, Log: a.log},
		ReviewQueue:    &services.ReviewQueue{Context: ctx, GetProvider: a.currentProviderSnapshot, DB: a.db.DB, Panics: a.panics, Log: a.log},
		Auth: &services.Auth{
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
