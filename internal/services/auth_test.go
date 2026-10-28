package services_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/auth"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/services"
	"github.com/itterno/bistro/internal/services/servicestest"
)

const (
	authTestDeadline = 3 * time.Second
	authPollInterval = time.Millisecond
	authShortTimeout = 50 * time.Millisecond
)

type fakeTokenStore struct {
	tokens   map[string]string
	storeErr error
}

func (f *fakeTokenStore) Get(name string) (string, error) { return f.tokens[name], nil }
func (f *fakeTokenStore) Store(name, token string) error {
	if f.storeErr != nil {
		return f.storeErr
	}
	if f.tokens == nil {
		f.tokens = make(map[string]string)
	}
	f.tokens[name] = token
	return nil
}
func (f *fakeTokenStore) Delete(name string) error { delete(f.tokens, name); return nil }

type fakeUsernameCache struct{ usernames map[string]string }

func (f *fakeUsernameCache) Get(name string) (string, bool, bool) {
	username, found := f.usernames[name]
	return username, false, found
}
func (f *fakeUsernameCache) Set(name, username string) {
	if f.usernames == nil {
		f.usernames = make(map[string]string)
	}
	f.usernames[name] = username
}
func (f *fakeUsernameCache) Delete(name string) { delete(f.usernames, name) }

type currentUserWriterFunc func(context.Context, string, string) error

func (f currentUserWriterFunc) Set(ctx context.Context, name, login string) error {
	return f(ctx, name, login)
}

type currentUserClearerFunc func(context.Context, string) error

func (f currentUserClearerFunc) Delete(ctx context.Context, name string) error { return f(ctx, name) }

type authRoundTripper func(*http.Request) (*http.Response, error)

func (f authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func newAuthService() *services.Auth {
	return &services.Auth{
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Tokens: &fakeTokenStore{}, Cache: &fakeUsernameCache{},
		Events: &servicestest.Recorder{}, GetClientID: func() string { return "test-client" },
		SetProvider: func(provider.Provider, string) {}, CallbackTimeout: authTestDeadline,
	}
}

func assertAuthError(t *testing.T, err error, code services.Code, reason string) {
	t.Helper()
	var serviceErr *services.ServiceError
	if !errors.As(err, &serviceErr) || serviceErr.Code != code || serviceErr.Reason != reason {
		t.Fatalf("error = %v, want %s/%s", err, code, reason)
	}
}

type authOutcome struct {
	identity services.AuthIdentity
	err      error
}

type pendingAuth struct {
	redirect services.AuthRedirect
	cancel   context.CancelFunc
	outcome  <-chan authOutcome
}

func beginAuth(t *testing.T, service *services.Auth) pendingAuth {
	t.Helper()
	recorder := service.Events.(*servicestest.Recorder)
	previous := len(recorder.Named(services.EventAuthRedirect))
	ctx, cancel := context.WithCancel(context.Background())
	outcome := make(chan authOutcome, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		identity, err := service.Start(ctx, services.AuthStartRequest{Provider: "github"})
		outcome <- authOutcome{identity: identity, err: err}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(authTestDeadline):
			t.Error("Start did not finish during cleanup")
		}
	})
	ticker := time.NewTicker(authPollInterval)
	defer ticker.Stop()
	deadline := time.NewTimer(authTestDeadline)
	defer deadline.Stop()
	for {
		events := recorder.Named(services.EventAuthRedirect)
		if len(events) > previous {
			redirect := events[previous].(services.AuthRedirect)
			if redirect.Provider != "github" {
				t.Fatalf("redirect provider = %q", redirect.Provider)
			}
			return pendingAuth{redirect: redirect, cancel: cancel, outcome: outcome}
		}
		select {
		case result := <-outcome:
			t.Fatalf("Start ended before redirect: %v", result.err)
		case <-deadline.C:
			t.Fatal("missing auth:redirect")
		case <-ticker.C:
		}
	}
}

func awaitAuth(t *testing.T, pending pendingAuth) authOutcome {
	t.Helper()
	select {
	case outcome := <-pending.outcome:
		return outcome
	case <-time.After(authTestDeadline):
		t.Fatal("Start did not finish")
		return authOutcome{}
	}
}

