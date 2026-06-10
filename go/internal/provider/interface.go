package provider

import "context"

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
}

// RateLimitTracker manages rate limit state persistence.
type RateLimitTracker interface {
	// Store persists rate limit state.
	Store(ctx context.Context, state *RateLimitState) error
	// Load retrieves last known rate limit state.
	Load(ctx context.Context) (*RateLimitState, error)
	// Clear resets stored rate limit state.
	Clear(ctx context.Context) error
}
