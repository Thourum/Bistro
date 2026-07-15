package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	stdsync "sync"
	"syscall"
	"time"

	"github.com/itterno/bistro/internal/cache"
	"github.com/itterno/bistro/internal/classify"
	"github.com/itterno/bistro/internal/classify/llm"
	"github.com/itterno/bistro/internal/expertise"
	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/keychain"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/server/handler"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/sync"
	"github.com/itterno/bistro/internal/ws"
)

// syncSettingsMsg is an alias for the handler type to keep the field
// declaration readable within the server package.
type syncSettingsMsg = handler.SyncSettingsMsg

// App is the top-level orchestrator for the bistro-sync process.
type App struct {
	log                *slog.Logger
	db                 *store.DB
	wsServer           *ws.Server
	wsPort             int
	dbPath             string
	orchestrator       *sync.Orchestrator
	prov               provider.Provider
	activeProviderName string
	gitClient          *git.Client
	githubClientID     string
	githubClientSecret string
	repoLocks          *git.RepoLocks
	providerCache      *cache.ProviderCache
	tokenManager       *keychain.TokenManager
	syncTrigger        chan string
	syncSettings       chan syncSettingsMsg
	provMu             stdsync.RWMutex
	syncInterval       time.Duration
	autoSync           bool
	readyWriter        io.Writer
	// bridgeCancel cancels the event-bridge goroutine for the current orchestrator.
	// Replaced each time a new orchestrator is wired up.
	bridgeCancel context.CancelFunc
	// replayOnStart causes Run() to replay the offline action queue after the
	// production orchestrator (with DB and queue) is attached.
	replayOnStart bool
	// provCtx / provCtxCancel form a per-provider generation context.
	// It is derived from the Run ctx (or context.Background before Run) and
	// cancelled on every provider disconnect/replacement.  The sync loop,
	// event bridge, and orchestrator liveness all hold a reference; cancelling
	// it propagates to all of them simultaneously.
	provCtx       context.Context
	provCtxCancel context.CancelFunc
	// currentUserStore is set in Run() and shared across all orchestrators.
	currentUserStore *store.CurrentUserStore
	// anchorResolver is set in Run() and shared across all orchestrators.
	anchorResolver *git.AnchorResolver
	// classifier is the Phase 3 comment-classification engine provider,
	// shared by the sync writer and the comments handler. Its mode getter
	// reads classificationState, so a mode change lazily invalidates the
	// rule fingerprint on the next Current() call.
	classifier *classify.EngineProvider
	// classificationState holds the live classification settings pushed via
	// SET_CLASSIFICATION_SETTINGS (mode, dismissal scope, provider, model).
	classificationState *handler.ClassificationState
	// llmTier is the async BYOK classification tier (Phase 3.8); kicked
	// post-sync-commit and on PR open, no-op outside LLM modes.
	llmTier *llm.Tier
	// llmKick kicks the tier for one PR; set in Run, read by orchestrator
	// wiring (post-commit hook) and the comments handler (PR-open kick).
	llmKick func(prID string)
	// expertiseEngine is the Phase 4 local expertise scorer, shared by handlers
	// and sync/review hooks.
	expertiseEngine *expertise.Engine
	// resolveRepoPath maps repo IDs to local checkout paths for expertise hooks.
	resolveRepoPath sync.RepoPathResolver
}

// slogPrintf adapts *slog.Logger to the llm.Logger Printf interface.
type slogPrintf struct{ log *slog.Logger }

func (s slogPrintf) Printf(format string, v ...interface{}) {
	s.log.Warn(fmt.Sprintf(format, v...))
}

// liveByokClient satisfies llm.ClassifierClient by constructing the right
// provider client per call from the LIVE settings + keychain key — so a
// provider/model/key change applies to the next call without restarts.
// The key never leaves the call path.
type liveByokClient struct {
	state  *handler.ClassificationState
	tokens *keychain.TokenManager
}

