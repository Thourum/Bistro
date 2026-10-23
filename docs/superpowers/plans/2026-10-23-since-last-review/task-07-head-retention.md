# Task 7: Head retention — refresh cache clones in place, pin PR heads

**Files:**
- Create: `go/internal/git/retention.go`
- Create: `go/internal/git/retention_test.go`
- Modify: `go/internal/git/refresh.go` (add `refreshOnDiskClone`)
- Modify: `go/internal/git/clone.go` (`WithPRClone`, `cloneWithOptionsLocked`) — pre-existing oversized file, see README

**Interfaces:**
- Consumes: `(*Client).fetchIntoRepo(ctx, repoDir, opts, depth)` (Task 2), `targetedRefSpecs`, `requiredSHAs`, `firstMissingCommit`, `repoCacheDir`, `needsFreshTempDir`, `SafeRepoID`, `ParseRemoteURL`, `safeIDLength` (all existing in `internal/git`).
- Produces:
  - `func pinnedHeadRef(repoURL string, prNumber int, headSHA string) plumbing.ReferenceName` → `refs/bistro/<safeID>/pull/<N>/heads/<sha>`
  - every `WithPRClone` with `opts.PRNumber > 0` pins `headSHA` in the repo it used (cache clone or user repo) before calling `use`.
  - test fixture `newForcePushFixture(t)` and `forcePush(t, fixture) string` in `retention_test.go`, reused by Task 8's test.

**Why:** Spec §5 head retention, with the deviation recorded in the spec's implementation notes: the patchset number isn't known at fetch time and `ps/<n>` lacks the PR number, so the pin is keyed by PR and head SHA. Today a missing commit (exactly what a force-push causes) re-creates the cache clone from scratch, which would discard pins, so the sync must first fetch into the existing clone.

- [ ] **Step 1: Write the failing test**

`go/internal/git/retention_test.go` (`buildRemoteWithPRRef` is in `clone_targeted_test.go`, `commitFile` in `localfetch_test.go`):

```go
package git

import (
	"context"
	"testing"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

const forcePushRepoURL = "https://github.com/acme/widgets.git"

// forcePushFixture is a local remote with PR #7 at head1, already cloned into
// the canonical cache dir of forcePushRepoURL. The clone's origin points at the
// local remote, so refreshes never touch the network.
type forcePushFixture struct {
	remote     *gitlib.Repository
	remoteDir  string
	cacheDir   string
	opts       *CloneOptions
	baseSHA    string
	head1      string
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
		plumbing.ReferenceName("refs/pull/7/head"):                  head2,
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
		t.Fatalf("WithPRClone %s: %v", truncateSHAForLog(headSHA), err)
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
			t.Errorf("pin for %s: ref=%v err=%v", truncateSHAForLog(head), ref, err)
		}
		if _, err := repo.CommitObject(plumbing.NewHash(head)); err != nil {
			t.Errorf("commit %s missing from cache clone: %v", truncateSHAForLog(head), err)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go && go test ./internal/git/ -run ForcePushKeepsPreviousHeadPinned -v`
Expected: FAIL to compile (`undefined: pinnedHeadRef`).

- [ ] **Step 3: Implement pins**

`go/internal/git/retention.go`:

