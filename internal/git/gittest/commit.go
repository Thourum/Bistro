// Package gittest provides on-disk Git fixtures shared by Git package tests.
package gittest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const (
	fixtureDirMode  = 0755
	fixtureFileMode = 0644
)

// FileCommit describes a file change to commit in a fixture repository.
type FileCommit struct {
	RepoPath string
	FileName string
	Content  string
	Message  string
}

// CommitFile writes and stages a file, then commits it with a fixture author.
// It creates missing parent directories and fails the test on any Git or I/O error.
func CommitFile(t testing.TB, repo *gitlib.Repository, commit FileCommit) plumbing.Hash {
	t.Helper()
	filePath := filepath.Join(commit.RepoPath, commit.FileName)
	if err := os.MkdirAll(filepath.Dir(filePath), fixtureDirMode); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(commit.Content), fixtureFileMode); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := worktree.Add(commit.FileName); err != nil {
		t.Fatalf("Add: %v", err)
	}
	hash, err := worktree.Commit(commit.Message, &gitlib.CommitOptions{Author: &object.Signature{
		Name:  "Bistro Test",
		Email: "bistro@example.com",
		When:  time.Now(),
	}})
	if err != nil {
		t.Fatalf("Commit %q: %v", commit.Message, err)
	}
	return hash
}
