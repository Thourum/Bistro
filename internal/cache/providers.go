package cache

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Cached usernames expire after a month so account changes are eventually refreshed.
const expiryDuration = 30 * 24 * time.Hour

// CacheEntry records the username and last refresh for a provider.
type CacheEntry struct {
	Username string    `json:"username"`
	CachedAt time.Time `json:"cachedAt"`
}

// ProviderCache stores provider usernames locally for offline display.
type ProviderCache struct {
	path    string
	mu      sync.Mutex
	entries map[string]CacheEntry
}

// NewProviderCache loads saved usernames, starting empty if the file is unavailable.
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
	var entries map[string]CacheEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		slog.Warn("ignoring invalid provider cache", "error", err)
		return
	}
	if entries != nil {
		pc.entries = entries
	}
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
	// Atomic replacement prevents a crash from leaving a partially written cache.
	file, err := os.CreateTemp(dir, ".providers-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0o600); err != nil {
		return errors.Join(err, file.Close())
	}
	if _, err := file.Write(data); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), pc.path)
}

// Get returns a cached username, whether it expired, and whether an entry exists.
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

// Set updates a provider username in memory and persists the cache.
func (pc *ProviderCache) Set(provider, username string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.entries[provider] = CacheEntry{
		Username: username,
		CachedAt: time.Now(),
	}
	if err := pc.flush(); err != nil {
		slog.Warn("persist provider cache after set", "error", err)
	}
}

// Delete removes a provider username in memory and persists the cache.
func (pc *ProviderCache) Delete(provider string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	delete(pc.entries, provider)
	if err := pc.flush(); err != nil {
		slog.Warn("persist provider cache after delete", "error", err)
	}
}

// All returns an independent copy of every cached provider entry.
func (pc *ProviderCache) All() map[string]CacheEntry {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	result := make(map[string]CacheEntry, len(pc.entries))
	for k, v := range pc.entries {
		result[k] = v
	}
	return result
}
