package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/itterno/bistro/internal/auth"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	github "github.com/itterno/bistro/internal/provider/github"
	"github.com/itterno/bistro/internal/ws"
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

func knownProviders() []string { return []string{githubProvider, "gitlab", "gitea"} }

// CurrentUserWriter is the narrow persistence interface used by auth handlers.
type CurrentUserWriter interface {
	SetCurrentUser(ctx context.Context, provider, login string) error
}

// CurrentUserClearer is the narrow persistence interface used on sign-out.
type CurrentUserClearer interface {
	DeleteCurrentUser(ctx context.Context, provider string) error
}

const (
	authStatusAuthenticated   = "authenticated"
	authStatusUnauthenticated = "unauthenticated"
	authReasonStoredToken     = "stored_token"
	authReasonNoStoredToken   = "no_stored_token"
	githubOAuthAuthorizeURL   = "https://github.com/login/oauth/authorize"
	githubOAuthTokenURL       = "https://github.com/login/oauth/access_token"
	bistroOAuthExchangeURL    = "https://get-bistro.com/api/oauth/token"
	usernameLookupTimeout     = 10 * time.Second
	callbackShutdownTimeout   = 5 * time.Second
)

// CurrentUserWriterFunc adapts a function to CurrentUserWriter.
type CurrentUserWriterFunc func(ctx context.Context, provider, login string) error

// SetCurrentUser persists the authenticated login through the wrapped function.
func (f CurrentUserWriterFunc) SetCurrentUser(ctx context.Context, prov, login string) error {
	return f(ctx, prov, login)
}

// AuthDeps bundles the dependencies needed by the authentication handlers.
type AuthDeps struct {
	Log                *slog.Logger
	Tokens             TokenStore
	Cache              UsernameCache
	Broadcast          func(msg interface{})
	SetProvider        func(prov provider.Provider, name string)
	GetClientID        func() string
	GetClientSecret    func() string
	CallbackTimeout    time.Duration      // zero defaults to auth.CallbackTimeout
	CurrentUserWriter  CurrentUserWriter  // optional; nil → skip
	CurrentUserClearer CurrentUserClearer // optional; nil → skip
}

func fetchUsername(providerName, token string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), usernameLookupTimeout)
	defer cancel()

	switch providerName {
	case githubProvider:
		client := github.NewClient(token)
		return client.GetUsername(ctx)
	default:
		return "", fmt.Errorf("username fetch not implemented for %s", providerName)
	}
}

func clientSecret(deps AuthDeps) string {
	if deps.GetClientSecret == nil {
		return ""
	}
	return deps.GetClientSecret()
}

func exchangeMode(deps AuthDeps) string {
	if clientSecret(deps) != "" {
		return "local"
	}
	return "proxy"
}

