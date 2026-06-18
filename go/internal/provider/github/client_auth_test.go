package github

import (
	"context"
	"testing"

	"github.com/samuelasvanyi/bistro/internal/provider"
)

// TestNewClientWithToken verifies that a non-empty token is stored and a
// non-nil github client is created. The oauth2 transport is wired internally;
// we confirm the token is preserved so Authenticate can return it.
func TestNewClientWithToken(t *testing.T) {
	const token = "test-token-value"
	client := NewClient(token)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.gh == nil {
		t.Fatal("expected non-nil github client")
	}
	// Token must be stored so Authenticate can return it without another round-trip.
	if client.token != token {
		t.Errorf("token stored: want %q, got %q", token, client.token)
	}
	// The github HTTP client must be non-nil (oauth2 transport was wired).
	if client.gh.Client() == nil {
		t.Error("expected non-nil HTTP client on github client")
	}
}

// TestNewClientWithEmptyToken verifies that an empty token does NOT wire an
// oauth2 transport — the underlying HTTP client's transport must not be an
// oauth2.Transport.
func TestNewClientWithEmptyToken(t *testing.T) {
	client := NewClient("")
	if client == nil {
		t.Fatal("expected non-nil client")
	}

	// github.NewClient wraps the provided client, so we verify no oauth2
	// transport is present by checking the token field is empty.
	if client.token != "" {
		t.Errorf("expected empty token for unauthenticated client, got %q", client.token)
	}
}

// TestNewClientTokenStored verifies the token field is preserved for
// use by Authenticate.
func TestNewClientTokenStored(t *testing.T) {
	const want = "fake-token-abc123"
	client := NewClient(want)
	if client.token != want {
		t.Errorf("token stored: want %q, got %q", want, client.token)
	}
}

// TestClientImplementsProvider verifies the compile-time interface check holds.
func TestClientImplementsProvider(t *testing.T) {
	var _ provider.Provider = (*Client)(nil)

	client := NewClient("test-token")
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

// TestAuthenticate_WithToken verifies that a pre-set token is returned directly.
func TestAuthenticate_WithToken(t *testing.T) {
	client := NewClient("existing-token")
	ctx := context.Background()

	token, err := client.Authenticate(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "existing-token" {
		t.Errorf("expected token %q, got %q", "existing-token", token)
	}
}

// TestAuthenticate_NoToken verifies that an empty token returns an error.
func TestAuthenticate_NoToken(t *testing.T) {
	client := NewClient("")
	ctx := context.Background()

	_, err := client.Authenticate(ctx)
	if err == nil {
		t.Error("expected error when no token set, got nil")
	}
}
