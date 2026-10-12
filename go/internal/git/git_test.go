package git

import (
	"testing"
)

func TestFullIntegration(t *testing.T) {
	// Verify all components are available and compile
	cache := NewCache()
	client := NewClient(cache)

	if cache.Size() != 0 {
		t.Error("cache should start empty")
	}

	if client.cache != cache {
		t.Error("client should reference the cache")
	}

	entry := &RepoEntry{
		URL:       "https://github.com/test/repo",
		LocalPath: "/tmp/test",
		BaseSHA:   "abc",
		HeadSHA:   "def",
	}

	cache.Set(entry.URL, entry.BaseSHA, entry.HeadSHA, entry)
	if cache.Size() != 1 {
		t.Error("cache should have 1 entry")
	}

	retrieved, ok := cache.Get(entry.URL, entry.BaseSHA, entry.HeadSHA)
	if !ok || retrieved.BaseSHA != "abc" {
		t.Error("cache retrieval failed")
	}

	cache.Clear(entry.URL, entry.BaseSHA, entry.HeadSHA)
	if cache.Size() != 0 {
		t.Error("cache should be empty after clear")
	}
}

func TestCacheClearAll(t *testing.T) {
	cache := NewCache()

	cache.Set("https://github.com/org/repo1", "base1", "head1", &RepoEntry{URL: "https://github.com/org/repo1", BaseSHA: "base1", HeadSHA: "head1"})
	cache.Set("https://github.com/org/repo2", "base2", "head2", &RepoEntry{URL: "https://github.com/org/repo2", BaseSHA: "base2", HeadSHA: "head2"})
	cache.Set("https://github.com/org/repo3", "base3", "head3", &RepoEntry{URL: "https://github.com/org/repo3", BaseSHA: "base3", HeadSHA: "head3"})

	if cache.Size() != 3 {
		t.Errorf("expected 3 entries, got %d", cache.Size())
	}

	cache.ClearAll()

	if cache.Size() != 0 {
		t.Error("cache should be empty after ClearAll")
	}
}

func TestAllTypesAvailable(t *testing.T) {
	// Verify all types are accessible
	var _ *RepoEntry
	var _ *CloneResult
	var _ *DiffFile
	var _ *DiffResult
}
