package github

import "testing"

func TestNewAuthenticator(t *testing.T) {
	auth, err := NewAuthenticator("test-client-id")
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}
	if auth == nil {
		t.Error("expected non-nil authenticator")
	}
}

func TestNewAuthenticator_EmptyClientID(t *testing.T) {
	_, err := NewAuthenticator("")
	if err == nil {
		t.Error("expected error for empty client ID, got nil")
	}
}
