package git

import (
	"testing"

	"github.com/itterno/bistro/internal/git/clonecache"
	"github.com/itterno/bistro/internal/git/diff"
)

func TestFullIntegration(t *testing.T) {
	// Verify all components are available and compile
	cache := clonecache.New()
	client := NewClient(cache)

	if cache.Size() != 0 {
		t.Error("cache should start empty")
	}

	if client.cache != cache {
		t.Error("client should reference the cache")
	}

	entry := &clonecache.RepoEntry{
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

func TestAllTypesAvailable(t *testing.T) {
	// Verify all types are accessible
	var _ *clonecache.RepoEntry
	var _ *CloneResult
	var _ *diff.File
	var _ *diff.Result
}
