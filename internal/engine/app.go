package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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
	"github.com/itterno/bistro/internal/prsync"
	"github.com/itterno/bistro/internal/services"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/currentuser"
	"github.com/itterno/bistro/internal/store/prdiff"
	"github.com/itterno/bistro/internal/store/repolocation"
)

// SyncSettingsMsg carries runtime sync preferences to the sync loop.
type SyncSettingsMsg struct {
	AutoSync bool
	Interval time.Duration
}

type syncSettingsMsg = SyncSettingsMsg

// App owns the in-process sync engine and its background lifetimes.
type App struct {
	log                 *slog.Logger
	emitter             services.Emitter
	panics              services.PanicReporter
	serviceSet          *services.Set
	db                  *store.DB
	dbPath              string
	orchestrator        *prsync.Orchestrator
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
	syncSettingsMu      sync.RWMutex // protects settings retained across sync-loop attempts
	provMu              sync.RWMutex
	syncInterval        time.Duration
	autoSync            bool
	runCtx              context.Context
	cancel              context.CancelFunc
	loops               sync.WaitGroup
	// bridgeCancel cancels the event-bridge goroutine for the current orchestrator.
	// Replaced each time a new orchestrator is wired up.
	bridgeCancel context.CancelFunc
	// replayOnStart causes Start() to replay the offline action queue after the
	// production orchestrator (with DB and queue) is attached.
	replayOnStart bool
	// provCtx / provCtxCancel form a per-provider generation context.
	// It is derived from the Start ctx (or context.Background before Start) and
	// cancelled on every provider disconnect/replacement.  The sync loop,
	// event bridge, and orchestrator liveness all hold a reference; cancelling
	// it propagates to all of them simultaneously.
	provCtx       context.Context
	provCtxCancel context.CancelFunc
	// currentUserStore is set in Start() and shared across all orchestrators.
	currentUserStore *currentuser.Store
	// anchorResolver is set in Start() and shared across all orchestrators.
	anchorResolver *anchor.Resolver
	// classifier is the Phase 3 comment-classification engine provider,
	// shared by the sync writer and the comments service. Its mode getter
	// reads classificationState, so a mode change lazily invalidates the
	// rule fingerprint on the next Current() call.
	classifier *classify.EngineProvider
	// classificationState holds the live classification settings pushed via
	// SET_CLASSIFICATION_SETTINGS (mode, dismissal scope, provider, model).
	classificationState *services.ClassificationState
	// llmTier is the async BYOK classification tier (Phase 3.8); kicked
	// post-sync-commit and on PR open, no-op outside LLM modes.
	llmTier *llm.Tier
	// llmKick kicks the tier for one PR; set in Start, read by orchestrator
	// wiring (post-commit hook) and the comments handler (PR-open kick).
	llmKick func(prID string)
	// expertiseEngine is the Phase 4 local expertise scorer, shared by handlers
	// and sync/review hooks.
	expertiseEngine *expertise.Engine
	// resolveRepoPath maps repo IDs to local checkout paths for expertise hooks.
	resolveRepoPath prsync.RepoPathResolver
}

// SetEmitter configures push event delivery before services are constructed.
func (a *App) SetEmitter(emitter services.Emitter) {
	a.emitter = emitter
}

// SetPanicReporter configures reporting for recovered service panics.
func (a *App) SetPanicReporter(reporter services.PanicReporter) {
	a.panics = reporter
}

type runStores struct {
	currentUser  *currentuser.Store
	repoLocation *repolocation.Store
	expertise    *expertise.Store
	diff         *prdiff.Store
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
		repoLocation: repolocation.New(a.db.DB),
		expertise:    expertise.NewStore(a.db.DB),
		diff:         prdiff.New(a.db.DB),
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

// Start prepares the engine and launches its supervised loops without blocking.
// Call SetEmitter and SetPanicReporter before Start; Stop owns shutdown.
func (a *App) Start(ctx context.Context) error {
	if a.cancel != nil {
		return errors.New("engine already started")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := os.MkdirAll(filepath.Dir(a.dbPath), 0o755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	if err := a.openDatabase(ctx); err != nil {
		return err
	}
	a.runCtx, a.cancel = context.WithCancel(ctx)
	stores := a.prepareStores()
	a.prepareExpertise(a.runCtx, stores.expertise)
	a.prepareLLMTier(a.runCtx)
	a.restoreStartupProvider()
	a.initializeStartupProvider(a.runCtx)
	a.serviceSet = a.buildServices(a.runCtx)
	if a.replayOnStart && a.getProvider() != nil {
		a.startLoop(a.runCtx, "startup-replay", func(ctx context.Context) {
			a.log.Info("replaying offline action queue after startup")
			if err := a.ReplayQueuedActions(ctx); err != nil {
				a.log.Error("startup queue replay failed", "error", err)
			}
		})
	}
	a.startLoop(a.runCtx, "sync", a.runSyncLoop)
	return nil
}

// Stop cancels background work, waits for it, and then closes the database.
func (a *App) Stop() {
	if a.cancel == nil {
		return
	}
	a.cancel()
	a.provMu.Lock()
	if a.provCtxCancel != nil {
		a.provCtxCancel()
	}
	a.provMu.Unlock()
	a.loops.Wait()
	a.expertiseEngine.Shutdown()
	a.llmTier.Wait()
	if err := a.db.Close(); err != nil {
		a.log.Error("close engine database", "error", err)
	}
	a.provMu.Lock()
	a.db = nil
	a.orchestrator = nil
	a.provMu.Unlock()
	a.cancel = nil
	a.serviceSet = nil
}

// ApplyClassificationSettings uses the same validation and state as service calls.
func (a *App) ApplyClassificationSettings(settings services.ClassificationSettings) error {
	s := services.Classification{State: a.classificationState, Panics: a.panics, Log: a.log}
	_, err := s.SetSettings(context.Background(), settings)
	return err
}

// ApplySyncSettings forwards sync preferences with a 30s–3600s interval clamp.
func (a *App) ApplySyncSettings(autoSync bool, interval time.Duration) {
	interval = min(max(interval, 30*time.Second), time.Hour)
	select {
	case a.syncSettings <- SyncSettingsMsg{AutoSync: autoSync, Interval: interval}:
	default:
		a.log.Warn("sync settings update dropped; channel full")
	}
}
