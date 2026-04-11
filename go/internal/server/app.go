package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	stdsync "sync"
	"time"

	"github.com/samuelasvanyi/bistro/internal/auth"
	"github.com/samuelasvanyi/bistro/internal/cache"
	"github.com/samuelasvanyi/bistro/internal/git"
	"github.com/samuelasvanyi/bistro/internal/keychain"
	"github.com/samuelasvanyi/bistro/internal/provider"
	github "github.com/samuelasvanyi/bistro/internal/provider/github"
	"github.com/samuelasvanyi/bistro/internal/store"
	"github.com/samuelasvanyi/bistro/internal/sync"
	"github.com/samuelasvanyi/bistro/internal/ws"
)

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
	authMu         stdsync.Mutex
	callbackServer *auth.CallbackServer // non-nil while auth is in progress; guarded by authMu
	providerCache  *cache.ProviderCache
	tokenManager       *keychain.TokenManager
	syncTrigger        chan string
	provMu             stdsync.RWMutex
	syncInterval       time.Duration
}

// SetProvider configures the GitHub provider used by the sync orchestrator.
// If the database and git client are already available, it also (re)creates the
// orchestrator so the sync loop can start working immediately after auth.
func (a *App) SetProvider(prov provider.Provider) {
	a.provMu.Lock()
	defer a.provMu.Unlock()
	a.prov = prov

	if prov != nil && a.db != nil && a.gitClient != nil {
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
		syncTrigger:   make(chan string, 8),
		syncInterval:  5 * time.Minute,
	}
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
		syncTrigger:   make(chan string, 8),
		syncInterval:  5 * time.Minute,
	}
}

// SetSyncInterval configures the interval between automatic sync cycles.
func (a *App) SetSyncInterval(d time.Duration) {
	a.syncInterval = d
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

// handleAuthStart processes an AUTH_START WebSocket message by starting the
// OAuth authorization code flow. Returns AUTH_REDIRECT to the requesting client
// only (not broadcast). AUTH_SUCCESS/AUTH_FAILED are broadcast to all clients.
func (a *App) handleAuthStart(msg ws.InboundMessage) (interface{}, error) {
	var data struct {
		Provider string `json:"provider"`
	}
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return nil, fmt.Errorf("invalid AUTH_START data: %w", err)
	}

	if data.Provider != "github" {
		return map[string]interface{}{
			"id":   msg.ID,
			"type": "AUTH_FAILED",
			"data": map[string]string{"error": "unsupported provider: " + data.Provider},
		}, nil
	}

	a.authMu.Lock()
	if a.callbackServer != nil {
		a.authMu.Unlock()
		return map[string]interface{}{
			"id":   msg.ID,
			"type": "AUTH_FAILED",
			"data": map[string]string{"error": "auth_already_in_progress"},
		}, nil
	}

	if a.githubClientID == "" || a.githubClientSecret == "" {
		a.authMu.Unlock()
		return map[string]interface{}{
			"id":   msg.ID,
			"type": "AUTH_FAILED",
			"data": map[string]string{"error": "GITHUB_CLIENT_ID or GITHUB_CLIENT_SECRET not configured"},
		}, nil
	}

	cfg := auth.ProviderConfig{
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		Scopes:       []string{"repo", "read:user"},
		ClientID:     a.githubClientID,
		ClientSecret: a.githubClientSecret,
	}

	authURL, state, err := auth.BuildAuthURL(cfg, auth.RedirectURI)
	if err != nil {
		a.authMu.Unlock()
		return nil, fmt.Errorf("building auth URL: %w", err)
	}

	resultCh := make(chan auth.CallbackResult, 1)
	a.callbackServer = auth.NewCallbackServer(cfg, state, auth.RedirectURI, resultCh)
	a.authMu.Unlock()

	go a.runAuthCodeFlow(data.Provider, resultCh)

	return map[string]interface{}{
		"id":   msg.ID,
		"type": "AUTH_REDIRECT",
		"data": map[string]interface{}{
			"provider": data.Provider,
			"url":      authURL,
		},
	}, nil
}