func (c *liveByokClient) Classify(ctx context.Context, comment llm.CommentInput) (string, int, error) {
	s := c.state.Snapshot()
	key, err := c.tokens.Get("byok." + s.Provider)
	if err != nil {
		// Missing key behaves like an auth failure: tier disables until a
		// key change calls ResetAuth.
		return "", 0, fmt.Errorf("no BYOK key for %s: %w", s.Provider, llm.ErrAuth)
	}
	if s.Provider == "anthropic" {
		return llm.NewAnthropicClient(s.Model, key, s.Endpoint).Classify(ctx, comment)
	}
	return llm.NewOpenAICompatibleClient(s.Provider, s.Model, key, s.Endpoint).Classify(ctx, comment)
}

// SetReadyWriter overrides where the machine-readable ready signal is written.
// Defaults to os.Stdout when unset.
func (a *App) SetReadyWriter(w io.Writer) {
	a.readyWriter = w
}

// teardownProviderLocked clears the active provider, orchestrator, and all
// associated per-provider state (event bridge, provCtx, activeProviderName).
// When name is non-empty, the teardown only proceeds if it matches the current
// activeProviderName — this prevents a disconnect for "github" from clearing a
// freshly-reconnected "github2" session, for example.
// Passing name="" unconditionally tears down (used by the external SetProvider(nil)).
// Must be called with a.provMu held for writing.
func (a *App) teardownProviderLocked(name string) {
	if name != "" && a.activeProviderName != name {
		return // named disconnect doesn't match current provider — no-op
	}
	a.prov = nil
	a.activeProviderName = ""
	a.orchestrator = nil
	// Cancel the per-provider context — stops sync runs, event bridge, and
	// in-flight actions that captured a reference before disconnect.
	if a.provCtxCancel != nil {
		a.provCtxCancel()
	}
	// Re-initialise to a non-cancelled context for the next connect.
	a.provCtx, a.provCtxCancel = context.WithCancel(context.Background())
	if a.bridgeCancel != nil {
		a.bridgeCancel()
		a.bridgeCancel = nil
	}
}

// attachOrchestratorLocked creates, wires, and stores a new orchestrator for
// the given provider, then starts the event bridge. Cancels any previous
// per-provider context first so all stale goroutines stop.
// runCtx is the context passed to Run(); it provides the lifetime ceiling for
// the new per-provider context. Must be called with a.provMu held for writing.
func (a *App) attachOrchestratorLocked(runCtx context.Context, prov provider.Provider, name string) {
	// Cancel previous per-provider context (stops stale syncs/bridge/actions).
	if a.provCtxCancel != nil {
		a.provCtxCancel()
	}
	a.provCtx, a.provCtxCancel = context.WithCancel(runCtx)

	diffStore := store.NewDiffStore(a.db.DB)
	a.orchestrator = sync.NewOrchestrator(a.log, prov, a.db, a.gitClient).
		WithDiffStore(diffStore).
		WithReviewTracking(a.currentUserStore, a.db).
		WithAnchorResolver(a.anchorResolver).
		WithActionQueue(store.NewActionQueue(a.db.DB)).
		WithLiveness(a.provCtx).
		WithClassifier(a.classifier).
		WithExpertise(a.expertiseEngine).
		WithRepoPathResolver(a.resolveRepoPath).
		WithPostCommit(func(prIDs []string) {
			if a.llmKick == nil {
				return
			}
			for _, prID := range prIDs {
				a.llmKick(prID)
			}
		}).
		WithPostReview(func(reviewCtx context.Context, prID string) {
			a.broadcastExpertiseNudge(reviewCtx, prID)
		})
	a.prov = prov
	a.activeProviderName = name
	a.startEventBridge(a.provCtx)
}

// SetProvider configures the GitHub provider used by the sync orchestrator.
// If the database and git client are already available, it also (re)creates the
// orchestrator so the sync loop can start working immediately after auth.
func (a *App) SetProvider(prov provider.Provider) {
	a.provMu.Lock()
	defer a.provMu.Unlock()

	if prov == nil {
		a.teardownProviderLocked("") // unconditional — external nil means disconnect
		return
	}

	// Record the provider name so AUTH_DISCONNECT can match it later.
	// This covers the keychain-startup path (main.go calls SetProvider before
	// Run) which otherwise leaves activeProviderName empty.
	a.prov = prov
	a.activeProviderName = "github"

	if a.db != nil {
		a.attachOrchestratorLocked(context.Background(), prov, "github")
	}
}

