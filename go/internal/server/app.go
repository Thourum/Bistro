package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/itterno/bistro/internal/store/currentuser"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	stdsync "sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/itterno/bistro/internal/cache"
	"github.com/itterno/bistro/internal/classify"
	"github.com/itterno/bistro/internal/classify/llm"
	"github.com/itterno/bistro/internal/expertise"
	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/git/anchor"
	"github.com/itterno/bistro/internal/git/clonecache"
	"github.com/itterno/bistro/internal/git/repolock"
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
	log                 *slog.Logger
	db                  *store.DB
	wsServer            *ws.Server
	wsPort              int
	dbPath              string
	orchestrator        *sync.Orchestrator
	prov                provider.Provider
	activeProviderName  string
	gitClient           *git.Client
	githubClientID      string
	githubClientSecret  string
	authCallbackTimeout time.Duration
	repoLocks           *repolock.Locks
	providerCache       *cache.ProviderCache
	tokenManager        *keychain.TokenManager
	syncTrigger         chan string
	pendingWatchSync    atomic.Bool
	syncSettings        chan syncSettingsMsg
	provMu              stdsync.RWMutex
	syncInterval        time.Duration
	autoSync            bool
	readyWriter         io.Writer
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
	currentUserStore *currentuser.Store
	// anchorResolver is set in Run() and shared across all orchestrators.
	anchorResolver *anchor.Resolver
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
		repoLocks:           repolock.New(),
		syncTrigger:         make(chan string, 8),
		syncSettings:        make(chan syncSettingsMsg, 4),
		syncInterval:        5 * time.Minute,
		autoSync:            true,
		provCtx:             provCtx,
		provCtxCancel:       provCtxCancel,
		classificationState: handler.NewClassificationState(),
	}
}

type runStores struct {
	currentUser  *currentuser.Store
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
		a.repoLocks = repolock.New()
	}
	if a.gitClient == nil {
		a.gitClient = git.NewClient(clonecache.New())
	}
	a.gitClient.SetLocks(a.repoLocks)
	a.gitClient.SetTokenSource(a.githubToken)

	stores := runStores{
		currentUser:  currentuser.New(a.db),
		repoLocation: store.NewRepoLocationStore(a.db.DB),
		expertise:    expertise.NewStore(a.db.DB),
		diff:         store.NewDiffStore(a.db.DB),
	}
	a.anchorResolver = anchor.NewResolver()
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

// Run initializes the database, starts the WebSocket server, and blocks until
// a SIGTERM, SIGINT, or context cancellation, then shuts down.
func (a *App) Run(ctx context.Context) (runErr error) {
	ctx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	if err := a.openDatabase(ctx); err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, a.db.Close()) }()

	stores := a.prepareStores()
	a.prepareExpertise(ctx, stores.expertise)
	defer a.expertiseEngine.Shutdown()
	a.prepareLLMTier(ctx)
	a.initializeStartupProvider(ctx)
	a.registerAuthHandlers(ctx, stores)
	waitReviewQueue := a.registerReviewHandlers(ctx, stores)
	a.registerDiffAndCheckoutHandlers(stores)
	a.registerExpertiseHandlers(stores)
	waitScans := a.registerRepoLocationHandlers(ctx, stores)
	defer func() {
		runErr = errors.Join(runErr, a.wsServer.Stop())
		cancelRun()
		waitReviewQueue()
		waitScans()
	}()

	wsAddr := wsListenAddr(a.wsPort)
	if err := a.wsServer.Start(wsAddr); err != nil {
		return fmt.Errorf("start ws server: %w", err)
	}
	actualPort := a.wsServer.Port()
	a.log.Info("WebSocket server listening", "addr", wsAddr, "port", actualPort)
	readyOut := a.readyWriter
	if readyOut == nil {
		readyOut = os.Stdout
	}
	if err := emitReady(readyOut, actualPort); err != nil {
		return fmt.Errorf("emit ready signal: %w", err)
	}
	go a.runSyncLoop(ctx)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigChan)
	select {
	case <-sigChan:
		a.log.Info("shutdown signal received")
	case <-ctx.Done():
		a.log.Info("context cancelled")
	}
	return nil
}
