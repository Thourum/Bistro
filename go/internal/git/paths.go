package git

import (
	"os"
	"path/filepath"
)

// CacheRoot returns the absolute directory for cloned repos.
// Priority: BISTRO_REPO_DIR env var > user cache dir / bistro/repos.
func CacheRoot() (string, error) {
	if envDir := os.Getenv("BISTRO_REPO_DIR"); envDir != "" {
		if err := os.MkdirAll(envDir, 0755); err != nil {
			return "", err
		}
		return envDir, nil
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	repoDir := filepath.Join(cacheDir, "bistro", "repos")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return "", err
	}

	return repoDir, nil
}

// getRepoDir returns the absolute directory for cloned repos.
func getRepoDir() (string, error) {
	return CacheRoot()
}

// getPatchDir returns the absolute directory for patch files.
// Priority: BISTRO_PATCH_DIR env var > user cache dir / bistro/patches.
func getPatchDir() (string, error) {
	if envDir := os.Getenv("BISTRO_PATCH_DIR"); envDir != "" {
		if err := os.MkdirAll(envDir, 0755); err != nil {
			return "", err
		}
		return envDir, nil
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	patchDir := filepath.Join(cacheDir, "bistro", "patches")
	if err := os.MkdirAll(patchDir, 0755); err != nil {
		return "", err
	}

	return patchDir, nil
}