// SetGitClient configures the git client used by the sync orchestrator.
// Must be called before Run.
func (a *App) SetGitClient(gitClient *git.Client) {
	a.gitClient = gitClient
	if gitClient != nil && a.repoLocks != nil {
		gitClient.SetLocks(a.repoLocks)
	}
}

func (a *App) configureRepoLocationStore(repoLocationStore *store.RepoLocationStore) {
	if a.gitClient == nil {
		return
	}
	a.gitClient.SetLocalResolver(newRepoLocationResolver(repoLocationStore, a.log))
}

// SetReplayOnStart configures whether Run() should replay the offline action
// queue after the production orchestrator is attached (DB open, queue wired).
// This replaces pre-Run calls to ReplayQueuedActions which are always no-ops
// because the orchestrator does not exist before Run opens the database.
func (a *App) SetReplayOnStart(replay bool) {
	a.replayOnStart = replay
}

// ReplayQueuedActions replays any pending offline queue actions.
// It is a no-op when the orchestrator has not been set up.
// Prefer SetReplayOnStart so replay happens inside Run() after the database and
// queue are initialised; calling this before Run() is always a no-op.
func (a *App) ReplayQueuedActions(ctx context.Context) error {
	if a.orchestrator == nil {
		return nil
	}
	return a.orchestrator.ReplayQueuedActions(ctx)
}

// New returns a new App configured with the given WebSocket port and database path.
// Uses default Info-level stderr logging.
func New(wsPort int, dbPath string) *App {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return NewWithLogger(wsPort, dbPath, logger)
}

// NewWithLogger returns a new App with a custom logger.
func NewWithLogger(wsPort int, dbPath string, logger *slog.Logger) *App {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	cachePath := filepath.Join(cacheDir, "bistro", "providers.json")
	// Initialise provCtx to a live background context so sync loop / tests that
	// access it before the first connect don't panic on a nil context.
	provCtx, provCtxCancel := context.WithCancel(context.Background())
	return &App{
		log:                 logger,
		wsServer:            ws.NewServerWithLogger(logger),
		wsPort:              wsPort,
		dbPath:              dbPath,
		providerCache:       cache.NewProviderCache(cachePath),
		tokenManager:        keychain.NewTokenManager(),
		repoLocks:           git.NewRepoLocks(),
		syncTrigger:         make(chan string, 8),
		syncSettings:        make(chan syncSettingsMsg, 4),
		syncInterval:        5 * time.Minute,
		autoSync:            true,
		provCtx:             provCtx,
		provCtxCancel:       provCtxCancel,
		classificationState: handler.NewClassificationState(),
	}
}

// SetRulesDir seeds the classification rules directory with the embedded
// built-ins (presence-gated) and constructs the engine provider shared by the
// sync writer and the comments handler. An empty dir skips seeding entirely;
// classification then runs on the embedded built-ins. Call before Run.
// The mode getter reads the live classification state, so a mode change from
// SET_CLASSIFICATION_SETTINGS invalidates fingerprints lazily.
func (a *App) SetRulesDir(dir string) {
	if dir != "" {
		if err := classify.SeedAndUpdate(dir); err != nil {
			a.log.Warn("rules dir unusable, classification falls back to embedded built-ins",
				"dir", dir, "err", err)
		}
	}
	a.classifier = classify.NewEngineProvider(dir, a.classificationState.Mode, a.log)
}

// SetSyncInterval configures the interval between automatic sync cycles.
func (a *App) SetSyncInterval(d time.Duration) {
	a.syncInterval = d
}

// SetAutoSync enables or disables the automatic periodic sync ticker.
// Manual FORCE_SYNC always works regardless of this setting.
func (a *App) SetAutoSync(enabled bool) {
	a.autoSync = enabled
}

