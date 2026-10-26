package diff_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/git/diff"
	"github.com/itterno/bistro/internal/git/repolock"
	"github.com/itterno/bistro/internal/logging"
)

func TestInterdiffAfterForcePush(t *testing.T) {
	fixture := newForcePushFixture(t)
	client := git.NewClient(git.NewCache())
	locks := repolock.New()
	client.SetLocks(locks)
	syncHead(t, client, fixture, fixture.head1)
	head2 := forcePush(t, fixture)
	syncHead(t, client, fixture, head2)
	request := diff.InterdiffRequest{
		RepoURL: forcePushRepoURL, RepoPath: fixture.cacheDir, Path: "foo.go",
		FromHead: fixture.head1, ToHead: head2,
		FromMergeBase: fixture.baseSHA, ToMergeBase: fixture.baseSHA,
	}

	result, err := diff.Interdiff(locks, request)
	if err != nil {
		t.Fatalf("Interdiff: %v", err)
	}
	if !strings.Contains(result.Patch, "-var x = 1") || !strings.Contains(result.Patch, "+var x = 2") {
		t.Fatalf("patch = %q, want head1 → head2 change", result.Patch)
	}
	if result.Rebased {
		t.Error("Rebased = true with identical merge bases")
	}

	// head1 changed foo.go relative to base, so treating them as the two merge
	// bases means upstream touched the file.
	request.ToMergeBase = fixture.head1
	if result, err = diff.Interdiff(locks, request); err != nil || !result.Rebased {
		t.Errorf("upstream-changed file: rebased=%v err=%v, want rebased", result.Rebased, err)
	}

	for _, tt := range []struct {
		name          string
		path          string
		fromHead      string
		toHead        string
		fromMergeBase string
		toMergeBase   string
		wantPatch     string
		wantRebased   bool
	}{
		{name: "identical heads", path: "foo.go", fromHead: head2, toHead: head2, fromMergeBase: fixture.baseSHA, toMergeBase: fixture.baseSHA},
		{name: "absent file", path: "missing.go", fromHead: fixture.head1, toHead: head2, fromMergeBase: fixture.baseSHA, toMergeBase: fixture.head1},
		{name: "absent directory", path: "missing/file.go", fromHead: fixture.head1, toHead: head2, fromMergeBase: fixture.baseSHA, toMergeBase: fixture.head1},
		{name: "unknown merge base", path: "foo.go", fromHead: head2, toHead: head2, toMergeBase: fixture.head1},
		{name: "unreadable merge base", path: "foo.go", fromHead: head2, toHead: head2, fromMergeBase: strings.Repeat("ab", 20), toMergeBase: fixture.baseSHA, wantRebased: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := diff.Interdiff(locks, diff.InterdiffRequest{
				RepoURL: request.RepoURL, RepoPath: request.RepoPath, Path: tt.path,
				FromHead: tt.fromHead, ToHead: tt.toHead,
				FromMergeBase: tt.fromMergeBase, ToMergeBase: tt.toMergeBase,
			})
			if err != nil || got.Patch != tt.wantPatch || got.Rebased != tt.wantRebased {
				t.Errorf("Interdiff = %+v, err=%v; want patch=%q rebased=%v", got, err, tt.wantPatch, tt.wantRebased)
			}
		})
	}

	request.FromHead = strings.Repeat("ab", 20)
	if _, err := diff.Interdiff(locks, request); !errors.Is(err, diff.ErrHeadUnavailable) {
		t.Errorf("missing head: err = %v, want ErrHeadUnavailable", err)
	}
	request.FromHead, request.ToHead = fixture.head1, request.FromHead
	if _, err := diff.Interdiff(locks, request); !errors.Is(err, diff.ErrHeadUnavailable) {
		t.Errorf("missing target head: err = %v, want ErrHeadUnavailable", err)
	}
}

func TestInterdiffAddedAndDeletedFile(t *testing.T) {
	fixture := newForcePushFixture(t)
	locks := repolock.New()
	const filePath = "added.go"
	addedHead := commitFile(t, fixture.remote, fixture.remoteDir, filePath, "package added\n", "add file").String()
	for _, tt := range []struct {
		name     string
		fromHead string
		toHead   string
		wantLine string
	}{
		{name: "addition", fromHead: fixture.baseSHA, toHead: addedHead, wantLine: "+package added"},
		{name: "deletion", fromHead: addedHead, toHead: fixture.baseSHA, wantLine: "-package added"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := diff.Interdiff(locks, diff.InterdiffRequest{
				RepoURL: forcePushRepoURL, RepoPath: fixture.remoteDir, Path: filePath,
				FromHead: tt.fromHead, ToHead: tt.toHead,
				FromMergeBase: tt.fromHead, ToMergeBase: tt.toHead,
			})
			if err != nil || !strings.Contains(result.Patch, tt.wantLine) || !result.Rebased {
				t.Fatalf("Interdiff = %+v, err=%v; want %q and rebased", result, err, tt.wantLine)
			}
			if strings.Contains(result.Patch, "foo.go") {
				t.Errorf("patch includes an unrelated changed file: %q", result.Patch)
			}
		})
	}
}

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
	remote, err := gitlib.PlainInit(remoteDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	baseSHA := commitFile(t, remote, remoteDir, "foo.go", "package main\n", "base")
	head1 := commitFile(t, remote, remoteDir, "foo.go", "package main\n\nvar x = 1\n", "head")
	head, err := remote.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if err := remote.Storer.SetReference(plumbing.NewHashReference("refs/pull/7/head", head1)); err != nil {
		t.Fatalf("SetReference pr ref: %v", err)
	}
	t.Setenv("BISTRO_REPO_DIR", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "")
	root, err := git.CacheRoot()
	if err != nil {
		t.Fatalf("CacheRoot: %v", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve cache root %s: %v", root, err)
	}
	cacheDir := filepath.Join(canonicalRoot, "acme", "widgets")
	if _, err := gitlib.PlainClone(cacheDir, true, &gitlib.CloneOptions{URL: "file://" + remoteDir}); err != nil {
		t.Fatalf("seed cache clone: %v", err)
	}
	return forcePushFixture{
		remote: remote, remoteDir: remoteDir, cacheDir: cacheDir,
		opts:    &git.CloneOptions{PRNumber: prNumber, BaseBranch: head.Name().Short()},
		baseSHA: baseSHA.String(), head1: head1.String(),
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

func commitFile(t *testing.T, repo *gitlib.Repository, repoPath, fileName, content, message string) plumbing.Hash {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoPath, fileName), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := worktree.Add(fileName); err != nil {
		t.Fatalf("Add: %v", err)
	}
	hash, err := worktree.Commit(message, &gitlib.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()}})
	if err != nil {
		t.Fatalf("Commit %q: %v", message, err)
	}
	return hash
}
