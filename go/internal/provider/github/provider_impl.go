package github

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/samuelasvanyi/bistro/internal/provider"
)

// parsePRID parses a PR ID in the format "github:owner/repo:number".
// Returns owner, repo, and PR number, or an error if the format is invalid.
func parsePRID(id string) (owner, repo string, number int, err error) {
	// Expected format: "github:owner/repo:number"
	parts := strings.SplitN(id, ":", 3)
	if len(parts) != 3 || parts[0] != "github" {
		return "", "", 0, fmt.Errorf("invalid PR ID format %q: expected github:owner/repo:number", id)
	}

	repoPath := parts[1]
	repoParts := strings.SplitN(repoPath, "/", 2)
	if len(repoParts) != 2 || repoParts[0] == "" || repoParts[1] == "" {
		return "", "", 0, fmt.Errorf("invalid PR ID format %q: owner/repo part must be non-empty", id)
	}

	number, err = strconv.Atoi(parts[2])
	if err != nil || number <= 0 {
		return "", "", 0, fmt.Errorf("invalid PR ID format %q: number must be a positive integer", id)
	}

	return repoParts[0], repoParts[1], number, nil
}

// Authenticate performs the GitHub OAuth device flow.
// The clientID must be set via environment configuration in Phase 3.
func (c *Client) Authenticate(ctx context.Context) (string, error) {
	if c.token != "" {
		return c.token, nil
	}

	return "", fmt.Errorf("Authenticate via device flow not yet implemented: set token directly in Phase 3")
}

// ListPRs implements provider.Provider by mapping PRFilters to GitHub-specific params.
// filters.Owner and filters.Repo are required. filters.State defaults to "open" if empty.
func (c *Client) ListPRs(ctx context.Context, filters provider.PRFilters) ([]*provider.PR, error) {
	if filters.Owner == "" || filters.Repo == "" {
		return nil, fmt.Errorf("owner and repo required in filters")
	}

	state := filters.State
	if state == "" {
		state = "open"
	}

	prs, err := c.listPRs(ctx, filters.Owner, filters.Repo, state)
	if err != nil {
		return nil, err
	}

	if filters.Limit > 0 && len(prs) > filters.Limit {
		prs = prs[:filters.Limit]
	}

	return prs, nil
}

// GetPR implements provider.Provider, parsing the "github:owner/repo:number" ID format.
func (c *Client) GetPR(ctx context.Context, id string) (*provider.PR, error) {
	owner, repo, number, err := parsePRID(id)
	if err != nil {
		return nil, err
	}

	return c.getPR(ctx, owner, repo, number)
}

// GetComments implements provider.Provider, parsing the "github:owner/repo:number" ID format.
func (c *Client) GetComments(ctx context.Context, prID string) ([]*provider.Comment, error) {
	owner, repo, number, err := parsePRID(prID)
	if err != nil {
		return nil, err
	}

	return c.getComments(ctx, owner, repo, number)
}

// GetCIStatus implements provider.Provider, parsing the "github:owner/repo:number" ID format.
// Fetches the PR to resolve the head SHA, then retrieves check runs.
func (c *Client) GetCIStatus(ctx context.Context, prID string) (*provider.CIStatus, error) {
	owner, repo, number, err := parsePRID(prID)
	if err != nil {
		return nil, err
	}

	pr, err := c.getPR(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("GetCIStatus: failed to fetch PR to resolve head SHA: %w", err)
	}

	return c.getCIStatus(ctx, owner, repo, pr.HeadSHA)
}

// Approve implements provider.Provider, parsing the "github:owner/repo:number" ID format.
func (c *Client) Approve(ctx context.Context, prID string) error {
	owner, repo, number, err := parsePRID(prID)
	if err != nil {
		return err
	}

	return c.approve(ctx, owner, repo, number, "")
}

// RequestChanges implements provider.Provider, parsing the "github:owner/repo:number" ID format.
func (c *Client) RequestChanges(ctx context.Context, prID string, body string) error {
	owner, repo, number, err := parsePRID(prID)
	if err != nil {
		return err
	}

	return c.requestChanges(ctx, owner, repo, number, body)
}

// PostComment implements provider.Provider, parsing the "github:owner/repo:number" ID format.
func (c *Client) PostComment(ctx context.Context, prID string, line int, body string) error {
	owner, repo, number, err := parsePRID(prID)
	if err != nil {
		return err
	}

	return c.postComment(ctx, owner, repo, number, line, body)
}
