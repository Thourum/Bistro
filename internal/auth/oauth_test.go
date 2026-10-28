package auth

import (
	"net/url"
	"testing"
)

func TestBuildAuthURL_GitHub(t *testing.T) {
	cfg := ProviderConfig{
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		Scopes:       []string{"repo", "read:user"},
		ClientID:     "test-client-id",
		ClientSecret: "test-secret",
	}

	authURL, state, err := BuildAuthURL(cfg, "http://localhost:19284/callback")
	if err != nil {
		t.Fatalf("BuildAuthURL failed: %v", err)
	}

	if state == "" {
		t.Fatal("expected non-empty state")
	}
	if len(state) != 64 { // 32 bytes hex-encoded
		t.Fatalf("expected 64 char state, got %d", len(state))
	}

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("invalid URL: %v", err)
	}

	if parsed.Host != "github.com" {
		t.Fatalf("expected github.com host, got %s", parsed.Host)
	}
	if parsed.Query().Get("client_id") != "test-client-id" {
		t.Fatalf("wrong client_id: %s", parsed.Query().Get("client_id"))
	}
	if parsed.Query().Get("redirect_uri") != "http://localhost:19284/callback" {
		t.Fatalf("wrong redirect_uri: %s", parsed.Query().Get("redirect_uri"))
	}
	if parsed.Query().Get("scope") != "repo read:user" {
		t.Fatalf("wrong scope: %s", parsed.Query().Get("scope"))
	}
	if parsed.Query().Get("state") != state {
		t.Fatal("state in URL doesn't match returned state")
	}
}

func TestBuildAuthURL_UniqueState(t *testing.T) {
	cfg := ProviderConfig{
		AuthURL:  "https://github.com/login/oauth/authorize",
		ClientID: "test",
	}

	_, state1, _ := BuildAuthURL(cfg, "http://localhost:19284/callback")
	_, state2, _ := BuildAuthURL(cfg, "http://localhost:19284/callback")

	if state1 == state2 {
		t.Fatal("expected unique state values")
	}
}

func TestBuildAuthURL_StateLength(t *testing.T) {
	cfg := ProviderConfig{
		AuthURL:  "https://example.com/authorize",
		ClientID: "cid",
	}
	_, state, err := BuildAuthURL(cfg, "http://localhost:19284/callback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state) != 64 {
		t.Fatalf("state must be 64 hex chars (32 bytes), got %d", len(state))
	}
}
