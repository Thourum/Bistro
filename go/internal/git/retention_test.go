package git

import (
	"context"
	"testing"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

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
	opts      *CloneOptions
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
	cacheDir, err := repoCacheDir(forcePushRepoURL)
	if err != nil {
		t.Fatalf("repoCacheDir: %v", err)
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
		opts:    &CloneOptions{PRNumber: prNumber, BaseBranch: baseBranch},
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

func syncHead(t *testing.T, client *Client, fixture forcePushFixture, headSHA string) {
	t.Helper()
	err := client.WithPRClone(context.Background(), forcePushRepoURL, fixture.baseSHA, headSHA, fixture.opts,
		func(clone *CloneResult) error {
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
	client := NewClient(NewCache())

	syncHead(t, client, fixture, fixture.head1)
	head2 := forcePush(t, fixture)
	syncHead(t, client, fixture, head2)

	repo, err := gitlib.PlainOpen(fixture.cacheDir)
	if err != nil {
		t.Fatalf("open cache clone: %v", err)
	}
	for _, head := range []string{fixture.head1, head2} {
		ref, err := repo.Reference(pinnedHeadRef(forcePushRepoURL, 7, head), true)
		if err != nil || ref.Hash().String() != head {
			t.Errorf("pin for %s: ref=%v err=%v", logging.ShortSHA(head), ref, err)
		}
		if _, err := repo.CommitObject(plumbing.NewHash(head)); err != nil {
			t.Errorf("commit %s missing from cache clone: %v", logging.ShortSHA(head), err)
		}
	}
}
