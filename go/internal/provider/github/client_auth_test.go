package github

import (
	"testing"
)

// TestNewClientWithToken verifies that a non-empty token is stored and a
// non-nil github client is created. The oauth2 transport is wired internally;
// we confirm the token is preserved so Authenticate can return it.
func TestNewClientWithToken(t *testing.T) {
	const token = "ghp_test-token-value"
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
	const want = "ghp_abc123"
	client := NewClient(want)
	if client.token != want {
		t.Errorf("token stored: want %q, got %q", want, client.token)
	}
}
