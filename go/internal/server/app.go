package server

import (
	"context"
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
	providerCache      *cache.ProviderCache
	tokenManager       *keychain.TokenManager
	syncTrigger        chan string
	syncSettings       chan syncSettingsMsg
	provMu             stdsync.RWMutex
	syncInterval       time.Duration
	autoSync           bool
	readyWriter        io.Writer
}

// SetReadyWriter overrides where the machine-readable ready signal is written.
// Defaults to os.Stdout when unset.
func (a *App) SetReadyWriter(w io.Writer) {
	a.readyWriter = w
}

// SetProvider configures the GitHub provider used by the sync orchestrator.
// If the database and git client are already available, it also (re)creates the
// orchestrator so the sync loop can start working immediately after auth.
func (a *App) SetProvider(prov provider.Provider) {
	a.provMu.Lock()
	defer a.provMu.Unlock()
	a.prov = prov

	if prov != nil && a.db != nil {
		diffStore := store.NewDiffStore(a.db.DB)
		a.orchestrator = sync.NewOrchestrator(a.log, prov, a.db, a.gitClient).
			WithDiffStore(diffStore)
		a.log.Info("orchestrator initialized after auth")
	}
}

// SetGitClient configures the git client used by the sync orchestrator.
// Must be called before Run.
func (a *App) SetGitClient(gitClient *git.Client) {
	a.gitClient = gitClient
}

// ReplayQueuedActions replays any pending offline queue actions.
// It is a no-op when the orchestrator has not been set up.
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
	return &App{
		log:           logger,
		wsServer:      ws.NewServerWithLogger(logger),
		wsPort:        wsPort,
		dbPath:        dbPath,
		providerCache: cache.NewProviderCache(cachePath),
		tokenManager:  keychain.NewTokenManager(),
		syncTrigger:  make(chan string, 8),
		syncSettings: make(chan syncSettingsMsg, 4),
		syncInterval: 5 * time.Minute,
		autoSync:     true,
	}
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

// SetGitHubClientID sets the GitHub OAuth app client ID.
func (a *App) SetGitHubClientID(clientID string) {
	a.githubClientID = clientID
}

// SetGitHubClientSecret sets the GitHub OAuth app client secret used by the auth code flow.
func (a *App) SetGitHubClientSecret(secret string) {
	a.githubClientSecret = secret
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

	diffStore := store.NewDiffStore(a.db.DB)

	if a.prov != nil {
		a.orchestrator = sync.NewOrchestrator(a.log, a.prov, a.db, a.gitClient).
			WithDiffStore(diffStore)
		a.log.Info("orchestrator initialized at startup (token from keychain)")
	}

	authDeps := handler.AuthDeps{
		Log:    a.log,
		Tokens: a.tokenManager,
		Cache:  a.providerCache,
		Broadcast: a.wsServer.Broadcast,
		SetProvider: func(prov provider.Provider, name string) {
			a.provMu.Lock()
			defer a.provMu.Unlock()
			if prov == nil {
				if a.activeProviderName == name {
					a.prov = nil
					a.activeProviderName = ""
				}
				return
			}
			a.prov = prov
			a.activeProviderName = name
			if a.db != nil {
				diffStore := store.NewDiffStore(a.db.DB)
				a.orchestrator = sync.NewOrchestrator(a.log, prov, a.db, a.gitClient).
					WithDiffStore(diffStore)
				a.log.Info("orchestrator initialized after auth")
			}
		},
		GetClientID:     func() string { return a.githubClientID },
		GetClientSecret: func() string { return a.githubClientSecret },
	}

	authCheck := handler.NewAuthCheckHandler(authDeps)

	a.wsServer.RegisterHandler("AUTH_START", handler.NewAuthStartHandler(authDeps))
	a.wsServer.RegisterHandler("AUTH_CHECK", authCheck)
	a.wsServer.RegisterHandler("AUTH_DISCONNECT", handler.NewAuthDisconnectHandler(authDeps, authCheck))
	a.wsServer.RegisterHandler("REQUEST_DIFF", handler.NewRequestDiffHandler(diffStore, a.log))
	a.wsServer.RegisterHandler("REQUEST_PR_LIST", handler.NewRequestPRListHandler(a.db.DB, a.log))
	a.wsServer.RegisterHandler("REQUEST_COMMENTS", handler.NewRequestCommentsHandler(a.db.DB, a.log))
	a.wsServer.RegisterHandler("REQUEST_REPO_LIST",
		handler.NewRequestRepoListHandler(a.getProvider, a.db.DB, a.log))
	a.wsServer.RegisterHandler("TOGGLE_WATCH_REPO",
		handler.NewToggleWatchHandler(a.db.DB, a.syncTrigger, a.log))
	a.wsServer.RegisterHandler("FORCE_SYNC", handler.NewForceSyncHandler(a.syncTrigger, a.log))
	a.wsServer.RegisterHandler("SET_SYNC_SETTINGS", handler.NewSetSyncSettingsHandler(a.syncSettings, a.log))

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
