package git

import (
	"context"
	"testing"
)

func TestNewClient(t *testing.T) {
	cache := NewCache()
	client := NewClient(cache)
	if client == nil {
		t.Error("expected non-nil client")
	}
}

func TestClientHasCache(t *testing.T) {
	cache := NewCache()
	client := NewClient(cache)
	if client.cache == nil {
		t.Error("expected cache to be set")
	}
}

func TestCloneEmptyURL(t *testing.T) {
	cache := NewCache()
	client := NewClient(cache)
	ctx := context.Background()

	_, err := client.Clone(ctx, "", "base", "head")
	if err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestCloneEmptyBaseSHA(t *testing.T) {
	cache := NewCache()
	client := NewClient(cache)
	ctx := context.Background()

	_, err := client.Clone(ctx, "https://github.com/org/repo", "", "head")
	if err == nil {
		t.Error("expected error for empty baseSHA")
	}
}

func TestCloneEmptyHeadSHA(t *testing.T) {
	cache := NewCache()
	client := NewClient(cache)
	ctx := context.Background()

	_, err := client.Clone(ctx, "https://github.com/org/repo", "base", "")
	if err == nil {
		t.Error("expected error for empty headSHA")
	}
}

func TestCloneWithCachedRepo(t *testing.T) {
	cache := NewCache()

	entry := &RepoEntry{
		URL:       "https://github.com/org/repo",
		LocalPath: "/tmp/cached",
		BaseSHA:   "base123",
		HeadSHA:   "head456",
	}
	cache.Set(entry.URL, entry.BaseSHA, entry.HeadSHA, entry)

	client := NewClient(cache)
	ctx := context.Background()

	result, err := client.Clone(ctx, entry.URL, entry.BaseSHA, entry.HeadSHA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.LocalPath != "/tmp/cached" {
		t.Errorf("expected cached path /tmp/cached, got %s", result.LocalPath)
	}
}

func TestCloneCachesResult(t *testing.T) {
	cache := NewCache()

	entry := &RepoEntry{
		URL:       "https://github.com/org/cached-repo",
		LocalPath: "/tmp/cached-repo",
		BaseSHA:   "abc",
		HeadSHA:   "def",
	}
	cache.Set(entry.URL, entry.BaseSHA, entry.HeadSHA, entry)

	client := NewClient(cache)
	ctx := context.Background()

	result, err := client.Clone(ctx, entry.URL, entry.BaseSHA, entry.HeadSHA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.BaseSHA != "abc" {
		t.Errorf("expected BaseSHA abc, got %s", result.BaseSHA)
	}
	if result.HeadSHA != "def" {
		t.Errorf("expected HeadSHA def, got %s", result.HeadSHA)
	}
}

// TestCloneDifferentSHAsMissCache verifies that a cache hit requires both SHAs to match
func TestCloneDifferentSHAsMissCache(t *testing.T) {
	cache := NewCache()
	url := "https://github.com/org/repo"

	// Cache entry for old headSHA
	cache.Set(url, "base1", "oldhead", &RepoEntry{
		URL:       url,
		LocalPath: "/tmp/old",
		BaseSHA:   "base1",
		HeadSHA:   "oldhead",
	})

	// Request with new headSHA — must miss cache, not return stale entry
	_, ok := cache.Get(url, "base1", "newhead")
	if ok {
		t.Error("expected cache miss for new headSHA")
	}
}

func TestFetchRefsEmptyPath(t *testing.T) {
	cache := NewCache()
	client := NewClient(cache)
	ctx := context.Background()

	err := client.FetchRefs(ctx, "", "base", "head")
	if err == nil {
		t.Error("expected error for empty local path")
	}
}

func TestFetchRefsEmptyBaseSHA(t *testing.T) {
	cache := NewCache()
	client := NewClient(cache)
	ctx := context.Background()

	err := client.FetchRefs(ctx, "/some/path", "", "head")
	if err == nil {
		t.Error("expected error for empty baseSHA")
	}
}

func TestFetchRefsEmptyHeadSHA(t *testing.T) {
	cache := NewCache()
	client := NewClient(cache)
	ctx := context.Background()

	err := client.FetchRefs(ctx, "/some/path", "base", "")
	if err == nil {
		t.Error("expected error for empty headSHA")
	}
}

func TestFetchRefsInvalidPath(t *testing.T) {
	cache := NewCache()
	client := NewClient(cache)
	ctx := context.Background()

	// Non-existent path should fail when PlainOpen is called
	err := client.FetchRefs(ctx, "/nonexistent/path/to/repo", "base", "head")
	if err == nil {
		t.Error("expected error for non-existent repo path")
	}
}
