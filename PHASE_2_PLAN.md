# Phase 2: Provider Interface & GitHub Adapter — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement a Provider interface abstraction and a fully functional GitHub adapter that can fetch PR data, comments, and CI status from GitHub API.

**Architecture:** The Provider interface defines methods that any Git service (GitHub, GitLab, Gitea) can implement. The GitHub adapter implements this interface by translating domain types to GitHub API calls, handling OAuth device flow for authentication, and normalizing GitHub responses to domain types.

**Tech Stack:** Go 1.21+, go-octokit (or google/go-github for GitHub API), go-github-oauth for device flow auth.

---

## File Structure

```
go/internal/
  ├── provider/
  │   ├── interface.go          ← Define Provider interface + domain types
  │   ├── models.go             ← Normalized PR, Comment, CIStatus types
  │   ├── github/
  │   │   ├── client.go         ← GitHub API client wrapper
  │   │   ├── auth.go           ← OAuth device flow + token refresh
  │   │   ├── prs.go            ← ListPRs, GetPR implementations
  │   │   ├── comments.go       ← GetComments, normalize
  │   │   ├── ci.go             ← GetCIStatus (check runs)
  │   │   ├── actions.go        ← Approve, RequestChanges, PostComment
  │   │   └── github_test.go    ← Integration tests
  │   └── mock/
  │       └── provider_mock.go   ← Mock provider for testing
```

---

## Phase 2 Tasks

### Task 1: Define Provider Interface & Domain Types

**Files:**
- Create: `go/internal/provider/interface.go`
- Create: `go/internal/provider/models.go`
- Create: `go/internal/provider/interface_test.go`

**Steps:**

- [ ] **Step 1: Define domain types**

```go
// provider/models.go
package provider

import "time"

// PR represents a normalized pull request
type PR struct {
	ID           string    // "github:owner/repo:123" format
	Provider     string    // "github", "gitlab", "gitea"
	Owner        string    // org name
	Repo         string    // repo name
	Number       int       // PR number
	Title        string
	Description  string
	Author       string
	State        string    // "open", "closed", "merged"
	ReviewStatus string    // "pending", "approved", "changes_requested"
	BaseBranch   string
	HeadBranch   string
	BaseSHA      string
	HeadSHA      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	MergedAt     *time.Time
	CommentCount int
	CIStatus     string    // "pending", "success", "failure"
	Draft        bool
}

// Comment represents a normalized comment
type Comment struct {
	ID        string
	PRID      string
	Author    string
	Body      string
	LineNo    int    // 0 if general comment, >0 for specific line
	FilePath  string // Only if line comment
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CIStatus represents CI/check run status
type CIStatus struct {
	Status   string // "pending", "success", "failure"
	Checks   []*Check
	UpdatedAt time.Time
}

// Check represents a single check run
type Check struct {
	Name      string // e.g., "Tests", "Lint"
	Status    string // "pending", "success", "failure"
	URL       string
	UpdatedAt time.Time
}
```

- [ ] **Step 2: Define Provider interface**

```go
// provider/interface.go
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

	// GetCIStatus fetches CI check runs for a PR
	GetCIStatus(ctx context.Context, prID string) (*CIStatus, error)

	// Approve approves a PR
	Approve(ctx context.Context, prID string) error

	// RequestChanges requests changes on a PR with comment
	RequestChanges(ctx context.Context, prID string, body string) error

	// PostComment posts a comment to a PR
	PostComment(ctx context.Context, prID string, line int, body string) error
}

// PRFilters defines optional PR list filters
type PRFilters struct {
	State  string // "open", "closed", "merged"
	Limit  int    // max results per page
	Offset int    // pagination offset
}
```

- [ ] **Step 3: Write tests (TDD)**

```go
// provider/interface_test.go
package provider

import "testing"

func TestPRValidation(t *testing.T) {
	pr := &PR{
		ID:       "github:org/repo:123",
		Provider: "github",
		Owner:    "org",
		Repo:     "repo",
		Number:   123,
		Title:    "Fix bug",
		Author:   "alice",
	}

	if pr.ID == "" {
		t.Error("PR ID should not be empty")
	}
	if pr.Provider != "github" {
		t.Error("expected provider github")
	}
}

func TestCommentValidation(t *testing.T) {
	comment := &Comment{
		ID:     "comment-1",
		PRID:   "github:org/repo:123",
		Author: "bob",
		Body:   "Great fix!",
	}

	if comment.ID == "" {
		t.Error("Comment ID should not be empty")
	}
}
```

- [ ] **Step 4: Run tests (should pass)**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/provider -v
```

---

### Task 2: GitHub OAuth Device Flow

**Files:**
- Create: `go/internal/provider/github/auth.go`
- Create: `go/internal/provider/github/auth_test.go`

**Steps:**

- [ ] **Step 1: Add github.com/google/go-github dependency**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go get github.com/google/go-github/v63/github
```

