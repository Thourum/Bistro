package git

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/itterno/bistro/internal/git/clonecache"
	"github.com/itterno/bistro/internal/logging"
)

func (c *Client) broadClone(ctx context.Context, log *slog.Logger, repoURL, baseSHA, headSHA string) (result *CloneResult, err error) {
	log.Debug("git clone start")
	overallStart := time.Now()

	tmpDir, cleanupCloneDir, err := clonecache.StageCloneDir(repoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare clone directory: %w", err)
	}
	cloneSucceeded := false
	defer func() {
		if cloneSucceeded {
			if cleanupErr := cleanupCloneDir.Commit(); cleanupErr != nil {
				log.Warn("failed to remove previous clone backup", "error", cleanupErr)
			}
			return
		}
		if cleanupErr := cleanupCloneDir.Rollback(); cleanupErr != nil {
			result = nil
			err = errors.Join(err, fmt.Errorf("restore previous clone after failure: %w", cleanupErr))
		}
	}()

	cloneOpts := &gitlib.CloneOptions{
		URL:  repoURL,
		Auth: c.auth(),
	}

	cloneStart := time.Now()
	// Clone bare (no working tree): diff generation reads commit objects and
	// trees directly via the object store, so a checkout is pure overhead —
	// it doubles disk I/O and writes files we never open. GenerateDiff and
	// FetchRefs both go through PlainOpen, which works against bare repos.
	repo, err := gitlib.PlainCloneContext(ctx, tmpDir, true, cloneOpts)
	cloneDur := time.Since(cloneStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("failed to clone repository %s: %w", repoURL, err)
	}
	log.Debug("git clone done", "duration_ms", cloneDur, "local_path", tmpDir)

	// Fetch both branch heads and PR refs so base and head commits are locally available
	fetchOpts := &gitlib.FetchOptions{
		Auth: c.auth(),
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
			config.RefSpec("+refs/pull/*/head:refs/remotes/origin/pr/*"),
		},
	}

	fetchStart := time.Now()
	if err := repo.FetchContext(ctx, fetchOpts); err != nil && !errors.Is(err, gitlib.NoErrAlreadyUpToDate) {
		return nil, fmt.Errorf("failed to fetch refs: %w", err)
	}
	log.Debug("git fetch refs done", "duration_ms", time.Since(fetchStart).Milliseconds())

	// Verify both commits are present — fail fast before writing to cache
	if _, err := repo.CommitObject(plumbing.NewHash(baseSHA)); err != nil {
		return nil, fmt.Errorf("base commit %s not found after fetch: %w", baseSHA, err)
	}
	if _, err := repo.CommitObject(plumbing.NewHash(headSHA)); err != nil {
		return nil, fmt.Errorf("head commit %s not found after fetch: %w", headSHA, err)
	}

	result = &CloneResult{
		LocalPath: tmpDir,
		BaseSHA:   baseSHA,
		HeadSHA:   headSHA,
	}

	c.cache.Set(repoURL, baseSHA, headSHA, &clonecache.RepoEntry{
		URL:       repoURL,
		LocalPath: tmpDir,
		BaseSHA:   baseSHA,
		HeadSHA:   headSHA,
		FetchedAt: time.Now(),
	})
	c.recordCacheCloned(repoURL, tmpDir)
	cloneSucceeded = true

	log.Debug("git clone complete",
		"total_duration_ms", time.Since(overallStart).Milliseconds(),
		"local_path", tmpDir,
	)
	return result, nil
}

// reuseOnDiskClone adopts the repo's shared cache directory when it already
// holds every required commit. Every PR of a repo shares that directory, so a
// PR whose commits an earlier clone already fetched must not trigger a re-clone.
// Temp-dir clones are never reused because they are not shared.
func (c *Client) reuseOnDiskClone(
	log *slog.Logger,
	repoURL string,
	baseSHA string,
	headSHA string,
	opts *CloneOptions,
) (*CloneResult, bool) {
	if clonecache.NeedsFreshTempDir(repoURL) {
		return nil, false
	}
	repoDir, err := clonecache.RepoDir(repoURL)
	if err != nil || !localRepoExists(repoDir) {
		return nil, false
	}
	missing, err := firstMissingCommit(repoDir, requiredSHAs(baseSHA, headSHA, opts))
	if err != nil || missing != "" {
		return nil, false
	}

	log.Debug("git clone reusing on-disk repo (commits verified)", "local_path", repoDir)
	c.cache.Set(repoURL, baseSHA, headSHA, &clonecache.RepoEntry{
		URL:       repoURL,
		LocalPath: repoDir,
		BaseSHA:   baseSHA,
		HeadSHA:   headSHA,
		FetchedAt: time.Now(),
	})
	c.recordCacheCloned(repoURL, repoDir)
	return &CloneResult{LocalPath: repoDir, BaseSHA: baseSHA, HeadSHA: headSHA}, true
}

func (c *Client) validateCacheHit(
	log *slog.Logger,
	repoURL string,
	baseSHA string,
	headSHA string,
	cached *clonecache.RepoEntry,
	opts *CloneOptions,
) bool {
	if !localRepoExists(cached.LocalPath) {
		log.Warn("git clone cache entry points at a missing directory; re-cloning",
			"local_path", cached.LocalPath)
		c.cache.Clear(repoURL, baseSHA, headSHA)
		return false
	}

	required := requiredSHAs(baseSHA, headSHA, opts)
	missing, err := firstMissingCommit(cached.LocalPath, required)
	if err != nil {
		log.Warn("git clone cache hit unverifiable; re-cloning",
			"local_path", cached.LocalPath, "error", err)
		c.cache.Clear(repoURL, baseSHA, headSHA)
		return false
	}
	if missing != "" {
		log.Debug("git clone cache hit missing a required commit; re-cloning",
			"local_path", cached.LocalPath, "missing_sha", logging.ShortSHA(missing))
		c.cache.Clear(repoURL, baseSHA, headSHA)
		return false
	}

	log.Debug("git clone cache hit (commits verified)", "local_path", cached.LocalPath)
	c.recordCacheCloned(repoURL, cached.LocalPath)
	return true
}
