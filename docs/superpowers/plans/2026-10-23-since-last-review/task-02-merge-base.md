# Task 2: Merge-base diffs in the sync

**Files:**
- Create: `go/internal/git/mergebase.go`
- Create: `go/internal/git/refresh.go`
- Create: `go/internal/git/mergebase_test.go`
- Modify: `go/internal/sync/fetcher.go` (`PRFetchResult`, `generateDiff`, `resolveDiffAnchors`)

**Interfaces:**
- Consumes: `git.DiffFile.PatchID` (Task 1) — unchanged here.
- Produces:
  - `var ErrMergeBaseNotFound = errors.New("merge base not found")`
  - `func MergeBase(localPath, baseSHA, headSHA string) (string, error)`
  - `func (c *Client) Deepen(ctx context.Context, localPath string, opts *CloneOptions) error`
  - `func (c *Client) fetchIntoRepo(ctx context.Context, repoDir string, opts *CloneOptions, depth int) error` (Task 7 reuses it with depth 0)
  - `PRFetchResult.MergeBaseSHA string` — the merge base the diff was generated against; `""` when the diff wasn't computed or the merge base stayed unknown (then the diff used `pr.BaseSHA`).

**Why:** Diffing base-tip..head leaks upstream changes into the PR diff and breaks old-side line numbers (spec §1). GitHub's PR diff and comment `position` are three-dot (merge-base) based, so anchors must use the same base.

- [ ] **Step 1: Write the failing test**

`go/internal/git/mergebase_test.go` (`commitFile` already exists in `localfetch_test.go`, same package):

```go
package git

import (
	"testing"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// A PR branched from root adds pr.txt; the base branch then advances with
// upstream.txt. The merge-base diff must contain only pr.txt.
func TestMergeBaseDiffShowsOnlyPRChanges(t *testing.T) {
	dir := t.TempDir()
	repo, err := gitlib.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	root := commitFile(t, repo, dir, "shared.txt", "v1\n", "root")
	head := commitFile(t, repo, dir, "pr.txt", "pr\n", "pr change")
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := worktree.Checkout(&gitlib.CheckoutOptions{
		Hash: root, Branch: plumbing.NewBranchReferenceName("base-tip"), Create: true,
	}); err != nil {
		t.Fatalf("Checkout base-tip: %v", err)
	}
	baseTip := commitFile(t, repo, dir, "upstream.txt", "upstream\n", "upstream change")

	mergeBase, err := MergeBase(dir, baseTip.String(), head.String())
	if err != nil {
		t.Fatalf("MergeBase: %v", err)
	}
	if mergeBase != root.String() {
		t.Fatalf("MergeBase = %s, want root %s", mergeBase, root)
	}
	diff, err := NewClient(nil).GenerateDiff(dir, mergeBase, head.String(), DefaultDiffOptions())
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
	first := t.TempDir()
	repo, err := gitlib.PlainInit(first, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	a := commitFile(t, repo, first, "a.txt", "a\n", "a")
	if _, err := MergeBase(first, a.String(), plumbing.NewHash("1111111111111111111111111111111111111111").String()); err == nil {
		t.Error("MergeBase with a missing commit should fail")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go && go test ./internal/git/ -run 'MergeBase' -v`
Expected: FAIL — `undefined: MergeBase`.

- [ ] **Step 3: Implement `MergeBase` and `Deepen`**

`go/internal/git/mergebase.go`:

```go
package git

import (
	"context"
	"errors"
	"fmt"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// ErrMergeBaseNotFound reports that no common ancestor is reachable locally,
// typically because a shallow clone stops short of it.
var ErrMergeBaseNotFound = errors.New("merge base not found")

// deepenFetchDepth is how far one deepen fetch extends a shallow clone when the
// merge base lies beyond defaultTargetedFetchDepth.
const deepenFetchDepth = 1000

// MergeBase returns the best common ancestor of baseSHA and headSHA in the
// repository at localPath.
func MergeBase(localPath, baseSHA, headSHA string) (string, error) {
	repo, err := gitlib.PlainOpen(localPath)
	if err != nil {
		return "", fmt.Errorf("open repository %s: %w", localPath, err)
	}
	base, err := repo.CommitObject(plumbing.NewHash(baseSHA))
	if err != nil {
		return "", fmt.Errorf("%w: base %s: %w", ErrMergeBaseNotFound, baseSHA, err)
	}
	head, err := repo.CommitObject(plumbing.NewHash(headSHA))
	if err != nil {
		return "", fmt.Errorf("%w: head %s: %w", ErrMergeBaseNotFound, headSHA, err)
	}
	// Walking past a shallow boundary surfaces as a missing-object error.
	bases, err := head.MergeBase(base)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrMergeBaseNotFound, err)
	}
	if len(bases) == 0 {
		return "", ErrMergeBaseNotFound
	}
	return bases[0].Hash.String(), nil
}

// Deepen extends a shallow clone's history along the PR's refs so a merge base
// beyond the shallow boundary becomes reachable. Full clones are left alone.
// Callers hold the repo lock (it runs inside WithPRClone's callback).
func (c *Client) Deepen(ctx context.Context, localPath string, opts *CloneOptions) error {
	repo, err := gitlib.PlainOpen(localPath)
	if err != nil {
		return fmt.Errorf("open repository %s: %w", localPath, err)
	}
	shallow, err := repo.Storer.Shallow()
	if err != nil {
		return fmt.Errorf("read shallow commits in %s: %w", localPath, err)
	}
	if len(shallow) == 0 {
		return nil
	}
	return c.fetchIntoRepo(ctx, localPath, opts, deepenFetchDepth)
}
```

