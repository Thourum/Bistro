package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func repoCacheDir(repoURL string) (string, error) {
	root, err := CacheRoot()
	if err != nil {
		return "", err
	}
	if os.Getenv("BISTRO_REPO_DIR") != "" {
		return root, nil
	}
	identity, err := ParseRemoteURL(repoURL)
	if err != nil {
		return root, nil
	}
	return safeRepoCacheDir(root, identity)
}

func safeRepoCacheDir(root string, identity Identity) (string, error) {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve cache root %s: %w", root, err)
	}
	ownerDir := filepath.Join(canonicalRoot, identity.Owner)
	if err := rejectSymlink(ownerDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(ownerDir, cacheDirPerm); err != nil {
		return "", err
	}
	if err := rejectSymlink(ownerDir); err != nil {
		return "", err
	}
	repoDir := filepath.Join(ownerDir, identity.Repo)
	if err := rejectSymlink(repoDir); err != nil {
		return "", err
	}
	if err := ensureContainedPath(canonicalRoot, repoDir); err != nil {
		return "", err
	}
	return repoDir, nil
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect cache path %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("cache path %s is a symlink", path)
	}
	return nil
}

func ensureContainedPath(root, child string) error {
	relative, err := filepath.Rel(root, child)
	if err != nil {
		return fmt.Errorf("compare cache path %s to root %s: %w", child, root, err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("cache path %s escapes cache root %s", child, root)
	}
	return nil
}

func prepareTargetedCloneDir(repoURL string) (string, error) {
	repoDir, err := repoCacheDir(repoURL)
	if err != nil {
		return "", err
	}
	if needsFreshTempDir(repoURL) {
		return os.MkdirTemp(repoDir, "repo-*")
	}
	return repoDir, nil
}

// needsFreshTempDir reports whether a clone must go into a throwaway temp dir
// instead of the canonical cache dir: either the BISTRO_REPO_DIR override is
// active, or the URL cannot be parsed into a stable identity.
func needsFreshTempDir(repoURL string) bool {
	if os.Getenv("BISTRO_REPO_DIR") != "" {
		return true
	}
	_, err := ParseRemoteURL(repoURL)
	return err != nil
}

type cloneCleanup struct {
	commit   func() error
	rollback func() error
}

func stageCacheCloneDir(repoURL string) (string, cloneCleanup, error) {
	repoDir, err := repoCacheDir(repoURL)
	if err != nil {
		return "", cloneCleanup{}, err
	}
	if needsFreshTempDir(repoURL) {
		return makeTemporaryCloneDir(repoDir)
	}
	return createFreshCacheCloneDir(repoDir)
}

func makeTemporaryCloneDir(parentDir string) (string, cloneCleanup, error) {
	tmpDir, err := os.MkdirTemp(parentDir, "repo-*")
	if err != nil {
		return "", cloneCleanup{}, err
	}
	return tmpDir, cloneCleanup{
		commit: func() error { return nil },
		rollback: func() error {
			return os.RemoveAll(tmpDir)
		},
	}, nil
}

func createFreshCacheCloneDir(repoDir string) (path string, cleanup cloneCleanup, err error) {
	parentDir := filepath.Dir(repoDir)
	if err := os.MkdirAll(parentDir, cacheDirPerm); err != nil {
		return "", cloneCleanup{}, err
	}
	if err := rejectSymlink(parentDir); err != nil {
		return "", cloneCleanup{}, err
	}

	stagingDir, err := os.MkdirTemp(parentDir, filepath.Base(repoDir)+"-staging-*")
	if err != nil {
		return "", cloneCleanup{}, err
	}
	shouldRemoveStaging := true
	defer func() {
		if shouldRemoveStaging {
			// Preserve the setup failure and include a cleanup failure when both occur.
			err = errors.Join(err, os.RemoveAll(stagingDir))
		}
	}()
	if err := ensureNonSymlinkDir(stagingDir); err != nil {
		return "", cloneCleanup{}, err
	}

	backupDir, err := moveExistingCacheDirAside(repoDir)
	if err != nil {
		return "", cloneCleanup{}, err
	}
	cleanup = cloneCleanup{
		commit: func() error {
			if backupDir == "" {
				return nil
			}
			return os.RemoveAll(backupDir)
		},
		rollback: func() error {
			removeErr := os.RemoveAll(repoDir)
			if backupDir == "" {
				return removeErr
			}
			// Never delete the backup merely because the incomplete clone cannot be removed.
			if removeErr != nil {
				return fmt.Errorf("remove incomplete clone %s: %w", repoDir, removeErr)
			}
			return os.Rename(backupDir, repoDir)
		},
	}

	if renameErr := os.Rename(stagingDir, repoDir); renameErr != nil {
		return "", cloneCleanup{}, errors.Join(renameErr, cleanup.rollback())
	}
	shouldRemoveStaging = false
	if inspectErr := ensureNonSymlinkDir(repoDir); inspectErr != nil {
		return "", cloneCleanup{}, errors.Join(inspectErr, cleanup.rollback())
	}
	return repoDir, cleanup, nil
}

func moveExistingCacheDirAside(repoDir string) (string, error) {
	info, err := os.Lstat(repoDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("cache path %s is a symlink", repoDir)
	}
	backupDir, err := os.MkdirTemp(filepath.Dir(repoDir), filepath.Base(repoDir)+"-backup-*")
	if err != nil {
		return "", err
	}
	if err := os.RemoveAll(backupDir); err != nil {
		return "", err
	}
	if err := os.Rename(repoDir, backupDir); err != nil {
		return "", err
	}
	return backupDir, nil
}

func ensureNonSymlinkDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("cache path %s is a symlink", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("cache path %s is not a directory", path)
	}
	return nil
}

func stageTargetedCloneDir(repoURL, repoDir string) (cloneCleanup, error) {
	removeOnFailure := cloneCleanup{
		commit:   func() error { return nil },
		rollback: func() error { return os.RemoveAll(repoDir) },
	}
	if needsFreshTempDir(repoURL) {
		return removeOnFailure, nil
	}
	if _, err := os.Stat(repoDir); err != nil {
		if os.IsNotExist(err) {
			return removeOnFailure, nil
		}
		return cloneCleanup{}, err
	}

	backupDir, err := os.MkdirTemp(filepath.Dir(repoDir), filepath.Base(repoDir)+"-backup-*")
	if err != nil {
		return cloneCleanup{}, err
	}
	if err := os.RemoveAll(backupDir); err != nil {
		return cloneCleanup{}, err
	}
	if err := os.Rename(repoDir, backupDir); err != nil {
		return cloneCleanup{}, err
	}

	return cloneCleanup{
		commit: func() error {
			return os.RemoveAll(backupDir)
		},
		rollback: func() error {
			// Keep the backup if the incomplete clone cannot be removed.
			if err := os.RemoveAll(repoDir); err != nil {
				return fmt.Errorf("remove incomplete clone %s: %w", repoDir, err)
			}
			return os.Rename(backupDir, repoDir)
		},
	}, nil
}
