package services

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/itterno/bistro/internal/auth"
	"github.com/itterno/bistro/internal/provider"
	github "github.com/itterno/bistro/internal/provider/github"
)

// TokenStore is the keychain subset used by authentication.
type TokenStore interface {
	Get(provider string) (string, error)
	Store(provider, token string) error
	Delete(provider string) error
}

// UsernameCache is the provider cache subset used by authentication.
type UsernameCache interface {
	Get(provider string) (username string, expired bool, found bool)
	Set(provider, username string)
	Delete(provider string)
}

// CurrentUserWriter persists the identity used by review tracking.
type CurrentUserWriter interface {
	Set(ctx context.Context, provider, login string) error
}

// CurrentUserClearer removes the review identity on sign-out.
type CurrentUserClearer interface {
	Delete(ctx context.Context, provider string) error
}

const (
	githubProvider                = "github"
	gitlabProvider                = "gitlab"
	giteaProvider                 = "gitea"
	authStatusAuthenticated       = "authenticated"
	authStatusUnauthenticated     = "unauthenticated"
	authReasonStoredToken         = "stored_token"
	authReasonNoStoredToken       = "no_stored_token"
	authReasonInProgress          = "auth_already_in_progress"
	authReasonClientIDMissing     = "client_id_missing"
	authReasonCallbackUnavailable = "callback_unavailable"
	authReasonFailed              = "authentication_failed"
	githubOAuthAuthorizeURL       = "https://github.com/login/oauth/authorize"
	githubOAuthTokenURL           = "https://github.com/login/oauth/access_token"
	bistroOAuthExchangeURL        = "https://get-bistro.com/api/oauth/token"
	usernameLookupTimeout         = 10 * time.Second
	callbackShutdownTimeout       = 5 * time.Second
	authExchangeLocal             = "local"
	authExchangeProxy             = "proxy"
)

// AuthStartRequest selects the OAuth provider.
type AuthStartRequest struct {
	Provider string `json:"provider"`
}

// AuthDisconnectRequest selects the provider to sign out.
type AuthDisconnectRequest struct {
	Provider string `json:"provider"`
}

// AuthIdentity identifies an authenticated provider account.
type AuthIdentity struct {
	Provider string `json:"provider"`
	Username string `json:"username"`
}

// AuthStatus describes locally stored authentication, without exposing tokens.
type AuthStatus struct {
	Authenticated bool           `json:"authenticated"`
	Providers     []AuthIdentity `json:"providers"`
	AuthStatus    string         `json:"auth_status"`
	AuthReason    string         `json:"auth_reason"`
	RequiresAuth  bool           `json:"requires_auth"`
	Exchange      string         `json:"exchange"`
}

// Auth owns a single OAuth flow and the local authentication state.
type Auth struct {
	Log                *slog.Logger
	Tokens             TokenStore
	Cache              UsernameCache
	Events             Emitter
	SyncTrigger        chan<- string
	SetProvider        func(provider.Provider, string)
	GetClientID        func() string
	GetClientSecret    func() string
	CallbackTimeout    time.Duration
	CurrentUserWriter  CurrentUserWriter
	CurrentUserClearer CurrentUserClearer
	Panics             PanicReporter

	ownerMu sync.Mutex
	owner   bool
}

// Start emits the browser redirect and blocks until authentication finishes.
func (s *Auth) Start(ctx context.Context, req AuthStartRequest) (AuthIdentity, error) {
	return guard(s.Panics, "Auth.Start", func() (AuthIdentity, error) {
		if req.Provider != githubProvider {
			return AuthIdentity{}, Errorf(CodeInvalid, "unsupported provider: %s", req.Provider)
		}
		if err := ctx.Err(); err != nil {
			return AuthIdentity{}, err
		}
		s.ownerMu.Lock()
		if s.owner {
			s.ownerMu.Unlock()
			return AuthIdentity{}, WithReason(CodeInvalid, authReasonInProgress, authReasonInProgress)
		}
		s.owner = true
		s.ownerMu.Unlock()
		defer func() {
			s.ownerMu.Lock()
			s.owner = false
			s.ownerMu.Unlock()
		}()

		flow, err := s.startCallback()
		if err != nil {
			return AuthIdentity{}, err
		}
		defer s.stopCallback(flow)
		s.Events.Emit(EventAuthRedirect, AuthRedirect{Provider: req.Provider, URL: flow.authorizeURL})
		return s.awaitCallback(ctx, flow)
	})
}

