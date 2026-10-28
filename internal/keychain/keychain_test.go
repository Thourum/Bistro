package keychain

import (
	"testing"
)

func TestStoreAndRetrieveToken(t *testing.T) {
	tm := &TokenManager{service: "bistro-test"}
	provider := "github"
	token := "test-token-value"

	// Store
	if err := tm.Store(provider, token); err != nil {
		t.Fatalf("failed to store token: %v", err)
	}

	// Retrieve
	retrieved, err := tm.Get(provider)
	if err != nil {
		t.Fatalf("failed to retrieve token: %v", err)
	}

	if retrieved != token {
		t.Errorf("expected %s, got %s", token, retrieved)
	}

	// Cleanup
	_ = tm.Delete(provider)
}

func TestDeleteToken(t *testing.T) {
	tm := &TokenManager{service: "bistro-test"}
	provider := "github"
	token := "test-token-value"

	_ = tm.Store(provider, token)
	if err := tm.Delete(provider); err != nil {
		t.Fatalf("failed to delete token: %v", err)
	}

	_, err := tm.Get(provider)
	if err == nil {
		t.Error("expected error retrieving deleted token")
	}
}

func TestTokenManager_GenericStoreAndGet(t *testing.T) {
	tm := NewTokenManager()
	if err := tm.Store("gitlab", "test-token-gitlab"); err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	token, err := tm.Get("gitlab")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if token != "test-token-gitlab" {
		t.Fatalf("expected test-token-gitlab, got %s", token)
	}
	_ = tm.Delete("gitlab")
}

func TestTokenManager_GenericDelete(t *testing.T) {
	tm := NewTokenManager()
	_ = tm.Store("gitea", "test-token-gitea")
	if err := tm.Delete("gitea"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	_, err := tm.Get("gitea")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}
