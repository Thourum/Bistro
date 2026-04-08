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
)

// Client handles git operations
type Client struct {
	cache *Cache
}

// NewClient creates a new git client with cache
func NewClient(cache *Cache) *Client {
	return &Client{cache: cache}
}

// ShallowClone fetches both baseSHA and headSHA into a local clone.
// It uses a full clone + explicit ref fetch so both commits are guaranteed
// to be present, regardless of how many commits separate them.
// Returns cached result when both SHAs already exist in the cache.
func (c *Client) ShallowClone(ctx context.Context, repoURL, baseSHA, headSHA string) (*CloneResult, error) {
	if repoURL == "" {
		return nil, fmt.Errorf("repository URL cannot be empty")
	}
	if baseSHA == "" {
		return nil, fmt.Errorf("baseSHA cannot be empty")
	}
	if headSHA == "" {
		return nil, fmt.Errorf("headSHA cannot be empty")
	}

	// Check composite cache — keyed on both SHAs to avoid stale hits after force-pushes
	if cached, ok := c.cache.Get(repoURL, baseSHA, headSHA); ok {
		return &CloneResult{
			LocalPath: cached.LocalPath,
			BaseSHA:   cached.BaseSHA,
			HeadSHA:   cached.HeadSHA,
		}, nil
	}

	repoDir, err := getRepoDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get repo dir: %w", err)
	}

	tmpDir, err := os.MkdirTemp(repoDir, "repo-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	opts := &gitlib.CloneOptions{
		URL:  repoURL,
		Auth: tokenFromEnv(),
	}

	repo, err := gitlib.PlainCloneContext(ctx, tmpDir, false, opts)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("failed to clone repository %s: %w", repoURL, err)
	}

	// Fetch both branch heads and PR refs so base and head commits are locally available
	fetchOpts := &gitlib.FetchOptions{
		Auth: tokenFromEnv(),
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
			config.RefSpec("+refs/pull/*/head:refs/remotes/origin/pr/*"),
		},
	}

	if err := repo.FetchContext(ctx, fetchOpts); err != nil && err != gitlib.NoErrAlreadyUpToDate {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("failed to fetch refs: %w", err)
	}

	// Verify both commits are present — fail fast before writing to cache
	if _, err := repo.CommitObject(plumbing.NewHash(baseSHA)); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("base commit %s not found after fetch: %w", baseSHA, err)
	}
	if _, err := repo.CommitObject(plumbing.NewHash(headSHA)); err != nil {
		os.RemoveAll(tmpDir)
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

	return result, nil
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
		Auth: tokenFromEnv(),
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
