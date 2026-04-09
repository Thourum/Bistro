package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const expiryDuration = 30 * 24 * time.Hour

type CacheEntry struct {
	Username string    `json:"username"`
	CachedAt time.Time `json:"cachedAt"`
}

type ProviderCache struct {
	path    string
	mu      sync.Mutex
	entries map[string]CacheEntry
}

func NewProviderCache(path string) *ProviderCache {
	pc := &ProviderCache{
		path:    path,
		entries: make(map[string]CacheEntry),
	}
	pc.load()
	return pc
}

func (pc *ProviderCache) load() {
	data, err := os.ReadFile(pc.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &pc.entries)
}

func (pc *ProviderCache) flush() error {
	dir := filepath.Dir(pc.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(pc.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(pc.path, data, 0644)
}

func (pc *ProviderCache) Get(provider string) (username string, expired bool, found bool) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	entry, ok := pc.entries[provider]
	if !ok {
		return "", false, false
	}
	isExpired := time.Since(entry.CachedAt) > expiryDuration
	return entry.Username, isExpired, true
}

func (pc *ProviderCache) Set(provider, username string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.entries[provider] = CacheEntry{
		Username: username,
		CachedAt: time.Now(),
	}
	_ = pc.flush()
}

func (pc *ProviderCache) Delete(provider string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	delete(pc.entries, provider)
	_ = pc.flush()
}

func (pc *ProviderCache) All() map[string]CacheEntry {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	result := make(map[string]CacheEntry, len(pc.entries))
	for k, v := range pc.entries {
		result[k] = v
	}
	return result
}
