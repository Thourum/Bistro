package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/auth"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/server/handler"
	"github.com/itterno/bistro/internal/ws"
)

type fakeTokenStore struct {
	tokens map[string]string
}

func (f *fakeTokenStore) Get(provider string) (string, error) { return f.tokens[provider], nil }
func (f *fakeTokenStore) Store(provider, token string) error {
	if f.tokens == nil {
		f.tokens = make(map[string]string)
	}
	f.tokens[provider] = token
	return nil
}
func (f *fakeTokenStore) Delete(provider string) error {
	delete(f.tokens, provider)
	return nil
}

type fakeUsernameCache struct {
	usernames map[string]string
}

func (f *fakeUsernameCache) Get(provider string) (string, bool, bool) {
	username, found := f.usernames[provider]
	return username, false, found
}
func (f *fakeUsernameCache) Set(provider, username string) {
	if f.usernames == nil {
		f.usernames = make(map[string]string)
	}
	f.usernames[provider] = username
}
func (f *fakeUsernameCache) Delete(provider string) { delete(f.usernames, provider) }

func TestAuthStartCallbackPortUnavailable(t *testing.T) {
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", auth.CallbackPort))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
	})
	start, cancel := handler.NewAuthFlowHandlers(handler.AuthDeps{
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		GetClientID: func() string { return "id" },
		Broadcast:   func(interface{}) {},
	})
	ctx := context.Background()
	request := ws.InboundMessage{ID: "occupied-port", Type: "AUTH_START", Data: json.RawMessage(`{"provider":"github"}`)}
	// Repeating the request also verifies that a failed start releases flow ownership.
	for attempt := 0; attempt < 2; attempt++ {
		response, err := start(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		envelope, ok := response.(map[string]interface{})
		if !ok || envelope["type"] != "AUTH_FAILED" || envelope["id"] != request.ID {
			t.Fatalf("start: %#v", response)
		}
		data, ok := envelope["data"].(map[string]string)
		if !ok || data["error"] != "authentication callback unavailable" {
			t.Fatalf("failure: %#v", envelope["data"])
		}
	}
	if _, err := cancel(ctx, ws.InboundMessage{Type: "AUTH_CANCEL"}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthCancelAllowsNextStart(t *testing.T) {
	var mu sync.Mutex
	var failures []string
	deps := handler.AuthDeps{
		Tokens:      &fakeTokenStore{tokens: map[string]string{}},
		Cache:       &fakeUsernameCache{usernames: map[string]string{}},
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		GetClientID: func() string { return "id" },
		Broadcast: func(message interface{}) {
			mu.Lock()
			defer mu.Unlock()
			if envelope, ok := message.(map[string]interface{}); ok {
				if data, ok := envelope["data"].(map[string]string); ok {
					failures = append(failures, data["error"])
				}
			}
		},
	}
	start, cancel := handler.NewAuthFlowHandlers(deps)
	ctx := context.Background()
	cancelRequest := ws.InboundMessage{Type: "AUTH_CANCEL"}
	t.Cleanup(func() {
		if _, err := cancel(ctx, cancelRequest); err != nil {
			t.Errorf("cleanup cancel: %v", err)
		}
	})
	request := ws.InboundMessage{Type: "AUTH_START", Data: json.RawMessage(`{"provider":"github"}`)}
	for attempt := 0; attempt < 2; attempt++ {
		response, err := start(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		envelope, ok := response.(map[string]interface{})
		if !ok || envelope["type"] != "AUTH_REDIRECT" {
			t.Fatalf("start: %#v", response)
		}
		if attempt > 0 {
			connection, dialErr := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", auth.CallbackPort), time.Second)
			if dialErr != nil {
				t.Fatalf("attempt %d listener unavailable: %v", attempt, dialErr)
			}
			if err := connection.Close(); err != nil {
				t.Fatal(err)
			}
		}
		response, err = cancel(ctx, cancelRequest)
		if err != nil || response != nil {
			t.Fatalf("cancel: %#v, %v", response, err)
		}
		connection, dialErr := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", auth.CallbackPort), time.Second)
		if dialErr == nil {
			// Closing this probe is best-effort; reaching the listener already fails the test.
			_ = connection.Close()
			t.Fatal("callback listener still open after cancellation")
		}
	}
	response, err := cancel(ctx, cancelRequest)
	if err != nil || response != nil {
		t.Fatalf("idle cancel: %#v, %v", response, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(failures) != 2 || failures[0] != "cancelled" || failures[1] != "cancelled" {
		t.Fatalf("failures: %#v", failures)
	}
}

type blockingAuthTokenStore struct {
	fakeTokenStore
	entered chan struct{}
	release <-chan struct{}
}

func (store *blockingAuthTokenStore) Store(provider, token string) error {
	close(store.entered)
	<-store.release
	return store.fakeTokenStore.Store(provider, token)
}

type authRoundTripper func(*http.Request) (*http.Response, error)

func (transport authRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestAuthCancelDuringFinalizationEmitsOneOutcome(t *testing.T) {
	const testDeadline = 2 * time.Second
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	store := &blockingAuthTokenStore{entered: make(chan struct{}), release: release}
	originalTransport := http.DefaultTransport
	http.DefaultTransport = authRoundTripper(func(request *http.Request) (*http.Response, error) {
		body := `{"access_token":"test-token","token_type":"bearer","scope":"repo,read:user"}`
		if request.URL.Path == "/user" {
			body = `{"login":"octocat"}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	defer func() { http.DefaultTransport = originalTransport }()

	var broadcasts []interface{}
	deps := handler.AuthDeps{
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		Tokens:          store,
		Cache:           &fakeUsernameCache{},
		GetClientID:     func() string { return "id" },
		CallbackTimeout: testDeadline,
		SetProvider:     func(provider.Provider, string) {},
		Broadcast:       func(message interface{}) { broadcasts = append(broadcasts, message) },
	}
	start, cancel := handler.NewAuthFlowHandlers(deps)
	ctx, stop := context.WithTimeout(context.Background(), testDeadline)
	defer stop()
	t.Cleanup(func() {
		if _, err := cancel(context.Background(), ws.InboundMessage{Type: "AUTH_CANCEL"}); err != nil {
			t.Errorf("cleanup cancel: %v", err)
		}
	})
	response, err := start(ctx, ws.InboundMessage{Type: "AUTH_START", Data: json.RawMessage(`{"provider":"github"}`)})
	if err != nil {
		t.Fatal(err)
	}
	envelope := response.(map[string]interface{})
	if envelope["type"] != "AUTH_REDIRECT" {
		t.Fatalf("start: %#v", response)
	}
	authorizeURL, err := url.Parse(envelope["data"].(map[string]interface{})["url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	callbackURL := auth.RedirectURI + "?code=test-code&state=" + url.QueryEscape(authorizeURL.Query().Get("state"))
	callbackRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, callbackURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	callbackClient := &http.Client{Transport: originalTransport, Timeout: testDeadline}
	callbackResponse, err := callbackClient.Do(callbackRequest)
	if err != nil {
		t.Fatal(err)
	}
	if err := callbackResponse.Body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.entered:
	case <-ctx.Done():
		t.Fatal("callback result did not reach token persistence")
	}

	cancelEntered := make(chan struct{})
	cancelDone := make(chan error, 1)
	go func() {
		close(cancelEntered)
		_, cancelErr := cancel(ctx, ws.InboundMessage{Type: "AUTH_CANCEL"})
		cancelDone <- cancelErr
	}()
	<-cancelEntered
	unblock()
	select {
	case err := <-cancelDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("cancel did not finish")
	}
	if len(broadcasts) != 1 {
		t.Fatalf("terminal broadcasts: %#v", broadcasts)
	}
	if event := broadcasts[0].(map[string]interface{}); event["type"] != "AUTH_SUCCESS" {
		t.Fatalf("terminal outcome: %#v", event)
	}
	if store.tokens["github"] != "test-token" {
		t.Fatalf("persisted token: %q", store.tokens["github"])
	}
}

func TestAuthExchangeStopsOnCancelAndTimeout(t *testing.T) {
	const testDeadline = 2 * time.Second
	const shortCallbackTimeout = 50 * time.Millisecond
	for _, outcome := range []string{"cancelled", "timeout"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, stop := context.WithTimeout(context.Background(), testDeadline)
			defer stop()
			originalTransport := http.DefaultTransport
			exchangeStarted := make(chan struct{})
			exchangeStopped := make(chan struct{})
			http.DefaultTransport = authRoundTripper(func(request *http.Request) (*http.Response, error) {
				close(exchangeStarted)
				<-request.Context().Done()
				close(exchangeStopped)
				return nil, request.Context().Err()
			})
			defer func() { http.DefaultTransport = originalTransport }()
			var broadcasts []interface{}
			terminal := make(chan struct{})
			triggers := make(chan string, 1)
			deps := handler.AuthDeps{
				SyncTrigger:     triggers,
				Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
				Tokens:          &fakeTokenStore{},
				Cache:           &fakeUsernameCache{},
				GetClientID:     func() string { return "id" },
				CallbackTimeout: testDeadline,
				Broadcast: func(message interface{}) {
					broadcasts = append(broadcasts, message)
					if len(broadcasts) == 1 {
						close(terminal)
					}
				},
			}
			if outcome == "timeout" {
				deps.CallbackTimeout = shortCallbackTimeout
			}
			start, cancel := handler.NewAuthFlowHandlers(deps)
			t.Cleanup(func() {
				if _, err := cancel(context.Background(), ws.InboundMessage{Type: "AUTH_CANCEL"}); err != nil {
					t.Errorf("cleanup cancel: %v", err)
				}
			})
			response, err := start(ctx, ws.InboundMessage{Type: "AUTH_START", Data: json.RawMessage(`{"provider":"github"}`)})
			if err != nil {
				t.Fatal(err)
			}
			envelope := response.(map[string]interface{})
			if envelope["type"] != "AUTH_REDIRECT" {
				t.Fatalf("start: %#v", response)
			}
			authorizeURL, err := url.Parse(envelope["data"].(map[string]interface{})["url"].(string))
			if err != nil {
				t.Fatal(err)
			}
			callbackURL := auth.RedirectURI + "?code=test-code&state=" + url.QueryEscape(authorizeURL.Query().Get("state"))
			callbackRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, callbackURL, nil)
			if err != nil {
				t.Fatal(err)
			}
			callbackDone := make(chan error, 1)
			go func() {
				client := &http.Client{Transport: originalTransport, Timeout: testDeadline}
				response, err := client.Do(callbackRequest)
				if err == nil {
					err = response.Body.Close()
				}
				callbackDone <- err
			}()
			select {
			case <-exchangeStarted:
			case <-ctx.Done():
				t.Fatal("token exchange did not start")
			}
			if outcome == "cancelled" {
				if response, err := cancel(ctx, ws.InboundMessage{Type: "AUTH_CANCEL"}); response != nil || err != nil {
					t.Fatalf("cancel: %#v, %v", response, err)
				}
			}
			select {
			case <-terminal:
			case <-ctx.Done():
				t.Fatal("missing terminal outcome")
			}
			select {
			case <-exchangeStopped:
			case <-ctx.Done():
				t.Fatal("token exchange was not cancelled")
			}
			select {
			case err := <-callbackDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("callback request did not finish")
			}
			if _, err := cancel(ctx, ws.InboundMessage{Type: "AUTH_CANCEL"}); err != nil {
				t.Fatal(err)
			}
			if len(broadcasts) != 1 {
				t.Fatalf("terminal broadcasts: %#v", broadcasts)
			}
			event := broadcasts[0].(map[string]interface{})
			if event["type"] != "AUTH_FAILED" || event["data"].(map[string]string)["error"] != outcome {
				t.Fatalf("terminal outcome: %#v", event)
			}
			select {
			case repoID := <-triggers:
				t.Fatalf("failed authentication triggered sync: %q", repoID)
			default:
			}
		})
	}
}

func TestAuthSuccessBroadcastsBeforeFullSync(t *testing.T) {
	const deadline = 2 * time.Second
	original := http.DefaultTransport
	http.DefaultTransport = authRoundTripper(func(r *http.Request) (*http.Response, error) {
		body := `{"access_token":"test-token","token_type":"bearer","scope":"repo,read:user"}`
		if r.URL.Path == "/user" {
			body = `{"login":"octocat"}`
		}
		return &http.Response{StatusCode: http.StatusOK,
			Header: http.Header{"Content-Type": {"application/json"}},
			Body:   io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	triggers := make(chan string, 1)
	terminal := make(chan string, 1)
	var providerInstalled bool
	var triggeredBeforeBroadcast bool
	tokens := &fakeTokenStore{}
	deps := handler.AuthDeps{
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Tokens: tokens, Cache: &fakeUsernameCache{},
		GetClientID:     func() string { return "test-client" },
		CallbackTimeout: deadline, SyncTrigger: triggers,
		SetProvider: func(provider.Provider, string) { providerInstalled = true },
		Broadcast: func(message interface{}) {
			select {
			case <-triggers:
				triggeredBeforeBroadcast = true
			default:
			}
			terminal <- message.(map[string]interface{})["type"].(string)
		},
	}
	start, cancel := handler.NewAuthFlowHandlers(deps)
	ctx, stop := context.WithTimeout(context.Background(), deadline)
	defer stop()
	t.Cleanup(func() {
		if _, err := cancel(context.Background(), ws.InboundMessage{Type: "AUTH_CANCEL"}); err != nil {
			t.Error(err)
		}
	})
	response, err := start(ctx, ws.InboundMessage{Type: "AUTH_START",
		Data: json.RawMessage(`{"provider":"github"}`)})
	if err != nil {
		t.Fatal(err)
	}
	envelope := response.(map[string]interface{})
	authorize, err := url.Parse(envelope["data"].(map[string]interface{})["url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	callbackURL := auth.RedirectURI + "?code=test-code&state=" + url.QueryEscape(authorize.Query().Get("state"))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, callbackURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := (&http.Client{Transport: original, Timeout: deadline}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := reply.Body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case repoID := <-triggers:
		if repoID != "" {
			t.Fatalf("trigger = %q", repoID)
		}
	case <-ctx.Done():
		t.Fatal("successful OAuth did not trigger sync")
	}
	if got := <-terminal; got != "AUTH_SUCCESS" {
		t.Fatalf("terminal = %s", got)
	}
	if triggeredBeforeBroadcast || !providerInstalled || tokens.tokens["github"] != "test-token" {
		t.Fatalf("wrong finalization order: early=%v installed=%v", triggeredBeforeBroadcast, providerInstalled)
	}
}

func TestAuthSuccess_CurrentUserWriterNotNil(t *testing.T) {
	deps := handler.AuthDeps{
		Log:             slog.Default(),
		Tokens:          &fakeTokenStore{},
		Cache:           &fakeUsernameCache{},
		Broadcast:       func(interface{}) {},
		SetProvider:     func(provider.Provider, string) {},
		GetClientID:     func() string { return "test-client-id" },
		GetClientSecret: func() string { return "test-client-secret" },
		CurrentUserWriter: handler.CurrentUserWriterFunc(func(_ context.Context, _, _ string) error {
			return nil
		}),
	}
	// Minimal check: the field is accepted at compile time and is not nil.
	if deps.CurrentUserWriter == nil {
		t.Error("CurrentUserWriter should not be nil")
	}
}

func TestAuthCheck_FirstRunNoTokenReportsRequiresAuth(t *testing.T) {
	currentUserWrites := 0
	deps := handler.AuthDeps{
		Log:    slog.Default(),
		Tokens: &fakeTokenStore{},
		Cache:  &fakeUsernameCache{},
		CurrentUserWriter: handler.CurrentUserWriterFunc(func(_ context.Context, _, _ string) error {
			currentUserWrites++
			return nil
		}),
	}

	resp, err := handler.NewAuthCheckHandler(deps)(context.Background(), ws.InboundMessage{
		ID:   "auth-check-1",
		Type: "AUTH_CHECK",
		Data: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("AUTH_CHECK failed: %v", err)
	}

	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	if data["authenticated"] != false {
		t.Fatalf("authenticated = %v, want false", data["authenticated"])
	}
	if data["auth_status"] != "unauthenticated" {
		t.Fatalf("auth_status = %v, want unauthenticated", data["auth_status"])
	}
	if data["auth_reason"] != "no_stored_token" {
		t.Fatalf("auth_reason = %v, want no_stored_token", data["auth_reason"])
	}
	if data["requires_auth"] != true {
		t.Fatalf("requires_auth = %v, want true", data["requires_auth"])
	}
	if data["exchange"] != "proxy" {
		t.Fatalf("exchange = %v, want proxy without a client secret", data["exchange"])
	}
	if currentUserWrites != 0 {
		t.Fatalf("current_user writes = %d, want 0 for unauthenticated first run", currentUserWrites)
	}
}

func TestAuthCheck_CachedStoredTokenReportsAuthenticated(t *testing.T) {
	var wroteProvider, wroteLogin string
	deps := handler.AuthDeps{
		Log:             slog.Default(),
		Tokens:          &fakeTokenStore{tokens: map[string]string{"github": "stored-token"}},
		Cache:           &fakeUsernameCache{usernames: map[string]string{"github": "octocat"}},
		GetClientSecret: func() string { return "test-client-secret" },
		CurrentUserWriter: handler.CurrentUserWriterFunc(func(_ context.Context, provider, login string) error {
			wroteProvider = provider
			wroteLogin = login
			return nil
		}),
	}

	resp, err := handler.NewAuthCheckHandler(deps)(context.Background(), ws.InboundMessage{
		ID:   "auth-check-2",
		Type: "AUTH_CHECK",
		Data: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("AUTH_CHECK failed: %v", err)
	}

	data := resp.(map[string]interface{})["data"].(map[string]interface{})
	if data["authenticated"] != true {
		t.Fatalf("authenticated = %v, want true", data["authenticated"])
	}
	if data["auth_status"] != "authenticated" {
		t.Fatalf("auth_status = %v, want authenticated", data["auth_status"])
	}
	if data["auth_reason"] != "stored_token" {
		t.Fatalf("auth_reason = %v, want stored_token", data["auth_reason"])
	}
	if data["requires_auth"] != false {
		t.Fatalf("requires_auth = %v, want false", data["requires_auth"])
	}
	if data["exchange"] != "local" {
		t.Fatalf("exchange = %v, want local with a client secret", data["exchange"])
	}
	if wroteProvider != "github" || wroteLogin != "octocat" {
		t.Fatalf("current_user write = (%q, %q), want (github, octocat)", wroteProvider, wroteLogin)
	}
}
