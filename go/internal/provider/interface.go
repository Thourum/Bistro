package provider

import (
	"context"
	"time"
)

// Provider abstracts operations for any Git service
type Provider interface {
	// Authenticate performs OAuth device flow
	Authenticate(ctx context.Context) (token string, err error)

	// ListPRs fetches list of PRs with optional filters
	ListPRs(ctx context.Context, filters PRFilters) ([]*PR, error)

	// GetPR fetches a single PR by ID
	GetPR(ctx context.Context, id string) (*PR, error)

	// GetComments fetches all comments for a PR
	GetComments(ctx context.Context, prID string) ([]*Comment, error)

	// ListReviews fetches all review submissions for a PR
	ListReviews(ctx context.Context, prID string) ([]*Review, error)

	// GetCIStatus fetches CI check runs for a PR
	GetCIStatus(ctx context.Context, prID string) (*CIStatus, error)

	// Approve approves a PR
	Approve(ctx context.Context, prID string) error

	// RequestChanges requests changes on a PR with comment
	RequestChanges(ctx context.Context, prID string, body string) error

	// PostComment posts a comment to a PR
	PostComment(ctx context.Context, prID string, line int, body string) error

	// GetUsername returns the authenticated user's login name
	GetUsername(ctx context.Context) (string, error)

	// ListUserRepos fetches all repos the authenticated user can access
	ListUserRepos(ctx context.Context) ([]*Repo, error)
}

// PRFilters defines optional PR list filters
type PRFilters struct {
	Owner  string // GitHub owner/org
	Repo   string // GitHub repository name
	State  string // "open", "closed", "merged"
	Limit  int    // max results per page
	Offset int    // pagination offset
	// UpdatedSince bounds the list to PRs updated at or after this time.
	// Older PRs are dropped, except that at least MinCount of the most
	// recently updated PRs are always returned when the repo has them.
	UpdatedSince *time.Time
	// MinCount is the floor applied with UpdatedSince: quiet repos still
	// return their MinCount most recently updated PRs. Ignored when
	// UpdatedSince is nil.
	MinCount int
}

// ThreadProvider is an optional capability for providers that support
// fetching inline review threads. Discovered via type assertion — it is NOT
// part of the core Provider interface so non-GitHub providers compile without it.
type ThreadProvider interface {
	GetReviewThreads(ctx context.Context, prID string) ([]*ReviewThread, error)
}

// InboxDiscoveryOptions scopes provider-native inbox discovery. Cursor and ETag
// values are local-only sync metadata, not GitHub-synced PR state.
type InboxDiscoveryOptions struct {
	Username string
	Since    *time.Time
	ETag     string
	Limit    int
}

// InboxDiscoveryResult contains fully-hydrated PRs discovered from lightweight
// provider search pointers. HighWater is the newest provider updated_at seen.
type InboxDiscoveryResult struct {
	PRs       []*PR
	HighWater time.Time
	ETag      string
}

// InboxDiscoveryProvider is an optional capability for providers that can find
// PRs relevant to the authenticated user's inbox without scanning every repo.
type InboxDiscoveryProvider interface {
	DiscoverInboxPRs(ctx context.Context, opts InboxDiscoveryOptions) (*InboxDiscoveryResult, error)
}

// ThreadResolver is an optional capability for providers that support
// resolving and unresolving review threads via GraphQL mutations.
// Requires StateKnown=true threads with a canonical GraphQL node ID.
type ThreadResolver interface {
	ResolveThread(ctx context.Context, threadID string) error
	UnresolveThread(ctx context.Context, threadID string) error
}

// CommentReplier is an optional capability for providers that support replying
// to an existing inline review comment. The inReplyTo value is provider-specific;
// GitHub accepts either a domain comment ID like "github-review-comment-123" or
// the raw numeric review-comment database ID.
type CommentReplier interface {
	ReplyToComment(ctx context.Context, prID string, inReplyTo string, body string) error
}
