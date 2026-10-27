package clonecache

import (
	"fmt"
	"sync"
	"time"
)

// RepoEntry represents a cached repository
type RepoEntry struct {
	URL       string    // GitHub repository URL
	LocalPath string    // Local filesystem path
	BaseSHA   string    // Base commit SHA
	HeadSHA   string    // Head commit SHA
	FetchedAt time.Time // When it was cached
}

// Cache stores cloned repositories in memory
type Cache struct {
	mu    sync.RWMutex
	repos map[string]*RepoEntry
}

// New creates a new repository cache
func New() *Cache {
	return &Cache{
		repos: make(map[string]*RepoEntry),
	}
}

// cacheKey builds a composite key from URL and both commit SHAs.
// Full SHAs are used (not truncated): a 7-char prefix risks collisions across
// distinct commits that share a prefix, which would surface as a cache hit
// pointing at a clone that lacks the requested commit. The exact-SHA key keeps
// cache identity aligned with the commits actually verified on disk.
func cacheKey(repoURL, baseSHA, headSHA string) string {
	return fmt.Sprintf("%s@%s..%s", repoURL, baseSHA, headSHA)
}

// Get retrieves a cached repo entry by URL and both SHAs.
// Keying on SHAs prevents stale cache hits after force-pushes.
func (c *Cache) Get(repoURL, baseSHA, headSHA string) (*RepoEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := cacheKey(repoURL, baseSHA, headSHA)
	entry, ok := c.repos[key]
	if !ok || entry == nil {
		return nil, false
	}
	entryCopy := *entry
	return &entryCopy, true
}

// Set caches a repo entry under the composite URL+SHA key.
func (c *Cache) Set(repoURL, baseSHA, headSHA string, entry *RepoEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey(repoURL, baseSHA, headSHA)
	if entry == nil {
		delete(c.repos, key)
		return
	}
	entryCopy := *entry
	c.repos[key] = &entryCopy
}

// Clear removes an entry from cache by composite key.
func (c *Cache) Clear(repoURL, baseSHA, headSHA string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey(repoURL, baseSHA, headSHA)
	delete(c.repos, key)
}

// Size returns number of cached repos
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.repos)
}

// ClearAll removes all entries from cache
func (c *Cache) ClearAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.repos = make(map[string]*RepoEntry)
}
