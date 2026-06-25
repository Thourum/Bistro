package git

import (
	"fmt"
	"sync"
)

// Cache stores cloned repositories in memory
type Cache struct {
	mu    sync.RWMutex
	repos map[string]*RepoEntry
}

// NewCache creates a new repository cache
func NewCache() *Cache {
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
	return entry, ok
}

// Set caches a repo entry under the composite URL+SHA key.
func (c *Cache) Set(repoURL, baseSHA, headSHA string, entry *RepoEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey(repoURL, baseSHA, headSHA)
	c.repos[key] = entry
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

// GetAll returns all cached entries (for testing)
func (c *Cache) GetAll() []*RepoEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries := make([]*RepoEntry, 0, len(c.repos))
	for _, entry := range c.repos {
		entries = append(entries, entry)
	}
	return entries
}
