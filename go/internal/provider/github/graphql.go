package github

import (
	"context"
	"fmt"
	"net/http"

	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"

	"github.com/itterno/bistro/internal/provider"
)

// NewGraphQLClient builds a githubv4 client. When httpClient is nil, it falls
// back to a small standalone client for tests and direct construction.
func NewGraphQLClient(token string, httpClient *http.Client) *githubv4.Client {
	if httpClient == nil && token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		httpClient = oauth2.NewClient(context.Background(), ts)
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return githubv4.NewClient(httpClient)
}

// resolveReviewThreadInput is the GraphQL input type for resolveReviewThread.
// Defined locally to avoid dependency on external type definitions that may
// not be present in all githubv4 library versions.
type resolveReviewThreadInput struct {
	ThreadID githubv4.ID `json:"threadId"`
}

// unresolveReviewThreadInput is the GraphQL input type for unresolveReviewThread.
type unresolveReviewThreadInput struct {
	ThreadID githubv4.ID `json:"threadId"`
}

// ResolveThread executes the resolveReviewThread GraphQL mutation.
// threadID must be the canonical GraphQL node ID (e.g. "PRRT_kwDO...").
// Only threads with StateKnown=true carry valid node IDs; callers should
// verify before calling.
func (c *Client) ResolveThread(ctx context.Context, threadID string) error {
	var m struct {
		ResolveReviewThread struct {
			Thread struct {
				ID         githubv4.ID `graphql:"id"`
				IsResolved bool        `graphql:"isResolved"`
			}
		} `graphql:"resolveReviewThread(input: $input)"`
	}
	if err := c.gqlClient.Mutate(ctx, &m, resolveReviewThreadInput{ThreadID: githubv4.ID(threadID)}, nil); err != nil {
		return fmt.Errorf("resolveReviewThread %s: %w", threadID, err)
	}
	return nil
}

// UnresolveThread executes the unresolveReviewThread GraphQL mutation.
func (c *Client) UnresolveThread(ctx context.Context, threadID string) error {
	var m struct {
		UnresolveReviewThread struct {
			Thread struct {
				ID         githubv4.ID `graphql:"id"`
				IsResolved bool        `graphql:"isResolved"`
			}
		} `graphql:"unresolveReviewThread(input: $input)"`
	}
	if err := c.gqlClient.Mutate(ctx, &m, unresolveReviewThreadInput{ThreadID: githubv4.ID(threadID)}, nil); err != nil {
		return fmt.Errorf("unresolveReviewThread %s: %w", threadID, err)
	}
	return nil
}

// Compile-time assertion: *Client implements ThreadResolver.
var _ provider.ThreadResolver = (*Client)(nil)
