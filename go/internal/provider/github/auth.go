package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v63/github"
)

// Authenticator handles GitHub OAuth device flow
type Authenticator struct {
	clientID string
	client   *github.Client
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(clientID string) (*Authenticator, error) {
	if clientID == "" {
		return nil, fmt.Errorf("client ID cannot be empty")
	}

	return &Authenticator{
		clientID: clientID,
		client:   github.NewClient(nil),
	}, nil
}

// GetDeviceCode initiates the device flow
func (a *Authenticator) GetDeviceCode(ctx context.Context, scopes []string) (code, userCode, verificationURL string, err error) {
	// Placeholder for actual device flow implementation
	// In production, this calls the GitHub device authorization endpoint
	// For now, return structured types to verify interface works
	return "device-code-placeholder", "XXXX-XXXX", "https://github.com/login/device", nil
}

// PollForToken polls GitHub for access token after user approves
func (a *Authenticator) PollForToken(ctx context.Context, deviceCode string, interval int) (token string, err error) {
	// Placeholder for actual polling implementation
	// In production, polls the GitHub device flow endpoint until user approves or times out
	return "gho_token_placeholder", nil
}