func callbackRequest(t *testing.T, redirect services.AuthRedirect) *http.Request {
	t.Helper()
	authorize, err := url.Parse(redirect.URL)
	if err != nil {
		t.Fatal(err)
	}
	callbackURL := auth.RedirectURI + "?code=test-code&state=" + url.QueryEscape(authorize.Query().Get("state"))
	req, err := http.NewRequest(http.MethodGet, callbackURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func sendCallback(t *testing.T, req *http.Request, transport http.RoundTripper) int {
	t.Helper()
	response, err := (&http.Client{Transport: transport, Timeout: authTestDeadline}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode
}

func mockAuthHTTP(t *testing.T, transport authRoundTripper) http.RoundTripper {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = original })
	return original
}

func successfulAuthHTTP(req *http.Request) (*http.Response, error) {
	body := `{"access_token":"test-token","token_type":"bearer","scope":"repo,read:user"}`
	if req.URL.Path == "/user" {
		body = `{"login":"octocat"}`
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

func TestAuthStartPreflight(t *testing.T) {
	service := newAuthService()
	_, err := service.Start(context.Background(), services.AuthStartRequest{Provider: "gitlab"})
	assertAuthError(t, err, services.CodeInvalid, "")
	service.GetClientID = func() string { return "" }
	_, err = service.Start(context.Background(), services.AuthStartRequest{Provider: "github"})
	assertAuthError(t, err, services.CodeInternal, "client_id_missing")
	if err.(*services.ServiceError).Message != "GITHUB_CLIENT_ID not configured" {
		t.Fatalf("message = %v", err)
	}
	service.GetClientID = func() string { return "test-client" }
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", auth.CallbackPort))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
	})
	for range 2 {
		_, err = service.Start(context.Background(), services.AuthStartRequest{Provider: "github"})
		assertAuthError(t, err, services.CodeInternal, "callback_unavailable")
		if err.(*services.ServiceError).Message != "authentication callback unavailable" {
			t.Fatalf("message = %v", err)
		}
	}
	if len(service.Events.(*servicestest.Recorder).Events()) != 0 {
		t.Fatal("preflight emitted redirect")
	}
}

func TestAuthCancelFreesSlot(t *testing.T) {
	service := newAuthService()
	for range 2 {
		pending := beginAuth(t, service)
		pending.cancel()
		assertAuthError(t, awaitAuth(t, pending).err, services.CodeCancelled, "")
		connection, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", auth.CallbackPort), authPollInterval)
		if err == nil {
			_ = connection.Close()
			t.Fatal("callback listener remains open after cancel")
		}
	}
}

func TestAuthConcurrentStartRejected(t *testing.T) {
	service := newAuthService()
	pending := beginAuth(t, service)
	_, err := service.Start(context.Background(), services.AuthStartRequest{Provider: "github"})
	assertAuthError(t, err, services.CodeInvalid, "auth_already_in_progress")
	if len(service.Events.(*servicestest.Recorder).Events()) != 1 {
		t.Fatal("concurrent Start emitted another redirect")
	}
	pending.cancel()
	assertAuthError(t, awaitAuth(t, pending).err, services.CodeCancelled, "")
}

func TestAuthSuccessPersistsIdentityAndTriggersSync(t *testing.T) {
	original := mockAuthHTTP(t, successfulAuthHTTP)
	service := newAuthService()
	triggers := make(chan string, 1)
	service.SyncTrigger = triggers
	var installed bool
	service.SetProvider = func(prov provider.Provider, name string) { installed = prov != nil && name == "github" }
	var written services.AuthIdentity
	service.CurrentUserWriter = currentUserWriterFunc(func(_ context.Context, name, login string) error {
		written = services.AuthIdentity{Provider: name, Username: login}
		return nil
	})
	pending := beginAuth(t, service)
	if status := sendCallback(t, callbackRequest(t, pending.redirect), original); status != http.StatusOK {
		t.Fatalf("callback status = %d", status)
	}
	outcome := awaitAuth(t, pending)
	want := services.AuthIdentity{Provider: "github", Username: "octocat"}
	if outcome.err != nil || outcome.identity != want || written != want || !installed {
		t.Fatalf("identity = %+v, write = %+v, installed = %v, err = %v", outcome.identity, written, installed, outcome.err)
	}
	if service.Tokens.(*fakeTokenStore).tokens["github"] != "test-token" {
		t.Fatal("token was not persisted")
	}
	if service.Cache.(*fakeUsernameCache).usernames["github"] != "octocat" {
		t.Fatal("username was not cached")
	}
	select {
	case repoID := <-triggers:
		if repoID != "" {
			t.Fatalf("trigger = %q", repoID)
		}
	default:
		t.Fatal("no full sync trigger before Start returned")
	}
	if len(service.Events.(*servicestest.Recorder).Events()) != 1 {
		t.Fatal("unexpected terminal event")
	}
}

type blockingAuthTokenStore struct {
	fakeTokenStore
	entered chan struct{}
	release <-chan struct{}
}

func (s *blockingAuthTokenStore) Store(name, token string) error {
	close(s.entered)
	<-s.release
	return s.fakeTokenStore.Store(name, token)
}

func TestAuthCancelDuringFinalizationKeepsSuccessfulCommit(t *testing.T) {
	original := mockAuthHTTP(t, successfulAuthHTTP)
	service := newAuthService()
	release := make(chan struct{})
	store := &blockingAuthTokenStore{entered: make(chan struct{}), release: release}
	service.Tokens = store
	pending := beginAuth(t, service)
	// Unblock persistence before beginAuth's cleanup waits for Start.
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	sendCallback(t, callbackRequest(t, pending.redirect), original)
	select {
	case <-store.entered:
	case <-time.After(authTestDeadline):
		t.Fatal("token persistence did not start")
	}
	pending.cancel()
	close(release)
	outcome := awaitAuth(t, pending)
	if outcome.err != nil || outcome.identity.Username != "octocat" {
		t.Fatalf("outcome = %+v", outcome)
	}
	if store.tokens["github"] != "test-token" {
		t.Fatal("cancel lost persisted token")
	}
}

func TestAuthExchangeStopsOnCancelAndTimeout(t *testing.T) {
	for _, code := range []services.Code{services.CodeCancelled, services.CodeTimeout} {
		t.Run(string(code), func(t *testing.T) {
			started, stopped := make(chan struct{}), make(chan struct{})
			original := mockAuthHTTP(t, func(req *http.Request) (*http.Response, error) {
				close(started)
				<-req.Context().Done()
				close(stopped)
				return nil, req.Context().Err()
			})
			service := newAuthService()
			if code == services.CodeTimeout {
				service.CallbackTimeout = authShortTimeout
			}
			triggers := make(chan string, 1)
			service.SyncTrigger = triggers
			pending := beginAuth(t, service)
			req := callbackRequest(t, pending.redirect)
			callbackDone := make(chan error, 1)
			go func() {
				response, err := (&http.Client{Transport: original, Timeout: authTestDeadline}).Do(req)
				if err == nil {
					err = response.Body.Close()
				}
				callbackDone <- err
			}()
			select {
			case <-started:
			case <-time.After(authTestDeadline):
				t.Fatal("exchange did not start")
			}
			if code == services.CodeCancelled {
				pending.cancel()
			}
			assertAuthError(t, awaitAuth(t, pending).err, code, "")
			select {
			case <-stopped:
			case <-time.After(authTestDeadline):
				t.Fatal("exchange was not cancelled")
			}
			select {
			case err := <-callbackDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(authTestDeadline):
				t.Fatal("callback did not drain")
			}
			select {
			case <-triggers:
				t.Fatal("failed auth triggered sync")
			default:
			}
			if len(service.Tokens.(*fakeTokenStore).tokens) != 0 {
				t.Fatal("failed auth persisted token")
			}
		})
	}
}

func TestAuthCallbackFailureAndPersistenceFailure(t *testing.T) {
	for _, persistenceFails := range []bool{false, true} {
		t.Run(fmt.Sprint(persistenceFails), func(t *testing.T) {
			original := mockAuthHTTP(t, successfulAuthHTTP)
			service := newAuthService()
			service.SyncTrigger = make(chan string)
			if persistenceFails {
				service.Tokens.(*fakeTokenStore).storeErr = errors.New("storage unavailable")
			}
			installed := false
			service.SetProvider = func(provider.Provider, string) { installed = true }
			pending := beginAuth(t, service)
			req := callbackRequest(t, pending.redirect)
			if !persistenceFails {
				query := req.URL.Query()
				query.Del("code")
				query.Set("error", "access_denied")
				req.URL.RawQuery = query.Encode()
			}
			sendCallback(t, req, original)
			outcome := awaitAuth(t, pending)
			if persistenceFails {
				assertAuthError(t, outcome.err, services.CodeInternal, "")
			} else {
				assertAuthError(t, outcome.err, services.CodeUnauthenticated, "authentication_failed")
			}
			if installed || outcome.identity != (services.AuthIdentity{}) {
				t.Fatal("failed auth installed provider or returned identity")
			}
		})
	}
}

func TestAuthCheckState(t *testing.T) {
	for _, hasToken := range []bool{false, true} {
		t.Run(fmt.Sprint(hasToken), func(t *testing.T) {
			service := newAuthService()
			var written services.AuthIdentity
			service.CurrentUserWriter = currentUserWriterFunc(func(_ context.Context, name, login string) error {
				written = services.AuthIdentity{Provider: name, Username: login}
				return nil
			})
			want := services.AuthStatus{AuthStatus: "unauthenticated", AuthReason: "no_stored_token", RequiresAuth: true, Exchange: "proxy"}
			if hasToken {
				service.Tokens.(*fakeTokenStore).tokens = map[string]string{"github": "stored-token"}
				service.Cache.(*fakeUsernameCache).usernames = map[string]string{"github": "octocat"}
				service.GetClientSecret = func() string { return "test-client-secret" }
				want = services.AuthStatus{Authenticated: true, Providers: []services.AuthIdentity{{Provider: "github", Username: "octocat"}}, AuthStatus: "authenticated", AuthReason: "stored_token", Exchange: "local"}
			}
			status, err := service.Check(context.Background())
			if err != nil || !reflect.DeepEqual(status, want) {
				t.Fatalf("status = %+v, err = %v, want %+v", status, err, want)
			}
			if hasToken && written != want.Providers[0] {
				t.Fatalf("current user = %+v", written)
			}
			if !hasToken && written != (services.AuthIdentity{}) {
				t.Fatal("unauthenticated Check wrote current user")
			}
		})
	}
}

type orderedTokenStore struct {
	fakeTokenStore
	order *[]string
}

func (s *orderedTokenStore) Delete(name string) error {
	*s.order = append(*s.order, "token")
	return s.fakeTokenStore.Delete(name)
}

type orderedUsernameCache struct {
	fakeUsernameCache
	order *[]string
}

func (s *orderedUsernameCache) Delete(name string) {
	*s.order = append(*s.order, "cache")
	s.fakeUsernameCache.Delete(name)
}

func TestAuthDisconnectClearsStateInOrder(t *testing.T) {
	service := newAuthService()
	var order []string
	service.Tokens = &orderedTokenStore{fakeTokenStore: fakeTokenStore{tokens: map[string]string{"github": "stored-token"}}, order: &order}
	service.Cache = &orderedUsernameCache{fakeUsernameCache: fakeUsernameCache{usernames: map[string]string{"github": "octocat"}}, order: &order}
	service.CurrentUserClearer = currentUserClearerFunc(func(_ context.Context, name string) error {
		if name != "github" {
			t.Fatalf("clear provider = %q", name)
		}
		order = append(order, "current_user")
		return nil
	})
	service.SetProvider = func(prov provider.Provider, name string) {
		if prov != nil || name != "github" {
			t.Fatal("disconnect did not clear provider")
		}
		order = append(order, "provider")
	}
	status, err := service.Disconnect(context.Background(), services.AuthDisconnectRequest{Provider: "github"})
	if err != nil || status.Authenticated || !status.RequiresAuth {
		t.Fatalf("status = %+v, err = %v", status, err)
	}
	if !reflect.DeepEqual(order, []string{"token", "cache", "current_user", "provider"}) {
		t.Fatalf("order = %v", order)
	}
}