// NewAuthFlowHandlers returns AUTH_START and AUTH_CANCEL handlers sharing one
// OAuth lifetime. Start returns AUTH_REDIRECT; cancel closes the listener before
// returning without a response. AUTH_SUCCESS / AUTH_FAILED are broadcast.
func NewAuthFlowHandlers(deps AuthDeps) (start, cancel ws.MessageHandler) {
	var (
		mu             sync.Mutex
		callbackServer *auth.CallbackServer
		cancelFlow     chan struct{}
	)

	type authCodeFlow struct {
		providerName string
		server       *auth.CallbackServer
		results      <-chan auth.CallbackResult
		cancelled    <-chan struct{}
	}
	runAuthCodeFlow := func(flow authCodeFlow) {
		providerName, ownServer := flow.providerName, flow.server
		resultCh, cancelCh := flow.results, flow.cancelled
		serverErrCh := make(chan error, 1)
		go func() {
			if err := ownServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serverErrCh <- err
			}
		}()

		var cleanupOnce sync.Once
		cleanup := func() {
			cleanupOnce.Do(func() {
				ctx, cancel := context.WithTimeout(context.Background(), callbackShutdownTimeout)
				defer cancel()
				if err := ownServer.Shutdown(ctx); err != nil {
					deps.Log.Warn("callback server shutdown failed", "error", err)
				}
				mu.Lock()
				defer mu.Unlock()
				if callbackServer == ownServer {
					callbackServer = nil
				}
			})
		}
		defer cleanup()

		timeout := deps.CallbackTimeout
		if timeout <= 0 {
			timeout = auth.CallbackTimeout
		}
		select {
		case result := <-resultCh:
			cleanup()
			select {
			case <-cancelCh:
				return
			default:
			}
			if result.Err != nil {
				deps.Log.Error("auth callback failed", "error", result.Err)
				deps.Broadcast(map[string]interface{}{
					"type": "AUTH_FAILED",
					"data": map[string]string{"error": "authentication failed"},
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
				if deps.CurrentUserWriter != nil {
					if werr := deps.CurrentUserWriter.SetCurrentUser(context.Background(), providerName, u); werr != nil {
						deps.Log.Warn("failed to persist current user", "provider", providerName, "error", werr)
						// non-fatal: auth still succeeds
					}
				}
			} else {
				deps.Log.Warn("failed to fetch username after OAuth", "error", err)
			}

			deps.Broadcast(map[string]interface{}{
				"type": "AUTH_SUCCESS",
				"data": map[string]string{"provider": providerName, "username": username},
			})
			deps.Log.Info("OAuth authorization code flow completed", "provider", providerName)

		case err := <-serverErrCh:
			cleanup()
			deps.Log.Error("callback server error", "error", err)
			deps.Broadcast(map[string]interface{}{
				"type": "AUTH_FAILED",
				"data": map[string]string{"error": "authentication callback unavailable"},
			})

		case <-time.After(timeout):
			cleanup()
			deps.Log.Warn("auth callback timed out")
			deps.Broadcast(map[string]interface{}{
				"type": "AUTH_FAILED",
				"data": map[string]string{"error": "timeout"},
			})
		case <-cancelCh:
			return
		}
	}

	start = func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
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

		if data.Provider != githubProvider {
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
		clientSecret := clientSecret(deps)
		if clientID == "" {
			mu.Unlock()
			logger.Error("oauth client ID missing")
			return map[string]interface{}{
				"id":   msg.ID,
				"type": "AUTH_FAILED",
				"data": map[string]string{"error": "GITHUB_CLIENT_ID not configured"},
			}, nil
		}

		// Without a secret (packaged builds) the code is exchanged via get-bistro.com,
		// which holds the secret. Dev builds with a secret talk to GitHub directly.
		tokenURL := bistroOAuthExchangeURL
		if clientSecret != "" {
			tokenURL = githubOAuthTokenURL
		}

		cfg := auth.ProviderConfig{
			AuthURL:      githubOAuthAuthorizeURL,
			TokenURL:     tokenURL,
			Scopes:       []string{"repo", "read:user"},
			ClientID:     clientID,
			ClientSecret: clientSecret,
		}

		authURL, state, err := auth.BuildAuthURL(cfg, auth.RedirectURI)
		if err != nil {
			mu.Unlock()
			return nil, fmt.Errorf("building auth URL: %w", err)
		}

		resultCh := make(chan auth.CallbackResult, 1)
		ownServer := auth.NewCallbackServer(cfg, state, auth.RedirectURI, resultCh)
		callbackServer = ownServer
		cancelFlow = make(chan struct{})
		cancelCh := cancelFlow
		mu.Unlock()

		logger.Info("starting oauth flow", "provider", data.Provider, "redirect_uri", auth.RedirectURI)

		go runAuthCodeFlow(authCodeFlow{
			providerName: data.Provider,
			server:       ownServer,
			results:      resultCh,
			cancelled:    cancelCh,
		})

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "AUTH_REDIRECT",
			"data": map[string]interface{}{
				"provider": data.Provider,
				"url":      authURL,
			},
		}, nil
	}
	cancel = func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		mu.Lock()
		ownServer := callbackServer
		if ownServer == nil {
			mu.Unlock()
			return nil, nil
		}
		close(cancelFlow)
		shutdownCtx, stop := context.WithTimeout(context.Background(), callbackShutdownTimeout)
		err := ownServer.Shutdown(shutdownCtx)
		stop()
		callbackServer = nil
		mu.Unlock()
		if err != nil {
			return nil, fmt.Errorf("cancel OAuth callback: %w", err)
		}
		deps.Broadcast(map[string]interface{}{
			"type": "AUTH_FAILED",
			"data": map[string]string{"error": "cancelled"},
		})
		return nil, nil
	}
	return start, cancel
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
		authReason := authReasonNoStoredToken

		for _, p := range knownProviders() {
			token, err := deps.Tokens.Get(p)
			if err != nil || token == "" {
				logger.Debug("no token for provider", "provider", p)
				continue
			}
			authenticated = true
			authReason = authReasonStoredToken

			username, expired, found := deps.Cache.Get(p)
			if found && !expired {
				logger.Debug("username cache hit", "provider", p)
				if deps.CurrentUserWriter != nil {
					if werr := deps.CurrentUserWriter.SetCurrentUser(ctx, p, username); werr != nil {
						logger.Warn("failed to persist current user", "provider", p, "error", werr)
					}
				}
				providers = append(providers, providerInfo{Provider: p, Username: username})
				continue
			}

			logger.Debug("username cache miss, fetching", "provider", p, "expired", expired)
			fetched, fetchErr := fetchUsername(p, token)
			if fetchErr != nil {
				logger.Warn("failed to fetch username", "provider", p, "error", fetchErr)
				providers = append(providers, providerInfo{Provider: p, Username: username})
				continue
			}
			deps.Cache.Set(p, fetched)
			if deps.CurrentUserWriter != nil {
				if werr := deps.CurrentUserWriter.SetCurrentUser(ctx, p, fetched); werr != nil {
					logger.Warn("failed to persist current user", "provider", p, "error", werr)
				}
			}
			providers = append(providers, providerInfo{Provider: p, Username: fetched})
		}

		logger.Debug("auth check complete", "authenticated", authenticated, "providers", len(providers))

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "AUTH_CHECK_RESULT",
			"data": map[string]interface{}{
				"authenticated": authenticated,
				"providers":     providers,
				"auth_status":   authStatus(authenticated),
				"auth_reason":   authReason,
				"requires_auth": !authenticated,
				"exchange":      exchangeMode(deps),
			},
		}, nil
	}
}

func authStatus(authenticated bool) string {
	if authenticated {
		return authStatusAuthenticated
	}
	return authStatusUnauthenticated
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
			logger.Warn("failed to delete token from keychain", "provider", data.Provider, "error", err)
		}
		deps.Cache.Delete(data.Provider)
		if deps.CurrentUserClearer != nil {
			if err := deps.CurrentUserClearer.DeleteCurrentUser(ctx, data.Provider); err != nil {
				logger.Warn("failed to clear current user", "provider", data.Provider, "error", err)
			}
		}
		deps.SetProvider(nil, data.Provider)

		checkResp, err := checkHandler(ctx, msg)
		if err != nil {
			return nil, err
		}
		respMap, ok := checkResp.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("AUTH_DISCONNECT: unexpected auth check response type %T", checkResp)
		}
		respMap["type"] = "AUTH_DISCONNECT_RESULT"
		return respMap, nil
	}
}
