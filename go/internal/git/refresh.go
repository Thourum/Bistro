package git

import (
	"context"
	"errors"
	"fmt"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

// broadRefSpecs mirrors the broad clone's fetch for callers without PR hints.
func broadRefSpecs() []config.RefSpec {
	return []config.RefSpec{
		"+refs/heads/*:refs/remotes/origin/*",
		"+refs/pull/*/head:refs/remotes/origin/pr/*",
	}
}

// fetchIntoRepo fetches the PR's refs from origin into an existing clone.
// depth 0 fetches without a depth limit, so a full clone stays full.
func (c *Client) fetchIntoRepo(ctx context.Context, repoDir string, opts *CloneOptions, depth int) error {
	repo, err := gitlib.PlainOpen(repoDir)
	if err != nil {
		return fmt.Errorf("open repository %s: %w", repoDir, err)
	}
	refSpecs := targetedRefSpecs(opts)
	if len(refSpecs) == 0 {
		refSpecs = broadRefSpecs()
	}
	fetchOpts := &gitlib.FetchOptions{Auth: c.auth(), RefSpecs: refSpecs, Depth: depth, Tags: gitlib.NoTags}
	if err := repo.FetchContext(ctx, fetchOpts); err != nil && !errors.Is(err, gitlib.NoErrAlreadyUpToDate) {
		return fmt.Errorf("fetch into %s: %w", repoDir, err)
	}
	return nil
}
