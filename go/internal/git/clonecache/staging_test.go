package clonecache

import (
	"os"
	"path/filepath"
	"testing"
)

const stagingRepoURL = "https://github.com/acme/api.git"

func TestPrepareTargetedCloneDirUsesOwnerRepoLayout(t *testing.T) {
	// Arrange
	home := t.TempDir()
	cacheRoot := filepath.Join(home, "cache")
	t.Setenv("BISTRO_REPO_DIR", "")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", cacheRoot)

	// Act
	clonePath, err := PrepareTargetedDir(stagingRepoURL)

	// Assert
	if err != nil {
		t.Fatalf("PrepareTargetedDir: %v", err)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("UserCacheDir: %v", err)
	}
	canonicalCacheDir, err := filepath.EvalSymlinks(cacheDir)
	if err != nil {
		t.Fatalf("EvalSymlinks cacheDir: %v", err)
	}
	expectedPath := filepath.Join(canonicalCacheDir, "bistro", "repos", "acme", "api")
	if clonePath != expectedPath {
		t.Fatalf("expected targeted cache path %q, got %q", expectedPath, clonePath)
	}
}

func TestPrepareCacheCloneDirRejectsSymlinkedOwner(t *testing.T) {
	// Arrange
	home := t.TempDir()
	outsideDir := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", "")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("UserCacheDir: %v", err)
	}
	cacheRoot := filepath.Join(cacheDir, "bistro", "repos")
	if err := os.MkdirAll(cacheRoot, 0755); err != nil {
		t.Fatalf("MkdirAll cacheRoot: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(cacheRoot, "acme")); err != nil {
		t.Fatalf("Symlink owner: %v", err)
	}

	// Act
	clonePath, _, err := StageCloneDir(stagingRepoURL)

	// Assert
	if err == nil {
		t.Fatalf("expected symlink rejection, got path %q", clonePath)
	}
	if entries, readErr := os.ReadDir(outsideDir); readErr != nil || len(entries) != 0 {
		t.Fatalf("expected outside dir untouched, entries=%d err=%v", len(entries), readErr)
	}
}

func TestPrepareCacheCloneDirRejectsSymlinkedRepo(t *testing.T) {
	// Arrange
	home := t.TempDir()
	outsideDir := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", "")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("UserCacheDir: %v", err)
	}
	ownerDir := filepath.Join(cacheDir, "bistro", "repos", "acme")
	if err := os.MkdirAll(ownerDir, 0755); err != nil {
		t.Fatalf("MkdirAll ownerDir: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(ownerDir, "api")); err != nil {
		t.Fatalf("Symlink repo: %v", err)
	}

	// Act
	clonePath, _, err := StageCloneDir(stagingRepoURL)

	// Assert
	if err == nil {
		t.Fatalf("expected symlink rejection, got path %q", clonePath)
	}
	if entries, readErr := os.ReadDir(outsideDir); readErr != nil || len(entries) != 0 {
		t.Fatalf("expected outside dir untouched, entries=%d err=%v", len(entries), readErr)
	}
}

func TestPrepareCacheCloneDirCreatesNonSymlinkRepoDir(t *testing.T) {
	// Arrange
	home := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", "")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))

	// Act
	clonePath, _, err := StageCloneDir(stagingRepoURL)

	// Assert
	if err != nil {
		t.Fatalf("StageCloneDir: %v", err)
	}
	info, err := os.Lstat(clonePath)
	if err != nil {
		t.Fatalf("expected clone dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected clone path to be a directory, mode=%s", info.Mode())
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("expected clone path not to be a symlink")
	}
}

func TestPrepareCacheCloneDirUsesOwnerRepoLayout(t *testing.T) {
	// Arrange
	home := t.TempDir()
	cacheRoot := filepath.Join(home, "cache")
	t.Setenv("BISTRO_REPO_DIR", "")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", cacheRoot)

	// Act
	clonePath, _, err := StageCloneDir(stagingRepoURL)

	// Assert
	if err != nil {
		t.Fatalf("StageCloneDir: %v", err)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("UserCacheDir: %v", err)
	}
	canonicalCacheDir, err := filepath.EvalSymlinks(cacheDir)
	if err != nil {
		t.Fatalf("EvalSymlinks cacheDir: %v", err)
	}
	expectedPath := filepath.Join(canonicalCacheDir, "bistro", "repos", "acme", "api")
	if clonePath != expectedPath {
		t.Fatalf("expected cache clone path %q, got %q", expectedPath, clonePath)
	}
	if len(filepath.Base(clonePath)) >= 5 && filepath.Base(clonePath)[:5] == "repo-" {
		t.Fatalf("expected owner/repo path, got temp child path %q", clonePath)
	}
}
