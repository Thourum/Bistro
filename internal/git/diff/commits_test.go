package diff

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"

	"github.com/itterno/bistro/internal/git/repolock"
)

type commitFixture struct {
	t    *testing.T
	path string
	repo *gitlib.Repository
	tick time.Time
}

func newCommitFixture(t *testing.T) *commitFixture {
	t.Helper()
	path := t.TempDir()
	repo, err := gitlib.PlainInit(path, false)
	if err != nil {
		t.Fatal(err)
	}
	return &commitFixture{t: t, path: path, repo: repo, tick: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)}
}

type fixtureCommit struct {
	file, content, message string
	parents                []plumbing.Hash // nil uses HEAD
}

func (f *commitFixture) commit(change fixtureCommit) plumbing.Hash {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.path, change.file), []byte(change.content), 0o644); err != nil {
		f.t.Fatal(err)
	}
	wt, err := f.repo.Worktree()
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := wt.Add(change.file); err != nil {
		f.t.Fatal(err)
	}
	f.tick = f.tick.Add(time.Minute)
	sig := &object.Signature{Name: "Bistro Test", Email: "t@example.com", When: f.tick}
	hash, err := wt.Commit(change.message, &gitlib.CommitOptions{Author: sig, Committer: sig, Parents: change.parents})
	if err != nil {
		f.t.Fatal(err)
	}
	return hash
}

func (f *commitFixture) reset(to plumbing.Hash) {
	f.t.Helper()
	wt, err := f.repo.Worktree()
	if err != nil {
		f.t.Fatal(err)
	}
	if err := wt.Reset(&gitlib.ResetOptions{Commit: to, Mode: gitlib.HardReset}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *commitFixture) request(head, base plumbing.Hash) CommitRangeRequest {
	return CommitRangeRequest{RepoURL: "u", RepoPath: f.path, HeadSHA: head.String(), MergeBaseSHA: base.String()}
}

func rangeSHAs(r CommitRange) []string {
	out := make([]string, len(r.Commits))
	for i, c := range r.Commits {
		out[i] = c.SHA
	}
	return out
}

func TestListCommitRange_ExcludesBaseHistoryIncludingMergedMain(t *testing.T) {
	f := newCommitFixture(t)
	b0 := f.commit(fixtureCommit{file: "base.txt", content: "base\n", message: "base"})
	m1 := f.commit(fixtureCommit{file: "main.txt", content: "main\n", message: "main work"})
	f.reset(b0)
	f1 := f.commit(fixtureCommit{file: "a.txt", content: "a\n", message: "feat: a\n\nbody line"})
	f2 := f.commit(fixtureCommit{file: "b.txt", content: "b\n", message: "feat: b"})
	mg := f.commit(fixtureCommit{file: "main.txt", content: "main\n", message: "Merge main", parents: []plumbing.Hash{f2, m1}})
	f3 := f.commit(fixtureCommit{file: "c.txt", content: "c\n", message: "feat: c"})
	req := f.request(f3, m1)
	got, err := ListCommitRange(repolock.New(), req)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{f1.String(), f2.String(), mg.String(), f3.String()}
	if !slices.Equal(rangeSHAs(got), want) || got.Truncated {
		t.Fatalf("range = %v, truncated=%v; want %v, false", rangeSHAs(got), got.Truncated, want)
	}
	first := got.Commits[0]
	if first.Subject != "feat: a" || first.Body != "body line" || first.AuthorName != "Bistro Test" || !first.AuthoredAt.Equal(f.tick.Add(-3*time.Minute)) {
		t.Fatalf("metadata = %+v", first)
	}
	if !got.Commits[2].IsMerge || got.Commits[2].Diff != nil || !slices.Equal(got.Commits[2].ParentSHAs, []string{f2.String(), m1.String()}) {
		t.Fatalf("merge commit = %+v, want two parents with nil Diff", got.Commits[2])
	}
	for _, commit := range got.Commits {
		if commit.Diff != nil {
			t.Fatalf("metadata-only listing includes diff for %s", commit.SHA)
		}
	}
	stats, err := ComputeCommitStats(context.Background(), repolock.New(), req, []string{f1.String()})
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].SHA != f1.String() || stats[0].Additions != 1 || stats[0].Deletions != 0 || stats[0].FileCount != 1 || stats[0].PatchID == "" {
		t.Fatalf("first commit stats = %+v, want a.txt +1", stats)
	}
	merge, err := GetRangeCommit(repolock.New(), req, mg.String())
	if err != nil || !merge.IsMerge || merge.Diff != nil {
		t.Fatalf("get merge = %+v, %v", merge, err)
	}
}

