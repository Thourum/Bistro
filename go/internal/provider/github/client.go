package github

import (
	"context"
	"net/http"

	"github.com/google/go-github/v63/github"
	"github.com/samuelasvanyi/bistro/internal/provider"
	"golang.org/x/oauth2"
)

// Verify Client implements Provider interface at compile time.
var _ provider.Provider = (*Client)(nil)

// Client wraps GitHub API client with authentication token
type Client struct {
	token string
	gh    *github.Client
}

// NewClient creates a new GitHub client with authentication token.
// When token is non-empty, an oauth2 transport is used so all API calls
// are authenticated. When token is empty, the default HTTP client is used
// (unauthenticated, rate-limited to 60 req/h).
func NewClient(token string) *Client {
	var httpClient *http.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		httpClient = oauth2.NewClient(context.Background(), ts)
	} else {
		httpClient = http.DefaultClient
	}
	return &Client{
		token: token,
		gh:    github.NewClient(httpClient),
	}
}
