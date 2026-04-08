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

// StoreGitHubToken stores the GitHub OAuth token in the OS keychain.
func (tm *TokenManager) StoreGitHubToken(token string) error {
	return StoreToken(tm.service, "github", token)
}

// GetGitHubToken retrieves the GitHub OAuth token from the OS keychain.
func (tm *TokenManager) GetGitHubToken() (string, error) {
	return GetToken(tm.service, "github")
}

// DeleteGitHubToken removes the GitHub OAuth token from the OS keychain.
func (tm *TokenManager) DeleteGitHubToken() error {
	return DeleteToken(tm.service, "github")
}
