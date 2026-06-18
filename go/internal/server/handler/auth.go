package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/samuelasvanyi/bistro/internal/auth"
	"github.com/samuelasvanyi/bistro/internal/logging"
	"github.com/samuelasvanyi/bistro/internal/provider"
	github "github.com/samuelasvanyi/bistro/internal/provider/github"
	"github.com/samuelasvanyi/bistro/internal/ws"
)

// TokenStore is the subset of keychain.TokenManager used by auth handlers.
type TokenStore interface {
	Get(provider string) (string, error)
	Store(provider, token string) error
	Delete(provider string) error
}

// UsernameCache is the subset of cache.ProviderCache used by auth handlers.
type UsernameCache interface {
	Get(provider string) (username string, expired bool, found bool)
	Set(provider, username string)
	Delete(provider string)
}

var knownProviders = []string{"github", "gitlab", "gitea"}

// AuthDeps bundles the dependencies needed by the authentication handlers.
type AuthDeps struct {
	Log             *slog.Logger
	Tokens          TokenStore
	Cache           UsernameCache
	Broadcast       func(msg interface{})
	SetProvider     func(prov provider.Provider, name string)
	GetClientID     func() string
	GetClientSecret func() string
}

func fetchUsername(providerName, token string) (string, error) {
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

// NewAuthStartHandler returns a handler for AUTH_START messages that begins
// the OAuth authorization code flow and returns AUTH_REDIRECT to the caller.
// AUTH_SUCCESS / AUTH_FAILED are broadcast to all clients.
func NewAuthStartHandler(deps AuthDeps) ws.MessageHandler {
	var (
		mu             sync.Mutex
		callbackServer *auth.CallbackServer
	)

	runAuthCodeFlow := func(providerName string, resultCh <-chan auth.CallbackResult) {
		serverErrCh := make(chan error, 1)
		go func() {
			if err := callbackServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				serverErrCh <- err
			}
			close(serverErrCh)
		}()

		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			mu.Lock()
			if callbackServer != nil {
				_ = callbackServer.Shutdown(ctx)
			}
			callbackServer = nil
			mu.Unlock()
		}()

		select {
		case result := <-resultCh:
			if result.Err != nil {
				deps.Log.Error("auth callback failed", "error", result.Err)
				deps.Broadcast(map[string]interface{}{
					"type": "AUTH_FAILED",
					"data": map[string]string{"error": result.Err.Error()},
				})
				return
			}

			if err := deps.Tokens.Store(providerName, result.Token); err != nil {
				deps.Log.Error("failed to store token", "error", err)
				deps.Broadcast(map[string]interface{}{
					"type": "AUTH_FAILED",
					"data": map[string]string{"error": "failed to persist token"},
				})
				return
			}

			client := github.NewClient(result.Token)
			deps.SetProvider(client, providerName)

			username := ""
			if u, err := fetchUsername(providerName, result.Token); err == nil {
				username = u
				deps.Cache.Set(providerName, u)
			} else {
				deps.Log.Warn("failed to fetch username after OAuth", "error", err)
			}

			deps.Broadcast(map[string]interface{}{
				"type": "AUTH_SUCCESS",
				"data": map[string]string{"provider": providerName, "username": username},
			})
			deps.Log.Info("OAuth authorization code flow completed", "provider", providerName)

		case err := <-serverErrCh:
			deps.Log.Error("callback server error", "error", err)
			deps.Broadcast(map[string]interface{}{
				"type": "AUTH_FAILED",
				"data": map[string]string{"error": err.Error()},
			})

		case <-time.After(auth.CallbackTimeout):
			deps.Log.Warn("auth callback timed out")
			deps.Broadcast(map[string]interface{}{
				"type": "AUTH_FAILED",
				"data": map[string]string{"error": "timeout"},
			})
		}
	}

	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if deps.Log != nil {
			logger = deps.Log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var data struct {
			Provider string `json:"provider"`
		}
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return nil, fmt.Errorf("invalid AUTH_START data: %w", err)
		}

		logger.Debug("auth start requested", "provider", data.Provider)

		if data.Provider != "github" {
			logger.Warn("unsupported auth provider", "provider", data.Provider)
			return map[string]interface{}{
				"id":   msg.ID,
				"type": "AUTH_FAILED",
				"data": map[string]string{"error": "unsupported provider: " + data.Provider},
			}, nil
		}

		mu.Lock()
		if callbackServer != nil {
			mu.Unlock()
			logger.Info("auth already in progress", "provider", data.Provider)
			return map[string]interface{}{
				"id":   msg.ID,
				"type": "AUTH_FAILED",
				"data": map[string]string{"error": "auth_already_in_progress"},
			}, nil
		}

		clientID := deps.GetClientID()
		clientSecret := deps.GetClientSecret()
		if clientID == "" || clientSecret == "" {
			mu.Unlock()
			logger.Error("oauth client credentials missing",
				"has_client_id", clientID != "",
				"has_client_secret", clientSecret != "",
			)
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
			ClientID:     clientID,
			ClientSecret: clientSecret,
		}

		authURL, state, err := auth.BuildAuthURL(cfg, auth.RedirectURI)
		if err != nil {
			mu.Unlock()
			logger.Error("building auth URL failed", "error", err)
			return nil, fmt.Errorf("building auth URL: %w", err)
		}

		resultCh := make(chan auth.CallbackResult, 1)
		callbackServer = auth.NewCallbackServer(cfg, state, auth.RedirectURI, resultCh)
		mu.Unlock()

		logger.Info("starting oauth flow", "provider", data.Provider, "redirect_uri", auth.RedirectURI)

		go runAuthCodeFlow(data.Provider, resultCh)

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "AUTH_REDIRECT",
			"data": map[string]interface{}{
				"provider": data.Provider,
				"url":      authURL,
			},
		}, nil
	}
}

