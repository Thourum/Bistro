package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	bistrogit "github.com/itterno/bistro/internal/git"
)

func TestScanFindsDepthOneAndTwoAndIgnoresDepthThree(t *testing.T) {
	// Arrange
	devFolder := t.TempDir()
	createRepo(t, filepath.Join(devFolder, "api"), "origin", "https://github.com/acme/api.git")
	createRepo(t, filepath.Join(devFolder, "team", "web"), "origin", "git@github.com:acme/web.git")
	createRepo(t, filepath.Join(devFolder, "team", "nested", "tool"), "origin", "https://github.com/acme/tool.git")

	// Act
	result, err := Scan(context.Background(), devFolder)

	// Assert
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	assertFoundIDs(t, result, []string{"github:acme/api", "github:acme/web"})
	if result.Partial {
		t.Fatalf("expected full scan, got partial result")
	}
	if result.Skipped != 0 {
		t.Fatalf("expected no skipped candidates, got %d", result.Skipped)
	}
}

func TestScanSkipsSymlinkedDir(t *testing.T) {
	// Arrange
	devFolder := t.TempDir()
	realRepo := createRepo(t, filepath.Join(t.TempDir(), "api"), "origin", "https://github.com/acme/api.git")
	linkPath := filepath.Join(devFolder, "api-link")
	if err := os.Symlink(realRepo, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	// Act
	result, err := Scan(context.Background(), devFolder)

	// Assert
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	assertFoundIDs(t, result, []string{})
}

func TestScanSkipsRepoWithNonGitHubRemoteAndNoRemote(t *testing.T) {
	// Arrange
	devFolder := t.TempDir()
	createRepo(t, filepath.Join(devFolder, "gitlab"), "origin", "https://gitlab.com/acme/api.git")
	createRepoWithoutRemote(t, filepath.Join(devFolder, "empty"))

	// Act
	result, err := Scan(context.Background(), devFolder)

	// Assert
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	assertFoundIDs(t, result, []string{})
}

func TestScanPrefersOriginWhenMultipleRemotesExist(t *testing.T) {
	// Arrange
	devFolder := t.TempDir()
	repo := createRepoWithoutRemote(t, filepath.Join(devFolder, "api"))
	createRemote(t, repo, "upstream", "https://github.com/acme/wrong.git")
	createRemote(t, repo, "origin", "https://github.com/acme/api.git")

	// Act
	result, err := Scan(context.Background(), devFolder)

	// Assert
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	assertFoundIDs(t, result, []string{"github:acme/api"})
}

func TestScanFallsBackToFirstParseableRemoteWhenOriginIsAbsent(t *testing.T) {
	// Arrange
	devFolder := t.TempDir()
	repo := createRepoWithoutRemote(t, filepath.Join(devFolder, "api"))
	createRemote(t, repo, "gitlab", "https://gitlab.com/acme/wrong.git")
	createRemote(t, repo, "upstream", "https://github.com/acme/api.git")

	// Act
	result, err := Scan(context.Background(), devFolder)

	// Assert
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	assertFoundIDs(t, result, []string{"github:acme/api"})
}

func TestScanSkipsRepoWhenOriginIsPresentButInvalid(t *testing.T) {
	// Arrange
	devFolder := t.TempDir()
	repo := createRepoWithoutRemote(t, filepath.Join(devFolder, "api"))
	createRemote(t, repo, "origin", "https://gitlab.com/acme/api.git")
	createRemote(t, repo, "upstream", "https://github.com/acme/api.git")

	// Act
	result, err := Scan(context.Background(), devFolder)

	// Assert
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	assertFoundIDs(t, result, []string{})
	if result.Skipped != 1 {
		t.Fatalf("expected one skipped candidate, got %d", result.Skipped)
	}
}

func TestValidateDevFolderRejectsUnsafePaths(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	devFolder := t.TempDir()
	filePath := filepath.Join(devFolder, "file.txt")
	if err := os.WriteFile(filePath, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "root", path: string(filepath.Separator)},
		{name: "home", path: homeDir},
		{name: "missing", path: filepath.Join(devFolder, "missing")},
		{name: "file", path: filePath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := ValidateDevFolder(tt.path)

			// Assert
			if err == nil {
				t.Fatalf("expected ValidateDevFolder(%q) to fail", tt.path)
			}
		})
	}
}

func TestValidateDevFolderRejectsUnreadableDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod unreadable semantics differ on Windows")
	}

	// Arrange
	devFolder := filepath.Join(t.TempDir(), "unreadable")
	if err := os.Mkdir(devFolder, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Chmod(devFolder, 0); err != nil {
		t.Fatalf("Chmod unreadable: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(devFolder, 0o700)
	})

	// Act
	_, err := ValidateDevFolder(devFolder)

	// Assert
	if err == nil {
		t.Fatalf("expected unreadable dir to be rejected")
	}
}

func TestScanMarksPartialWhenCandidateLimitIsExceeded(t *testing.T) {
	// Arrange
	devFolder := t.TempDir()
	createRepo(t, filepath.Join(devFolder, "one"), "origin", "https://github.com/acme/one.git")
	createRepo(t, filepath.Join(devFolder, "two"), "origin", "https://github.com/acme/two.git")
	createRepo(t, filepath.Join(devFolder, "three"), "origin", "https://github.com/acme/three.git")

	originalMaxCandidates := maxCandidates
	maxCandidates = 2
	t.Cleanup(func() {
		maxCandidates = originalMaxCandidates
	})

	// Act
	result, err := Scan(context.Background(), devFolder)

	// Assert
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !result.Partial {
		t.Fatalf("expected partial result")
	}
	if result.Skipped < 1 {
		t.Fatalf("expected at least one skipped candidate, got %d", result.Skipped)
	}
	if len(result.Found) != 2 {
		t.Fatalf("expected two found repos, got %d", len(result.Found))
	}
}

