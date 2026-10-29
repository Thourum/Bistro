package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/git/repolock"
	"github.com/itterno/bistro/internal/store/repolocation"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestRepoLocationCacheMaintenance(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "cache")
	t.Setenv("BISTRO_REPO_DIR", cacheRoot)
	database := storetest.Open(t)
	fresh := time.Now().Add(-29 * 24 * time.Hour)
	stale := time.Now().Add(-31 * 24 * time.Hour)
	paths := map[string]string{}
	for _, name := range []string{"local", "fresh", "stale", "null-used", "failed", "delete-one"} {
		path := filepath.Join(cacheRoot, "acme", name)
		paths[name] = path
		mustMkdirAll(path)
		mustWriteFile(filepath.Join(path, "payload"), "bytes")
		source := "cache"
		used := &fresh
		if name == "local" {
			source = "local"
		}
		if name == "stale" || name == "failed" {
			used = &stale
		}
		if name == "null-used" {
			used = nil
		}
		insertRepoWithLocation(t, database, "github:acme/"+name, "github", "acme", name, source, path, used)
	}
	unsafeTarget := t.TempDir()
	unsafePath := filepath.Join(cacheRoot, "acme", "unsafe")
	if err := os.Symlink(unsafeTarget, unsafePath); err != nil {
		t.Fatal(err)
	}
	insertRepoWithLocation(t, database, "github:acme/unsafe", "github", "acme", "unsafe", "cache", unsafePath, &fresh)
	failedPath, err := filepath.EvalSymlinks(paths["failed"])
	if err != nil {
		t.Fatal(err)
	}
	deleter := &fakeRepoLocationCacheDeleter{failed: map[string]error{failedPath: errors.New("delete failed")}}
	locks := repolock.New()
	service := &RepoLocation{Store: repolocation.New(database.DB), Locks: locks, DeleteCacheDir: deleter.DeleteCacheDir, SweepOrphans: (&fakeRepoLocationSweeper{removed: []string{"orphan"}}).SweepOrphans}
	ctx := context.Background()
	stats, err := service.CacheStats(ctx)
	if err != nil || stats.TotalBytes <= 0 || stats.ActiveBytes+stats.StaleBytes != stats.TotalBytes {
		t.Fatalf("stats = %+v, %v", stats, err)
	}
	present := map[string]bool{}
	for _, repo := range stats.Repos {
		present[repo.RepoID] = true
	}
	if len(present) != 5 || present["github:acme/local"] || present["github:acme/unsafe"] {
		t.Fatalf("incorrect cache stats membership: %v", present)
	}
	pruned, err := service.PruneCache(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertRemovedRepos(t, pruned.Removed, []string{"github:acme/stale", "github:acme/null-used"})
	if len(pruned.Failed) != 1 || pruned.Failed[0].RepoID != "github:acme/failed" || pruned.Failed[0].Code != "delete_failed" {
		t.Fatalf("prune failures = %+v", pruned.Failed)
	}
	assertSourcePath(t, database.DB, "github:acme/failed", "cache", paths["failed"])
	assertSourcePath(t, database.DB, "github:acme/local", "local", paths["local"])
	assertSourcePath(t, database.DB, "github:acme/stale", "", "")
	if err := service.DeleteCacheRepo(ctx, RepoRef{RepoID: "github:ACME/DELETE-ONE"}); err != nil {
		t.Fatal(err)
	}
	assertSourcePath(t, database.DB, "github:acme/delete-one", "", "")
	assertLocationError(t, service.DeleteCacheRepo(ctx, RepoRef{RepoID: "github:acme/local"}), "delete_failed")
	lockHeld := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = locks.WithRepo("github:acme/concurrent", func() error { close(lockHeld); <-release; return nil })
	}()
	<-lockHeld
	done := make(chan struct{})
	var cleared CacheCleanup
	var clearErr error
	go func() { cleared, clearErr = service.ClearCache(ctx); close(done) }()
	select {
	case <-done:
		t.Fatal("clear ignored maintenance lock")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-done
	if clearErr != nil || cleared.OrphansRemoved != 1 {
		t.Fatalf("clear = %+v, %v", cleared, clearErr)
	}
	assertRemovedRepos(t, cleared.Removed, []string{"github:acme/fresh", "github:acme/failed", "github:acme/unsafe"})
}