- [ ] **Step 2: Write failing test**

```go
// provider/github/auth_test.go
package github

import "testing"

func TestDeviceFlow(t *testing.T) {
	// Just verify function exists and doesn't crash
	_, err := NewAuthenticator("test-client-id")
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}
}
```

- [ ] **Step 3: Implement OAuth device flow**

```go
// provider/github/auth.go
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
	return &Authenticator{
		clientID: clientID,
		client:   github.NewClient(nil),
	}, nil
}

// GetDeviceCode starts the device flow
func (a *Authenticator) GetDeviceCode(ctx context.Context) (code, userCode, verificationURL string, err error) {
	// Placeholder: actual implementation calls GitHub device endpoint
	// Returns: device_code, user_code, verification_url
	// For now, just verify structure works
	return "device-code", "XXXX-XXXX", "https://github.com/login/device", nil
}

// PollForToken polls for access token
func (a *Authenticator) PollForToken(ctx context.Context, deviceCode string, pollInterval int) (token string, err error) {
	// Placeholder: actual implementation polls GitHub until user approves
	// For now, just verify structure works
	return "gho_token_placeholder", nil
}
```

- [ ] **Step 4: Run tests (should pass)**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/provider/github -v
```

---

### Task 3: GitHub ListPRs Implementation

**Files:**
- Create: `go/internal/provider/github/prs.go`
- Create: `go/internal/provider/github/prs_test.go`

**Steps:**

- [ ] **Step 1: Write failing test**

```go
// provider/github/prs_test.go
package github

import "testing"

func TestListPRs(t *testing.T) {
	client := NewClient("test-token")
	_, err := client.ListPRs(nil, "owner", "repo", "open")
	if err == nil {
		t.Error("expected error for uninitialized context")
	}
}
```

- [ ] **Step 2: Implement ListPRs**

```go
// provider/github/prs.go
package github

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/go-github/v63/github"
	"github.com/samuelasvanyi/bistro/internal/provider"
)

// Client wraps GitHub API client
type Client struct {
	token string
	gh    *github.Client
}

// NewClient creates a new GitHub client
func NewClient(token string) *Client {
	return &Client{
		token: token,
		gh:    github.NewClient(nil), // Will add auth transport
	}
}

// ListPRs fetches list of PRs
func (c *Client) ListPRs(ctx context.Context, owner, repo, state string) ([]*provider.PR, error) {
	// Fetch PRs from GitHub API
	opts := &github.PullRequestListOptions{
		State: state,
		ListOptions: github.ListOptions{
			PerPage: 30,
		},
	}

	prs, _, err := c.gh.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list PRs: %w", err)
	}

	// Normalize to domain types
	result := make([]*provider.PR, 0, len(prs))
	for _, ghPR := range prs {
		pr := &provider.PR{
			ID:         fmt.Sprintf("github:%s/%s:%d", owner, repo, ghPR.GetNumber()),
			Provider:   "github",
			Owner:      owner,
			Repo:       repo,
			Number:     ghPR.GetNumber(),
			Title:      ghPR.GetTitle(),
			Description: ghPR.GetBody(),
			Author:     ghPR.User.GetLogin(),
			State:      ghPR.GetState(),
			BaseBranch: ghPR.Base.GetRef(),
			HeadBranch: ghPR.Head.GetRef(),
			BaseSHA:    ghPR.Base.GetSHA(),
			HeadSHA:    ghPR.Head.GetSHA(),
			CreatedAt:  ghPR.CreatedAt.Time,
			UpdatedAt:  ghPR.UpdatedAt.Time,
			Draft:      ghPR.GetDraft(),
		}

		if ghPR.MergedAt != nil {
			pr.MergedAt = &ghPR.MergedAt.Time
		}

		result = append(result, pr)
	}

	return result, nil
}

// GetPR fetches a single PR
func (c *Client) GetPR(ctx context.Context, owner, repo string, number int) (*provider.PR, error) {
	ghPR, _, err := c.gh.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR: %w", err)
	}

	pr := &provider.PR{
		ID:          fmt.Sprintf("github:%s/%s:%d", owner, repo, ghPR.GetNumber()),
		Provider:    "github",
		Owner:       owner,
		Repo:        repo,
		Number:      ghPR.GetNumber(),
		Title:       ghPR.GetTitle(),
		Description: ghPR.GetBody(),
		Author:      ghPR.User.GetLogin(),
		State:       ghPR.GetState(),
		BaseBranch:  ghPR.Base.GetRef(),
		HeadBranch:  ghPR.Head.GetRef(),
		BaseSHA:     ghPR.Base.GetSHA(),
		HeadSHA:     ghPR.Head.GetSHA(),
		CreatedAt:   ghPR.CreatedAt.Time,
		UpdatedAt:   ghPR.UpdatedAt.Time,
		Draft:       ghPR.GetDraft(),
	}

	if ghPR.MergedAt != nil {
		pr.MergedAt = &ghPR.MergedAt.Time
	}

	return pr, nil
}
```

- [ ] **Step 3: Run tests (should pass)**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/provider/github -v
```