`go/internal/git/refresh.go`:

```go
package git

import (
	"context"
	"errors"
	"fmt"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

// broadRefSpecs mirrors the broad clone's fetch for callers without PR hints.
func broadRefSpecs() []config.RefSpec {
	return []config.RefSpec{
		"+refs/heads/*:refs/remotes/origin/*",
		"+refs/pull/*/head:refs/remotes/origin/pr/*",
	}
}

// fetchIntoRepo fetches the PR's refs from origin into an existing clone.
// depth 0 fetches without a depth limit, so a full clone stays full.
func (c *Client) fetchIntoRepo(ctx context.Context, repoDir string, opts *CloneOptions, depth int) error {
	repo, err := gitlib.PlainOpen(repoDir)
	if err != nil {
		return fmt.Errorf("open repository %s: %w", repoDir, err)
	}
	refSpecs := targetedRefSpecs(opts)
	if len(refSpecs) == 0 {
		refSpecs = broadRefSpecs()
	}
	fetchOpts := &gitlib.FetchOptions{Auth: c.auth(), RefSpecs: refSpecs, Depth: depth, Tags: gitlib.NoTags}
	if err := repo.FetchContext(ctx, fetchOpts); err != nil && !errors.Is(err, gitlib.NoErrAlreadyUpToDate) {
		return fmt.Errorf("fetch into %s: %w", repoDir, err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go && go test ./internal/git/ -run 'MergeBase' -v`
Expected: PASS.

- [ ] **Step 5: Diff against the merge base in the sync**

In `go/internal/sync/fetcher.go`:

Add to `PRFetchResult` (after `DiffComputed`):

```go
	// MergeBaseSHA is the merge base the diff was generated against; empty when
	// the diff was not computed or the merge base stayed unknown (the diff then
	// used the base tip). Part of the patchset identity.
	MergeBaseSHA string
```

Add `"errors"` to imports. Replace the start of `generateDiff` so it diffs from the merge base:

```go
func (o *Orchestrator) generateDiff(ctx context.Context, result *PRFetchResult, clone *git.CloneResult, log *slog.Logger) error {
	pr := result.PR
	started := time.Now()
	mergeBase := o.resolveMergeBase(ctx, clone, pr, log)
	diffBase := pr.BaseSHA
	if mergeBase != "" {
		diffBase = mergeBase
	}
	diff, err := o.git.GenerateDiff(clone.LocalPath, diffBase, pr.HeadSHA, git.DefaultDiffOptions())
```

and, where `result.DiffComputed = true` is set, also set `result.MergeBaseSHA = mergeBase`. Change the call `o.resolveDiffAnchors(ctx, result, clone, log)` to `o.resolveDiffAnchors(ctx, result, clone, diffBase, log)` and in `resolveDiffAnchors` add the `diffBase string` parameter (after `clone`) and pass `prBaseSHA: diffBase` instead of `pr.BaseSHA`.

Add the helper below `generateDiff`:

```go
// resolveMergeBase returns the PR's merge base, deepening a shallow clone once
// when history stops short of it. "" means it stayed unknown; the caller then
// diffs against the base tip.
func (o *Orchestrator) resolveMergeBase(ctx context.Context, clone *git.CloneResult, pr *provider.PR, log *slog.Logger) string {
	mergeBase, err := git.MergeBase(clone.LocalPath, pr.BaseSHA, pr.HeadSHA)
	if errors.Is(err, git.ErrMergeBaseNotFound) {
		if deepenErr := o.git.Deepen(ctx, clone.LocalPath, diffCloneOptions(pr)); deepenErr != nil {
			log.Debug("merge base deepen failed", "error", deepenErr)
		} else {
			mergeBase, err = git.MergeBase(clone.LocalPath, pr.BaseSHA, pr.HeadSHA)
		}
	}
	if err != nil {
		log.Warn("merge base unavailable; diffing against base tip", "error", err)
		return ""
	}
	return mergeBase
}
```

- [ ] **Step 6: Full Go verification**

Run: `cd go && go vet ./... && go test -race ./...`
Expected: PASS. If an existing sync/anchor test asserted base-tip diff contents for a fixture where base tip ≠ merge base, update the expectation to the merge-base result (that is the intended behavior change) and say so in the commit body.

- [ ] **Step 7: Commit**

```bash
git add go/internal/git/mergebase.go go/internal/git/refresh.go go/internal/git/mergebase_test.go go/internal/sync/fetcher.go
git commit -m "fix: diff PRs against the merge base instead of the base tip"
```
