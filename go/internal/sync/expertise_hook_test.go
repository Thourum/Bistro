package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"

	gitclient "github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
)

func TestFetchPRDetailsCallsExpertiseHookAfterSuccessfulDiff(t *testing.T) {
	repoPath, baseSHA, headSHA := makeExpertiseHookRepo(t)
	const repoID = "github:org/repo"

	hook := &recordingExpertiseHook{calls: make(chan expertiseHookCall, 1)}
	localResolver := &expertiseHookLocalResolver{repoURL: "https://github.com/org/repo.git", path: repoPath}
	gitClient := gitclient.NewClient(gitclient.NewCache())
	gitClient.SetLocalResolver(localResolver)

	orch := NewOrchestrator(nil, mock.NewMockProvider(), nil, gitClient).
		WithExpertise(hook).
		WithRepoPathResolver(func(id string) (string, bool) {
			if id != repoID {
				return "", false
			}
			return repoPath, true
		})

	prs := []*provider.PR{{
		ID:         repoID + ":7",
		Provider:   "github",
		Owner:      "org",
		Repo:       "repo",
		Number:     7,
		Title:      "Update app",
		Author:     "alice",
		State:      "open",
		BaseBranch: "master",
		HeadBranch: "feature",
		BaseSHA:    baseSHA,
		HeadSHA:    headSHA,
		CreatedAt:  time.Now().UTC().Add(-time.Hour),
		UpdatedAt:  time.Now().UTC(),
	}}

	results, err := orch.FetchPRDetails(context.Background(), prs, false, false, true)
	if err != nil {
		t.Fatalf("FetchPRDetails: %v", err)
	}
	if len(results) != 1 || results[0].Error != nil || !results[0].DiffComputed {
		t.Fatalf("expected successful diff result, got %#v", results)
	}

	select {
	case call := <-hook.calls:
		if call.repoID != repoID {
			t.Fatalf("repoID = %q, want %q", call.repoID, repoID)
		}
		if call.repoPath != repoPath {
			t.Fatalf("repoPath = %q, want %q", call.repoPath, repoPath)
		}
		gotFiles := append([]string(nil), call.files...)
		sort.Strings(gotFiles)
		wantFiles := []string{"docs/readme.md", "src/app.go"}
		if fmt.Sprint(gotFiles) != fmt.Sprint(wantFiles) {
			t.Fatalf("files = %v, want %v", gotFiles, wantFiles)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expertise hook was not called")
	}
}

type expertiseHookCall struct {
	repoID   string
	repoPath string
	files    []string
}

type recordingExpertiseHook struct {
	calls chan expertiseHookCall
}

func (h *recordingExpertiseHook) OnPRSynced(_ context.Context, repoID, repoPath string, files []string) {
	h.calls <- expertiseHookCall{
		repoID:   repoID,
		repoPath: repoPath,
		files:    append([]string(nil), files...),
	}
}

type expertiseHookLocalResolver struct {
	repoURL string
	path    string
}

func (r *expertiseHookLocalResolver) LocalPath(repoURL string) (string, bool) {
	return r.path, repoURL == r.repoURL
}

func (r *expertiseHookLocalResolver) OnLocalDegraded(string, string, string, string) {}
func (r *expertiseHookLocalResolver) OnUsed(string)                                  {}
func (r *expertiseHookLocalResolver) OnCacheCloned(string, string)                   {}

func makeExpertiseHookRepo(t *testing.T) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{"https://github.com/org/repo.git"}}); err != nil {
		t.Fatalf("CreateRemote: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	writeExpertiseHookFile(t, dir, "src/app.go", "package main\n\nfunc main() {}\n")
	baseSHA := commitExpertiseHookRepo(t, worktree, "base")

	writeExpertiseHookFile(t, dir, "src/app.go", "package main\n\nfunc main() { println(\"hi\") }\n")
	writeExpertiseHookFile(t, dir, "docs/readme.md", "# Docs\n")
	headSHA := commitExpertiseHookRepo(t, worktree, "head")

	return dir, baseSHA, headSHA
}

func writeExpertiseHookFile(t *testing.T, dir, relPath, body string) {
	t.Helper()
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", relPath, err)
	}
	if err := os.WriteFile(fullPath, []byte(body), 0644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
}

func commitExpertiseHookRepo(t *testing.T, worktree *gogit.Worktree, message string) string {
	t.Helper()
	if _, err := worktree.Add("."); err != nil {
		t.Fatalf("add files: %v", err)
	}
	hash, err := worktree.Commit(message, &gogit.CommitOptions{Author: &object.Signature{
		Name:  "test",
		Email: "test@example.com",
		When:  time.Now().UTC(),
	}})
	if err != nil {
		t.Fatalf("commit %q: %v", message, err)
	}
	return hash.String()
}