// getProvider safely returns the current provider under a read lock.
func (a *App) getProvider() provider.Provider {
	a.provMu.RLock()
	defer a.provMu.RUnlock()
	return a.prov
}

// currentApplier returns the live orchestrator as an ActionApplier under a
// read lock, or nil when no provider is authenticated. Satisfies
// handler.ApplierGetter so SUBMIT_ACTION always routes to the current
// orchestrator even after an auth/provider replacement.
// Both prov and orchestrator must be non-nil; either being nil (e.g. after
// AUTH_DISCONNECT) causes a nil return so the handler rejects cleanly.
func (a *App) currentApplier() handler.ActionApplier {
	a.provMu.RLock()
	defer a.provMu.RUnlock()
	if a.prov == nil || a.orchestrator == nil {
		return nil
	}
	return a.orchestrator
}

// SetGitHubClientID sets the GitHub OAuth app client ID.
func (a *App) SetGitHubClientID(clientID string) {
	a.githubClientID = clientID
}

// SetGitHubClientSecret sets the GitHub OAuth app client secret used by the auth code flow.
func (a *App) SetGitHubClientSecret(secret string) {
	a.githubClientSecret = secret
}

func (a *App) githubToken() string {
	token, err := a.tokenManager.GetGitHubToken()
	if err != nil {
		return ""
	}
	return token
}

func (a *App) cloneRepoLocationRepo(ctx context.Context, repoURL, destination string, bare bool) error {
	return a.gitClient.CloneRepoToPath(ctx, repoURL, destination, bare)
}

// wsEventMsg is the envelope sent to WebSocket clients for orchestrator events.
type wsEventMsg struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// startEventBridge subscribes to the current orchestrator's event channel and
// broadcasts each event to all connected WebSocket clients. Any previously
// running bridge goroutine is cancelled first so stale goroutines do not leak
// or deliver events from a superseded orchestrator.
//
// Must be called with a.provMu held for writing (or before the App is shared).
func (a *App) startEventBridge(parent context.Context) {
	if a.bridgeCancel != nil {
		a.bridgeCancel()
	}
	if a.orchestrator == nil || a.wsServer == nil {
		a.bridgeCancel = nil
		return
	}

	ctx, cancel := context.WithCancel(parent)
	a.bridgeCancel = cancel

	o := a.orchestrator
	ch := o.GetEventChannel()
	go func() {
		defer o.UnsubscribeEventChannel(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				a.wsServer.Broadcast(wsEventMsg{Type: ev.Type, Data: ev.Data})
			}
		}
	}()
}

func (a *App) broadcastExpertiseEvent(eventType string, payload any) {
	a.wsServer.Broadcast(wsEventMsg{Type: eventType, Data: expertisePayloadToMap(payload)})
}