func TestCollectRangeStats_ReusesCachedAndSkipsMerges(t *testing.T) {
	f := newCommitFixture(t)
	base := f.commit(fixtureCommit{file: "base.txt", content: "base\n", message: "base"})
	main := f.commit(fixtureCommit{file: "main.txt", content: "main\n", message: "main work"})
	f.reset(base)
	first := f.commit(fixtureCommit{file: "a.txt", content: "a\n", message: "first"})
	second := f.commit(fixtureCommit{file: "b.txt", content: "b\n", message: "second"})
	merge := f.commit(fixtureCommit{file: "main.txt", content: "main\n", message: "Merge main", parents: []plumbing.Hash{second, main}})
	const cachedPatchID = "sentinel"
	cached := map[string]CommitStats{first.String(): {SHA: first.String(), PatchID: cachedPatchID}}
	got, err := CollectRangeStats(context.Background(), f.path, merge.String(), main.String(), cached)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SHA != first.String() || got[1].SHA != second.String() {
		t.Fatalf("stats = %+v, want first and second oldest first without merge", got)
	}
	if got[0] != cached[first.String()] {
		t.Fatalf("cached stats = %+v, want sentinel reused", got[0])
	}
	computed, err := ComputeCommitStats(context.Background(), repolock.New(), f.request(merge, main), []string{second.String()})
	if err != nil {
		t.Fatal(err)
	}
	if len(computed) != 1 || got[1] != computed[0] {
		t.Fatalf("uncached stats = %+v, want %+v", got[1], computed)
	}
}

func TestComputeCommitStats_MatchesGetRangeCommit(t *testing.T) {
	f := newCommitFixture(t)
	base := f.commit(fixtureCommit{file: "f.txt", content: "old\n", message: "base"})
	head := f.commit(fixtureCommit{file: "f.txt", content: "new\nextra\n", message: "change"})
	req := f.request(head, base)
	got, err := ComputeCommitStats(context.Background(), repolock.New(), req, []string{head.String()})
	if err != nil {
		t.Fatal(err)
	}
	commit, err := GetRangeCommit(repolock.New(), req, head.String())
	if err != nil {
		t.Fatal(err)
	}
	if commit.Diff == nil {
		t.Fatal("single-parent commit must have a diff")
	}
	want := CommitStats{SHA: head.String(), Additions: commit.Diff.Additions, Deletions: commit.Diff.Deletions, FileCount: len(commit.Diff.Files), PatchID: commit.Diff.PatchID}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("stats = %+v, want %+v", got, want)
	}
}

