package clonecache

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RepoStat describes a cached repository's size, age, and safety checks.
type RepoStat struct {
	RepoID     string
	Path       string
	SizeBytes  int64
	LastUsedAt *time.Time
	Stale      bool
	Skipped    bool
	Error      string
}

// EntryInput identifies a registered cache directory for maintenance.
type EntryInput struct {
	RepoID     string
	Path       string
	LastUsedAt *time.Time
}

// StaleAfter is the age at which an unused cache entry becomes eligible for pruning.
const StaleAfter = 30 * 24 * time.Hour

// Stats measures registered cache entries without deleting them.
func Stats(repos []EntryInput) ([]RepoStat, error) {
	root, err := canonicalRoot()
	if err != nil {
		return nil, err
	}

	stats := make([]RepoStat, 0, len(repos))
	for _, repo := range repos {
		stat := RepoStat{
			RepoID:     repo.RepoID,
			Path:       filepath.Clean(repo.Path),
			LastUsedAt: repo.LastUsedAt,
			Stale:      isStale(repo.LastUsedAt),
		}

		canonicalPath, err := safeCacheDirPath(root, repo.Path)
		if err != nil {
			stats = append(stats, skippedCacheStat(stat, err))
			continue
		}
		stat.Path = canonicalPath

		sizeBytes, err := dirSize(canonicalPath)
		if err != nil {
			stats = append(stats, skippedCacheStat(stat, fmt.Errorf("size cache repo %s at %q: %w", repo.RepoID, canonicalPath, err)))
			continue
		}

		stat.SizeBytes = sizeBytes
		stats = append(stats, stat)
	}

	return stats, nil
}

// DeleteDir removes a safe cache directory under Root.
// Registered legacy directories can be supplied when they lack owner/repo depth.
func DeleteDir(path string, registeredLegacyPaths ...map[string]bool) error {
	root, err := canonicalRoot()
	if err != nil {
		return err
	}

	canonicalPath, err := safeCacheDirPath(root, path)
	if err != nil {
		return err
	}
	if !isDepthTwoCachePath(root, canonicalPath) && !isRegisteredLegacyPath(registeredLegacyPaths, canonicalPath) {
		return fmt.Errorf("cache path %q is not an owner/repo cache dir or registered legacy cache dir", canonicalPath)
	}

	if err := os.RemoveAll(canonicalPath); err != nil {
		return fmt.Errorf("delete cache dir %q: %w", canonicalPath, err)
	}
	return nil
}

// SweepOrphans removes owner/repo cache directories absent from known.
// Callers must hold the maintenance lock while sweeping live cache entries.
func SweepOrphans(known map[string]bool) ([]string, error) {
	root, err := canonicalRoot()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read cache root %q: %w", root, err)
	}

	removed := make([]string, 0)
	skipped := make([]error, 0)
	for _, ownerEntry := range entries {
		ownerPath := filepath.Join(root, ownerEntry.Name())
		if !ownerEntry.IsDir() || ownerEntry.Type()&os.ModeSymlink != 0 {
			skipped = append(skipped, fmt.Errorf("skip cache owner entry %q: not a directory", ownerPath))
			continue
		}

		repoEntries, err := os.ReadDir(ownerPath)
		if err != nil {
			skipped = append(skipped, fmt.Errorf("skip cache owner dir %q: %w", ownerPath, err))
			continue
		}

		for _, repoEntry := range repoEntries {
			repoPath := filepath.Join(ownerPath, repoEntry.Name())
			if !repoEntry.IsDir() || repoEntry.Type()&os.ModeSymlink != 0 {
				skipped = append(skipped, fmt.Errorf("skip cache repo entry %q: not a directory", repoPath))
				continue
			}

			canonicalRepoPath, err := safeCacheDirPath(root, repoPath)
			if err != nil {
				skipped = append(skipped, fmt.Errorf("skip cache repo entry %q: %w", repoPath, err))
				continue
			}
			if !isDepthTwoCachePath(root, canonicalRepoPath) {
				skipped = append(skipped, fmt.Errorf("skip cache repo entry %q: not an owner/repo cache dir", repoPath))
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

	return removed, errors.Join(skipped...)
}

func skippedCacheStat(stat RepoStat, err error) RepoStat {
	return RepoStat{
		RepoID:     stat.RepoID,
		Path:       stat.Path,
		SizeBytes:  0,
		LastUsedAt: stat.LastUsedAt,
		Stale:      stat.Stale,
		Skipped:    true,
		Error:      err.Error(),
	}
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

func canonicalRoot() (string, error) {
	root, err := Root()
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
	return canonicalPath, nil
}

func isRegisteredLegacyPath(registeredLegacyPaths []map[string]bool, path string) bool {
	for _, registered := range registeredLegacyPaths {
		if registered[path] {
			return true
		}
	}
	return false
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
