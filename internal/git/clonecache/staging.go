package clonecache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/itterno/bistro/internal/git/remote"
)

// RepoDir resolves the cache directory for repoURL.
func RepoDir(repoURL string) (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	if _, hasOverride := OverrideDir(); hasOverride {
		return root, nil
	}
	identity, err := remote.ParseURL(repoURL)
	if err != nil {
		return root, nil
	}
	return safeRepoCacheDir(root, identity)
}

func safeRepoCacheDir(root string, identity remote.Identity) (string, error) {
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

// PrepareTargetedDir resolves a targeted clone path, using a temp dir when needed.
func PrepareTargetedDir(repoURL string) (string, error) {
	repoDir, err := RepoDir(repoURL)
	if err != nil {
		return "", err
	}
	if NeedsFreshTempDir(repoURL) {
		return os.MkdirTemp(repoDir, "repo-*")
	}
	return repoDir, nil
}

// NeedsFreshTempDir reports whether a clone must go into a throwaway temp dir
// instead of the canonical cache dir: either the BISTRO_REPO_DIR override is
// active, or the URL cannot be parsed into a stable identity.
func NeedsFreshTempDir(repoURL string) bool {
	if _, hasOverride := OverrideDir(); hasOverride {
		return true
	}
	_, err := remote.ParseURL(repoURL)
	return err != nil
}

// Cleanup commits or rolls back a staged clone directory.
type Cleanup struct {
	// Commit removes any backup after a successful clone.
	Commit func() error
	// Rollback removes the incomplete clone and restores any backup.
	Rollback func() error
}

// StageCloneDir prepares a fresh clone directory while preserving any existing clone.
func StageCloneDir(repoURL string) (string, Cleanup, error) {
	repoDir, err := RepoDir(repoURL)
	if err != nil {
		return "", Cleanup{}, err
	}
	if NeedsFreshTempDir(repoURL) {
		return makeTemporaryCloneDir(repoDir)
	}
	return createFreshCacheCloneDir(repoDir)
}

func makeTemporaryCloneDir(parentDir string) (string, Cleanup, error) {
	tmpDir, err := os.MkdirTemp(parentDir, "repo-*")
	if err != nil {
		return "", Cleanup{}, err
	}
	return tmpDir, Cleanup{
		Commit: func() error { return nil },
		Rollback: func() error {
			return os.RemoveAll(tmpDir)
		},
	}, nil
}

func createFreshCacheCloneDir(repoDir string) (path string, cleanup Cleanup, err error) {
	parentDir := filepath.Dir(repoDir)
	if err := os.MkdirAll(parentDir, cacheDirPerm); err != nil {
		return "", Cleanup{}, err
	}
	if err := rejectSymlink(parentDir); err != nil {
		return "", Cleanup{}, err
	}

	stagingDir, err := os.MkdirTemp(parentDir, filepath.Base(repoDir)+"-staging-*")
	if err != nil {
		return "", Cleanup{}, err
	}
	shouldRemoveStaging := true
	defer func() {
		if shouldRemoveStaging {
			// Preserve the setup failure and include a cleanup failure when both occur.
			err = errors.Join(err, os.RemoveAll(stagingDir))
		}
	}()
	if err := ensureNonSymlinkDir(stagingDir); err != nil {
		return "", Cleanup{}, err
	}

	backupDir, err := moveExistingCacheDirAside(repoDir)
	if err != nil {
		return "", Cleanup{}, err
	}
	cleanup = Cleanup{
		Commit: func() error {
			if backupDir == "" {
				return nil
			}
			return os.RemoveAll(backupDir)
		},
		Rollback: func() error {
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
		return "", Cleanup{}, errors.Join(renameErr, cleanup.Rollback())
	}
	shouldRemoveStaging = false
	if inspectErr := ensureNonSymlinkDir(repoDir); inspectErr != nil {
		return "", Cleanup{}, errors.Join(inspectErr, cleanup.Rollback())
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

// StageTargetedDir preserves an existing clone before a targeted clone attempt.
func StageTargetedDir(repoURL, repoDir string) (Cleanup, error) {
	removeOnFailure := Cleanup{
		Commit:   func() error { return nil },
		Rollback: func() error { return os.RemoveAll(repoDir) },
	}
	if NeedsFreshTempDir(repoURL) {
		return removeOnFailure, nil
	}
	if _, err := os.Stat(repoDir); err != nil {
		if os.IsNotExist(err) {
			return removeOnFailure, nil
		}
		return Cleanup{}, err
	}

	backupDir, err := os.MkdirTemp(filepath.Dir(repoDir), filepath.Base(repoDir)+"-backup-*")
	if err != nil {
		return Cleanup{}, err
	}
	if err := os.RemoveAll(backupDir); err != nil {
		return Cleanup{}, err
	}
	if err := os.Rename(repoDir, backupDir); err != nil {
		return Cleanup{}, err
	}

	return Cleanup{
		Commit: func() error {
			return os.RemoveAll(backupDir)
		},
		Rollback: func() error {
			// Keep the backup if the incomplete clone cannot be removed.
			if err := os.RemoveAll(repoDir); err != nil {
				return fmt.Errorf("remove incomplete clone %s: %w", repoDir, err)
			}
			return os.Rename(backupDir, repoDir)
		},
	}, nil
}