// runAuthCodeFlow starts the callback server and waits for the OAuth result,
// then broadcasts AUTH_SUCCESS or AUTH_FAILED to all connected clients.
func (a *App) runAuthCodeFlow(providerName string, resultCh <-chan auth.CallbackResult) {
	serverErrCh := make(chan error, 1)
	go func() {
		if err := a.callbackServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
		}
		close(serverErrCh)
	}()

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		a.authMu.Lock()
		if a.callbackServer != nil {
			_ = a.callbackServer.Shutdown(ctx)
		}
		a.callbackServer = nil
		a.authMu.Unlock()
	}()

	select {
	case result := <-resultCh:
		if result.Err != nil {
			a.log.Error("auth callback failed", "error", result.Err)
			a.wsServer.Broadcast(map[string]interface{}{
				"type": "AUTH_FAILED",
				"data": map[string]string{"error": result.Err.Error()},
			})
			return
		}

		if err := a.tokenManager.Store(providerName, result.Token); err != nil {
			a.log.Error("failed to store token", "error", err)
			a.wsServer.Broadcast(map[string]interface{}{
				"type": "AUTH_FAILED",
				"data": map[string]string{"error": "failed to persist token"},
			})
			return
		}

		client := github.NewClient(result.Token)
		a.SetProvider(client)
		a.activeProviderName = providerName

		username := ""
		if u, err := a.fetchUsername(providerName, result.Token); err == nil {
			username = u
			a.providerCache.Set(providerName, u)
		} else {
			a.log.Warn("failed to fetch username after OAuth", "error", err)
		}

		a.wsServer.Broadcast(map[string]interface{}{
			"type": "AUTH_SUCCESS",
			"data": map[string]string{"provider": providerName, "username": username},
		})
		a.log.Info("OAuth authorization code flow completed", "provider", providerName)

	case err := <-serverErrCh:
		a.log.Error("callback server error", "error", err)
		a.wsServer.Broadcast(map[string]interface{}{
			"type": "AUTH_FAILED",
			"data": map[string]string{"error": err.Error()},
		})

	case <-time.After(auth.CallbackTimeout):
		a.log.Warn("auth callback timed out")
		a.wsServer.Broadcast(map[string]interface{}{
			"type": "AUTH_FAILED",
			"data": map[string]string{"error": "timeout"},
		})
	}
}

var knownProviders = []string{"github", "gitlab", "gitea"}

// handleAuthCheck responds with the current authentication state for all known providers.
func (a *App) handleAuthCheck(msg ws.InboundMessage) (interface{}, error) {
	type providerInfo struct {
		Provider string `json:"provider"`
		Username string `json:"username"`
	}

	var providers []providerInfo
	authenticated := false

	for _, p := range knownProviders {
		token, err := a.tokenManager.Get(p)
		if err != nil || token == "" {
			continue
		}
		authenticated = true

		username, expired, found := a.providerCache.Get(p)
		if found && !expired {
			providers = append(providers, providerInfo{Provider: p, Username: username})
			continue
		}

		fetched, fetchErr := a.fetchUsername(p, token)
		if fetchErr != nil {
			a.log.Warn("failed to fetch username", "provider", p, "error", fetchErr)
			// Always include the provider — use stale cache or empty string
			providers = append(providers, providerInfo{Provider: p, Username: username})
			continue
		}
		a.providerCache.Set(p, fetched)
		providers = append(providers, providerInfo{Provider: p, Username: fetched})
	}

	return map[string]interface{}{
		"id":   msg.ID,
		"type": "AUTH_CHECK_RESULT",
		"data": map[string]interface{}{
			"authenticated": authenticated,
			"providers":     providers,
		},
	}, nil
}

// fetchUsername retrieves the authenticated username for the given provider using the supplied token.
// Uses a 10-second timeout to avoid blocking indefinitely on slow provider APIs.
func (a *App) fetchUsername(providerName, token string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch providerName {
	case "github":
		client := github.NewClient(token)
		return client.GetUsername(ctx)
	default:
		return "", fmt.Errorf("username fetch not implemented for %s", providerName)
	}
}

// handleAuthDisconnect removes the token and cache entry for a provider and
// returns a refreshed AUTH_DISCONNECT_RESULT with the remaining auth state.
func (a *App) handleAuthDisconnect(msg ws.InboundMessage) (interface{}, error) {
	var data struct {
		Provider string `json:"provider"`
	}
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return nil, fmt.Errorf("invalid AUTH_DISCONNECT data: %w", err)
	}

	if err := a.tokenManager.Delete(data.Provider); err != nil {
		a.log.Warn("failed to delete token from keychain", "provider", data.Provider, "error", err)
	}
	a.providerCache.Delete(data.Provider)
	if a.activeProviderName == data.Provider {
		a.provMu.Lock()
		a.prov = nil
		a.activeProviderName = ""
		a.provMu.Unlock()
	}

	checkResp, err := a.handleAuthCheck(msg)
	if err != nil {
		return nil, err
	}
	respMap := checkResp.(map[string]interface{})
	respMap["type"] = "AUTH_DISCONNECT_RESULT"
	return respMap, nil
}

