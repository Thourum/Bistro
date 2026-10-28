package github

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	ghlib "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/provider"
)

// approve approves a PR with an optional review body.
func (c *Client) approve(ctx context.Context, owner, repo string, number int, body string) error {
	if owner == "" || repo == "" || number <= 0 {
		return fmt.Errorf("invalid PR parameters: owner=%q, repo=%q, number=%d", owner, repo, number)
	}

	review := &ghlib.PullRequestReviewRequest{
		Body:  ghlib.Ptr(body),
		Event: ghlib.Ptr("APPROVE"),
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
		Body:  ghlib.Ptr(body),
		Event: ghlib.Ptr("REQUEST_CHANGES"),
	}

	_, _, err := c.gh.PullRequests.CreateReview(ctx, owner, repo, number, review)
	if err != nil {
		return fmt.Errorf("failed to request changes on PR %s/%s#%d: %w", owner, repo, number, err)
	}

	return nil
}

// reviewEvents are the CreateReview events submit_review may send.
var reviewEvents = map[string]bool{"APPROVE": true, "REQUEST_CHANGES": true, "COMMENT": true}

// submitReview creates one review carrying every inline comment.
func (c *Client) submitReview(ctx context.Context, owner, repo string, number int, sub provider.ReviewSubmission) error {
	if !reviewEvents[sub.Event] {
		return fmt.Errorf("invalid review event %q", sub.Event)
	}
	review := &ghlib.PullRequestReviewRequest{Event: ghlib.Ptr(sub.Event)}
	if sub.Body != "" {
		review.Body = ghlib.Ptr(sub.Body)
	}
	if sub.CommitID != "" {
		review.CommitID = ghlib.Ptr(sub.CommitID)
	}
	for _, comment := range sub.Comments {
		draft := &ghlib.DraftReviewComment{
			Path: ghlib.Ptr(comment.Path), Body: ghlib.Ptr(comment.Body),
			Side: ghlib.Ptr(comment.Side), Line: ghlib.Ptr(comment.Line),
		}
		if comment.StartLine > 0 {
			draft.StartLine = ghlib.Ptr(comment.StartLine)
			draft.StartSide = ghlib.Ptr(comment.Side)
		}
		review.Comments = append(review.Comments, draft)
	}
	if _, _, err := c.gh.PullRequests.CreateReview(ctx, owner, repo, number, review); err != nil {
		return fmt.Errorf("failed to submit review on PR %s/%s#%d: %w", owner, repo, number, err)
	}
	return nil
}

// SubmitReview implements provider.ReviewSubmitter, parsing the "github:owner/repo:number" ID format.
func (c *Client) SubmitReview(ctx context.Context, prID string, sub provider.ReviewSubmission) error {
	owner, repo, number, err := parsePRID(prID)
	if err != nil {
		return err
	}
	return c.submitReview(ctx, owner, repo, number, sub)
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
		Body: ghlib.Ptr(body),
	}

	_, _, err := c.gh.Issues.CreateComment(ctx, owner, repo, number, comment)
	if err != nil {
		return fmt.Errorf("failed to post comment on PR %s/%s#%d: %w", owner, repo, number, err)
	}

	return nil
}

// replyToComment replies to an existing pull request review comment.
func (c *Client) replyToComment(ctx context.Context, owner, repo string, number int, inReplyTo string, body string) error {
	if owner == "" || repo == "" || number <= 0 {
		return fmt.Errorf("invalid PR parameters: owner=%q, repo=%q, number=%d", owner, repo, number)
	}
	if body == "" {
		return fmt.Errorf("reply body cannot be empty")
	}

	commentID := parseReviewCommentReplyID(inReplyTo)
	if commentID <= 0 {
		return fmt.Errorf("in_reply_to must be a review comment ID")
	}

	_, _, err := c.gh.PullRequests.CreateCommentInReplyTo(ctx, owner, repo, number, body, commentID)
	if err != nil {
		return fmt.Errorf("failed to reply to review comment %d on PR %s/%s#%d: %w", commentID, owner, repo, number, err)
	}

	return nil
}

// parseReviewCommentReplyID accepts the Bistro domain ID shape used in cached
// comments and the raw numeric GitHub review-comment database ID.
func parseReviewCommentReplyID(inReplyTo string) int64 {
	inReplyTo = strings.TrimSpace(inReplyTo)
	if inReplyTo == "" {
		return 0
	}
	if id := restDBIDFromDomainID(inReplyTo); id > 0 {
		return id
	}
	id, err := strconv.ParseInt(inReplyTo, 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
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

// ReplyToComment implements provider.CommentReplier, parsing the
// "github:owner/repo:number" ID format and replying to an inline review comment.
func (c *Client) ReplyToComment(ctx context.Context, prID string, inReplyTo string, body string) error {
	owner, repo, number, err := parsePRID(prID)
	if err != nil {
		return err
	}

	return c.replyToComment(ctx, owner, repo, number, inReplyTo, body)
}

// listReviews fetches all review submissions for a PR, normalized to domain types.
func (c *Client) listReviews(ctx context.Context, owner, repo string, number int) ([]*provider.Review, error) {
	var result []*provider.Review
	opts := &ghlib.ListOptions{PerPage: 100}

	prID := formatPRID(owner, repo, number)
	for {
		ghReviews, resp, err := c.gh.PullRequests.ListReviews(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list reviews for %s/%s#%d: %w", owner, repo, number, err)
		}

		for _, ghReview := range ghReviews {
			result = append(result, &provider.Review{
				PRID:        prID,
				Author:      ghReview.GetUser().GetLogin(),
				State:       strings.ToLower(ghReview.GetState()),
				CommitID:    ghReview.GetCommitID(),
				SubmittedAt: ghReview.GetSubmittedAt().Time,
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return result, nil
}

// ListReviews implements provider.Provider, parsing the "github:owner/repo:number" ID format.
func (c *Client) ListReviews(ctx context.Context, prID string) ([]*provider.Review, error) {
	owner, repo, number, err := parsePRID(prID)
	if err != nil {
		return nil, err
	}

	return c.listReviews(ctx, owner, repo, number)
}