func expertisePayloadToMap(payload any) map[string]interface{} {
	if data, ok := payload.(map[string]interface{}); ok {
		return data
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return map[string]interface{}{"payload": payload}
	}
	data := map[string]interface{}{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return map[string]interface{}{"payload": payload}
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
	login, ok, err := a.currentUserStore.GetCurrentUser(ctx, "github")
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
	a.wsServer.Broadcast(wsEventMsg{Type: "EXPERTISE_NUDGE", Data: map[string]interface{}{"repo_id": repoID, "reviewed_count": count}})
}

// Run initializes the database, starts the WebSocket server, and blocks until
// a SIGTERM or SIGINT signal is received, then shuts down gracefully.
func (a *App) Run(ctx context.Context) error {
	var err error

	a.db, err = store.OpenDB(a.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer a.db.Close()

	if err := store.NewMigrationRunner(a.log, a.db.DB).Run(ctx); err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}

	a.log.Info("database initialized")

	if a.repoLocks == nil {
		a.repoLocks = git.NewRepoLocks()
	}
	if a.gitClient == nil {
		a.gitClient = git.NewClient(git.NewCache())
	}
	a.gitClient.SetLocks(a.repoLocks)
	a.gitClient.SetTokenSource(a.githubToken)

	diffStore := store.NewDiffStore(a.db.DB)

	// RangeDiffer: one-time probe at startup; result logged so ops can see capability.
	rangeDiffer := git.NewRangeDiffer(a.log, a.gitClient)
	a.log.Info("range-diff probe",
		"available", rangeDiffer.Available(),
		"reason", rangeDiffer.FallbackReason(),
	)

	// AnchorResolver: shared stateless instance for comment re-anchoring.
	a.anchorResolver = git.NewAnchorResolver(a.log)

	currentUserStore := store.NewCurrentUserStore(a.db)
	a.currentUserStore = currentUserStore // stored on App for SetProvider re-creation

	repoLocationStore := store.NewRepoLocationStore(a.db.DB)
	a.configureRepoLocationStore(repoLocationStore)
	a.resolveRepoPath = func(repoID string) (string, bool) {
		location, err := repoLocationStore.Get(context.Background(), repoID)
		if err != nil {
			a.log.Warn("expertise repo path resolve failed", "repo_id", repoID, "error", err)
			return "", false
		}
		if location == nil || location.Source != "local" || location.LocalPath == "" {
			return "", false
		}
		return location.LocalPath, true
	}
	expertiseStore := expertise.NewStore(a.db.DB)
	expertiseGit := expertise.NewGitCLI(a.log)
	expertiseIdentity := func(identityCtx context.Context) expertise.Identity {
		login, ok, err := currentUserStore.GetCurrentUser(identityCtx, "github")
		if err != nil || !ok {
			if err != nil {
				a.log.Warn("expertise identity current user lookup failed", "error", err)
			}
			return expertise.Identity{}
		}
		settings, err := handler.NewExpertiseSQLSettingsStore(a.db.DB, expertiseStore).ExpertiseSettings(identityCtx)
		if err != nil {
			a.log.Warn("expertise identity settings lookup failed", "error", err)
		}
		return expertise.Identity{Login: login, AliasEmails: settings.AliasEmails}
	}
	expertiseIndexer := expertise.NewIndexer(a.log, expertiseStore, expertiseGit, expertiseIdentity).WithEmitter(a.broadcastExpertiseEvent)
	expertiseUpdater := expertise.NewUpdater(a.log, expertiseStore, expertiseGit, expertiseIdentity)
	a.expertiseEngine = expertise.NewEngine(a.log, expertiseStore, expertiseGit, expertiseIndexer, expertiseUpdater)
	go func() {
		if err := a.expertiseEngine.RestartInterrupted(ctx, a.resolveRepoPath); err != nil {
			a.log.Warn("expertise restart interrupted indexing failed", "error", err)
		}
	}()
	defer a.expertiseEngine.Shutdown()

	// BYOK LLM tier (Phase 3.8): async post-commit/PR-open refinement.
	// Emit bridges straight to the WS broadcast; the tier no-ops outside
	// rules+llm / llm-all modes so constructing it unconditionally is safe.
	a.llmTier = llm.NewTier(llm.TierConfig{
		DB:     a.db.DB,
		Client: &liveByokClient{state: a.classificationState, tokens: a.tokenManager},
		Mode:   a.classificationState.Mode,
		Settings: func() llm.SettingsSnapshot {
			s := a.classificationState.Snapshot()
			return llm.SettingsSnapshot{Provider: s.Provider, Model: s.Model}
		},
		Emit: func(eventType string, data map[string]interface{}) {
			a.wsServer.Broadcast(wsEventMsg{Type: eventType, Data: data})
		},
		Log: slogPrintf{a.log},
	})
	llmKick := func(prID string) { a.llmTier.KickPR(ctx, prID) }
	a.llmKick = llmKick

	if a.prov != nil {
		a.provMu.Lock()
		a.attachOrchestratorLocked(ctx, a.prov, "github")
		a.provMu.Unlock()
		a.log.Info("orchestrator initialized at startup (token from keychain)")
		if a.replayOnStart {
			a.log.Info("replaying offline action queue after startup")
			if err := a.ReplayQueuedActions(ctx); err != nil {
				a.log.Error("startup queue replay failed", "error", err)
			}
		}
	}

	authDeps := handler.AuthDeps{
		Log:       a.log,
		Tokens:    a.tokenManager,
		Cache:     a.providerCache,
		Broadcast: a.wsServer.Broadcast,
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
		CurrentUserWriter:  currentUserStore,
		CurrentUserClearer: currentUserStore,
	}

	authCheck := handler.NewAuthCheckHandler(authDeps)

	a.wsServer.RegisterHandler("AUTH_START", handler.NewAuthStartHandler(authDeps))
	a.wsServer.RegisterHandler("AUTH_CHECK", authCheck)
	a.wsServer.RegisterHandler("AUTH_DISCONNECT", handler.NewAuthDisconnectHandler(authDeps, authCheck))
	// REQUEST_DIFF: enriched with per-file content_hash and viewed flag.
	a.wsServer.RegisterHandler("REQUEST_DIFF", handler.NewRequestDiffHandlerWithDeps(handler.DiffHandlerDeps{
		DiffStore:   diffStore,
		ViewedStore: a.db,
		CurrentUser: currentUserStore,
		Log:         a.log,
	}))
	a.wsServer.RegisterHandler("REQUEST_PR_LIST", handler.NewRequestPRListHandlerWithExpertise(a.db.DB, a.expertiseEngine, a.log))
	a.wsServer.RegisterHandler("REQUEST_COMMENTS",
		handler.NewRequestCommentsHandlerWithLLMKick(a.db.DB, a.log, a.classifier, llmKick))
	dismissDeps := handler.DismissDeps{
		DB:          a.db.DB,
		Dismissals:  store.NewDismissals(a.db.DB),
		CurrentUser: currentUserStore,
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
		handler.NewToggleWatchHandler(a.db.DB, a.syncTrigger, a.log))
	a.wsServer.RegisterHandler("FORCE_SYNC", handler.NewForceSyncHandler(a.syncTrigger, a.log))
	a.wsServer.RegisterHandler("SET_SYNC_SETTINGS", handler.NewSetSyncSettingsHandler(a.syncSettings, a.log))
	// Phase 2.1+2.2 handlers.
	a.wsServer.RegisterHandler("GET_PATCHSETS",
		handler.NewListPatchsetsHandler(a.db, a.log))
	a.wsServer.RegisterHandler("REQUEST_RANGE_DIFF",
		handler.NewRangeDiffHandlerWithDeps(handler.RangeDiffHandlerDeps{
			Runner:       rangeDiffer,
			Patchsets:    a.db,
			LastReviewed: a.db,
			CurrentUser:  currentUserStore,
			PlainDiff:    diffStore,
		}, a.log))
	a.wsServer.RegisterHandler("MARK_FILE_VIEWED",
		handler.NewMarkFileViewedHandlerWithDiffStore(a.db, currentUserStore, diffStore, a.log))
	// Phase 2.4 handlers.
	a.wsServer.RegisterHandler("GET_REVIEW_THREADS",
		handler.NewGetReviewThreadsHandler(a.db, a.log))
	a.wsServer.RegisterHandler("SUBMIT_ACTION",
		handler.NewSubmitActionHandler(a.currentApplier, a.log))

	expertiseRepoStore := handler.NewExpertiseSQLRepoStore(a.db.DB)
	expertiseDataStore := handler.NewExpertiseSQLDataStore(a.db.DB, expertiseStore)
	expertiseSettingsStore := handler.NewExpertiseSQLSettingsStore(a.db.DB, expertiseStore)
	expertiseDeps := handler.ExpertiseHandlerDeps{
		Engine:      a.expertiseEngine,
		Repos:       expertiseRepoStore,
		Data:        expertiseDataStore,
		Settings:    expertiseSettingsStore,
		CurrentUser: currentUserStore,
		Log:         a.log,
	}
	a.wsServer.RegisterHandler("EXPERTISE_ENABLE", handler.NewExpertiseEnableHandler(expertiseDeps))
	a.wsServer.RegisterHandler("EXPERTISE_DISABLE", handler.NewExpertiseDisableHandler(expertiseDeps))
	a.wsServer.RegisterHandler("GET_EXPERTISE", handler.NewGetExpertiseHandler(expertiseDeps))
	a.wsServer.RegisterHandler("EXPERTISE_NUDGE_DISMISS", handler.NewExpertiseNudgeDismissHandler(expertiseDeps))
	a.wsServer.RegisterHandler("GET_EXPERTISE_SETTINGS", handler.NewGetExpertiseSettingsHandler(expertiseDeps))
	a.wsServer.RegisterHandler("SET_EXPERTISE_SETTINGS", handler.NewSetExpertiseSettingsHandler(expertiseDeps))

	deleteCacheDir := func(path string) error {
		return git.DeleteCacheDir(path)
	}
	cacheDeps := handler.CacheMaintenanceDeps{
		Store:          repoLocationStore,
		Locks:          a.repoLocks,
		DeleteCacheDir: deleteCacheDir,
		SweepOrphans:   git.SweepOrphans,
		Log:            a.log,
	}
	a.wsServer.RegisterHandler("VALIDATE_DEV_FOLDER", handler.NewValidateDevFolderHandler(a.log))
	a.wsServer.RegisterHandler("SCAN_DEV_FOLDER", handler.NewScanDevFolderHandler(repoLocationStore, a.syncTrigger, a.log))
	a.wsServer.RegisterHandler("ADD_REPO_BY_URL", handler.NewAddRepoByURLHandler(handler.AddRepoByURLDeps{
		DB:        a.db.DB,
		Store:     repoLocationStore,
		Clone:     a.cloneRepoLocationRepo,
		CacheRoot: git.CacheRoot,
		Log:       a.log,
	}))
	a.wsServer.RegisterHandler("REMOVE_REPO_PREFLIGHT", handler.NewRemoveRepoPreflightHandler(a.db.DB, a.log))
	a.wsServer.RegisterHandler("GET_CACHE_STATS", handler.NewGetCacheStatsHandler(cacheDeps))
	a.wsServer.RegisterHandler("PRUNE_CACHE", handler.NewPruneCacheHandler(cacheDeps))
	a.wsServer.RegisterHandler("CLEAR_CACHE", handler.NewClearCacheHandler(cacheDeps))
	a.wsServer.RegisterHandler("DELETE_CACHE_REPO", handler.NewDeleteCacheRepoHandler(cacheDeps))
	a.wsServer.RegisterHandler("RECOVER_REPO_PATH", handler.NewRecoverRepoPathHandler(repoLocationStore, a.log))
	a.wsServer.RegisterHandler("RECOVER_REPO_TO_CACHE", handler.NewRecoverRepoToCacheHandler(handler.RecoverRepoToCacheDeps{
		DB:        a.db.DB,
		Store:     repoLocationStore,
		Clone:     a.cloneRepoLocationRepo,
		CacheRoot: git.CacheRoot,
		Log:       a.log,
	}))
	a.wsServer.RegisterHandler("REMOVE_REPO", handler.NewRemoveRepoHandler(handler.RemoveRepoDeps{
		DB:             a.db.DB,
		Store:          repoLocationStore,
		Locks:          a.repoLocks,
		DeleteCacheDir: deleteCacheDir,
		Log:            a.log,
	}))

	wsAddr := fmt.Sprintf(":%d", a.wsPort)
	if err := a.wsServer.Start(wsAddr); err != nil {
		return fmt.Errorf("failed to start ws server: %w", err)
	}

	actualPort := a.wsServer.Port()
	a.log.Info("WebSocket server listening", "addr", wsAddr, "port", actualPort)

	readyOut := a.readyWriter
	if readyOut == nil {
		readyOut = os.Stdout
	}
	if err := emitReady(readyOut, actualPort); err != nil {
		a.log.Warn("failed to emit ready signal", "error", err)
	}

	go a.runSyncLoop(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	select {
	case <-sigChan:
		a.log.Info("shutdown signal received")
	case <-ctx.Done():
		a.log.Info("context cancelled")
	}

	return a.wsServer.Stop()
}