---

### Task 4: GitHub Comments & CI Status

**Files:**
- Create: `go/internal/provider/github/comments.go`
- Create: `go/internal/provider/github/ci.go`
- Create: `go/internal/provider/github/comments_test.go`
- Create: `go/internal/provider/github/ci_test.go`

**Steps:** (Similar TDD pattern as Task 3)

- [ ] Implement GetComments (fetches PR comments from GitHub API, normalizes to domain types)
- [ ] Implement GetCIStatus (fetches check runs, normalizes to domain types)
- [ ] Write tests for both
- [ ] Verify build and tests pass

---

### Task 5: GitHub Actions (Approve, Request Changes, Post Comment)

**Files:**
- Create: `go/internal/provider/github/actions.go`
- Create: `go/internal/provider/github/actions_test.go`

**Steps:** (Similar TDD pattern)

- [ ] Implement Approve (POST /repos/{owner}/{repo}/pulls/{number}/reviews)
- [ ] Implement RequestChanges (POST with CHANGES_REQUESTED event)
- [ ] Implement PostComment (POST /repos/{owner}/{repo}/issues/{number}/comments)
- [ ] Write tests
- [ ] Verify build and tests pass

---

### Task 6: Integrate Provider Interface with GitHub Client

**Files:**
- Modify: `go/internal/provider/github/client.go` (from Task 3)

**Steps:**

- [ ] **Step 1: Make Client implement Provider interface**

```go
// Verify that Client implements all Provider methods
var _ provider.Provider = (*Client)(nil)
```

- [ ] **Step 2: Test full Provider interface**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/provider/... -v
```

- [ ] **Step 3: Build verification**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go build ./...
```

---

### Task 7: Mock Provider for Testing (No GitHub API calls)

**Files:**
- Create: `go/internal/provider/mock/provider_mock.go`
- Create: `go/internal/provider/mock/provider_mock_test.go`

**Steps:**

- [ ] **Step 1: Implement mock provider**

```go
// provider/mock/provider_mock.go
package mock

import (
	"context"

	"github.com/samuelasvanyi/bistro/internal/provider"
)

// MockProvider implements Provider interface with canned data
type MockProvider struct {
	PRs      []*provider.PR
	Comments []*provider.Comment
	CIStatus *provider.CIStatus
}

func (m *MockProvider) Authenticate(ctx context.Context) (string, error) {
	return "mock-token", nil
}

func (m *MockProvider) ListPRs(ctx context.Context, filters provider.PRFilters) ([]*provider.PR, error) {
	return m.PRs, nil
}

func (m *MockProvider) GetPR(ctx context.Context, id string) (*provider.PR, error) {
	if len(m.PRs) > 0 {
		return m.PRs[0], nil
	}
	return nil, nil
}

func (m *MockProvider) GetComments(ctx context.Context, prID string) ([]*provider.Comment, error) {
	return m.Comments, nil
}

func (m *MockProvider) GetCIStatus(ctx context.Context, prID string) (*provider.CIStatus, error) {
	return m.CIStatus, nil
}

func (m *MockProvider) Approve(ctx context.Context, prID string) error {
	return nil
}

func (m *MockProvider) RequestChanges(ctx context.Context, prID string, body string) error {
	return nil
}

func (m *MockProvider) PostComment(ctx context.Context, prID string, line int, body string) error {
	return nil
}
```

- [ ] **Step 2: Write tests using mock**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go test ./internal/provider/mock -v
```

---

## Phase 2 Verification Checklist

- [ ] Provider interface defined with all 8 methods
- [ ] Domain types (PR, Comment, CIStatus) normalized correctly
- [ ] GitHub OAuth device flow scaffold in place
- [ ] ListPRs fetches and normalizes GitHub PRs
- [ ] GetPR fetches single PR with full details
- [ ] GetComments fetches PR comments normalized
- [ ] GetCIStatus fetches check runs normalized
- [ ] Approve/RequestChanges/PostComment implemented
- [ ] GitHub Client implements Provider interface
- [ ] Mock provider ready for unit testing
- [ ] All tests pass: `go test ./internal/provider/... -v`
- [ ] Build succeeds: `go build ./...`
- [ ] No new lint warnings

---

## Notes

- TDD discipline: RED → GREEN → REFACTOR → COMMIT for each task
- Tests use mock provider where GitHub API not needed
- Real GitHub API tests deferred to Phase 3 (integration tests with test repo)
- Error handling: All functions return error, explicit propagation
- No panics, no silent failures
- Next phase (Phase 3): Git operations & diff generation

