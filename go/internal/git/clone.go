package git

import (
	"context"
	"fmt"
	"os"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/itterno/bistro/internal/logging"
)

// Client handles git operations
type Client struct {
	cache *Cache
	// tokenSource supplies the auth token for clone/fetch (e.g. from the OS
	// keychain). Queried per operation so token rotation is picked up.
	tokenSource func() string
}

// NewClient creates a new git client with cache
func NewClient(cache *Cache) *Client {
	return &Client{cache: cache}
}

// SetTokenSource configures where clone/fetch auth tokens come from.
// Must be called before the client is shared across goroutines.
func (c *Client) SetTokenSource(fn func() string) {
	c.tokenSource = fn
}

// auth returns HTTP basic auth from the configured token source, falling
// back to the GITHUB_TOKEN env var. Returns nil (anonymous) when neither is
// set — public repos still work.
func (c *Client) auth() *http.BasicAuth {
	if c.tokenSource != nil {
		if token := c.tokenSource(); token != "" {
			return &http.BasicAuth{
				Username: "x-token", // GitHub ignores username when token is used
				Password: token,
			}
		}
	}
	return tokenFromEnv()
}

// Clone fetches both baseSHA and headSHA into a local clone.
// A full clone is required because diff generation needs both commit objects
// present in the repository; the base and head commits can be arbitrarily far
// apart in history, so a shallow clone with Depth:1 would miss one or both.
// Returns cached result when both SHAs already exist in the cache.
func (c *Client) Clone(ctx context.Context, repoURL, baseSHA, headSHA string) (*CloneResult, error) {
	log := logging.FromContext(ctx).With(
		"repo_url", repoURL,
		"base_sha", truncateSHAForLog(baseSHA),
		"head_sha", truncateSHAForLog(headSHA),
	)
	if repoURL == "" {
		return nil, fmt.Errorf("repository URL cannot be empty")
	}
	if baseSHA == "" {
		return nil, fmt.Errorf("baseSHA cannot be empty")
	}
	if headSHA == "" {
		return nil, fmt.Errorf("headSHA cannot be empty")
	}

	// Check composite cache — keyed on both SHAs to avoid stale hits after force-pushes.
	// A hit is only honored when the clone still exists on disk: the in-memory cache
	// outlives the directory, which can be removed by the OS cache reaper or a user
	// clearing ~/Library/Caches. Returning a path that no longer exists would surface
	// go-git's misleading "repository does not exist" on the next diff. If the dir is
	// gone, drop the stale entry and fall through to a fresh clone.
	if cached, ok := c.cache.Get(repoURL, baseSHA, headSHA); ok {
		if localRepoExists(cached.LocalPath) {
			log.Debug("git clone cache hit", "local_path", cached.LocalPath)
			return &CloneResult{
				LocalPath: cached.LocalPath,
				BaseSHA:   cached.BaseSHA,
				HeadSHA:   cached.HeadSHA,
			}, nil
		}
		log.Warn("git clone cache entry points at a missing directory; re-cloning",
			"local_path", cached.LocalPath)
		c.cache.Clear(repoURL, baseSHA, headSHA)
	}

	log.Debug("git clone start")
	overallStart := time.Now()

	repoDir, err := getRepoDir()
	if err != nil {
		log.Error("failed to get repo dir", "error", err)
		return nil, fmt.Errorf("failed to get repo dir: %w", err)
	}

	tmpDir, err := os.MkdirTemp(repoDir, "repo-*")
	if err != nil {
		log.Error("failed to create temp dir", "repo_dir", repoDir, "error", err)
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	opts := &gitlib.CloneOptions{
		URL:  repoURL,
		Auth: c.auth(),
	}

	cloneStart := time.Now()
	repo, err := gitlib.PlainCloneContext(ctx, tmpDir, false, opts)
	cloneDur := time.Since(cloneStart).Milliseconds()
	if err != nil {
		os.RemoveAll(tmpDir)
		log.Error("git clone failed",
			"duration_ms", cloneDur,
			"error", err,
		)
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
	if err := repo.FetchContext(ctx, fetchOpts); err != nil && err != gitlib.NoErrAlreadyUpToDate {
		os.RemoveAll(tmpDir)
		log.Error("git fetch refs failed",
			"duration_ms", time.Since(fetchStart).Milliseconds(),
			"error", err,
		)
		return nil, fmt.Errorf("failed to fetch refs: %w", err)
	}
	log.Debug("git fetch refs done", "duration_ms", time.Since(fetchStart).Milliseconds())

	// Verify both commits are present — fail fast before writing to cache
	if _, err := repo.CommitObject(plumbing.NewHash(baseSHA)); err != nil {
		os.RemoveAll(tmpDir)
		log.Warn("base commit missing after fetch", "error", err)
		return nil, fmt.Errorf("base commit %s not found after fetch: %w", baseSHA, err)
	}
	if _, err := repo.CommitObject(plumbing.NewHash(headSHA)); err != nil {
		os.RemoveAll(tmpDir)
		log.Warn("head commit missing after fetch", "error", err)
		return nil, fmt.Errorf("head commit %s not found after fetch: %w", headSHA, err)
	}

	result := &CloneResult{
		LocalPath: tmpDir,
		BaseSHA:   baseSHA,
		HeadSHA:   headSHA,
	}

	c.cache.Set(repoURL, baseSHA, headSHA, &RepoEntry{
		URL:       repoURL,
		LocalPath: tmpDir,
		BaseSHA:   baseSHA,
		HeadSHA:   headSHA,
		FetchedAt: time.Now(),
	})

	log.Debug("git clone complete",
		"total_duration_ms", time.Since(overallStart).Milliseconds(),
		"local_path", tmpDir,
	)
	return result, nil
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

// truncateSHAForLog returns a 7-char prefix of a SHA for compact log output.
func truncateSHAForLog(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

// FetchRefs updates an existing local repository by fetching all branch and PR
// refs, then verifies both baseSHA and headSHA are locally available.
func (c *Client) FetchRefs(ctx context.Context, localPath, baseSHA, headSHA string) error {
	if localPath == "" {
		return fmt.Errorf("local path cannot be empty")
	}
	if baseSHA == "" {
		return fmt.Errorf("baseSHA cannot be empty")
	}
	if headSHA == "" {
		return fmt.Errorf("headSHA cannot be empty")
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

	if err := repo.FetchContext(ctx, fetchOpts); err != nil && err != gitlib.NoErrAlreadyUpToDate {
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

// tokenFromEnv returns HTTP basic auth from GITHUB_TOKEN env var, or nil if unset.
func tokenFromEnv() *http.BasicAuth {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil
	}
	return &http.BasicAuth{
		Username: "x-token", // GitHub ignores username when token is used
		Password: token,
	}
}
