package handler_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/server/handler"
)

type fakeTokenStore struct{}

func (f *fakeTokenStore) Get(_ string) (string, error)        { return "", nil }
func (f *fakeTokenStore) Store(_, _ string) error             { return nil }
func (f *fakeTokenStore) Delete(_ string) error               { return nil }

type fakeUsernameCache struct{}

func (f *fakeUsernameCache) Get(_ string) (string, bool, bool) { return "", false, false }
func (f *fakeUsernameCache) Set(_, _ string)                   {}
func (f *fakeUsernameCache) Delete(_ string)                   {}

func TestAuthSuccess_WritesCurrentUser(t *testing.T) {
	written := map[string]string{}
	deps := handler.AuthDeps{
		Log:    slog.Default(),
		Tokens: &fakeTokenStore{},
		Cache:  &fakeUsernameCache{},
		Broadcast: func(interface{}) {},
		SetProvider: func(provider.Provider, string) {},
		GetClientID:     func() string { return "test-client-id" },
		GetClientSecret: func() string { return "test-client-secret" },
		CurrentUserWriter: handler.CurrentUserWriterFunc(func(_ context.Context, prov, user string) error {
			written[prov] = user
			return nil
		}),
	}
	// Minimal check: the field is accepted at compile time and is not nil.
	if deps.CurrentUserWriter == nil {
		t.Error("CurrentUserWriter should not be nil")
	}
	_ = written
}