func assertRemovedRepos(t *testing.T, actual, expected []string) {
	t.Helper()
	found := map[string]bool{}
	for _, repoID := range actual {
		found[repoID] = true
	}
	if len(found) != len(expected) {
		t.Fatalf("removed = %v, want %v", actual, expected)
	}
	for _, repoID := range expected {
		if !found[repoID] {
			t.Fatalf("removed = %v, want %v", actual, expected)
		}
	}
}

func TestRepoLocationCachePartialFailures(t *testing.T) {
	database := storetest.Open(t)
	stale := time.Now().Add(-31 * 24 * time.Hour)
	path := filepath.Join(t.TempDir(), "api")
	mustMkdirAll(path)
	mustWriteFile(filepath.Join(path, "payload"), "bytes")
	insertRepoWithLocation(t, database, "github:acme/api", "github", "acme", "api", "cache", path, &stale)
	service := &RepoLocation{Store: repolocation.New(database.DB)}
	ctx := context.Background()
	pruned, err := service.PruneCache(ctx)
	if err != nil || len(pruned.Failed) != 1 || pruned.Failed[0].Code != "delete_failed" {
		t.Fatalf("prune = %+v, %v", pruned, err)
	}
	cleared, err := service.ClearCache(ctx)
	if err != nil || len(cleared.Failed) != 1 {
		t.Fatalf("clear = %+v, %v", cleared, err)
	}
	assertSourcePath(t, database.DB, "github:acme/api", "cache", path)
	service.DeleteCacheDir = func(string) error { return nil }
	service.SweepOrphans = func(map[string]bool) ([]string, error) {
		return []string{"already-removed"}, errors.New("sweep failed")
	}
	cleared, err = service.ClearCache(ctx)
	if err != nil || len(cleared.Removed) != 1 || len(cleared.Failed) != 1 || cleared.Failed[0].Message != "unable to sweep cache orphans" || cleared.OrphansRemoved != 1 {
		t.Fatalf("sweep partial result = %+v, %v", cleared, err)
	}
	assertSourcePath(t, database.DB, "github:acme/api", "", "")
	if _, err := database.Exec("DROP TABLE repos"); err != nil {
		t.Fatal(err)
	}
	_, err = service.PruneCache(ctx)
	assertLocationError(t, err, "delete_failed")
	_, err = service.ClearCache(ctx)
	assertLocationError(t, err, "delete_failed")
}

func TestRepoLocationDeleteCacheRepoRereadsAfterLock(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", root)
	database := storetest.Open(t)
	path := filepath.Join(root, "acme", "api")
	mustMkdirAll(path)
	insertRepoWithLocation(t, database, "github:acme/api", "github", "acme", "api", "cache", path, nil)
	locks := repolock.New()
	held := make(chan struct{})
	release := make(chan struct{})
	go func() { _ = locks.WithRepo("github:acme/api", func() error { close(held); <-release; return nil }) }()
	<-held
	service := &RepoLocation{Store: repolocation.New(database.DB), Locks: locks, DeleteCacheDir: os.RemoveAll}
	done := make(chan error, 1)
	go func() { done <- service.DeleteCacheRepo(context.Background(), RepoRef{RepoID: "github:acme/api"}) }()
	select {
	case <-done:
		t.Fatal("delete ignored lock")
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := database.Exec("UPDATE repos SET source='local' WHERE id='github:acme/api'"); err != nil {
		t.Fatal(err)
	}
	close(release)
	assertLocationError(t, <-done, "delete_failed")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stale deletion removed checkout: %v", err)
	}
}