type authFlow struct {
	server       *auth.CallbackServer
	results      <-chan auth.CallbackResult
	serveDone    <-chan struct{}
	authorizeURL string
}

func (s *Auth) startCallback() (*authFlow, error) {
	clientID := s.GetClientID()
	if clientID == "" {
		return nil, WithReason(CodeInternal, authReasonClientIDMissing, "GITHUB_CLIENT_ID not configured")
	}
	secret := s.clientSecret()
	tokenURL := bistroOAuthExchangeURL
	if secret != "" {
		tokenURL = githubOAuthTokenURL
	}
	cfg := auth.ProviderConfig{
		AuthURL: githubOAuthAuthorizeURL, TokenURL: tokenURL,
		Scopes: []string{"repo", "read:user"}, ClientID: clientID, ClientSecret: secret,
	}
	authorizeURL, state, err := auth.BuildAuthURL(cfg, auth.RedirectURI)
	if err != nil {
		return nil, WithReason(CodeInternal, authReasonCallbackUnavailable, "authentication callback unavailable")
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", auth.CallbackPort))
	if err != nil {
		return nil, WithReason(CodeInternal, authReasonCallbackUnavailable, "authentication callback unavailable")
	}
	results := make(chan auth.CallbackResult, 1)
	server := auth.NewCallbackServer(cfg, state, auth.RedirectURI, results)
	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		// The caller observes premature termination through serveDone.
		_ = server.Serve(listener)
	}()
	return &authFlow{server: server, results: results, serveDone: serveDone, authorizeURL: authorizeURL}, nil
}

func (s *Auth) stopCallback(flow *authFlow) {
	ctx, cancel := context.WithTimeout(context.Background(), callbackShutdownTimeout)
	defer cancel()
	if err := flow.server.Shutdown(ctx); err != nil {
		s.logger().Warn("authentication callback required forced shutdown")
	}
	<-flow.serveDone
}

func (s *Auth) awaitCallback(ctx context.Context, flow *authFlow) (AuthIdentity, error) {
	timeout := s.CallbackTimeout
	if timeout <= 0 {
		timeout = auth.CallbackTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case callback := <-flow.results:
		if callback.Err != nil {
			return AuthIdentity{}, WithReason(CodeUnauthenticated, authReasonFailed, "authentication failed")
		}
		identity, err := s.persistAuthResult(callback.Token)
		if err != nil {
			return AuthIdentity{}, err
		}
		s.triggerSync()
		return identity, nil
	case <-flow.serveDone:
		return AuthIdentity{}, WithReason(CodeInternal, authReasonCallbackUnavailable, "authentication callback unavailable")
	case <-timer.C:
		return AuthIdentity{}, Errorf(CodeTimeout, "authentication timed out")
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return AuthIdentity{}, ctx.Err()
		}
		return AuthIdentity{}, Errorf(CodeCancelled, "authentication cancelled")
	}
}

func (s *Auth) persistAuthResult(token string) (AuthIdentity, error) {
	if err := s.Tokens.Store(githubProvider, token); err != nil {
		return AuthIdentity{}, Errorf(CodeInternal, "failed to persist token")
	}
	s.SetProvider(github.NewClient(token), githubProvider)
	identity := AuthIdentity{Provider: githubProvider}
	// Once persistence begins, finish the identity commit even if the caller cancels.
	ctx, cancel := context.WithTimeout(context.Background(), usernameLookupTimeout)
	defer cancel()
	username, err := fetchAuthUsername(ctx, githubProvider, token)
	if err != nil {
		s.logger().Warn("failed to fetch username after OAuth")
		return identity, nil
	}
	identity.Username = username
	s.Cache.Set(githubProvider, username)
	s.writeCurrentUser(ctx, identity)
	return identity, nil
}

