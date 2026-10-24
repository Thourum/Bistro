package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func TestCallbackFailurePage(t *testing.T) {
	cases := []struct {
		name, query, code, detail string
		status                    int
	}{
		{name: "state", query: "state=wrong&code=x", code: "INVALID_STATE", detail: "state mismatch", status: http.StatusForbidden},
		{name: "denied", query: "state=valid&error=access_denied&error_description=%3Cscript%3E", code: "access_denied", detail: "&lt;script&gt;", status: http.StatusBadRequest},
		{name: "missing", query: "state=valid", code: "MISSING_CODE", detail: "missing code", status: http.StatusBadRequest},
		{name: "exchange", query: "state=valid&code=x", code: "TOKEN_EXCHANGE_FAILED", detail: "bad_verification_code", status: http.StatusInternalServerError},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(`{"error":"bad_verification_code","error_description":"<script>"}`)); err != nil {
					t.Errorf("write token response: %v", err)
				}
			}))
			defer tokenEndpoint.Close()
			results := make(chan CallbackResult, 1)
			cs := NewCallbackServer(ProviderConfig{TokenURL: tokenEndpoint.URL}, "valid", RedirectURI, results)
			response := httptest.NewRecorder()
			cs.handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/callback?"+test.query, nil))
			if response.Code != test.status {
				t.Fatalf("status %d", response.Code)
			}
			body := response.Body.String()
			for _, want := range []string{test.code, test.detail, "Sign-in didn", "Nothing was saved", "description", "at"} {
				if !strings.Contains(body, want) {
					t.Fatalf("missing %q in %s", want, body)
				}
			}
			if strings.Contains(body, "<script>") {
				t.Fatal("unescaped detail")
			}
			select {
			case result := <-results:
				if result.Err == nil {
					t.Fatal("failure did not reach auth flow")
				}
			case <-time.After(time.Second):
				t.Fatal("missing callback result")
			}
		})
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

// TestExchangeToken_WithoutSecretPostsJSONToExchangeEndpoint verifies that a
// build without a client secret sends {code, redirect_uri} as JSON and no
// client_id or client_secret, as the get-bistro.com exchange expects.
func TestExchangeToken_WithoutSecretPostsJSONToExchangeEndpoint(t *testing.T) {
	var gotContentType string
	var gotBody map[string]any
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			http.Error(w, "decode error", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{
			"access_token": "tok-exchanged",
			"token_type":   "bearer",
			"scope":        "repo,read:user",
		}); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	}))
	defer mockServer.Close()

	cfg := ProviderConfig{
		TokenURL: mockServer.URL,
		ClientID: "cid",
		Scopes:   []string{"repo", "read:user"},
	}

	token, err := ExchangeToken(context.Background(), cfg, "auth-code", "http://localhost:19284/callback")
	if err != nil {
		t.Fatalf("ExchangeToken failed: %v", err)
	}
	if token != "tok-exchanged" {
		t.Fatalf("expected tok-exchanged, got %s", token)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected application/json content type, got %q", gotContentType)
	}
	if gotBody["code"] != "auth-code" || gotBody["redirect_uri"] != "http://localhost:19284/callback" {
		t.Fatalf("unexpected body: %v", gotBody)
	}
	for _, forbidden := range []string{"client_id", "client_secret"} {
		if _, ok := gotBody[forbidden]; ok {
			t.Fatalf("body must not contain %s: %v", forbidden, gotBody)
		}
	}
}
