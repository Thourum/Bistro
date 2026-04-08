package github

import (
	"context"
	"fmt"
	"strconv"

	ghlib "github.com/google/go-github/v63/github"
	"github.com/samuelasvanyi/bistro/internal/provider"
)

// listPRs fetches the list of PRs for a repository and normalizes them to domain types.
func (c *Client) listPRs(ctx context.Context, owner, repo, state string) ([]*provider.PR, error) {
	opts := &ghlib.PullRequestListOptions{
		State: state,
		ListOptions: ghlib.ListOptions{
			PerPage: 30,
		},
	}

	ghPRs, _, err := c.gh.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list PRs for %s/%s: %w", owner, repo, err)
	}

	result := make([]*provider.PR, 0, len(ghPRs))
	for _, ghPR := range ghPRs {
		result = append(result, normalizePR(owner, repo, ghPR))
	}

	return result, nil
}

// getPR fetches a single PR by number and normalizes it to a domain type.
func (c *Client) getPR(ctx context.Context, owner, repo string, number int) (*provider.PR, error) {
	ghPR, _, err := c.gh.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR %s/%s#%d: %w", owner, repo, number, err)
	}

	return normalizePR(owner, repo, ghPR), nil
}

// normalizePR converts a GitHub PullRequest to the domain PR type.
// ID format: "github:owner/repo:number"
func normalizePR(owner, repo string, ghPR *ghlib.PullRequest) *provider.PR {
	pr := &provider.PR{
		ID:          "github:" + owner + "/" + repo + ":" + strconv.Itoa(ghPR.GetNumber()),
		Provider:    "github",
		Owner:       owner,
		Repo:        repo,
		Number:      ghPR.GetNumber(),
		Title:       ghPR.GetTitle(),
		Description: ghPR.GetBody(),
		Author:      ghPR.GetUser().GetLogin(),
		State:       ghPR.GetState(),
		BaseBranch:  ghPR.GetBase().GetRef(),
		HeadBranch:  ghPR.GetHead().GetRef(),
		BaseSHA:     ghPR.GetBase().GetSHA(),
		HeadSHA:     ghPR.GetHead().GetSHA(),
		CreatedAt:   ghPR.GetCreatedAt().Time,
		UpdatedAt:   ghPR.GetUpdatedAt().Time,
		Draft:       ghPR.GetDraft(),
	}

	if ghPR.MergedAt != nil {
		mergedAt := ghPR.MergedAt.Time
		pr.MergedAt = &mergedAt
	}

	return pr
}
