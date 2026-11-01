package clonecache

import (
	"os"
	"path/filepath"
)

// cacheDirPerm allows other local users to inspect cache paths, matching the
// existing cache layout and preserving permissions of previously created dirs.
const cacheDirPerm = 0755

// OverrideDir returns the repo-directory override and whether it is non-empty.
func OverrideDir() (string, bool) {
	dir := os.Getenv("BISTRO_REPO_DIR")
	return dir, dir != ""
}

// Root returns the absolute directory for cloned repos.
// Priority: BISTRO_REPO_DIR env var > user cache dir / bistro/repos.
func Root() (string, error) {
	if envDir, hasOverride := OverrideDir(); hasOverride {
		if err := os.MkdirAll(envDir, cacheDirPerm); err != nil {
			return "", err
		}
		return envDir, nil
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	repoDir := filepath.Join(cacheDir, "bistro", "repos")
	if err := os.MkdirAll(repoDir, cacheDirPerm); err != nil {
		return "", err
	}

	return repoDir, nil
}
