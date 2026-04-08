package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Authenticator handles GitHub OAuth device flow.
type Authenticator struct {
	clientID string
	baseURL  string
}

// DeviceCodeResponse holds the response from the device code endpoint.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// TokenResponse holds the response from the token polling endpoint.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
}

// NewAuthenticator creates a new Authenticator with the given client ID.
// baseURL defaults to "https://github.com" and can be overridden in tests.
func NewAuthenticator(clientID string) *Authenticator {
	return &Authenticator{
		clientID: clientID,
		baseURL:  "https://github.com",
	}
}

// GetDeviceCode initiates the GitHub device flow and returns the device code,
// user code, verification URI, and polling interval in seconds.
func (a *Authenticator) GetDeviceCode(ctx context.Context) (deviceCode, userCode, verificationURI string, interval int, err error) {
	url := a.baseURL + "/login/device/code"

	body := fmt.Sprintf("client_id=%s&scope=repo", a.clientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return "", "", "", 0, fmt.Errorf("creating device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("device code request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", 0, fmt.Errorf("device code endpoint returned status %d", resp.StatusCode)
	}

	var result DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", "", 0, fmt.Errorf("decoding device code response: %w", err)
	}

	if result.DeviceCode == "" {
		return "", "", "", 0, fmt.Errorf("empty device_code in response")
	}

	return result.DeviceCode, result.UserCode, result.VerificationURI, result.Interval, nil
}

// PollForToken polls the GitHub access token endpoint until the user approves
// the device flow, the code expires, or the context is cancelled.
func (a *Authenticator) PollForToken(ctx context.Context, deviceCode string, interval int) (string, error) {
	url := a.baseURL + "/login/oauth/access_token"

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}

		body := fmt.Sprintf(
			"client_id=%s&device_code=%s&grant_type=urn:ietf:params:oauth:grant-type:device_code",
			a.clientID, deviceCode,
		)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("creating poll request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("poll request: %w", err)
		}

		var tokenResp TokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
			resp.Body.Close()
			return "", fmt.Errorf("decoding poll response: %w", err)
		}
		resp.Body.Close()

		switch tokenResp.Error {
		case "":
			if tokenResp.AccessToken == "" {
				return "", fmt.Errorf("empty access_token in successful response")
			}
			return tokenResp.AccessToken, nil
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5
			continue
		case "expired_token":
			return "", fmt.Errorf("device code expired — please restart the login flow")
		case "access_denied":
			return "", fmt.Errorf("access denied by user")
		default:
			return "", fmt.Errorf("unexpected error from GitHub: %s", tokenResp.Error)
		}
	}
}
