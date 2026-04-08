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
// Using first 7 chars of each SHA matches git convention and avoids bloat.
func cacheKey(repoURL, baseSHA, headSHA string) string {
	base := baseSHA
	if len(base) > 7 {
		base = base[:7]
	}
	head := headSHA
	if len(head) > 7 {
		head = head[:7]
	}
	return fmt.Sprintf("%s@%s..%s", repoURL, base, head)
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
