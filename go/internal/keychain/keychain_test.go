package keychain

import (
	"testing"
)

func TestStoreAndRetrieveToken(t *testing.T) {
	service := "bistro-test"
	provider := "github"
	token := "test-token-value"

	// Store
	err := StoreToken(service, provider, token)
	if err != nil {
		t.Fatalf("failed to store token: %v", err)
	}

	// Retrieve
	retrieved, err := GetToken(service, provider)
	if err != nil {
		t.Fatalf("failed to retrieve token: %v", err)
	}

	if retrieved != token {
		t.Errorf("expected %s, got %s", token, retrieved)
	}

	// Cleanup
	DeleteToken(service, provider)
}

func TestDeleteToken(t *testing.T) {
	service := "bistro-test"
	provider := "github"
	token := "test-token-value"

	StoreToken(service, provider, token)
	err := DeleteToken(service, provider)
	if err != nil {
		t.Fatalf("failed to delete token: %v", err)
	}

	_, err = GetToken(service, provider)
	if err == nil {
		t.Error("expected error retrieving deleted token")
	}
}

func TestTokenManager_GenericStoreAndGet(t *testing.T) {
	tm := NewTokenManager()
	err := tm.Store("gitlab", "test-token-gitlab")
	if err != nil {
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
	err := tm.Delete("gitea")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	_, err = tm.Get("gitea")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}
