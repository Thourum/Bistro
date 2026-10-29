package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/git/clonecache"
	"github.com/itterno/bistro/internal/store/repolocation"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestRepoLocationRecovery(t *testing.T) {
	root := t.TempDir()
	database := storetest.Open(t)
	locations := repolocation.New(database.DB)
	insertRepoWithLocation(t, database, "github:acme/api", "github", "acme", "api", "local", filepath.Join(root, "missing"), nil)
	if err := locations.MarkDegraded(context.Background(), "github:acme/api", "missing", "path_missing", "folder not found"); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(root, "good")
	bad := filepath.Join(root, "bad")
	if err := createRepo(good, "origin", "https://github.com/acme/api.git"); err != nil {
		t.Fatal(err)
	}
	if err := createRepo(bad, "origin", "https://github.com/acme/bad.git"); err != nil {
		t.Fatal(err)
	}
	cloner := &fakeRepoLocationCloner{}
	cacheRoot := filepath.Join(root, "cache")
	mustMkdirAll(cacheRoot)
	service := &RepoLocation{DB: database.DB, Store: locations, Clone: cloner.CloneRepo, CacheRoot: func() (string, error) { return cacheRoot, nil }}
	ctx := context.Background()
	req := RepoRef{RepoID: "github:ACME/API"}
	cloner.overrideRemoteURL = "https://github.com/acme/other.git"
	assertLocationError(t, service.RecoverToCache(ctx, req), "clone_failed")
	assertLocationStatus(t, locations, "github:acme/api", "missing", "path_missing")
	cachePath := filepath.Join(cacheRoot, "acme", "api")
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("mismatched clone not cleaned: %v", err)
	}
	cloner.overrideRemoteURL = ""
	assertLocationError(t, service.RecoverPath(ctx, RecoverPathRequest{RepoID: req.RepoID, Path: bad}), "remote_mismatch")
	if err := service.RecoverPath(ctx, RecoverPathRequest{RepoID: req.RepoID, Path: good}); err != nil {
		t.Fatal(err)
	}
	assertLocationStatus(t, locations, "github:acme/api", "ok", "")
	assertLocationError(t, service.RecoverPath(ctx, RecoverPathRequest{RepoID: req.RepoID, Path: filepath.Join(root, "missing")}), "path_missing")
	file := filepath.Join(root, "file")
	mustWriteFile(file, "not a repository")
	assertLocationError(t, service.RecoverPath(ctx, RecoverPathRequest{RepoID: req.RepoID, Path: file}), "path_missing")
	mustMkdirAll(cachePath)
	cloner.err = errors.New("clone failed")
	assertLocationError(t, service.RecoverToCache(ctx, req), "clone_failed")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("preexisting clone directory lost: %v", err)
	}
	if err := os.RemoveAll(cachePath); err != nil {
		t.Fatal(err)
	}
	cloner.err = nil
	if err := service.RecoverToCache(ctx, req); err != nil {
		t.Fatal(err)
	}
	assertSourcePath(t, database.DB, "github:acme/api", "cache", cachePath)
	assertLocationStatus(t, locations, "github:acme/api", "ok", "")
}

func TestRepoLocationMissingAndStorageErrors(t *testing.T) {
	database := storetest.Open(t)
	service := &RepoLocation{DB: database.DB, Store: repolocation.New(database.DB)}
	ctx := context.Background()
	for _, call := range []func() error{
		func() error { return service.RecoverPath(ctx, RecoverPathRequest{RepoID: "github:acme/missing"}) },
		func() error { return service.RecoverToCache(ctx, RepoRef{RepoID: "github:acme/missing"}) },
		func() error { return service.DeleteCacheRepo(ctx, RepoRef{RepoID: "github:acme/missing"}) },
	} {
		assertServiceCode(t, call(), CodeNotFound)
	}
	if _, err := database.Exec("DROP TABLE repos"); err != nil {
		t.Fatal(err)
	}
	assertServiceCode(t, service.RecoverPath(ctx, RecoverPathRequest{RepoID: "github:acme/api"}), CodeInternal)
	assertServiceCode(t, service.RecoverToCache(ctx, RepoRef{RepoID: "github:acme/api"}), CodeInternal)
	assertServiceCode(t, service.DeleteCacheRepo(ctx, RepoRef{RepoID: "github:acme/api"}), CodeInternal)
}

func TestRepoLocationCloneCallerCancellation(t *testing.T) {
	t.Setenv("BISTRO_REPO_DIR", "")
	database := storetest.Open(t)
	cacheRoot := t.TempDir()
	entered := make(chan struct{})
	done := make(chan error, 1)
	service := &RepoLocation{DB: database.DB, Store: repolocation.New(database.DB), CacheRoot: func() (string, error) { return cacheRoot, nil }, Clone: func(ctx context.Context, _, _ string, _ bool) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_, err := service.AddByURL(ctx, AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: locationStringPtr("cache")})
		done <- err
	}()
	const cancellationTestTimeout = 5 * time.Second
	select {
	case <-entered:
	case <-time.After(cancellationTestTimeout):
		t.Fatal("clone did not start")
	}
	cancel()
	select {
	case err := <-done:
		assertLocationError(t, err, "cancelled")
	case <-time.After(cancellationTestTimeout):
		t.Fatal("clone did not honour cancellation")
	}
	if _, err := os.Stat(filepath.Join(cacheRoot, "acme", "api")); !os.IsNotExist(err) {
		t.Fatalf("cancelled clone not cleaned: %v", err)
	}
	service.CacheRoot = func() (string, error) { return "", context.DeadlineExceeded }
	_, err := service.AddByURL(context.Background(), AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: locationStringPtr("cache")})
	assertServiceCode(t, err, CodeTimeout)
}

func TestRepoLocationAddUsesCacheOverride(t *testing.T) {
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "cache")
	t.Setenv("BISTRO_REPO_DIR", cacheRoot)
	dev := filepath.Join(root, "dev")
	if err := createRepo(filepath.Join(dev, "acme", "api"), "origin", "https://github.com/acme/api.git"); err != nil {
		t.Fatal(err)
	}
	database := storetest.Open(t)
	cloner := &fakeRepoLocationCloner{}
	service := &RepoLocation{DB: database.DB, Store: repolocation.New(database.DB), Clone: cloner.CloneRepo, CacheRoot: clonecache.Root}
	result, err := service.AddByURL(context.Background(), AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: locationStringPtr("dev_folder"), DevFolderPath: &dev})
	if err != nil || result.Source != "cache" {
		t.Fatalf("override add = %+v, %v", result, err)
	}
	assertSourcePath(t, database.DB, "github:acme/api", "cache", filepath.Join(cacheRoot, "acme", "api"))
	calls := cloner.Calls()
	if len(calls) != 1 || !calls[0].Bare {
		t.Fatalf("expected bare cache clone: %+v", calls)
	}
}
