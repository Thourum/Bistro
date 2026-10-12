package github

import (
	"context"
	"fmt"
	"time"

	ghlib "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/provider"
)

// listPRs fetches the list of PRs for a repository and normalizes them to
// domain types. When updatedSince is non-nil the list is fetched most recently
// updated first and pagination stops once results fall out of the window —
// keeping at least minCount PRs so quiet repos are never empty. A nil
// updatedSince fetches every page (legacy behavior).
func (c *Client) listPRs(ctx context.Context, owner, repo, state string, updatedSince *time.Time, minCount int) ([]*provider.PR, error) {
	var result []*provider.PR
	opts := &ghlib.PullRequestListOptions{
		State: state,
		ListOptions: ghlib.ListOptions{
			PerPage: 100,
		},
	}
	if updatedSince != nil {
		opts.Sort = "updated"
		opts.Direction = "desc"
	}

	for {
		ghPRs, resp, err := c.gh.PullRequests.List(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list PRs for %s/%s: %w", owner, repo, err)
		}

		pastWindow := false
		for _, ghPR := range ghPRs {
			pr := normalizePR(owner, repo, ghPR)
			if updatedSince != nil && pr.UpdatedAt.Before(*updatedSince) {
				if len(result) >= minCount {
					pastWindow = true
					break
				}
			}
			result = append(result, pr)
		}
		if pastWindow {
			break
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
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
		ID:          formatPRID(owner, repo, ghPR.GetNumber()),
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

	for _, reviewer := range ghPR.RequestedReviewers {
		if login := reviewer.GetLogin(); login != "" {
			pr.RequestedReviewers = append(pr.RequestedReviewers, login)
		}
	}

	return pr
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

	prs, err := c.listPRs(ctx, filters.Owner, filters.Repo, state, filters.UpdatedSince, filters.MinCount)
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
