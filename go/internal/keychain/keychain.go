package keychain

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

// StoreToken stores an OAuth token in the OS keychain.
func StoreToken(service, provider, token string) error {
	key := fmt.Sprintf("%s:%s", service, provider)
	return keyring.Set(service, key, token)
}

// GetToken retrieves an OAuth token from the OS keychain.
func GetToken(service, provider string) (string, error) {
	key := fmt.Sprintf("%s:%s", service, provider)
	return keyring.Get(service, key)
}

// DeleteToken removes an OAuth token from the OS keychain.
func DeleteToken(service, provider string) error {
	key := fmt.Sprintf("%s:%s", service, provider)
	return keyring.Delete(service, key)
}

const BistroService = "bistro-sync"

// TokenManager provides a simplified interface for token storage.
type TokenManager struct {
	service string
}

// NewTokenManager returns a TokenManager configured with the default service name.
func NewTokenManager() *TokenManager {
	return &TokenManager{service: BistroService}
}

// Store saves the token for the given provider in the OS keychain.
func (tm *TokenManager) Store(provider, token string) error {
	return StoreToken(tm.service, provider, token)
}

// Get retrieves the token for the given provider from the OS keychain.
func (tm *TokenManager) Get(provider string) (string, error) {
	return GetToken(tm.service, provider)
}

// Delete removes the token for the given provider from the OS keychain.
func (tm *TokenManager) Delete(provider string) error {
	return DeleteToken(tm.service, provider)
}

// StoreGitHubToken stores the GitHub OAuth token in the OS keychain.
func (tm *TokenManager) StoreGitHubToken(token string) error {
	return tm.Store("github", token)
}

// GetGitHubToken retrieves the GitHub OAuth token from the OS keychain.
func (tm *TokenManager) GetGitHubToken() (string, error) {
	return tm.Get("github")
}

// DeleteGitHubToken removes the GitHub OAuth token from the OS keychain.
func (tm *TokenManager) DeleteGitHubToken() error {
	return tm.Delete("github")
}