func TestCommitStats_PackedBareRepository(t *testing.T) {
	f := newCommitFixture(t)
	base := f.commit(fixtureCommit{file: "f.txt", content: "old\n", message: "base"})
	first := f.commit(fixtureCommit{file: "f.txt", content: "new\n", message: "first"})
	head := f.commit(fixtureCommit{file: "extra.txt", content: "extra\n", message: "second"})
	req := f.request(head, base)
	req.RepoPath = t.TempDir()
	if _, err := gitlib.PlainClone(req.RepoPath, true, &gitlib.CloneOptions{URL: "file://" + f.path}); err != nil {
		t.Fatal(err)
	}
	listed, err := ListCommitRange(repolock.New(), req)
	if err != nil || !slices.Equal(rangeSHAs(listed), []string{first.String(), head.String()}) {
		t.Fatalf("packed bare range = %+v, %v", listed, err)
	}
	collected, err := CollectRangeStats(context.Background(), req.RepoPath, req.HeadSHA, req.MergeBaseSHA, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Requested order differs from the range to pin the stats API's ordering.
	computed, err := ComputeCommitStats(context.Background(), repolock.New(), req, []string{head.String(), first.String()})
	if err != nil {
		t.Fatal(err)
	}
	if len(collected) != 2 || len(computed) != 2 || collected[0] != computed[1] || collected[1] != computed[0] {
		t.Fatalf("packed bare stats: collected=%+v computed=%+v", collected, computed)
	}
	commit, err := GetRangeCommit(repolock.New(), req, head.String())
	if err != nil || commit.Diff == nil || statsOf(head.String(), *commit.Diff) != computed[0] {
		t.Fatalf("packed bare commit diff = %+v, %v", commit.Diff, err)
	}
}

func TestGetRangeCommit_SeparateGitDir(t *testing.T) {
	f := &commitFixture{t: t, path: t.TempDir(), tick: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)}
	gitDir := t.TempDir()
	storage := filesystem.NewStorage(osfs.New(gitDir), cache.NewObjectLRUDefault())
	repo, err := gitlib.Init(storage, osfs.New(f.path))
	if err != nil {
		t.Fatal(err)
	}
	f.repo = repo
	base := f.commit(fixtureCommit{file: "f.txt", content: "old\n", message: "base"})
	head := f.commit(fixtureCommit{file: "f.txt", content: "new\n", message: "change"})
	if _, err := gitlib.PlainOpen(f.path); err != nil {
		t.Fatalf("fixture must open with PlainOpen: %v", err)
	}
	got, err := GetRangeCommit(repolock.New(), f.request(head, base), head.String())
	if err != nil || got.SHA != head.String() || got.Diff == nil || got.Diff.Additions != 1 || got.Diff.Deletions != 1 {
		t.Fatalf("separate-git-dir commit = %+v, diff=%+v, err=%v", got, got.Diff, err)
	}
}

