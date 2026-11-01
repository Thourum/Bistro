package engine

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/itterno/bistro/internal/auth"
	"github.com/itterno/bistro/internal/cache"
	"github.com/itterno/bistro/internal/keychain"
	"github.com/itterno/bistro/internal/services"
)

const (
	defaultDBPath       = "./data/sync.db"
	defaultSyncInterval = 5 * time.Minute
	syncTriggerBuffer   = 8
	syncSettingsBuffer  = 4
)

// Options configures the in-process engine. Set emitters before Start.
type Options struct {
	DBPath              string
	Log                 *slog.Logger
	SyncInterval        time.Duration
	AutoSync            bool // The caller sets this explicitly; false disables automatic sync.
	ReplayQueue         bool // The caller sets this explicitly; false skips startup replay.
	AuthCallbackTimeout time.Duration
	RulesDir            string
	GitHubClientID      string
	GitHubClientSecret  string
}

// New constructs the engine; Start opens the database and restores credentials.
func New(opts Options) *App {
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	if opts.DBPath == "" {
		opts.DBPath = defaultDBPath
	}
	if opts.SyncInterval <= 0 {
		opts.SyncInterval = defaultSyncInterval
	}
	if opts.AuthCallbackTimeout <= 0 {
		opts.AuthCallbackTimeout = auth.CallbackTimeout
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	provCtx, cancel := context.WithCancel(context.Background())
	app := &App{
		log: opts.Log, dbPath: opts.DBPath,
		providerCache: cache.NewProviderCache(filepath.Join(cacheDir, "bistro", "providers.json")),
		tokenManager:  keychain.NewTokenManager(),
		syncTrigger:   make(chan string, syncTriggerBuffer),
		syncSettings:  make(chan syncSettingsMsg, syncSettingsBuffer),
		syncInterval:  opts.SyncInterval, autoSync: opts.AutoSync, replayOnStart: opts.ReplayQueue,
		authCallbackTimeout: opts.AuthCallbackTimeout,
		githubClientID:      opts.GitHubClientID, githubClientSecret: opts.GitHubClientSecret,
		provCtx: provCtx, provCtxCancel: cancel,
		classificationState: services.NewClassificationState(),
	}
	app.syncStatus = newSyncStatusTracker(app.publishSyncStatus)
	app.SetEmitter(services.NopEmitter{})
	if opts.RulesDir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			opts.Log.Warn("no user config dir; classification uses embedded built-ins", "error", err)
		} else {
			opts.RulesDir = filepath.Join(configDir, "bistro", "rules")
		}
	}
	app.configureRules(opts.RulesDir)
	return app
}
