package git

import (
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	cache := NewCache()
	if cache == nil {
		t.Error("expected non-nil cache")
	}
	if cache.Size() != 0 {
		t.Error("expected empty cache")
	}
}

func TestCacheSetGet(t *testing.T) {
	cache := NewCache()
	entry := &RepoEntry{
		URL:       "https://github.com/org/repo",
		LocalPath: "/tmp/repo",
		BaseSHA:   "abc123",
		HeadSHA:   "def456",
		FetchedAt: time.Now(),
	}

	cache.Set(entry.URL, entry.BaseSHA, entry.HeadSHA, entry)

	retrieved, ok := cache.Get(entry.URL, entry.BaseSHA, entry.HeadSHA)
	if !ok {
		t.Error("expected entry to be cached")
	}
	if retrieved.BaseSHA != "abc123" {
		t.Errorf("expected BaseSHA abc123, got %s", retrieved.BaseSHA)
	}
}

func TestCacheEntriesAreNotShared(t *testing.T) {
	cache := NewCache()
	entry := &RepoEntry{URL: "https://github.com/org/repo", BaseSHA: "base", HeadSHA: "head", LocalPath: "/original"}
	cache.Set(entry.URL, entry.BaseSHA, entry.HeadSHA, entry)
	entry.LocalPath = "/changed-after-set"

	first, ok := cache.Get(entry.URL, entry.BaseSHA, entry.HeadSHA)
	if !ok || first.LocalPath != "/original" {
		t.Fatalf("cache.Set retained caller's pointer: %+v", first)
	}
	first.LocalPath = "/changed-after-get"
	second, ok := cache.Get(entry.URL, entry.BaseSHA, entry.HeadSHA)
	if !ok || second.LocalPath != "/original" {
		t.Fatalf("cache.Get exposed internal pointer: %+v", second)
	}
}

func TestCacheSetNilEvictsEntry(t *testing.T) {
	cache := NewCache()
	cache.Set("url", "base", "head", &RepoEntry{LocalPath: "/clone"})
	cache.Set("url", "base", "head", nil)
	if entry, ok := cache.Get("url", "base", "head"); ok || entry != nil || cache.Size() != 0 {
		t.Fatalf("nil cache entry must be treated as absent, got entry=%v present=%v size=%d", entry, ok, cache.Size())
	}
}

func TestCacheClear(t *testing.T) {
	cache := NewCache()
	entry := &RepoEntry{
		URL:     "https://github.com/org/repo",
		BaseSHA: "aaa",
		HeadSHA: "bbb",
	}
	cache.Set(entry.URL, entry.BaseSHA, entry.HeadSHA, entry)

	cache.Clear(entry.URL, entry.BaseSHA, entry.HeadSHA)

	_, ok := cache.Get(entry.URL, entry.BaseSHA, entry.HeadSHA)
	if ok {
		t.Error("expected entry to be cleared")
	}
}

func TestCacheSize(t *testing.T) {
	cache := NewCache()
	if cache.Size() != 0 {
		t.Error("expected empty cache")
	}

	cache.Set("url1", "base1", "head1", &RepoEntry{URL: "url1", BaseSHA: "base1", HeadSHA: "head1"})
	if cache.Size() != 1 {
		t.Error("expected size 1")
	}

	cache.Set("url2", "base2", "head2", &RepoEntry{URL: "url2", BaseSHA: "base2", HeadSHA: "head2"})
	if cache.Size() != 2 {
		t.Error("expected size 2")
	}

	cache.Clear("url1", "base1", "head1")
	if cache.Size() != 1 {
		t.Error("expected size 1 after clear")
	}
}

func TestCacheThreadSafety(t *testing.T) {
	cache := NewCache()

	// Verifies no race conditions — run with `go test -race` to detect issues
	entry := &RepoEntry{URL: "https://github.com/org/repo", BaseSHA: "aaa", HeadSHA: "bbb"}
	cache.Set(entry.URL, entry.BaseSHA, entry.HeadSHA, entry)
	cache.Get(entry.URL, entry.BaseSHA, entry.HeadSHA)
	cache.Size()
	cache.Clear(entry.URL, entry.BaseSHA, entry.HeadSHA)
}

// TestCacheKeyWithDifferentSHAs verifies that same URL with different SHAs produces distinct entries
func TestCacheKeyWithDifferentSHAs(t *testing.T) {
	cache := NewCache()
	url := "https://github.com/org/repo"

	entryA := &RepoEntry{URL: url, LocalPath: "/tmp/a", BaseSHA: "aaa", HeadSHA: "bbb"}
	entryB := &RepoEntry{URL: url, LocalPath: "/tmp/b", BaseSHA: "ccc", HeadSHA: "ddd"}

	cache.Set(url, "aaa", "bbb", entryA)
	cache.Set(url, "ccc", "ddd", entryB)

	if cache.Size() != 2 {
		t.Errorf("expected 2 distinct cache entries, got %d", cache.Size())
	}

	gotA, okA := cache.Get(url, "aaa", "bbb")
	if !okA || gotA.LocalPath != "/tmp/a" {
		t.Errorf("expected entryA at /tmp/a, got %v", gotA)
	}

	gotB, okB := cache.Get(url, "ccc", "ddd")
	if !okB || gotB.LocalPath != "/tmp/b" {
		t.Errorf("expected entryB at /tmp/b, got %v", gotB)
	}
}

// TestCacheKeyCollisionPrevented ensures a force-push (same URL, new headSHA) doesn't
// return a stale cache entry
func TestCacheKeyCollisionPrevented(t *testing.T) {
	cache := NewCache()
	url := "https://github.com/org/repo"

	// Original head
	original := &RepoEntry{URL: url, LocalPath: "/tmp/original", BaseSHA: "base1", HeadSHA: "old-head"}
	cache.Set(url, "base1", "old-head", original)

	// After force-push: same base, new head
	_, found := cache.Get(url, "base1", "new-head")
	if found {
		t.Error("expected cache miss for new headSHA after force-push")
	}
}