// runSyncLoop runs the periodic sync goroutine. It exits when ctx is cancelled.
func (a *App) runSyncLoop(ctx context.Context) {
	ticker := time.NewTicker(a.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.syncWatchedRepos(ctx)
		case repoID := <-a.syncTrigger:
			a.syncSingleRepo(ctx, repoID)
		}
	}
}

// syncWatchedRepos syncs all watched repos. No-ops if not yet authenticated.
func (a *App) syncWatchedRepos(ctx context.Context) {
	if a.getProvider() == nil || a.orchestrator == nil {
		return
	}

	rows, err := a.db.DB.QueryContext(ctx,
		"SELECT id, provider, owner, repo FROM repos WHERE watched = 1",
	)
	if err != nil {
		a.log.Error("syncWatchedRepos: query failed", "err", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, prov, owner, repo string
		if err := rows.Scan(&id, &prov, &owner, &repo); err != nil {
			a.log.Error("syncWatchedRepos: scan failed", "err", err)
			continue
		}
		req := &sync.SyncRequest{
			Provider:   prov,
			Owner:      owner,
			Repo:       repo,
			IncludePRs: true,
			State:      "open",
		}
		result, syncErr := a.orchestrator.Sync(ctx, req)
		if syncErr != nil {
			a.log.Error("syncWatchedRepos: sync failed", "repo", id, "err", syncErr)
			continue
		}
		a.log.Info("syncWatchedRepos: synced", "repo", id, "prs", result.PRCount)
	}
	if err := rows.Err(); err != nil {
		a.log.Error("syncWatchedRepos: rows error", "err", err)
	}

	a.wsServer.Broadcast(map[string]interface{}{
		"type": "SYNC_COMPLETE",
		"data": map[string]interface{}{},
	})
}

// syncSingleRepo syncs a single repo identified by its repo_id (e.g. "github:owner/repo").
func (a *App) syncSingleRepo(ctx context.Context, repoID string) {
	if a.getProvider() == nil || a.orchestrator == nil {
		return
	}

	// Parse "provider:owner/repo"
	colonIdx := -1
	for i, c := range repoID {
		if c == ':' {
			colonIdx = i
			break
		}
	}
	if colonIdx < 0 {
		a.log.Error("syncSingleRepo: invalid repo_id format", "repo_id", repoID)
		return
	}
	providerName := repoID[:colonIdx]
	ownerRepo := repoID[colonIdx+1:]
	slashIdx := -1
	for i, c := range ownerRepo {
		if c == '/' {
			slashIdx = i
			break
		}
	}
	if slashIdx < 0 {
		a.log.Error("syncSingleRepo: invalid repo_id format (no slash)", "repo_id", repoID)
		return
	}
	owner := ownerRepo[:slashIdx]
	repo := ownerRepo[slashIdx+1:]

	req := &sync.SyncRequest{
		Provider:   providerName,
		Owner:      owner,
		Repo:       repo,
		IncludePRs: true,
		State:      "open",
	}
	result, err := a.orchestrator.Sync(ctx, req)
	if err != nil {
		a.log.Error("syncSingleRepo: sync failed", "repo_id", repoID, "err", err)
		return
	}
	a.log.Info("syncSingleRepo: synced", "repo_id", repoID, "prs", result.PRCount)

	a.wsServer.Broadcast(map[string]interface{}{
		"type": "SYNC_COMPLETE",
		"data": map[string]interface{}{"repo_id": repoID},
	})
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

	if a.prov != nil && a.gitClient != nil {
		a.orchestrator = sync.NewOrchestrator(a.log, a.prov, a.db, a.gitClient).
			WithDiffStore(diffStore)
	}

	// Register inbound WebSocket handlers.
	a.wsServer.RegisterHandler("AUTH_START", a.handleAuthStart)
	a.wsServer.RegisterHandler("AUTH_CHECK", a.handleAuthCheck)
	a.wsServer.RegisterHandler("AUTH_DISCONNECT", a.handleAuthDisconnect)
	a.wsServer.RegisterHandler("REQUEST_DIFF", newRequestDiffHandler(diffStore, a.log))
	a.wsServer.RegisterHandler("REQUEST_PR_LIST", newRequestPRListHandler(a.db.DB, a.log))
	a.wsServer.RegisterHandler("REQUEST_REPO_LIST",
		newRequestRepoListHandler(a.getProvider, a.db.DB, a.log))
	a.wsServer.RegisterHandler("TOGGLE_WATCH_REPO",
		newToggleWatchHandler(a.db.DB, a.syncTrigger, a.log))

	wsAddr := fmt.Sprintf(":%d", a.wsPort)
	if err := a.wsServer.Start(wsAddr); err != nil {
		return fmt.Errorf("failed to start ws server: %w", err)
	}

	a.log.Info("WebSocket server listening", "addr", wsAddr)

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
