package github

import (
	"context"
	"fmt"
	"strings"

	gh "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/provider"
)

// ListUserRepos fetches all repos accessible by the authenticated user, paginating
// until all results are retrieved.
func (c *Client) ListUserRepos(ctx context.Context) ([]*provider.Repo, error) {
	var allRepos []*provider.Repo
	opts := &gh.RepositoryListByAuthenticatedUserOptions{
		Sort:        "updated",
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	for {
		repos, resp, err := c.gh.Repositories.ListByAuthenticatedUser(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("list repos: %w", err)
		}

		for _, r := range repos {
			allRepos = append(allRepos, &provider.Repo{
				Owner:    strings.ToLower(r.GetOwner().GetLogin()),
				Name:     strings.ToLower(r.GetName()),
				FullName: strings.ToLower(r.GetFullName()),
				Private:  r.GetPrivate(),
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allRepos, nil
}
