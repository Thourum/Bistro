package github

import (
	"context"
	"fmt"
	"strconv"

	ghlib "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/provider"
)

// getComments fetches all comments for a PR.
func (c *Client) getComments(ctx context.Context, owner, repo string, number int) ([]*provider.Comment, error) {
	opts := &ghlib.IssueListCommentsOptions{
		ListOptions: ghlib.ListOptions{PerPage: 100},
	}

	prID := formatPRID(owner, repo, number)
	var result []*provider.Comment

	for {
		ghComments, resp, err := c.gh.Issues.ListComments(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to get comments for %s/%s#%d: %w", owner, repo, number, err)
		}
		for _, ghComment := range ghComments {
			result = append(result, normalizeComment(prID, ghComment))
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// normalizeComment converts a GitHub IssueComment to the domain Comment type.
func normalizeComment(prID string, ghComment *ghlib.IssueComment) *provider.Comment {
	return &provider.Comment{
		ID:        "github-comment-" + strconv.FormatInt(ghComment.GetID(), 10),
		PRID:      prID,
		Author:    ghComment.GetUser().GetLogin(),
		Body:      ghComment.GetBody(),
		CreatedAt: ghComment.GetCreatedAt().Time,
		UpdatedAt: ghComment.GetUpdatedAt().Time,
	}
}

// GetComments implements provider.Provider, parsing the "github:owner/repo:number" ID format.
func (c *Client) GetComments(ctx context.Context, prID string) ([]*provider.Comment, error) {
	owner, repo, number, err := parsePRID(prID)
	if err != nil {
		return nil, err
	}

	return c.getComments(ctx, owner, repo, number)
}
