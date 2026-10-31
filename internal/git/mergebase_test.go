package git

import (
	"context"
	"errors"
	"testing"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	gitdiff "github.com/itterno/bistro/internal/git/diff"
	"github.com/itterno/bistro/internal/git/gittest"
)

// Upstream-only changes must not leak into the PR diff.
func TestMergeBaseDiffShowsOnlyPRChanges(t *testing.T) {
	dir := t.TempDir()
	repo, err := gitlib.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	root := gittest.CommitFile(t, repo, gittest.FileCommit{RepoPath: dir, FileName: "shared.txt", Content: "v1\n", Message: "root"})
	head := gittest.CommitFile(t, repo, gittest.FileCommit{RepoPath: dir, FileName: "pr.txt", Content: "pr\n", Message: "pr change"})

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := worktree.Checkout(&gitlib.CheckoutOptions{
		Hash: root, Branch: plumbing.NewBranchReferenceName("base-tip"), Create: true,
	}); err != nil {
		t.Fatalf("Checkout base-tip: %v", err)
	}
	baseTip := gittest.CommitFile(t, repo, gittest.FileCommit{RepoPath: dir, FileName: "upstream.txt", Content: "upstream\n", Message: "upstream change"})

	mergeBase, err := MergeBase(dir, baseTip.String(), head.String())
	if err != nil {
		t.Fatalf("MergeBase: %v", err)
	}
	if mergeBase != root.String() {
		t.Fatalf("MergeBase = %s, want root %s", mergeBase, root)
	}
	diff, err := gitdiff.Generate(dir, mergeBase, head.String(), gitdiff.DefaultOptions())
	if err != nil {
		t.Fatalf("GenerateDiff: %v", err)
	}
	if len(diff.Files) != 1 || diff.Files[0].Path != "pr.txt" {
		paths := make([]string, 0, len(diff.Files))
		for _, file := range diff.Files {
			paths = append(paths, file.Path)
		}
		t.Errorf("merge-base diff files = %v, want [pr.txt]", paths)
	}
}

func TestMergeBaseUnrelatedHistoriesNotFound(t *testing.T) {
	dir := t.TempDir()
	repo, err := gitlib.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	base := gittest.CommitFile(t, repo, gittest.FileCommit{RepoPath: dir, FileName: "a.txt", Content: "a\n", Message: "a"})
	// Point HEAD at an unborn branch so the next commit has no parents.
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD,
		plumbing.NewBranchReferenceName("unrelated"))); err != nil {
		t.Fatalf("SetReference unrelated HEAD: %v", err)
	}
	unrelated := gittest.CommitFile(t, repo, gittest.FileCommit{RepoPath: dir, FileName: "b.txt", Content: "b\n", Message: "unrelated root"})
	const missingCommitSHA = "1111111111111111111111111111111111111111"
	for _, test := range []struct {
		name string
		base string
		head string
	}{
		{name: "missing head", base: base.String(), head: missingCommitSHA},
		{name: "missing base", base: missingCommitSHA, head: base.String()},
		{name: "unrelated histories", base: base.String(), head: unrelated.String()},
	} {
		t.Run(test.name, func(t *testing.T) {
			mergeBase, err := MergeBase(dir, test.base, test.head)
			if !errors.Is(err, ErrMergeBaseNotFound) || mergeBase != "" {
				t.Errorf("MergeBase = %q, %v; want empty SHA and ErrMergeBaseNotFound", mergeBase, err)
			}
		})
	}
}

func TestDeepenMakesMergeBaseReachable(t *testing.T) {
	fixture := newLocalFetchFixture(t)
	const prNumber = 123
	head := fixture.commitUpstreamBranch(t, "feature", "pr.txt", "pr\n", "pr change")
	fixture.setUpstreamPRRef(t, prNumber, head)
	baseTip := fixture.commitUpstreamBranch(t, fixture.baseBranch, "upstream.txt", "upstream\n", "upstream change")

	dir := t.TempDir()
	repo, err := initTargetedRepo(dir, "file://"+fixture.upstreamPath)
	if err != nil {
		t.Fatalf("initTargetedRepo: %v", err)
	}
	client := NewClient(nil)
	opts := &CloneOptions{PRNumber: prNumber, BaseBranch: fixture.baseBranch}
	if err := client.fetchTargetedRefs(context.Background(), repo, opts, 1); err != nil {
		t.Fatalf("fetchTargetedRefs: %v", err)
	}
	if _, err := MergeBase(dir, baseTip.String(), head.String()); !errors.Is(err, ErrMergeBaseNotFound) {
		t.Fatalf("shallow MergeBase error = %v, want ErrMergeBaseNotFound", err)
	}

	if err := client.Deepen(context.Background(), dir, opts); err != nil {
		t.Fatalf("Deepen: %v", err)
	}
	mergeBase, err := MergeBase(dir, baseTip.String(), head.String())
	if err != nil || mergeBase != fixture.baseSHA.String() {
		t.Errorf("deepened MergeBase = %q, %v; want %s, nil", mergeBase, err, fixture.baseSHA)
	}
}

func TestDeepenLeavesFullCloneAlone(t *testing.T) {
	dir := t.TempDir()
	repo, err := gitlib.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	gittest.CommitFile(t, repo, gittest.FileCommit{RepoPath: dir, FileName: "a.txt", Content: "a\n", Message: "root"})
	// No origin exists: a full clone must not attempt a fetch.
	if err := NewClient(nil).Deepen(context.Background(), dir, nil); err != nil {
		t.Fatalf("Deepen full clone: %v", err)
	}
}
