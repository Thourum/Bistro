package clonecache_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/git/clonecache"
	"github.com/itterno/bistro/internal/git/remote"
	"github.com/itterno/bistro/internal/logging"
)

const forcePushRepoURL = "https://github.com/acme/widgets.git"

// forcePushFixture is a local remote with PR #7 at head1, already cloned into
// the canonical cache dir of forcePushRepoURL. The clone's origin points at the
// local remote, so refreshes never touch the network.
type forcePushFixture struct {
	remote    *gitlib.Repository
	remoteDir string
	cacheDir  string
	opts      *git.CloneOptions
	baseSHA   string
	head1     string
}

func newForcePushFixture(t *testing.T) forcePushFixture {
	t.Helper()
	remoteDir := t.TempDir()
	const prNumber = 7
	baseBranch, baseSHA, head1 := buildRemoteWithPRRef(t, remoteDir, prNumber)
	t.Setenv("BISTRO_REPO_DIR", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "")
	cacheDir, err := clonecache.RepoDir(forcePushRepoURL)
	if err != nil {
		t.Fatalf("RepoDir: %v", err)
	}
	if _, err := gitlib.PlainClone(cacheDir, true, &gitlib.CloneOptions{URL: "file://" + remoteDir}); err != nil {
		t.Fatalf("seed cache clone: %v", err)
	}
	remote, err := gitlib.PlainOpen(remoteDir)
	if err != nil {
		t.Fatalf("open remote: %v", err)
	}
	return forcePushFixture{
		remote: remote, remoteDir: remoteDir, cacheDir: cacheDir,
		opts:    &git.CloneOptions{PRNumber: prNumber, BaseBranch: baseBranch},
		baseSHA: baseSHA, head1: head1,
	}
}

// forcePush rewrites PR #7 to a new head on top of base and rewinds the base
// branch, so head1 is unreachable from every remote ref. Returns head2.
func forcePush(t *testing.T, fixture forcePushFixture) string {
	t.Helper()
	worktree, err := fixture.remote.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := worktree.Checkout(&gitlib.CheckoutOptions{
		Hash: plumbing.NewHash(fixture.baseSHA), Branch: plumbing.NewBranchReferenceName("rewritten"), Create: true,
	}); err != nil {
		t.Fatalf("Checkout base: %v", err)
	}
	head2 := commitFile(t, fixture.remote, fixture.remoteDir, "foo.go", "package main\n\nvar x = 2\n", "rewritten head")
	for name, target := range map[plumbing.ReferenceName]plumbing.Hash{
		plumbing.ReferenceName("refs/pull/7/head"):               head2,
		plumbing.NewBranchReferenceName(fixture.opts.BaseBranch): plumbing.NewHash(fixture.baseSHA),
	} {
		if err := fixture.remote.Storer.SetReference(plumbing.NewHashReference(name, target)); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}
	return head2.String()
}

func syncHead(t *testing.T, client *git.Client, fixture forcePushFixture, headSHA string) {
	t.Helper()
	err := client.WithPRClone(context.Background(), forcePushRepoURL, fixture.baseSHA, headSHA, fixture.opts,
		func(clone *git.CloneResult) error {
			if clone.LocalPath != fixture.cacheDir {
				t.Errorf("clone path = %s, want the existing cache dir %s", clone.LocalPath, fixture.cacheDir)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("WithPRClone %s: %v", logging.ShortSHA(headSHA), err)
	}
}

func TestForcePushKeepsPreviousHeadPinned(t *testing.T) {
	fixture := newForcePushFixture(t)
	client := git.NewClient(clonecache.New())

	syncHead(t, client, fixture, fixture.head1)
	head2 := forcePush(t, fixture)
	syncHead(t, client, fixture, head2)

	repo, err := gitlib.PlainOpen(fixture.cacheDir)
	if err != nil {
		t.Fatalf("open cache clone: %v", err)
	}
	identity, err := remote.ParseURL(forcePushRepoURL)
	if err != nil {
		t.Fatalf("ParseURL: %v", err)
	}
	for _, head := range []string{fixture.head1, head2} {
		refName := plumbing.ReferenceName(fmt.Sprintf("refs/bistro/%s/pull/%d/heads/%s", remote.SafeRepoID(identity), 7, head))
		ref, err := repo.Reference(refName, true)
		if err != nil || ref.Hash().String() != head {
			t.Errorf("pin for %s: ref=%v err=%v", logging.ShortSHA(head), ref, err)
		}
		if _, err := repo.CommitObject(plumbing.NewHash(head)); err != nil {
			t.Errorf("commit %s missing from cache clone: %v", logging.ShortSHA(head), err)
		}
	}
}

// buildRemoteWithPRRef creates an on-disk repo at dir with two commits on the
// base branch (base then head), and publishes the head commit under
// refs/pull/<prNumber>/head so a targeted fetch can resolve it. Returns the
// base branch name, baseSHA, and headSHA.
func buildRemoteWithPRRef(t *testing.T, dir string, prNumber int) (baseBranch, baseSHA, headSHA string) {
	t.Helper()
	repo, err := gitlib.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	sig := &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()}

	writeAndCommit := func(content, msg string) plumbing.Hash {
		if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := wt.Add("foo.go"); err != nil {
			t.Fatalf("Add: %v", err)
		}
		h, err := wt.Commit(msg, &gitlib.CommitOptions{Author: sig})
		if err != nil {
			t.Fatalf("Commit %q: %v", msg, err)
		}
		return h
	}

	baseHash := writeAndCommit("package main\n", "base")
	headHash := writeAndCommit("package main\n\nvar x = 1\n", "head")

	// Determine the current (default) branch name to use as the base branch.
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	baseBranch = head.Name().Short()

	// Publish the head commit under the PR head ref, mirroring GitHub's
	// refs/pull/<N>/head. This is what the targeted fetch resolves.
	prRef := plumbing.NewHashReference(
		plumbing.ReferenceName(fmt.Sprintf("refs/pull/%d/head", prNumber)), headHash)
	if err := repo.Storer.SetReference(prRef); err != nil {
		t.Fatalf("SetReference pr ref: %v", err)
	}

	return baseBranch, baseHash.String(), headHash.String()
}

func commitFile(t *testing.T, repo *gitlib.Repository, repoPath, fileName, content, message string) plumbing.Hash {
	t.Helper()
	filePath := filepath.Join(repoPath, fileName)
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := worktree.Add(fileName); err != nil {
		t.Fatalf("Add: %v", err)
	}
	hash, err := worktree.Commit(message, &gitlib.CommitOptions{Author: &object.Signature{
		Name:  "Bistro Test",
		Email: "bistro@example.com",
		When:  time.Now(),
	}})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return hash
}
