package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewAuthenticator_DefaultBaseURL(t *testing.T) {
	auth := NewAuthenticator("my-client-id")
	if auth.baseURL != "https://github.com" {
		t.Errorf("expected default baseURL=https://github.com, got %q", auth.baseURL)
	}
	if auth.clientID != "my-client-id" {
		t.Errorf("expected clientID=my-client-id, got %q", auth.clientID)
	}
}

func TestGetDeviceCode_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login/device/code" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "client_id=test-client-id") {
			t.Errorf("missing client_id in body: %s", string(body))
		}
		if !strings.Contains(string(body), "scope=repo") {
			t.Errorf("missing scope in body: %s", string(body))
		}
		ct := r.Header.Get("Content-Type")
		if ct != "application/x-www-form-urlencoded" {
			t.Errorf("unexpected Content-Type: %s", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"device_code":      "dc-test-123",
			"user_code":        "ABCD-1234",
			"verification_uri": "https://github.com/login/device",
			"expires_in":       900,
			"interval":         5,
		}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	auth := NewAuthenticator("test-client-id")
	auth.baseURL = server.URL

	code, userCode, uri, interval, err := auth.GetDeviceCode(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if code != "dc-test-123" {
		t.Errorf("expected device_code=dc-test-123, got %q", code)
	}
	if userCode != "ABCD-1234" {
		t.Errorf("expected user_code=ABCD-1234, got %q", userCode)
	}
	if uri != "https://github.com/login/device" {
		t.Errorf("expected verification_uri=https://github.com/login/device, got %q", uri)
	}
	if interval != 5 {
		t.Errorf("expected interval=5, got %d", interval)
	}
}

func TestGetDeviceCode_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	auth := NewAuthenticator("bad-client-id")
	auth.baseURL = server.URL

	_, _, _, _, err := auth.GetDeviceCode(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
}

func TestGetDeviceCode_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	auth := NewAuthenticator("test-client-id")
	auth.baseURL = "http://127.0.0.1:1" // unreachable

	_, _, _, _, err := auth.GetDeviceCode(ctx)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestPollForToken_Success(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login/oauth/access_token" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "device_code=dc-test-123") {
			t.Errorf("missing device_code in body: %s", string(body))
		}
		if !strings.Contains(string(body), "grant_type=urn") {
			t.Errorf("missing grant_type in body: %s", string(body))
		}
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount < 3 {
			if err := json.NewEncoder(w).Encode(map[string]string{
				"error": "authorization_pending",
			}); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]string{
			"access_token": "gho_test_token_123",
			"token_type":   "bearer",
			"scope":        "repo",
		}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	auth := NewAuthenticator("test-client-id")
	auth.baseURL = server.URL

	// interval=0 avoids real-second waits in tests
	token, err := auth.PollForToken(context.Background(), "dc-test-123", 0)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if token != "gho_test_token_123" {
		t.Errorf("expected gho_test_token_123, got %q", token)
	}
	if callCount < 3 {
		t.Errorf("expected at least 3 calls, got %d", callCount)
	}
}

func TestPollForToken_SlowDown(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			if err := json.NewEncoder(w).Encode(map[string]string{"error": "slow_down"}); err != nil {
				t.Errorf("failed to encode: %v", err)
			}
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]string{
			"access_token": "gho_slow_token",
		}); err != nil {
			t.Errorf("failed to encode: %v", err)
		}
	}))
	defer server.Close()

	auth := NewAuthenticator("test-client-id")
	auth.baseURL = server.URL

	token, err := auth.PollForToken(context.Background(), "dc-slow", 0)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if token != "gho_slow_token" {
		t.Errorf("expected gho_slow_token, got %q", token)
	}
}

func TestPollForToken_ExpiredToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "expired_token"}); err != nil {
			t.Errorf("failed to encode: %v", err)
		}
	}))
	defer server.Close()

	auth := NewAuthenticator("test-client-id")
	auth.baseURL = server.URL

	_, err := auth.PollForToken(context.Background(), "dc-expired", 0)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected error to mention 'expired', got: %v", err)
	}
}

func TestPollForToken_AccessDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "access_denied"}); err != nil {
			t.Errorf("failed to encode: %v", err)
		}
	}))
	defer server.Close()

	auth := NewAuthenticator("test-client-id")
	auth.baseURL = server.URL

	_, err := auth.PollForToken(context.Background(), "dc-denied", 0)
	if err == nil {
		t.Fatal("expected error for access denied, got nil")
	}
	if !strings.Contains(err.Error(), "denied") {
		t.Errorf("expected error to mention 'denied', got: %v", err)
	}
}

func TestPollForToken_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	auth := NewAuthenticator("test-client-id")
	auth.baseURL = "http://127.0.0.1:1" // unreachable

	_, err := auth.PollForToken(ctx, "dc-cancel", 0)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}