func TestScanMarksPartialWhenInvalidCandidatesExceedLimit(t *testing.T) {
	// Arrange
	devFolder := t.TempDir()
	createRepo(t, filepath.Join(devFolder, "a-gitlab-one"), "origin", "https://gitlab.com/acme/one.git")
	createRepo(t, filepath.Join(devFolder, "b-gitlab-two"), "origin", "https://gitlab.com/acme/two.git")
	createRepo(t, filepath.Join(devFolder, "c-github-three"), "origin", "https://github.com/acme/three.git")

	originalMaxCandidates := maxCandidates
	maxCandidates = 2
	t.Cleanup(func() {
		maxCandidates = originalMaxCandidates
	})

	// Act
	result, err := Scan(context.Background(), devFolder)

	// Assert
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !result.Partial {
		t.Fatalf("expected partial result")
	}
	if len(result.Found) != 0 {
		t.Fatalf("expected no found repos after candidate cap, got %d", len(result.Found))
	}
	if result.Skipped < 3 {
		t.Fatalf("expected all candidates to be skipped or capped, got %d", result.Skipped)
	}
}

func TestScanFindsNonBareCloneCreatedForDevFolderDestination(t *testing.T) {
	tests := []struct {
		name string
		bare bool
	}{
		{name: "dev folder clone", bare: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			remoteDir := t.TempDir()
			createCloneableRepo(t, remoteDir, "origin", "https://github.com/acme/api.git")
			devFolder := t.TempDir()
			destination := filepath.Join(devFolder, "acme", "api")
			client := bistrogit.NewClient(bistrogit.NewCache())

			// Act
			cloneErr := client.CloneRepoToPath(context.Background(), "file://"+remoteDir, destination, tt.bare)
			if cloneErr == nil {
				clonedRepo, openErr := gogit.PlainOpen(destination)
				if openErr != nil {
					t.Fatalf("PlainOpen cloned repo: %v", openErr)
				}
				if deleteErr := clonedRepo.DeleteRemote("origin"); deleteErr != nil {
					t.Fatalf("DeleteRemote origin: %v", deleteErr)
				}
				createRemote(t, clonedRepo, "origin", "https://github.com/acme/api.git")
			}
			result, scanErr := Scan(context.Background(), devFolder)

			// Assert
			if cloneErr != nil {
				t.Fatalf("CloneRepoToPath: %v", cloneErr)
			}
			if scanErr != nil {
				t.Fatalf("Scan: %v", scanErr)
			}
			assertFoundIDs(t, result, []string{"github:acme/api"})
		})
	}
}

func TestScanSkipsBareReposAndGitPointerFiles(t *testing.T) {
	// Arrange
	devFolder := t.TempDir()
	barePath := filepath.Join(devFolder, "bare.git")
	if _, err := gogit.PlainInit(barePath, true); err != nil {
		t.Fatalf("PlainInit bare: %v", err)
	}
	pointerPath := filepath.Join(devFolder, "pointer")
	if err := os.MkdirAll(pointerPath, 0o700); err != nil {
		t.Fatalf("MkdirAll pointer: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pointerPath, ".git"), []byte("gitdir: ../real/.git\n"), 0o600); err != nil {
		t.Fatalf("WriteFile .git pointer: %v", err)
	}
	createRepo(t, filepath.Join(devFolder, "real"), "origin", "https://github.com/acme/real.git")

	// Act
	result, err := Scan(context.Background(), devFolder)

	// Assert
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	assertFoundIDs(t, result, []string{"github:acme/real"})
	if result.Skipped != 0 {
		t.Fatalf("expected bare repo and pointer file to be ignored, got skipped %d", result.Skipped)
	}
}

func createRepo(t *testing.T, path string, remoteName string, remoteURL string) string {
	t.Helper()
	repo := createRepoWithoutRemote(t, path)
	createRemote(t, repo, remoteName, remoteURL)
	return path
}

func createCloneableRepo(t *testing.T, path string, remoteName string, remoteURL string) string {
	t.Helper()
	repo := createRepoWithoutRemote(t, path)
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "README.md"), []byte("# test repo\n"), 0o600); err != nil {
		t.Fatalf("WriteFile README: %v", err)
	}
	if _, err := worktree.Add("README.md"); err != nil {
		t.Fatalf("Add README: %v", err)
	}
	_, err = worktree.Commit("initial commit", &gogit.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()}})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	createRemote(t, repo, remoteName, remoteURL)
	return path
}

func createRepoWithoutRemote(t *testing.T, path string) *gogit.Repository {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	repo, err := gogit.PlainInit(path, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	return repo
}

func createRemote(t *testing.T, repo *gogit.Repository, name string, remoteURL string) {
	t.Helper()
	_, err := repo.CreateRemote(&config.RemoteConfig{Name: name, URLs: []string{remoteURL}})
	if err != nil && !errors.Is(err, gogit.ErrRemoteExists) {
		t.Fatalf("CreateRemote(%s): %v", name, err)
	}
}

func assertFoundIDs(t *testing.T, result Result, expected []string) {
	t.Helper()
	actual := make(map[string]string, len(result.Found))
	for _, found := range result.Found {
		actual[found.Identity.ID()] = found.Path
		if !filepath.IsAbs(found.Path) {
			t.Fatalf("expected canonical absolute path, got %q", found.Path)
		}
	}
	if len(actual) != len(expected) {
		t.Fatalf("expected IDs %v, got %v", expected, actual)
	}
	for _, id := range expected {
		if _, ok := actual[id]; !ok {
			t.Fatalf("expected ID %q in %v", id, actual)
		}
	}
}