func TestListCommitRange_CapsAtLimitKeepingOldest(t *testing.T) {
	f := newCommitFixture(t)
	base := f.commit(fixtureCommit{file: "f.txt", content: "0\n", message: "base"})
	var first, lastKept, head plumbing.Hash
	const overflowCommits = 5
	for i := 1; i <= CommitRangeLimit+overflowCommits; i++ {
		head = f.commit(fixtureCommit{file: "f.txt", content: fmt.Sprintf("%d\n", i), message: fmt.Sprintf("c%d", i)})
		if i == 1 {
			first = head
		}
		if i == CommitRangeLimit {
			lastKept = head
		}
	}
	req := f.request(head, base)
	got, err := ListCommitRange(repolock.New(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || len(got.Commits) != CommitRangeLimit || got.Commits[0].SHA != first.String() || got.Commits[CommitRangeLimit-1].SHA != lastKept.String() {
		t.Fatalf("range = %v, truncated=%v; want oldest %d", rangeSHAs(got), got.Truncated, CommitRangeLimit)
	}
	if _, err := GetRangeCommit(repolock.New(), req, head.String()); !errors.Is(err, ErrCommitNotInRange) {
		t.Fatalf("beyond cap: %v, want ErrCommitNotInRange", err)
	}
	exact, err := ListCommitRange(repolock.New(), f.request(lastKept, base))
	if err != nil || exact.Truncated || len(exact.Commits) != CommitRangeLimit {
		t.Fatalf("exact limit: len=%d truncated=%v err=%v", len(exact.Commits), exact.Truncated, err)
	}
}

func TestGetRangeCommit_RejectsCommitOutsideRange(t *testing.T) {
	f := newCommitFixture(t)
	base := f.commit(fixtureCommit{file: "f.txt", content: "0\n", message: "base"})
	head := f.commit(fixtureCommit{file: "f.txt", content: "1\n", message: "change"})
	req := f.request(head, base)
	if _, err := GetRangeCommit(repolock.New(), req, base.String()); !errors.Is(err, ErrCommitNotInRange) {
		t.Fatalf("base commit err = %v, want ErrCommitNotInRange", err)
	}
	got, err := GetRangeCommit(repolock.New(), req, head.String())
	if err != nil || got.Diff == nil || got.Diff.Additions != 1 || got.Diff.Deletions != 1 {
		t.Fatalf("head commit = %+v, %v; want +1 -1", got.Diff, err)
	}
	listed, err := ListCommitRange(repolock.New(), req)
	if err != nil || len(listed.Commits) != 1 || listed.Commits[0].Diff != nil {
		t.Fatalf("metadata-only range = %+v, %v", listed, err)
	}
	empty, err := ListCommitRange(repolock.New(), f.request(base, base))
	if err != nil || len(empty.Commits) != 0 || empty.Truncated {
		t.Fatalf("identical heads = %+v, %v", empty, err)
	}
	missing := strings.Repeat("ab", 20)
	for _, req := range []CommitRangeRequest{
		{RepoURL: "u", RepoPath: f.path, HeadSHA: missing, MergeBaseSHA: base.String()},
		{RepoURL: "u", RepoPath: f.path, HeadSHA: head.String(), MergeBaseSHA: missing},
	} {
		if _, err := ListCommitRange(repolock.New(), req); !errors.Is(err, ErrHeadUnavailable) {
			t.Fatalf("missing commit: %v, want ErrHeadUnavailable", err)
		}
	}
}

func TestListCommitRange_DoesNotVisitDeepSharedHistory(t *testing.T) {
	f := newCommitFixture(t)
	root := f.commit(fixtureCommit{file: "f.txt", content: "root\n", message: "root"})
	var base plumbing.Hash
	const sharedHistoryLength = 20
	for i := 0; i < sharedHistoryLength; i++ {
		base = f.commit(fixtureCommit{file: "f.txt", content: fmt.Sprintf("%d\n", i), message: "base history"})
	}
	head := f.commit(fixtureCommit{file: "feature.txt", content: "feature\n", message: "feature"})
	// Missing deep history makes an unbounded walk fail instead of merely slow.
	rootSHA := root.String()
	if err := os.Remove(filepath.Join(f.path, ".git", "objects", rootSHA[:2], rootSHA[2:])); err != nil {
		t.Fatal(err)
	}
	got, err := ListCommitRange(repolock.New(), f.request(head, base))
	if err != nil || !slices.Equal(rangeSHAs(got), []string{head.String()}) {
		t.Fatalf("bounded walk: range=%v err=%v, want only head", rangeSHAs(got), err)
	}
	commit, err := f.repo.CommitObject(base)
	if err != nil {
		t.Fatal(err)
	}
	parentSHA := commit.ParentHashes[0].String()
	if err := os.Remove(filepath.Join(f.path, ".git", "objects", parentSHA[:2], parentSHA[2:])); err != nil {
		t.Fatal(err)
	}
	empty, err := ListCommitRange(repolock.New(), f.request(base, base))
	if err != nil || len(empty.Commits) != 0 || empty.Truncated {
		t.Fatalf("identical endpoints must not visit parents: %+v, %v", empty, err)
	}
}

func TestListCommitRange_ClockSkewWithinSlopExcludesVisitedBaseAncestors(t *testing.T) {
	f := newCommitFixture(t)
	root := f.commit(fixtureCommit{file: "base.txt", content: "base\n", message: "root"})
	const clockSkew = 2 * time.Minute
	f.tick = f.tick.Add(-clockSkew)
	base := f.commit(fixtureCommit{file: "main.txt", content: "main\n", message: "base with old clock"})
	f.reset(root)
	f.tick = f.tick.Add(2 * clockSkew)
	head := f.commit(fixtureCommit{file: "feature.txt", content: "feature\n", message: "feature"})
	got, err := ListCommitRange(repolock.New(), f.request(head, base))
	if err != nil || !slices.Equal(rangeSHAs(got), []string{head.String()}) {
		t.Fatalf("clock skew: range=%v err=%v, want only head", rangeSHAs(got), err)
	}
}

func TestTopoOldestFirst_ParentsBeforeChildrenAndDeterministicTies(t *testing.T) {
	when := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	parent := &object.Commit{Hash: plumbing.NewHash(strings.Repeat("c", 40)), Committer: object.Signature{When: when}}
	child := &object.Commit{Hash: plumbing.NewHash(strings.Repeat("a", 40)), ParentHashes: []plumbing.Hash{parent.Hash}, Committer: object.Signature{When: when.Add(-time.Hour)}}
	sibling := &object.Commit{Hash: plumbing.NewHash(strings.Repeat("b", 40)), Committer: object.Signature{When: when}}
	got := topoOldestFirst([]*object.Commit{child, parent, sibling})
	if !slices.Equal(got, []*object.Commit{sibling, parent, child}) {
		t.Fatalf("order = %v, want sibling, parent, child", got)
	}
}

func TestCommitDiff_RootAndContentPreservingRebase(t *testing.T) {
	f := newCommitFixture(t)
	root := f.commit(fixtureCommit{file: "f.txt", content: "0\n", message: "root"})
	commit, err := f.repo.CommitObject(root)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := commitDiff(commit)
	if err != nil || diff.Additions != 1 || diff.Deletions != 0 || len(diff.Files) != 1 || diff.Files[0].Path != "f.txt" {
		t.Fatalf("root diff = %+v, %v", diff, err)
	}
	first := f.commit(fixtureCommit{file: "f.txt", content: "1\n", message: "change"})
	original, err := GetRangeCommit(repolock.New(), f.request(first, root), first.String())
	if err != nil {
		t.Fatal(err)
	}
	f.reset(root)
	newBase := f.commit(fixtureCommit{file: "other.txt", content: "other\n", message: "new base"})
	rebased := f.commit(fixtureCommit{file: "f.txt", content: "1\n", message: "change rebased"})
	got, err := GetRangeCommit(repolock.New(), f.request(rebased, newBase), rebased.String())
	if err != nil || got.Diff == nil || got.Diff.PatchID != original.Diff.PatchID {
		t.Fatalf("rebased diff = %+v, %v; want original fingerprint %s", got.Diff, err, original.Diff.PatchID)
	}
}

func TestCommitDiff_BinaryPathRewriteChangesCommitFingerprint(t *testing.T) {
	f := newCommitFixture(t)
	f.commit(fixtureCommit{file: "a.png", content: "\x00old", message: "first binary"})
	base := f.commit(fixtureCommit{file: "b.png", content: "\x00old", message: "second binary"})
	first := f.commit(fixtureCommit{file: "a.png", content: "\x00new", message: "change a"})
	original, err := GetRangeCommit(repolock.New(), f.request(first, base), first.String())
	if err != nil {
		t.Fatal(err)
	}
	f.reset(base)
	rewritten := f.commit(fixtureCommit{file: "b.png", content: "\x00new", message: "change b"})
	got, err := GetRangeCommit(repolock.New(), f.request(rewritten, base), rewritten.String())
	if err != nil {
		t.Fatal(err)
	}
	if original.Diff == nil || got.Diff == nil || len(original.Diff.Files) != 1 || len(got.Diff.Files) != 1 {
		t.Fatalf("expected two single-file diffs: %+v, %+v", original.Diff, got.Diff)
	}
	if !original.Diff.Files[0].Binary || !got.Diff.Files[0].Binary || original.Diff.Files[0].PatchID != got.Diff.Files[0].PatchID {
		t.Fatal("fixture must retain identical binary blob transitions and per-file fingerprints")
	}
	if original.Diff.PatchID == got.Diff.PatchID {
		t.Fatal("rewriting a binary change onto a different path must reset the commit fingerprint")
	}
}

func TestUnifiedPatch_KeepsUnavailableFilePatchesVisible(t *testing.T) {
	files := []*File{
		{Path: "binary.dat", Binary: true},
		{Path: "new.txt", OldPath: "old.txt", Truncated: true},
		{Path: "text.txt", Patch: "diff --git a/text.txt b/text.txt\n+text"},
	}
	want := "diff --git a/binary.dat b/binary.dat\nBinary files a/binary.dat and b/binary.dat differ\ndiff --git a/old.txt b/new.txt\ndiff --git a/text.txt b/text.txt\n+text\n"
	if got := unifiedPatch(files); got != want {
		t.Fatalf("unified = %q, want %q", got, want)
	}
}

func TestCommitStats_CancelledBeforeWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// An invalid path would fail if cancellation did not precede repository work.
	req := CommitRangeRequest{RepoPath: "missing-repository"}
	collected, err := CollectRangeStats(ctx, req.RepoPath, req.HeadSHA, req.MergeBaseSHA, nil)
	if !errors.Is(err, context.Canceled) || len(collected) != 0 {
		t.Fatalf("CollectRangeStats = %v, %v", collected, err)
	}
	computed, err := ComputeCommitStats(ctx, repolock.New(), req, []string{"missing-commit"})
	if !errors.Is(err, context.Canceled) || len(computed) != 0 {
		t.Fatalf("ComputeCommitStats = %v, %v", computed, err)
	}
}
