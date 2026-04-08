package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetRepoDir(t *testing.T) {
	// Unset env var to test default behaviour
	t.Setenv("BISTRO_REPO_DIR", "")

	dir, err := getRepoDir()
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

func TestGetPatchDir(t *testing.T) {
	t.Setenv("BISTRO_PATCH_DIR", "")

	dir, err := getPatchDir()
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

	dir, err := getRepoDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != tmp {
		t.Errorf("expected env override %s, got %s", tmp, dir)
	}
}

func TestEnvVarOverridePatchDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BISTRO_PATCH_DIR", tmp)

	dir, err := getPatchDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != tmp {
		t.Errorf("expected env override %s, got %s", tmp, dir)
	}
}

func TestRepoDirIsAbsoluteNotRelative(t *testing.T) {
	t.Setenv("BISTRO_REPO_DIR", "")

	dir, err := getRepoDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must not start with "data/" (old relative path bug)
	if len(dir) >= 5 && dir[:5] == "data/" {
		t.Errorf("path must not be relative, got %s", dir)
	}
}
