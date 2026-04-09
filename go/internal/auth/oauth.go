package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

// ProviderConfig holds OAuth configuration for a provider.
type ProviderConfig struct {
	AuthURL      string
	TokenURL     string
	Scopes       []string
	ClientID     string
	ClientSecret string
}

// BuildAuthURL constructs the OAuth authorization URL and returns it along with
// the random state parameter. The state must be validated on the callback.
func BuildAuthURL(cfg ProviderConfig, redirectURI string) (authURL string, state string, err error) {
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", "", fmt.Errorf("generating state: %w", err)
	}
	state = hex.EncodeToString(stateBytes)

	params := url.Values{
		"client_id":    {cfg.ClientID},
		"redirect_uri": {redirectURI},
		"scope":        {strings.Join(cfg.Scopes, " ")},
		"state":        {state},
	}

	return cfg.AuthURL + "?" + params.Encode(), state, nil
}
