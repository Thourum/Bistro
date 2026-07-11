package git

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CacheRepoStat struct {
	RepoID     string
	Path       string
	SizeBytes  int64
	LastUsedAt *time.Time
	Stale      bool
}

type CacheEntryInput struct {
	RepoID     string
	Path       string
	LastUsedAt *time.Time
}

const StaleAfter = 30 * 24 * time.Hour

func CacheStats(repos []CacheEntryInput) ([]CacheRepoStat, error) {
	stats := make([]CacheRepoStat, 0, len(repos))
	for _, repo := range repos {
		canonicalPath, err := canonicalExistingPath(repo.Path)
		if err != nil {
			return nil, fmt.Errorf("canonicalize cache path %q: %w", repo.Path, err)
		}

		sizeBytes, err := dirSize(canonicalPath)
		if err != nil {
			return nil, fmt.Errorf("size cache repo %s at %q: %w", repo.RepoID, canonicalPath, err)
		}

		stats = append(stats, CacheRepoStat{
			RepoID:     repo.RepoID,
			Path:       canonicalPath,
			SizeBytes:  sizeBytes,
			LastUsedAt: repo.LastUsedAt,
			Stale:      isStale(repo.LastUsedAt),
		})
	}

	return stats, nil
}

func DeleteCacheDir(path string) error {
	root, err := canonicalCacheRoot()
	if err != nil {
		return err
	}

	canonicalPath, err := safeCacheDirPath(root, path)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(canonicalPath); err != nil {
		return fmt.Errorf("delete cache dir %q: %w", canonicalPath, err)
	}
	return nil
}

func SweepOrphans(known map[string]bool) ([]string, error) {
	root, err := canonicalCacheRoot()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read cache root %q: %w", root, err)
	}

	removed := make([]string, 0)
	for _, ownerEntry := range entries {
		if !ownerEntry.IsDir() || ownerEntry.Type()&os.ModeSymlink != 0 {
			continue
		}

		ownerPath := filepath.Join(root, ownerEntry.Name())
		repoEntries, err := os.ReadDir(ownerPath)
		if err != nil {
			return nil, fmt.Errorf("read cache owner dir %q: %w", ownerPath, err)
		}

		for _, repoEntry := range repoEntries {
			if !repoEntry.IsDir() || repoEntry.Type()&os.ModeSymlink != 0 {
				continue
			}

			repoPath := filepath.Join(ownerPath, repoEntry.Name())
			canonicalRepoPath, err := safeCacheDirPath(root, repoPath)
			if err != nil {
				continue
			}
			if known[canonicalRepoPath] {
				continue
			}

			if err := os.RemoveAll(canonicalRepoPath); err != nil {
				return removed, fmt.Errorf("delete orphan cache dir %q: %w", canonicalRepoPath, err)
			}
			removed = append(removed, canonicalRepoPath)
		}
	}

	return removed, nil
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		size += info.Size()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return size, nil
}

func isStale(lastUsedAt *time.Time) bool {
	if lastUsedAt == nil {
		return true
	}
	return time.Since(*lastUsedAt) > StaleAfter
}

func canonicalCacheRoot() (string, error) {
	root, err := CacheRoot()
	if err != nil {
		return "", fmt.Errorf("get cache root: %w", err)
	}
	canonicalRoot, err := canonicalExistingPath(root)
	if err != nil {
		return "", fmt.Errorf("canonicalize cache root %q: %w", root, err)
	}
	return canonicalRoot, nil
}

func safeCacheDirPath(root, path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("stat cache path %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("cache path %q is a symlink", path)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cache path %q is not a directory", path)
	}

	canonicalPath, err := canonicalExistingPath(path)
	if err != nil {
		return "", fmt.Errorf("canonicalize cache path %q: %w", path, err)
	}
	if canonicalPath == root {
		return "", fmt.Errorf("cache path %q is cache root", path)
	}
	if !isPathInside(root, canonicalPath) {
		return "", fmt.Errorf("cache path %q is outside cache root %q", canonicalPath, root)
	}
	if !isDepthTwoCachePath(root, canonicalPath) {
		return "", fmt.Errorf("cache path %q is not an owner/repo cache dir", canonicalPath)
	}
	return canonicalPath, nil
}

func isDepthTwoCachePath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && parts[0] != "." && parts[1] != "."
}

func canonicalExistingPath(path string) (string, error) {
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(canonical), nil
}

func isPathInside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
