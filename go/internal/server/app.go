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
	gitClient          *git.Client
	githubClientID     string
	githubClientSecret string
	callbackServer     *auth.CallbackServer // non-nil while auth is in progress
	providerCache      *cache.ProviderCache
	tokenManager       *keychain.TokenManager
}

// SetProvider configures the GitHub provider used by the sync orchestrator.
// Must be called before Run.
func (a *App) SetProvider(prov provider.Provider) {
	a.prov = prov
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
	cacheDir, _ := os.UserCacheDir()
	cachePath := filepath.Join(cacheDir, "bistro", "providers.json")
	return &App{
		log:           logger,
		wsServer:      ws.NewServerWithLogger(logger),
		wsPort:        wsPort,
		dbPath:        dbPath,
		providerCache: cache.NewProviderCache(cachePath),
		tokenManager:  keychain.NewTokenManager(),
	}
}

// NewWithLogger returns a new App with a custom logger.
func NewWithLogger(wsPort int, dbPath string, logger *slog.Logger) *App {
	cacheDir, _ := os.UserCacheDir()
	cachePath := filepath.Join(cacheDir, "bistro", "providers.json")
	return &App{
		log:           logger,
		wsServer:      ws.NewServerWithLogger(logger),
		wsPort:        wsPort,
		dbPath:        dbPath,
		providerCache: cache.NewProviderCache(cachePath),
		tokenManager:  keychain.NewTokenManager(),
	}
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

	if a.callbackServer != nil {
		return map[string]interface{}{
			"id":   msg.ID,
			"type": "AUTH_FAILED",
			"data": map[string]string{"error": "auth_already_in_progress"},
		}, nil
	}

	if a.githubClientID == "" || a.githubClientSecret == "" {
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
		return nil, fmt.Errorf("building auth URL: %w", err)
	}

	resultCh := make(chan auth.CallbackResult, 1)
	a.callbackServer = auth.NewCallbackServer(cfg, state, auth.RedirectURI, resultCh)

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
		if a.callbackServer != nil {
			_ = a.callbackServer.Shutdown(ctx)
		}
		a.callbackServer = nil
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
		}

		client := github.NewClient(result.Token)
		a.SetProvider(client)

		username := ""
		if u, err := client.GetUsername(context.Background()); err == nil {
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

	for _, p := range knownProviders {
		token, err := a.tokenManager.Get(p)
		if err != nil || token == "" {
			continue
		}

		username, expired, found := a.providerCache.Get(p)
		if found && !expired {
			providers = append(providers, providerInfo{Provider: p, Username: username})
			continue
		}

		fetched, fetchErr := a.fetchUsername(p, token)
		if fetchErr != nil {
			a.log.Warn("failed to fetch username", "provider", p, "error", fetchErr)
			if found {
				providers = append(providers, providerInfo{Provider: p, Username: username})
			}
			continue
		}
		a.providerCache.Set(p, fetched)
		providers = append(providers, providerInfo{Provider: p, Username: fetched})
	}

	authenticated := len(providers) > 0

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
func (a *App) fetchUsername(providerName, token string) (string, error) {
	switch providerName {
	case "github":
		client := github.NewClient(token)
		return client.GetUsername(context.Background())
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
	a.prov = nil

	checkResp, err := a.handleAuthCheck(msg)
	if err != nil {
		return nil, err
	}
	respMap := checkResp.(map[string]interface{})
	respMap["type"] = "AUTH_DISCONNECT_RESULT"
	return respMap, nil
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

	wsAddr := fmt.Sprintf(":%d", a.wsPort)
	if err := a.wsServer.Start(wsAddr); err != nil {
		return fmt.Errorf("failed to start ws server: %w", err)
	}

	a.log.Info("WebSocket server listening", "addr", wsAddr)

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
