package git

import (
	"os"

	"github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/itterno/bistro/internal/git/clonecache"
	"github.com/itterno/bistro/internal/git/repolock"
)

// LocalRepoResolver reports a registered local checkout for a repo URL.
// Implemented by an adapter over store.RepoLocationStore in package server.
type LocalRepoResolver interface {
	// LocalPath returns (path, true) only for source='local' rows.
	LocalPath(repoURL string) (string, bool)
	// OnLocalDegraded records a broken local repo (missing path / mismatch).
	OnLocalDegraded(repoURL, status, code, msg string)
	// OnUsed bumps last_used_at.
	OnUsed(repoURL string)
	// OnCacheCloned records a cache clone: source='cache', local_path=path,
	// last_used_at=now (wraps RepoLocationStore.SetCache). Called after every
	// successful cache clone AND on cache-hit reuse (idempotent upsert).
	OnCacheCloned(repoURL, path string)
}

// Client handles git operations
type Client struct {
	cache *clonecache.Cache
	// tokenSource supplies the auth token for clone/fetch (e.g. from the OS
	// keychain). Queried per operation so token rotation is picked up.
	tokenSource   func() string
	localResolver LocalRepoResolver
	locks         *repolock.Locks
}

// NewClient creates a new git client with cache
func NewClient(cache *clonecache.Cache) *Client {
	if cache == nil {
		cache = clonecache.New()
	}
	return &Client{cache: cache, locks: repolock.New()}
}

// SetTokenSource configures where clone/fetch auth tokens come from.
// Must be called before the client is shared across goroutines.
func (c *Client) SetTokenSource(fn func() string) {
	c.tokenSource = fn
}

// SetLocalResolver configures local repo resolution. Nil disables local lookup.
func (c *Client) SetLocalResolver(r LocalRepoResolver) {
	c.localResolver = r
}

// SetLocks configures repo-level operation locks.
func (c *Client) SetLocks(l *repolock.Locks) {
	c.locks = l
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
