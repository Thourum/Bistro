package clonecache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheRoot(t *testing.T) {
	// Unset env var to test default behaviour
	t.Setenv("BISTRO_REPO_DIR", "")

	dir, err := Root()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path, got %s", dir)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("expected directory to exist at %s", dir)
	}
}

func TestEnvVarOverrideRepoDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", tmp)

	dir, err := Root()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != tmp {
		t.Errorf("expected env override %s, got %s", tmp, dir)
	}
}