// NewAuthCheckHandler returns a handler for AUTH_CHECK messages that responds
// with the current authentication state for all known providers.
func NewAuthCheckHandler(deps AuthDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if deps.Log != nil {
			logger = deps.Log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		type providerInfo struct {
			Provider string `json:"provider"`
			Username string `json:"username"`
		}

		var providers []providerInfo
		authenticated := false

		for _, p := range knownProviders {
			token, err := deps.Tokens.Get(p)
			if err != nil || token == "" {
				logger.Debug("no token for provider", "provider", p)
				continue
			}
			authenticated = true

			username, expired, found := deps.Cache.Get(p)
			if found && !expired {
				logger.Debug("username cache hit", "provider", p)
				providers = append(providers, providerInfo{Provider: p, Username: username})
				continue
			}

			logger.Debug("username cache miss, fetching", "provider", p, "expired", expired)
			fetched, fetchErr := fetchUsername(p, token)
			if fetchErr != nil {
				deps.Log.Warn("failed to fetch username", "provider", p, "error", fetchErr)
				providers = append(providers, providerInfo{Provider: p, Username: username})
				continue
			}
			deps.Cache.Set(p, fetched)
			providers = append(providers, providerInfo{Provider: p, Username: fetched})
		}

		logger.Debug("auth check complete", "authenticated", authenticated, "providers", len(providers))

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "AUTH_CHECK_RESULT",
			"data": map[string]interface{}{
				"authenticated": authenticated,
				"providers":     providers,
			},
		}, nil
	}
}

// NewAuthDisconnectHandler returns a handler for AUTH_DISCONNECT messages that
// removes the token and cache entry for a provider and returns refreshed state.
func NewAuthDisconnectHandler(deps AuthDeps, checkHandler ws.MessageHandler) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if deps.Log != nil {
			logger = deps.Log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var data struct {
			Provider string `json:"provider"`
		}
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return nil, fmt.Errorf("invalid AUTH_DISCONNECT data: %w", err)
		}

		logger.Info("disconnecting provider", "provider", data.Provider)

		if err := deps.Tokens.Delete(data.Provider); err != nil {
			deps.Log.Warn("failed to delete token from keychain", "provider", data.Provider, "error", err)
		}
		deps.Cache.Delete(data.Provider)
		deps.SetProvider(nil, data.Provider)

		checkResp, err := checkHandler(ctx, msg)
		if err != nil {
			return nil, err
		}
		respMap := checkResp.(map[string]interface{})
		respMap["type"] = "AUTH_DISCONNECT_RESULT"
		return respMap, nil
	}
}
