package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/itterno/bistro/internal/provider"
)

// MockProvider implements Provider interface with canned data for testing.
type MockProvider struct {
	prs             []*provider.PR
	comments        []*provider.Comment
	reviews         []*provider.Review
	ciStatus        *provider.CIStatus
	err             error
	GetUsernameFunc func(ctx context.Context) (string, error)
	// Repos is the list returned by ListUserRepos.
	Repos []*provider.Repo
	// Thread provider fields.
	reviewThreads    []*provider.ReviewThread
	reviewThreadsErr error
	// ThreadResolver fields.
	resolveErr   error
	unresolveErr error
	// ResolvedCalls records thread IDs passed to ResolveThread.
	ResolvedCalls []string
	// UnresolvedCalls records thread IDs passed to UnresolveThread.
	UnresolvedCalls []string
	// ReplyCalls records reply_comment provider calls.
	ReplyCalls []ReplyCall
	// PostCommentCalls records post_comment provider calls.
	PostCommentCalls []PostCommentCall
	// Inbox discovery fields.
	InboxDiscoveryResult *provider.InboxDiscoveryResult
	InboxDiscoveryErr    error
	InboxDiscoveryCalls  []provider.InboxDiscoveryOptions
}

// ReplyCall records a comment reply mutation request.
type ReplyCall struct {
	PRID      string
	InReplyTo string
	Body      string
}

// PostCommentCall records a PR comment mutation request.
type PostCommentCall struct {
	PRID string
	Line int
	Body string
}

// NewMockProvider creates a new mock provider with empty state.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		prs:      make([]*provider.PR, 0),
		comments: make([]*provider.Comment, 0),
	}
}

// WithPRs returns a new MockProvider with the given PR list configured.
func (m *MockProvider) WithPRs(prs []*provider.PR) *MockProvider {
	clone := *m
	clone.prs = prs
	return &clone
}

// WithComments returns a new MockProvider with the given comments configured.
func (m *MockProvider) WithComments(comments []*provider.Comment) *MockProvider {
	clone := *m
	clone.comments = comments
	return &clone
}

// WithReviews returns a new MockProvider with the given reviews configured.
func (m *MockProvider) WithReviews(reviews []*provider.Review) *MockProvider {
	clone := *m
	clone.reviews = reviews
	return &clone
}

// WithCIStatus returns a new MockProvider with the given CI status configured.
func (m *MockProvider) WithCIStatus(status *provider.CIStatus) *MockProvider {
	clone := *m
	clone.ciStatus = status
	return &clone
}

// WithError returns a new MockProvider that returns err from all methods.
func (m *MockProvider) WithError(err error) *MockProvider {
	clone := *m
	clone.err = err
	return &clone
}

// WithRepos returns a new MockProvider with the given repos configured.
func (m *MockProvider) WithRepos(repos []*provider.Repo) *MockProvider {
	clone := *m
	clone.Repos = repos
	return &clone
}

// WithInboxDiscovery returns a new MockProvider with inbox discovery configured.
func (m *MockProvider) WithInboxDiscovery(result *provider.InboxDiscoveryResult, err error) *MockProvider {
	clone := *m
	clone.InboxDiscoveryResult = result
	clone.InboxDiscoveryErr = err
	return &clone
}

// Authenticate returns a mock token.
func (m *MockProvider) Authenticate(_ context.Context) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return "mock-token", nil
}

// ListPRs returns the configured PRs.
func (m *MockProvider) ListPRs(_ context.Context, _ provider.PRFilters) ([]*provider.PR, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.prs, nil
}

// GetPR returns the first PR matching id, or an error if not found.
func (m *MockProvider) GetPR(_ context.Context, id string) (*provider.PR, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, pr := range m.prs {
		if pr.ID == id {
			return pr, nil
		}
	}
	return nil, fmt.Errorf("PR not found: %s", id)
}

// GetComments returns the configured comments.
func (m *MockProvider) GetComments(_ context.Context, _ string) ([]*provider.Comment, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.comments, nil
}

// ListReviews returns the configured reviews.
func (m *MockProvider) ListReviews(_ context.Context, _ string) ([]*provider.Review, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.reviews, nil
}

