package git

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

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

// refreshOnDiskClone fetches into the repo's existing cache clone instead of
// replacing it, so pinned PR heads survive a force-push. It reports false when
// there is no reusable clone or a required commit is still missing.
func (c *Client) refreshOnDiskClone(
	ctx context.Context,
	log *slog.Logger,
	repoURL, baseSHA, headSHA string,
	opts *CloneOptions,
) (*CloneResult, bool) {
	if needsFreshTempDir(repoURL) {
		return nil, false
	}
	repoDir, err := repoCacheDir(repoURL)
	if err != nil || !localRepoExists(repoDir) {
		return nil, false
	}
	if err := c.fetchIntoRepo(ctx, repoDir, opts, 0); err != nil {
		log.Debug("cache refresh fetch failed; re-cloning", "local_path", repoDir, "error", err)
		return nil, false
	}
	missing, err := firstMissingCommit(repoDir, requiredSHAs(baseSHA, headSHA, opts))
	if err != nil || missing != "" {
		log.Debug("cache refresh left a required commit missing; re-cloning",
			"missing_sha", truncateSHAForLog(missing), "error", err)
		return nil, false
	}
	c.cache.Set(repoURL, baseSHA, headSHA, &RepoEntry{
		URL: repoURL, LocalPath: repoDir, BaseSHA: baseSHA, HeadSHA: headSHA, FetchedAt: time.Now(),
	})
	c.recordCacheCloned(repoURL, repoDir)
	return &CloneResult{LocalPath: repoDir, BaseSHA: baseSHA, HeadSHA: headSHA}, true
}
