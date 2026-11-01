package clonecache

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestCacheStatsSumsNestedFilesAndMarksStaleRepos(t *testing.T) {
	// Arrange
	cacheRoot := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", cacheRoot)

	freshUsedAt := time.Now().Add(-StaleAfter + time.Hour)
	staleUsedAt := time.Now().Add(-StaleAfter - 24*time.Hour)
	freshRepo := filepath.Join(cacheRoot, "acme", "api")
	staleRepo := filepath.Join(cacheRoot, "acme", "old")
	neverUsedRepo := filepath.Join(cacheRoot, "acme", "never")
	writeTestFile(t, filepath.Join(freshRepo, "objects", "pack", "pack-1"), []byte("12345"))
	writeTestFile(t, filepath.Join(freshRepo, "refs", "bistro", "main"), []byte("123"))
	writeTestFile(t, filepath.Join(staleRepo, "objects", "pack", "pack-2"), []byte("1234567"))
	writeTestFile(t, filepath.Join(neverUsedRepo, "HEAD"), []byte("ref: refs/heads/main\n"))

	repos := []EntryInput{
		{RepoID: "github:acme/api", Path: freshRepo, LastUsedAt: &freshUsedAt},
		{RepoID: "github:acme/old", Path: staleRepo, LastUsedAt: &staleUsedAt},
		{RepoID: "github:acme/never", Path: neverUsedRepo},
	}

	// Act
	stats, err := Stats(repos)

	// Assert
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if len(stats) != 3 {
		t.Fatalf("Stats() returned %d stats, want 3", len(stats))
	}
	assertCacheStat(t, stats[0], "github:acme/api", freshRepo, 8, false)
	assertCacheStat(t, stats[1], "github:acme/old", staleRepo, 7, true)
	assertCacheStat(t, stats[2], "github:acme/never", neverUsedRepo, 21, true)
}

func TestCacheStatsSkipsUnsafeRowsWithoutSizingTargets(t *testing.T) {
	// Arrange
	cacheRoot := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", cacheRoot)

	validRepo := filepath.Join(cacheRoot, "acme", "api")
	symlinkTarget := filepath.Join(t.TempDir(), "outside-target")
	symlinkRepo := filepath.Join(cacheRoot, "acme", "linked")
	outsideRepo := filepath.Join(t.TempDir(), "acme", "tampered")
	writeTestFile(t, filepath.Join(validRepo, "objects", "pack"), []byte("valid"))
	writeTestFile(t, filepath.Join(symlinkTarget, "objects", "pack"), []byte("must-not-count"))
	writeTestFile(t, filepath.Join(outsideRepo, "objects", "pack"), []byte("must-not-count"))
	if err := os.Symlink(symlinkTarget, symlinkRepo); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	repos := []EntryInput{
		{RepoID: "github:acme/api", Path: validRepo},
		{RepoID: "github:acme/linked", Path: symlinkRepo},
		{RepoID: "github:acme/tampered", Path: outsideRepo},
	}

	// Act
	stats, err := Stats(repos)

	// Assert
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if len(stats) != 3 {
		t.Fatalf("Stats() returned %d stats, want 3", len(stats))
	}
	assertCacheStat(t, stats[0], "github:acme/api", validRepo, 5, true)
	for _, stat := range stats[1:] {
		if !stat.Skipped {
			t.Fatalf("Stats() stat for %s Skipped = false, want true", stat.RepoID)
		}
		if stat.SizeBytes != 0 {
			t.Fatalf("Stats() stat for %s SizeBytes = %d, want 0", stat.RepoID, stat.SizeBytes)
		}
		if stat.Error == "" {
			t.Fatalf("Stats() stat for %s Error = empty, want safety error", stat.RepoID)
		}
	}
}

