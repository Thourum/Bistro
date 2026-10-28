package git

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/itterno/bistro/internal/git/clonecache"
	"github.com/itterno/bistro/internal/logging"
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
	if clonecache.NeedsFreshTempDir(repoURL) {
		return nil, false
	}
	repoDir, err := clonecache.RepoDir(repoURL)
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
			"missing_sha", logging.ShortSHA(missing), "error", err)
		return nil, false
	}
	c.cache.Set(repoURL, baseSHA, headSHA, &clonecache.RepoEntry{
		URL: repoURL, LocalPath: repoDir, BaseSHA: baseSHA, HeadSHA: headSHA, FetchedAt: time.Now(),
	})
	c.recordCacheCloned(repoURL, repoDir)
	return &CloneResult{LocalPath: repoDir, BaseSHA: baseSHA, HeadSHA: headSHA}, true
}

// localRepoExists reports whether path is a non-empty string naming an existing
// filesystem entry. Used to validate a cache hit before trusting its clone path.
func localRepoExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// FetchRefs updates an existing local repository by fetching all branch and PR
// refs, then verifies both baseSHA and headSHA are locally available.
func (c *Client) FetchRefs(ctx context.Context, localPath, baseSHA, headSHA string) error {
	if localPath == "" {
		return errors.New("local path cannot be empty")
	}
	if baseSHA == "" {
		return errors.New("baseSHA cannot be empty")
	}
	if headSHA == "" {
		return errors.New("headSHA cannot be empty")
	}

	repo, err := gitlib.PlainOpen(localPath)
	if err != nil {
		return fmt.Errorf("failed to open repository at %s: %w", localPath, err)
	}

	fetchOpts := &gitlib.FetchOptions{
		Auth: c.auth(),
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
			config.RefSpec("+refs/pull/*/head:refs/remotes/origin/pr/*"),
		},
	}

	if err := repo.FetchContext(ctx, fetchOpts); err != nil && !errors.Is(err, gitlib.NoErrAlreadyUpToDate) {
		return fmt.Errorf("failed to fetch refs: %w", err)
	}

	if _, err := repo.CommitObject(plumbing.NewHash(baseSHA)); err != nil {
		return fmt.Errorf("base commit %s not available after fetch: %w", baseSHA, err)
	}
	if _, err := repo.CommitObject(plumbing.NewHash(headSHA)); err != nil {
		return fmt.Errorf("head commit %s not available after fetch: %w", headSHA, err)
	}

	return nil
}
