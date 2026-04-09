package keychain

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
