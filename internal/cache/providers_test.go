package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProviderCache_InvalidJSONDoesNotReplaceLiveEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	cache := NewProviderCache(path)
	cache.entries["github"] = CacheEntry{Username: "octocat", CachedAt: time.Now()}
	if err := os.WriteFile(path, []byte(`{"github":null,"bad":`), 0o600); err != nil {
		t.Fatal(err)
	}
	cache.load()
	if name, _, found := cache.Get("github"); !found || name != "octocat" {
		t.Fatalf("invalid file overwrote live cache: found=%v, name=%q", found, name)
	}
}

func TestProviderCache_NullJSONStillAllowsSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	if err := os.WriteFile(path, []byte("null"), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := NewProviderCache(path)
	cache.Set("github", "octocat")
	if name, _, found := cache.Get("github"); !found || name != "octocat" {
		t.Fatalf("set after null cache: found=%v, name=%q", found, name)
	}
}

func TestProviderCache_PersistKeepsPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := NewProviderCache(path)
	cache.Set("github", "octocat")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("cache mode = %o, want 600", got)
	}
}

func TestProviderCache_SetAndGet(t *testing.T) {
	dir := t.TempDir()
	pc := NewProviderCache(filepath.Join(dir, "providers.json"))

	pc.Set("github", "octocat")

	username, expired, found := pc.Get("github")
	if !found {
		t.Fatal("expected to find github entry")
	}
	if expired {
		t.Fatal("expected entry to not be expired")
	}
	if username != "octocat" {
		t.Fatalf("expected octocat, got %s", username)
	}
}

func TestProviderCache_GetMissing(t *testing.T) {
	dir := t.TempDir()
	pc := NewProviderCache(filepath.Join(dir, "providers.json"))

	_, _, found := pc.Get("github")
	if found {
		t.Fatal("expected not found for missing provider")
	}
}

func TestProviderCache_Expiry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.json")
	pc := NewProviderCache(path)

	pc.Set("github", "octocat")

	// Manually backdate the cachedAt to 31 days ago
	pc.mu.Lock()
	entry := pc.entries["github"]
	entry.CachedAt = time.Now().Add(-31 * 24 * time.Hour)
	pc.entries["github"] = entry
	pc.mu.Unlock()
	_ = pc.flush()

	username, expired, found := pc.Get("github")
	if !found {
		t.Fatal("expected to find expired entry")
	}
	if !expired {
		t.Fatal("expected entry to be expired")
	}
	if username != "octocat" {
		t.Fatalf("expected octocat (stale), got %s", username)
	}
}

func TestProviderCache_Delete(t *testing.T) {
	dir := t.TempDir()
	pc := NewProviderCache(filepath.Join(dir, "providers.json"))

	pc.Set("github", "octocat")
	pc.Delete("github")

	_, _, found := pc.Get("github")
	if found {
		t.Fatal("expected not found after delete")
	}
}

func TestProviderCache_PersistAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.json")

	pc1 := NewProviderCache(path)
	pc1.Set("github", "octocat")

	pc2 := NewProviderCache(path)
	username, _, found := pc2.Get("github")
	if !found {
		t.Fatal("expected to find entry from previous instance")
	}
	if username != "octocat" {
		t.Fatalf("expected octocat, got %s", username)
	}
}

func TestProviderCache_AllProviders(t *testing.T) {
	dir := t.TempDir()
	pc := NewProviderCache(filepath.Join(dir, "providers.json"))

	pc.Set("github", "octocat")
	pc.Set("gitlab", "gluser")

	all := pc.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(all))
	}
}

func TestProviderCache_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "providers.json")
	pc := NewProviderCache(path)

	pc.Set("github", "octocat")

	if _, err := os.Stat(filepath.Join(dir, "subdir")); os.IsNotExist(err) {
		t.Fatal("expected directory to be created")
	}
}