// GetCIStatus returns the configured CI status.
func (m *MockProvider) GetCIStatus(_ context.Context, _ string) (*provider.CIStatus, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.ciStatus, nil
}

// Approve succeeds without error unless WithError was configured.
func (m *MockProvider) Approve(_ context.Context, _ string) error {
	return m.err
}

// RequestChanges succeeds without error unless WithError was configured.
func (m *MockProvider) RequestChanges(_ context.Context, _ string, _ string) error {
	return m.err
}

// PostComment succeeds without error unless WithError was configured.
func (m *MockProvider) PostComment(_ context.Context, prID string, line int, body string) error {
	m.PostCommentCalls = append(m.PostCommentCalls, PostCommentCall{PRID: prID, Line: line, Body: body})
	return m.err
}

// ReplyToComment implements provider.CommentReplier.
func (m *MockProvider) ReplyToComment(_ context.Context, prID string, inReplyTo string, body string) error {
	m.ReplyCalls = append(m.ReplyCalls, ReplyCall{PRID: prID, InReplyTo: inReplyTo, Body: body})
	return m.err
}

// GetUsername returns the mock username or delegates to GetUsernameFunc if set.
func (m *MockProvider) GetUsername(ctx context.Context) (string, error) {
	if m.GetUsernameFunc != nil {
		return m.GetUsernameFunc(ctx)
	}
	return "mock-user", nil
}

// ListUserRepos returns the configured repos.
func (m *MockProvider) ListUserRepos(_ context.Context) ([]*provider.Repo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.Repos, nil
}

// DiscoverInboxPRs implements provider.InboxDiscoveryProvider.
func (m *MockProvider) DiscoverInboxPRs(_ context.Context, opts provider.InboxDiscoveryOptions) (*provider.InboxDiscoveryResult, error) {
	m.InboxDiscoveryCalls = append(m.InboxDiscoveryCalls, opts)
	if m.InboxDiscoveryErr != nil {
		return nil, m.InboxDiscoveryErr
	}
	if m.InboxDiscoveryResult != nil {
		return m.InboxDiscoveryResult, nil
	}
	return &provider.InboxDiscoveryResult{HighWater: time.Now().UTC()}, nil
}

// PRs returns the configured PR list (useful in tests to grab a reference).
func (m *MockProvider) PRs() []*provider.PR {
	return m.prs
}

// SetPRs sets the mock's PR list in-place.
func (m *MockProvider) SetPRs(prs []*provider.PR) {
	m.prs = prs
}

// SetReviewThreads configures the mock's GetReviewThreads return value.
func (m *MockProvider) SetReviewThreads(threads []*provider.ReviewThread) {
	m.reviewThreads = threads
}

// SetReviewThreadsError configures GetReviewThreads to return an error.
func (m *MockProvider) SetReviewThreadsError(err error) {
	m.reviewThreadsErr = err
}

// GetReviewThreads implements provider.ThreadProvider.
func (m *MockProvider) GetReviewThreads(_ context.Context, _ string) ([]*provider.ReviewThread, error) {
	if m.reviewThreadsErr != nil {
		return nil, m.reviewThreadsErr
	}
	return m.reviewThreads, nil
}

// SetResolveError configures ResolveThread to return an error.
func (m *MockProvider) SetResolveError(err error) {
	m.resolveErr = err
}

// SetUnresolveError configures UnresolveThread to return an error.
func (m *MockProvider) SetUnresolveError(err error) {
	m.unresolveErr = err
}

// ResolveThread implements provider.ThreadResolver.
func (m *MockProvider) ResolveThread(_ context.Context, threadID string) error {
	m.ResolvedCalls = append(m.ResolvedCalls, threadID)
	return m.resolveErr
}

// UnresolveThread implements provider.ThreadResolver.
func (m *MockProvider) UnresolveThread(_ context.Context, threadID string) error {
	m.UnresolvedCalls = append(m.UnresolvedCalls, threadID)
	return m.unresolveErr
}

// Compile-time assertions.
var _ provider.ThreadProvider = (*MockProvider)(nil)
var _ provider.ThreadResolver = (*MockProvider)(nil)
var _ provider.CommentReplier = (*MockProvider)(nil)
var _ provider.InboxDiscoveryProvider = (*MockProvider)(nil)
