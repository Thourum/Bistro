package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestCallbackServer_ValidCallback verifies that a valid state+code triggers
// token exchange and sends a successful result on the channel.
func TestCallbackServer_ValidCallback(t *testing.T) {
	// Mock token endpoint
	mockTokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{
			"access_token": "gho_test_token_123",
			"token_type":   "bearer",
			"scope":        "repo,read:user",
		}); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	}))
	defer mockTokenServer.Close()

	cfg := ProviderConfig{
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     mockTokenServer.URL,
		Scopes:       []string{"repo", "read:user"},
		ClientID:     "test-client-id",
		ClientSecret: "test-secret",
	}

	resultCh := make(chan CallbackResult, 1)
	cs := NewCallbackServer(cfg, "test-state-value", "http://localhost:19284/callback", resultCh)

	// Test the handler directly via httptest.
	handler := cs.handler()

	req := httptest.NewRequest("GET", "/callback?state=test-state-value&code=auth-code-xyz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	select {
	case result := <-resultCh:
		if result.Err != nil {
			t.Fatalf("unexpected error: %v", result.Err)
		}
		if result.Token != "gho_test_token_123" {
			t.Fatalf("expected gho_test_token_123, got %s", result.Token)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for result")
	}
}

// TestCallbackServer_StateMismatch verifies that a wrong state is rejected.
func TestCallbackServer_StateMismatch(t *testing.T) {
	cfg := ProviderConfig{
		AuthURL:  "https://github.com/login/oauth/authorize",
		TokenURL: "https://example.com/token",
		ClientID: "cid",
	}

	resultCh := make(chan CallbackResult, 1)
	cs := NewCallbackServer(cfg, "correct-state", "http://localhost:19284/callback", resultCh)

	handler := cs.handler()
	req := httptest.NewRequest("GET", "/callback?state=wrong-state&code=code123", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}

	select {
	case result := <-resultCh:
		if result.Err == nil {
			t.Fatal("expected error for state mismatch")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for result")
	}
}

// TestCallbackServer_MissingCode verifies that a missing code is rejected.
func TestCallbackServer_MissingCode(t *testing.T) {
	cfg := ProviderConfig{
		AuthURL:  "https://github.com/login/oauth/authorize",
		TokenURL: "https://example.com/token",
		ClientID: "cid",
	}

	resultCh := make(chan CallbackResult, 1)
	cs := NewCallbackServer(cfg, "state123", "http://localhost:19284/callback", resultCh)

	handler := cs.handler()
	req := httptest.NewRequest("GET", "/callback?state=state123", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	select {
	case result := <-resultCh:
		if result.Err == nil {
			t.Fatal("expected error for missing code")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for result")
	}
}

// TestExchangeToken_Success verifies token exchange with a mock server.
func TestExchangeToken_Success(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "expected POST", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "parse error", http.StatusBadRequest)
			return
		}
		if r.Form.Get("code") != "auth-code" {
			http.Error(w, "wrong code", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{
			"access_token": "tok123",
			"token_type":   "bearer",
			"scope":        "repo,read:user",
		}); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	}))
	defer mockServer.Close()

	cfg := ProviderConfig{
		TokenURL:     mockServer.URL,
		ClientID:     "cid",
		ClientSecret: "secret",
		Scopes:       []string{"repo", "read:user"},
	}

	token, err := ExchangeToken(context.Background(), cfg, "auth-code", "http://localhost:19284/callback")
	if err != nil {
		t.Fatalf("ExchangeToken failed: %v", err)
	}
	if token != "tok123" {
		t.Fatalf("expected tok123, got %s", token)
	}
}

// TestExchangeToken_ScopeMismatch verifies that a scope mismatch is rejected.
func TestExchangeToken_ScopeMismatch(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{
			"access_token": "tok123",
			"token_type":   "bearer",
			"scope":        "public_repo", // missing read:user
		}); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	}))
	defer mockServer.Close()

	cfg := ProviderConfig{
		TokenURL:     mockServer.URL,
		ClientID:     "cid",
		ClientSecret: "secret",
		Scopes:       []string{"repo", "read:user"},
	}

	_, err := ExchangeToken(context.Background(), cfg, "auth-code", "http://localhost:19284/callback")
	if err == nil {
		t.Fatal("expected scope mismatch error")
	}
	if !strings.Contains(err.Error(), "scope_mismatch") {
		t.Fatalf("expected scope_mismatch in error, got: %v", err)
	}
}

// TestExchangeToken_ErrorResponse verifies that an error field in the response is handled.
func TestExchangeToken_ErrorResponse(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{
			"error":             "bad_verification_code",
			"error_description": "The code passed is incorrect or expired.",
		}); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	}))
	defer mockServer.Close()

	cfg := ProviderConfig{
		TokenURL:     mockServer.URL,
		ClientID:     "cid",
		ClientSecret: "secret",
		Scopes:       []string{"repo"},
	}

	_, err := ExchangeToken(context.Background(), cfg, "bad-code", "http://localhost:19284/callback")
	if err == nil {
		t.Fatal("expected error for bad_verification_code")
	}
	if !strings.Contains(err.Error(), "bad_verification_code") {
		t.Fatalf("expected bad_verification_code in error, got: %v", err)
	}
}

// TestCallbackConstants verifies the exported constants have expected values.
func TestCallbackConstants(t *testing.T) {
	if CallbackPort != 19284 {
		t.Fatalf("expected port 19284, got %d", CallbackPort)
	}
	if CallbackPath != "/callback" {
		t.Fatalf("expected /callback path, got %s", CallbackPath)
	}
	if RedirectURI != fmt.Sprintf("http://localhost:%d/callback", CallbackPort) {
		t.Fatalf("unexpected RedirectURI: %s", RedirectURI)
	}
	if CallbackTimeout != 5*time.Minute {
		t.Fatalf("expected 5m timeout, got %v", CallbackTimeout)
	}
}