```go
package git

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// pinnedHeadRef keeps every PR head the sync diffed reachable, so a force-push
// or gc cannot drop the head an earlier patchset recorded.
func pinnedHeadRef(repoURL string, prNumber int, headSHA string) plumbing.ReferenceName {
	return plumbing.ReferenceName(fmt.Sprintf("refs/bistro/%s/pull/%d/heads/%s", refSafeID(repoURL), prNumber, headSHA))
}

// refSafeID namespaces pins by repo. URLs without a GitHub identity (tests,
// BISTRO_REPO_DIR setups) hash the raw URL instead.
func refSafeID(repoURL string) string {
	if identity, err := ParseRemoteURL(repoURL); err == nil {
		return SafeRepoID(identity)
	}
	sum := sha256.Sum256([]byte(repoURL))
	return hex.EncodeToString(sum[:])[:safeIDLength]
}

// pinPRHead writes the pin for headSHA. Best-effort: a missing pin only makes
// a later interdiff unavailable, so it never fails the sync.
func pinPRHead(log *slog.Logger, localPath, repoURL string, opts *CloneOptions, headSHA string) {
	if opts == nil || opts.PRNumber <= 0 {
		return
	}
	repo, err := gitlib.PlainOpen(localPath)
	if err == nil {
		ref := plumbing.NewHashReference(pinnedHeadRef(repoURL, opts.PRNumber, headSHA), plumbing.NewHash(headSHA))
		err = repo.Storer.SetReference(ref)
	}
	if err != nil {
		log.Warn("pin PR head failed", "local_path", localPath, "pr_number", opts.PRNumber, "error", err)
	}
}
```

In `clone.go` `WithPRClone`, inside `operation` after the `cloneWithOptionsLocked` error check: `pinPRHead(log, clone.LocalPath, repoURL, opts, headSHA)`.

- [ ] **Step 4: Refresh the existing cache clone before re-cloning**

Append to `go/internal/git/refresh.go` (add `"log/slog"` and `"time"` imports):

```go
// refreshOnDiskClone fetches into the repo's existing cache clone instead of
// replacing it, so pinned PR heads survive a force-push. It reports false when
// there is no reusable clone or a required commit is still missing.
func (c *Client) refreshOnDiskClone(
	ctx context.Context,
	log *slog.Logger,
	repoURL, baseSHA, headSHA string,
	opts *CloneOptions,
) (*CloneResult, bool) {
	if needsFreshTempDir(repoURL) {
		return nil, false
	}
	repoDir, err := repoCacheDir(repoURL)
	if err != nil || !localRepoExists(repoDir) {
		return nil, false
	}
	if err := c.fetchIntoRepo(ctx, repoDir, opts, 0); err != nil {
		log.Debug("cache refresh fetch failed; re-cloning", "local_path", repoDir, "error", err)
		return nil, false
	}
	missing, err := firstMissingCommit(repoDir, requiredSHAs(baseSHA, headSHA, opts))
	if err != nil || missing != "" {
		log.Debug("cache refresh left a required commit missing; re-cloning",
			"missing_sha", truncateSHAForLog(missing), "error", err)
		return nil, false
	}
	c.cache.Set(repoURL, baseSHA, headSHA, &RepoEntry{
		URL: repoURL, LocalPath: repoDir, BaseSHA: baseSHA, HeadSHA: headSHA, FetchedAt: time.Now(),
	})
	c.recordCacheCloned(repoURL, repoDir)
	return &CloneResult{LocalPath: repoDir, BaseSHA: baseSHA, HeadSHA: headSHA}, true
}
```

In `clone.go` `cloneWithOptionsLocked`, right after the `reuseOnDiskClone` block:

```go
	if result, ok := c.refreshOnDiskClone(ctx, log, repoURL, baseSHA, headSHA, opts); ok {
		return result, nil
	}
```

- [ ] **Step 5: Run tests**

Run: `cd go && go test ./internal/git/ -v -run 'ForcePush|CloneForPR|Local'`
Expected: PASS. If a local-fetch test asserts the exact set of `refs/bistro/*` refs in a user repo, add the pin ref to its expectation (pins are part of the `refs/bistro/` namespace by design).

- [ ] **Step 6: Full Go verification**

Run: `cd go && go vet ./... && go test -race ./...`
Expected: PASS.

- [ ] **Step 7: Commit** (clone.go pre-existing oversized → `--no-verify` if blocked, note it)

```bash
git add go/internal/git
git commit -m "feat: keep force-pushed PR heads reachable via pinned refs

Cache clones are refreshed in place before falling back to a re-clone so
the pins survive."
```
