package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/itterno/bistro/internal/git"
)

const (
	MaxCandidates = 500
	MaxDuration   = 30 * time.Second
)

var maxCandidates = MaxCandidates

// Found is a local GitHub repo found under a dev folder.
type Found struct {
	Identity git.Identity
	Path     string // canonical
}

// Result is the outcome of scanning a dev folder.
type Result struct {
	Found   []Found
	Partial bool
	Skipped int
}

// ValidateDevFolder canonicalizes and rejects root/home/system dirs,
// non-dirs, unreadable dirs. Returns the canonical path.
func ValidateDevFolder(path string) (string, error) {
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("validate dev folder %q: %w", path, err)
	}

	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("validate dev folder %q: %w", canonical, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("validate dev folder %q: not a directory", canonical)
	}

	if err := rejectUnsafeDevFolder(canonical); err != nil {
		return "", err
	}

	entries, err := os.ReadDir(canonical)
	if err != nil {
		return "", fmt.Errorf("validate dev folder %q: unreadable: %w", canonical, err)
	}
	if entries == nil {
		return "", fmt.Errorf("validate dev folder %q: unreadable", canonical)
	}

	return canonical, nil
}

// Scan walks depth<=2 (children + one nested level) for .git dirs.
// Symlinked dirs are not followed. Bare repos/submodules skipped
// (a .git *file* = submodule/worktree pointer → skip).
func Scan(ctx context.Context, devFolder string) (Result, error) {
	canonical, err := ValidateDevFolder(devFolder)
	if err != nil {
		return Result{}, err
	}

	scanCtx, cancel := context.WithTimeout(ctx, MaxDuration)
	defer cancel()

	result := Result{Found: []Found{}}
	candidatesExamined := 0
	if err := scanChildren(scanCtx, canonical, 1, &result, &candidatesExamined); err != nil {
		return Result{}, err
	}

	return result, nil
}

func scanChildren(ctx context.Context, dir string, depth int, result *Result, candidatesExamined *int) error {
	if err := ctx.Err(); err != nil {
		result.Partial = true
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		result.Partial = true
		result.Skipped++
		return nil
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			result.Partial = true
			return nil
		}
		if !entry.IsDir() || isSymlink(entry) {
			continue
		}

		candidatePath := filepath.Join(dir, entry.Name())
		if hasGitDirectory(candidatePath) {
			addCandidate(candidatePath, result, candidatesExamined)
			continue
		}
		if depth < 2 {
			if err := scanChildren(ctx, candidatePath, depth+1, result, candidatesExamined); err != nil {
				return err
			}
		}
	}

	return nil
}

func addCandidate(path string, result *Result, candidatesExamined *int) {
	(*candidatesExamined)++
	if *candidatesExamined > maxCandidates {
		result.Partial = true
		result.Skipped++
		return
	}

	identity, ok := repoIdentity(path)
	if !ok {
		result.Skipped++
		return
	}

	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		result.Skipped++
		return
	}

	result.Found = append(result.Found, Found{Identity: identity, Path: canonical})
}

func repoIdentity(path string) (git.Identity, bool) {
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return git.Identity{}, false
	}

	remotes, err := repo.Remotes()
	if err != nil {
		return git.Identity{}, false
	}

	originURLs := []string{}
	fallbackURLs := []string{}
	for _, remote := range remotes {
		config := remote.Config()
		if config.Name == "origin" {
			originURLs = append(originURLs, config.URLs...)
			continue
		}
		fallbackURLs = append(fallbackURLs, config.URLs...)
	}

	if len(originURLs) > 0 {
		return firstParseable(originURLs)
	}
	return firstParseable(fallbackURLs)
}

func firstParseable(urls []string) (git.Identity, bool) {
	for _, rawURL := range urls {
		identity, err := git.ParseRemoteURL(rawURL)
		if err == nil {
			return identity, true
		}
	}
	return git.Identity{}, false
}

func hasGitDirectory(path string) bool {
	info, err := os.Lstat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir()
}

func isSymlink(entry os.DirEntry) bool {
	return entry.Type()&os.ModeSymlink != 0
}

func rejectUnsafeDevFolder(path string) error {
	cleanPath := filepath.Clean(path)
	if cleanPath == filepath.VolumeName(cleanPath)+string(filepath.Separator) {
		return fmt.Errorf("validate dev folder %q: root directory is not allowed", path)
	}

	homeDir, err := os.UserHomeDir()
	if err == nil {
		canonicalHome, homeErr := filepath.EvalSymlinks(homeDir)
		if homeErr == nil && cleanPath == filepath.Clean(canonicalHome) {
			return fmt.Errorf("validate dev folder %q: home directory is not allowed", path)
		}
	}

	for _, systemDir := range systemDirs() {
		if cleanPath == systemDir {
			return fmt.Errorf("validate dev folder %q: system directory is not allowed", path)
		}
	}

	return nil
}

func systemDirs() []string {
	if runtime.GOOS == "windows" {
		return []string{`C:\Windows`, `C:\Program Files`, `C:\Program Files (x86)`}
	}
	return []string{"/System", "/Library", "/Applications", "/bin", "/etc", "/sbin", "/usr", "/var"}
}
