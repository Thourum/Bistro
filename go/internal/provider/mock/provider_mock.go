package mock

import (
	"context"
	"fmt"

	"github.com/samuelasvanyi/bistro/internal/provider"
)

// MockProvider implements Provider interface with canned data for testing.
type MockProvider struct {
	prs             []*provider.PR
	comments        []*provider.Comment
	ciStatus        *provider.CIStatus
	err             error
	GetUsernameFunc func(ctx context.Context) (string, error)
	// Repos is the list returned by ListUserRepos.
	Repos []*provider.Repo
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
	return &MockProvider{
		prs:             prs,
		comments:        m.comments,
		ciStatus:        m.ciStatus,
		err:             m.err,
		GetUsernameFunc: m.GetUsernameFunc,
		Repos:           m.Repos,
	}
}

// WithComments returns a new MockProvider with the given comments configured.
func (m *MockProvider) WithComments(comments []*provider.Comment) *MockProvider {
	return &MockProvider{
		prs:             m.prs,
		comments:        comments,
		ciStatus:        m.ciStatus,
		err:             m.err,
		GetUsernameFunc: m.GetUsernameFunc,
		Repos:           m.Repos,
	}
}

// WithCIStatus returns a new MockProvider with the given CI status configured.
func (m *MockProvider) WithCIStatus(status *provider.CIStatus) *MockProvider {
	return &MockProvider{
		prs:             m.prs,
		comments:        m.comments,
		ciStatus:        status,
		err:             m.err,
		GetUsernameFunc: m.GetUsernameFunc,
		Repos:           m.Repos,
	}
}

// WithError returns a new MockProvider that returns err from all methods.
func (m *MockProvider) WithError(err error) *MockProvider {
	return &MockProvider{
		prs:             m.prs,
		comments:        m.comments,
		ciStatus:        m.ciStatus,
		err:             err,
		GetUsernameFunc: m.GetUsernameFunc,
		Repos:           m.Repos,
	}
}

// WithRepos returns a new MockProvider with the given repos configured.
func (m *MockProvider) WithRepos(repos []*provider.Repo) *MockProvider {
	return &MockProvider{
		prs:             m.prs,
		comments:        m.comments,
		ciStatus:        m.ciStatus,
		err:             m.err,
		GetUsernameFunc: m.GetUsernameFunc,
		Repos:           repos,
	}
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
func (m *MockProvider) PostComment(_ context.Context, _ string, _ int, _ string) error {
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
