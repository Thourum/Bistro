package github

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/provider"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

// Verify Client implements Provider interface at compile time.
var _ provider.Provider = (*Client)(nil)

// Client wraps GitHub API client with authentication token
type Client struct {
	token     string
	gh        *github.Client
	gqlClient *githubv4.Client
}

// NewClient creates a new GitHub client with authentication token.
// When token is non-empty, an oauth2 transport is used so all API calls
// are authenticated. When token is empty, the default HTTP client is used
// (unauthenticated, rate-limited to 60 req/h).
//
// All outbound requests are instrumented with a logging round-tripper; see
// logging_transport.go. The round-tripper reads the logger from the request
// context (see internal/logging), falling back to slog.Default().
func NewClient(token string) *Client {
	var httpClient *http.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		httpClient = oauth2.NewClient(context.Background(), ts)
	} else {
		httpClient = &http.Client{Transport: http.DefaultTransport}
	}
	// The logging transport wraps OUTSIDE the oauth2 transport, so the
	// Authorization header is added after logging and never appears in log
	// output. Redaction in logging_transport covers manually-set headers.
	httpClient.Transport = newLoggingTransport(httpClient.Transport)
	// Without a timeout a stalled request blocks a sync worker forever and
	// the whole sync pass silently never completes.
	httpClient.Timeout = 30 * time.Second
	return &Client{
		token:     token,
		gh:        github.NewClient(httpClient),
		gqlClient: NewGraphQLClient(token, httpClient),
	}
}

// GetUsername returns the login name of the authenticated GitHub user.
func (c *Client) GetUsername(ctx context.Context) (string, error) {
	user, _, err := c.gh.Users.Get(ctx, "")
	if err != nil {
		return "", fmt.Errorf("failed to get authenticated user: %w", mapAPIError(err))
	}
	return user.GetLogin(), nil
}
