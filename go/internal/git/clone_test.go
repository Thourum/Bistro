package git

import (
	"context"
	"os"
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
	remoteDir := t.TempDir()
	_, baseSHA, headSHA := buildRemoteWithPRRef(t, remoteDir, 1)
	cache := NewCache()

	// Cache hits are only honored when the path still exists on disk and contains
	// the requested commits, so point at a real repo that has both SHAs.
	entry := &RepoEntry{
		URL:       "file://" + remoteDir,
		LocalPath: remoteDir,
		BaseSHA:   baseSHA,
		HeadSHA:   headSHA,
	}
	cache.Set(entry.URL, entry.BaseSHA, entry.HeadSHA, entry)

	client := NewClient(cache)
	ctx := context.Background()

	result, err := client.Clone(ctx, entry.URL, entry.BaseSHA, entry.HeadSHA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.LocalPath != remoteDir {
		t.Errorf("expected cached path %s, got %s", remoteDir, result.LocalPath)
	}
}

// TestCloneCacheHitMissingDirNotReturned reproduces the production bug: an
// in-memory cache entry survives the on-disk clone being deleted (by the OS
// cache reaper, a user clearing ~/Library/Caches, or a stray cleanup). A cache
// hit pointing at a now-missing directory must NOT be returned — go-git would
// then fail with the misleading "repository does not exist".
func TestCloneCacheHitMissingDirNotReturned(t *testing.T) {
	cache := NewCache()
	const url = "https://github.com/org/repo"
	deadDir := t.TempDir()
	cache.Set(url, "base1", "head1", &RepoEntry{
		URL:       url,
		LocalPath: deadDir,
		BaseSHA:   "base1",
		HeadSHA:   "head1",
	})
	// Delete the clone out from under the cache.
	if err := os.RemoveAll(deadDir); err != nil {
		t.Fatalf("setup: remove dir: %v", err)
	}

	client := NewClient(cache)
	// The bogus host fails DNS fast and offline; we only assert that Clone does
	// NOT short-circuit to the dead cached path. (A real re-clone is exercised
	// in live sync; here we just prove the stale hit is rejected.)
	result, err := client.Clone(context.Background(), url, "base1", "head1")
	if err == nil && result != nil && result.LocalPath == deadDir {
		t.Fatalf("returned stale cache path pointing at a deleted dir: %s", deadDir)
	}
	// The stale entry must have been cleared so a future clone can repopulate it.
	if _, ok := cache.Get(url, "base1", "head1"); ok {
		t.Error("stale cache entry should have been cleared after detecting missing dir")
	}
}

// TestLocalRepoExists covers the on-disk validity guard used by the cache-hit path.
func TestLocalRepoExists(t *testing.T) {
	if localRepoExists("") {
		t.Error("empty path should report not-exists")
	}
	if localRepoExists("/nonexistent/bistro/definitely/not/here") {
		t.Error("missing path should report not-exists")
	}
	dir := t.TempDir()
	if !localRepoExists(dir) {
		t.Error("existing directory should report exists")
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove dir: %v", err)
	}
	if localRepoExists(dir) {
		t.Error("removed directory should report not-exists")
	}
}

func TestCloneCachesResult(t *testing.T) {
	remoteDir := t.TempDir()
	_, baseSHA, headSHA := buildRemoteWithPRRef(t, remoteDir, 2)
	cache := NewCache()

	entry := &RepoEntry{
		URL:       "file://" + remoteDir,
		LocalPath: remoteDir,
		BaseSHA:   baseSHA,
		HeadSHA:   headSHA,
	}
	cache.Set(entry.URL, entry.BaseSHA, entry.HeadSHA, entry)

	client := NewClient(cache)
	ctx := context.Background()

	result, err := client.Clone(ctx, entry.URL, entry.BaseSHA, entry.HeadSHA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.BaseSHA != baseSHA {
		t.Errorf("expected BaseSHA %s, got %s", baseSHA, result.BaseSHA)
	}
	if result.HeadSHA != headSHA {
		t.Errorf("expected HeadSHA %s, got %s", headSHA, result.HeadSHA)
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
