package github

import (
	"context"
	"fmt"
	"strconv"

	ghlib "github.com/google/go-github/v63/github"
	"github.com/samuelasvanyi/bistro/internal/provider"
)

// getComments fetches all comments for a PR.
func (c *Client) getComments(ctx context.Context, owner, repo string, number int) ([]*provider.Comment, error) {
	opts := &ghlib.IssueListCommentsOptions{
		ListOptions: ghlib.ListOptions{PerPage: 100},
	}

	ghComments, _, err := c.gh.Issues.ListComments(ctx, owner, repo, number, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get comments for %s/%s#%d: %w", owner, repo, number, err)
	}

	prID := "github:" + owner + "/" + repo + ":" + strconv.Itoa(number)
	result := make([]*provider.Comment, 0, len(ghComments))
	for _, ghComment := range ghComments {
		result = append(result, normalizeComment(prID, ghComment))
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
