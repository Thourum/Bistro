package git

import (
	"os"
	"path/filepath"
)

// getRepoDir returns the absolute directory for cloned repos.
// Priority: BISTRO_REPO_DIR env var > working directory / data/repos.
func getRepoDir() (string, error) {
	if envDir := os.Getenv("BISTRO_REPO_DIR"); envDir != "" {
		if err := os.MkdirAll(envDir, 0755); err != nil {
			return "", err
		}
		return envDir, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	repoDir := filepath.Join(wd, "data", "repos")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return "", err
	}

	return repoDir, nil
}

// getPatchDir returns the absolute directory for patch files.
// Priority: BISTRO_PATCH_DIR env var > working directory / data/patches.
func getPatchDir() (string, error) {
	if envDir := os.Getenv("BISTRO_PATCH_DIR"); envDir != "" {
		if err := os.MkdirAll(envDir, 0755); err != nil {
			return "", err
		}
		return envDir, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	patchDir := filepath.Join(wd, "data", "patches")
	if err := os.MkdirAll(patchDir, 0755); err != nil {
		return "", err
	}

	return patchDir, nil
}
