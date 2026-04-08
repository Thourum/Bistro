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
