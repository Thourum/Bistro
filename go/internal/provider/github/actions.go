package github

import (
	"context"
	"fmt"

	ghlib "github.com/google/go-github/v63/github"
)

// approve approves a PR with an optional review body.
func (c *Client) approve(ctx context.Context, owner, repo string, number int, body string) error {
	if owner == "" || repo == "" || number <= 0 {
		return fmt.Errorf("invalid PR parameters: owner=%q, repo=%q, number=%d", owner, repo, number)
	}

	review := &ghlib.PullRequestReviewRequest{
		Body:  ghlib.String(body),
		Event: ghlib.String("APPROVE"),
	}

	_, _, err := c.gh.PullRequests.CreateReview(ctx, owner, repo, number, review)
	if err != nil {
		return fmt.Errorf("failed to approve PR %s/%s#%d: %w", owner, repo, number, err)
	}

	return nil
}

// requestChanges requests changes on a PR with a required review body.
func (c *Client) requestChanges(ctx context.Context, owner, repo string, number int, body string) error {
	if owner == "" || repo == "" || number <= 0 {
		return fmt.Errorf("invalid PR parameters: owner=%q, repo=%q, number=%d", owner, repo, number)
	}

	review := &ghlib.PullRequestReviewRequest{
		Body:  ghlib.String(body),
		Event: ghlib.String("REQUEST_CHANGES"),
	}

	_, _, err := c.gh.PullRequests.CreateReview(ctx, owner, repo, number, review)
	if err != nil {
		return fmt.Errorf("failed to request changes on PR %s/%s#%d: %w", owner, repo, number, err)
	}

	return nil
}

// postComment posts a general issue comment on a PR.
// If line > 0, it is recorded for future use as a review comment (requires commit SHA; not yet implemented).
func (c *Client) postComment(ctx context.Context, owner, repo string, number, line int, body string) error {
	if owner == "" || repo == "" || number <= 0 {
		return fmt.Errorf("invalid PR parameters: owner=%q, repo=%q, number=%d", owner, repo, number)
	}

	if line > 0 {
		return fmt.Errorf("line-level review comments not yet supported")
	}

	if body == "" {
		return fmt.Errorf("comment body cannot be empty")
	}

	comment := &ghlib.IssueComment{
		Body: ghlib.String(body),
	}

	_, _, err := c.gh.Issues.CreateComment(ctx, owner, repo, number, comment)
	if err != nil {
		return fmt.Errorf("failed to post comment on PR %s/%s#%d: %w", owner, repo, number, err)
	}

	return nil
}