func TestCacheDeleteDirSafetyChecks(t *testing.T) {
	tests := []struct {
		name        string
		path        func(t *testing.T, cacheRoot string) string
		known       func(t *testing.T, path string) map[string]bool
		wantDeleted bool
	}{
		{
			name: "refuses symlink path",
			path: func(t *testing.T, cacheRoot string) string {
				target := filepath.Join(cacheRoot, "acme", "target")
				mustMkdirAll(t, target)
				link := filepath.Join(cacheRoot, "acme", "link")
				if err := os.Symlink(target, link); err != nil {
					t.Fatalf("create symlink: %v", err)
				}
				return link
			},
		},
		{
			name: "refuses path outside root",
			path: func(t *testing.T, cacheRoot string) string {
				outside := filepath.Join(t.TempDir(), "acme", "api")
				mustMkdirAll(t, outside)
				return outside
			},
		},
		{
			name: "refuses root itself",
			path: func(t *testing.T, cacheRoot string) string {
				return cacheRoot
			},
		},
		{
			name: "refuses owner dir without repo segment",
			path: func(t *testing.T, cacheRoot string) string {
				ownerDir := filepath.Join(cacheRoot, "acme")
				mustMkdirAll(t, ownerDir)
				return ownerDir
			},
		},
		{
			name: "deletes valid owner repo dir",
			path: func(t *testing.T, cacheRoot string) string {
				repoDir := filepath.Join(cacheRoot, "acme", "api")
				writeTestFile(t, filepath.Join(repoDir, "objects", "pack"), []byte("pack"))
				return repoDir
			},
			wantDeleted: true,
		},
		{
			name: "deletes registered legacy path",
			path: func(t *testing.T, cacheRoot string) string {
				legacyDir := filepath.Join(cacheRoot, "repo-legacy")
				writeTestFile(t, filepath.Join(legacyDir, "objects", "pack"), []byte("pack"))
				return legacyDir
			},
			known: func(t *testing.T, path string) map[string]bool {
				return map[string]bool{canonicalTestPath(t, path): true}
			},
			wantDeleted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cacheRoot := t.TempDir()
			t.Setenv("BISTRO_REPO_DIR", cacheRoot)
			path := tt.path(t, cacheRoot)

			// Act
			known := map[string]bool{}
			if tt.known != nil {
				known = tt.known(t, path)
			}

			// Act
			err := DeleteDir(path, known)

			// Assert
			if tt.wantDeleted {
				if err != nil {
					t.Fatalf("DeleteDir() error = %v", err)
				}
				if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
					t.Fatalf("DeleteDir() left %q on disk, stat err = %v", path, statErr)
				}
				return
			}
			if err == nil {
				t.Fatal("DeleteDir() error = nil, want safety error")
			}
			if _, statErr := os.Lstat(path); statErr != nil {
				t.Fatalf("DeleteDir() removed/refused path unexpectedly, stat err = %v", statErr)
			}
		})
	}
}

func TestCacheSweepOrphansRemovesUnknownKeepsKnownAndReportsSkippedEntries(t *testing.T) {
	// Arrange
	cacheRoot := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", cacheRoot)
	knownRepo := filepath.Join(cacheRoot, "acme", "api")
	unknownRepo := filepath.Join(cacheRoot, "acme", "old")
	symlinkTarget := filepath.Join(t.TempDir(), "target")
	symlinkRepo := filepath.Join(cacheRoot, "acme", "linked")
	fileRepo := filepath.Join(cacheRoot, "acme", "file")
	ownerFile := filepath.Join(cacheRoot, "owner-file")
	writeTestFile(t, filepath.Join(knownRepo, "objects", "pack"), []byte("known"))
	writeTestFile(t, filepath.Join(unknownRepo, "objects", "pack"), []byte("unknown"))
	writeTestFile(t, fileRepo, []byte("not a dir"))
	writeTestFile(t, ownerFile, []byte("not a dir"))
	mustMkdirAll(t, symlinkTarget)
	if err := os.Symlink(symlinkTarget, symlinkRepo); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	known := map[string]bool{canonicalTestPath(t, knownRepo): true}
	unknownCanonicalPath := canonicalTestPath(t, unknownRepo)

	// Act
	removed, err := SweepOrphans(known)

	// Assert
	if err == nil {
		t.Fatal("SweepOrphans() error = nil, want skipped entry report")
	}
	sort.Strings(removed)
	if len(removed) != 1 || removed[0] != unknownCanonicalPath {
		t.Fatalf("SweepOrphans() removed = %#v, want only %q", removed, unknownCanonicalPath)
	}
	for _, want := range []string{symlinkRepo, fileRepo, ownerFile} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("SweepOrphans() error = %q, want skipped path %q", err.Error(), want)
		}
	}
	if _, err := os.Lstat(knownRepo); err != nil {
		t.Fatalf("known repo was removed: %v", err)
	}
	if _, err := os.Lstat(unknownRepo); !os.IsNotExist(err) {
		t.Fatalf("unknown repo still exists, stat err = %v", err)
	}
	info, err := os.Lstat(symlinkRepo)
	if err != nil {
		t.Fatalf("symlink repo was removed: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink repo mode = %v, want symlink", info.Mode())
	}
}

func assertCacheStat(t *testing.T, got RepoStat, repoID, path string, sizeBytes int64, stale bool) {
	t.Helper()
	if got.RepoID != repoID {
		t.Fatalf("RepoID = %q, want %q", got.RepoID, repoID)
	}
	if got.Path != canonicalTestPath(t, path) {
		t.Fatalf("Path = %q, want %q", got.Path, canonicalTestPath(t, path))
	}
	if got.SizeBytes != sizeBytes {
		t.Fatalf("SizeBytes = %d, want %d", got.SizeBytes, sizeBytes)
	}
	if got.Stale != stale {
		t.Fatalf("Stale = %v, want %v", got.Stale, stale)
	}
}

func writeTestFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, contents, 0644); err != nil {
		t.Fatalf("write test file %q: %v", path, err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir %q: %v", path, err)
	}
}

func canonicalTestPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		canonical, err = filepath.Abs(path)
	}
	if err != nil {
		t.Fatalf("canonical path %q: %v", path, err)
	}
	return canonical
}