// Check returns the authentication state from the keychain and username cache.
func (s *Auth) Check(ctx context.Context) (AuthStatus, error) {
	return guard(s.Panics, "Auth.Check", func() (AuthStatus, error) {
		return s.check(ctx)
	})
}

func (s *Auth) check(ctx context.Context) (AuthStatus, error) {
	status := AuthStatus{AuthStatus: authStatusUnauthenticated, AuthReason: authReasonNoStoredToken, RequiresAuth: true, Exchange: authExchangeProxy}
	if s.clientSecret() != "" {
		status.Exchange = authExchangeLocal
	}
	for _, name := range []string{githubProvider, gitlabProvider, giteaProvider} {
		if err := ctx.Err(); err != nil {
			return AuthStatus{}, err
		}
		token, err := s.Tokens.Get(name)
		if err != nil || token == "" {
			continue
		}
		status.Authenticated = true
		status.AuthStatus = authStatusAuthenticated
		status.AuthReason = authReasonStoredToken
		status.RequiresAuth = false
		username, expired, found := s.Cache.Get(name)
		identity := AuthIdentity{Provider: name, Username: username}
		if found && !expired {
			s.writeCurrentUser(ctx, identity)
			status.Providers = append(status.Providers, identity)
			continue
		}
		fetched, fetchErr := fetchAuthUsername(ctx, name, token)
		if err := ctx.Err(); err != nil {
			return AuthStatus{}, err
		}
		if fetchErr != nil {
			s.logger().Warn("failed to fetch username", "provider", name)
			status.Providers = append(status.Providers, identity)
			continue
		}
		identity.Username = fetched
		s.Cache.Set(name, fetched)
		s.writeCurrentUser(ctx, identity)
		status.Providers = append(status.Providers, identity)
	}
	return status, nil
}

// Disconnect clears the provider's local credentials and returns fresh status.
func (s *Auth) Disconnect(ctx context.Context, req AuthDisconnectRequest) (AuthStatus, error) {
	return guard(s.Panics, "Auth.Disconnect", func() (AuthStatus, error) {
		if req.Provider != githubProvider && req.Provider != gitlabProvider && req.Provider != giteaProvider {
			return AuthStatus{}, Errorf(CodeInvalid, "unsupported provider: %s", req.Provider)
		}
		if err := ctx.Err(); err != nil {
			return AuthStatus{}, err
		}
		if err := s.Tokens.Delete(req.Provider); err != nil {
			s.logger().Warn("failed to delete token from keychain", "provider", req.Provider)
		}
		s.Cache.Delete(req.Provider)
		if s.CurrentUserClearer != nil {
			if err := s.CurrentUserClearer.Delete(ctx, req.Provider); err != nil {
				s.logger().Warn("failed to clear current user", "provider", req.Provider)
			}
		}
		s.SetProvider(nil, req.Provider)
		return s.check(ctx)
	})
}

func (s *Auth) writeCurrentUser(ctx context.Context, identity AuthIdentity) {
	if s.CurrentUserWriter == nil {
		return
	}
	if err := s.CurrentUserWriter.Set(ctx, identity.Provider, identity.Username); err != nil {
		s.logger().Warn("failed to persist current user", "provider", identity.Provider)
	}
}

func fetchAuthUsername(ctx context.Context, name, token string) (string, error) {
	if name != githubProvider {
		return "", fmt.Errorf("username fetch not implemented for %s", name)
	}
	ctx, cancel := context.WithTimeout(ctx, usernameLookupTimeout)
	defer cancel()
	return github.NewClient(token).GetUsername(ctx)
}

func (s *Auth) clientSecret() string {
	if s.GetClientSecret == nil {
		return ""
	}
	return s.GetClientSecret()
}

func (s *Auth) logger() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

func (s *Auth) triggerSync() {
	if s.SyncTrigger == nil {
		return
	}
	select {
	case s.SyncTrigger <- "":
	default:
		s.logger().Warn("sync trigger channel full, skipping auth sync trigger")
	}
}
