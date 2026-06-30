package handler_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

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
	if currentUserWrites != 0 {
		t.Fatalf("current_user writes = %d, want 0 for unauthenticated first run", currentUserWrites)
	}
}

func TestAuthCheck_CachedStoredTokenReportsAuthenticated(t *testing.T) {
	var wroteProvider, wroteLogin string
	deps := handler.AuthDeps{
		Log:    slog.Default(),
		Tokens: &fakeTokenStore{tokens: map[string]string{"github": "stored-token"}},
		Cache:  &fakeUsernameCache{usernames: map[string]string{"github": "octocat"}},
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
	if wroteProvider != "github" || wroteLogin != "octocat" {
		t.Fatalf("current_user write = (%q, %q), want (github, octocat)", wroteProvider, wroteLogin)
	}
}
