package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/itterno/bistro/internal/logging"
)

// buildRemoteWithPRRef creates an on-disk repo at dir with two commits on the
// base branch (base then head), and publishes the head commit under
// refs/pull/<prNumber>/head so a targeted fetch can resolve it. Returns the
// base branch name, baseSHA, and headSHA.
func buildRemoteWithPRRef(t *testing.T, dir string, prNumber int) (baseBranch, baseSHA, headSHA string) {
	t.Helper()
	repo, err := gogit.PlainInit(dir, false)
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
		h, err := wt.Commit(msg, &gogit.CommitOptions{Author: sig})
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

// TestCloneForPR_TargetedSuccess verifies the targeted shallow-fetch path
// resolves both base and head commits from a local remote and caches the result.
func TestCloneForPR_TargetedSuccess(t *testing.T) {
	remoteDir := t.TempDir()
	const prNumber = 7
	baseBranch, baseSHA, headSHA := buildRemoteWithPRRef(t, remoteDir, prNumber)

	// Isolate the clone destination to a temp dir (BISTRO_REPO_DIR used for its
	// intended purpose, not repurposed).
	t.Setenv("BISTRO_REPO_DIR", t.TempDir())

	cache := NewCache()
	client := NewClient(cache)

	result, err := client.CloneForPR(context.Background(), "file://"+remoteDir, baseSHA, headSHA,
		&CloneOptions{PRNumber: prNumber, BaseBranch: baseBranch, Depth: 50})
	if err != nil {
		t.Fatalf("CloneForPR targeted: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.BaseSHA != baseSHA || result.HeadSHA != headSHA {
		t.Errorf("SHA mismatch: got base=%s head=%s", result.BaseSHA, result.HeadSHA)
	}
	if !localRepoExists(result.LocalPath) {
		t.Errorf("clone path does not exist: %s", result.LocalPath)
	}

	// Both commits must be present in the resulting clone.
	repo, err := gogit.PlainOpen(result.LocalPath)
	if err != nil {
		t.Fatalf("PlainOpen result: %v", err)
	}
	if _, err := repo.CommitObject(plumbing.NewHash(baseSHA)); err != nil {
		t.Errorf("base commit missing in targeted clone: %v", err)
	}
	if _, err := repo.CommitObject(plumbing.NewHash(headSHA)); err != nil {
		t.Errorf("head commit missing in targeted clone: %v", err)
	}

	// Result must be cached.
	if _, ok := cache.Get("file://"+remoteDir, baseSHA, headSHA); !ok {
		t.Error("expected targeted clone result to be cached")
	}
}

// TestCloneForPR_FallbackOnMissingBase proves that when the targeted fetch
// cannot prove the base commit exists, CloneForPR falls back to the broad path.
// Here the base SHA is bogus and exists nowhere on the remote, so both the
// targeted and broad attempts fail to find it — the broad path being reached
// (and erroring on the genuinely-missing base) demonstrates the fallback.
func TestCloneForPR_FallbackOnMissingBase(t *testing.T) {
	remoteDir := t.TempDir()
	const prNumber = 9
	baseBranch, _, headSHA := buildRemoteWithPRRef(t, remoteDir, prNumber)

	t.Setenv("BISTRO_REPO_DIR", t.TempDir())

	cache := NewCache()
	client := NewClient(cache)

	const bogusBase = "0000000000000000000000000000000000000000"
	_, err := client.CloneForPR(context.Background(), "file://"+remoteDir, bogusBase, headSHA,
		&CloneOptions{PRNumber: prNumber, BaseBranch: baseBranch, Depth: 1})
	if err == nil {
		t.Fatal("expected error: bogus base commit exists nowhere, broad fallback must also fail")
	}
	// Nothing should have been cached for a failed clone.
	if _, ok := cache.Get("file://"+remoteDir, bogusBase, headSHA); ok {
		t.Error("failed clone must not be cached")
	}
}

// TestCloneForPR_NilOptsDelegatesToClone verifies CloneForPR with nil opts
// behaves like Clone (e.g. honors the on-disk cache hit without any fetch).
func TestCloneForPR_NilOptsDelegatesToClone(t *testing.T) {
	cache := NewCache()
	cachedDir := t.TempDir()
	const url = "https://github.com/org/repo"
	cache.Set(url, "b1", "h1", &RepoEntry{
		URL: url, LocalPath: cachedDir, BaseSHA: "b1", HeadSHA: "h1",
	})

	client := NewClient(cache)
	result, err := client.CloneForPR(context.Background(), url, "b1", "h1", nil)
	if err != nil {
		t.Fatalf("CloneForPR nil opts: %v", err)
	}
	if result.LocalPath != cachedDir {
		t.Errorf("expected cached path %s, got %s", cachedDir, result.LocalPath)
	}
}

// TestCloneForPR_EmptyOptsSkipsTargeted verifies that opts with no PR number
// and no base branch does not engage the targeted path (cache hit still works).
func TestCloneForPR_EmptyOptsSkipsTargeted(t *testing.T) {
	cache := NewCache()
	cachedDir := t.TempDir()
	const url = "https://github.com/org/repo2"
	cache.Set(url, "b2", "h2", &RepoEntry{
		URL: url, LocalPath: cachedDir, BaseSHA: "b2", HeadSHA: "h2",
	})

	client := NewClient(cache)
	result, err := client.CloneForPR(context.Background(), url, "b2", "h2", &CloneOptions{})
	if err != nil {
		t.Fatalf("CloneForPR empty opts: %v", err)
	}
	if result.LocalPath != cachedDir {
		t.Errorf("expected cached path %s, got %s", cachedDir, result.LocalPath)
	}
}

// buildDeepRemote builds a linear-history repo c1 -> c2 -> c3 on the base
// branch and publishes c3 under refs/pull/<prNumber>/head. The PR base is c1
// (an older ancestor) and the PR head is c3. With a shallow Depth:1 fetch the
// base commit c1 lies outside the fetched window, so a targeted fetch misses
// it; a broad full clone resolves it. Returns base branch, baseSHA (c1),
// headSHA (c3).
func buildDeepRemote(t *testing.T, dir string, prNumber int) (baseBranch, baseSHA, headSHA string) {
	t.Helper()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	sig := &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()}

	commit := func(content, msg string) plumbing.Hash {
		if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := wt.Add("foo.go"); err != nil {
			t.Fatalf("Add: %v", err)
		}
		h, err := wt.Commit(msg, &gogit.CommitOptions{Author: sig})
		if err != nil {
			t.Fatalf("Commit %q: %v", msg, err)
		}
		return h
	}

	c1 := commit("package main\n", "c1 base")
	commit("package main\n\nvar a = 1\n", "c2 mid")
	c3 := commit("package main\n\nvar a = 1\nvar b = 2\n", "c3 head")

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	baseBranch = head.Name().Short()

	prRef := plumbing.NewHashReference(
		plumbing.ReferenceName(fmt.Sprintf("refs/pull/%d/head", prNumber)), c3)
	if err := repo.Storer.SetReference(prRef); err != nil {
		t.Fatalf("SetReference pr ref: %v", err)
	}

	return baseBranch, c1.String(), c3.String()
}

// TestCloneForPR_DepthMissFallsBackToBroad is the real fallback test: the PR
// base commit exists in the remote but lies outside a Depth:1 shallow window,
// so the targeted fetch cannot prove it present and must fall back to a broad
// full clone, which succeeds. It first asserts the targeted path genuinely
// misses (white-box), then asserts CloneForPR overall succeeds with both
// commits resolvable on disk.
func TestCloneForPR_DepthMissFallsBackToBroad(t *testing.T) {
	remoteDir := t.TempDir()
	const prNumber = 11
	baseBranch, baseSHA, headSHA := buildDeepRemote(t, remoteDir, prNumber)
	url := "file://" + remoteDir

	t.Setenv("BISTRO_REPO_DIR", t.TempDir())

	cache := NewCache()
	client := NewClient(cache)
	log := logging.FromContext(context.Background())

	// White-box: the targeted shallow fetch at Depth:1 must miss the deep base
	// commit and report failure (nil, false) so the broad fallback engages.
	opts := &CloneOptions{PRNumber: prNumber, BaseBranch: baseBranch, Depth: 1}
	if _, ok := client.targetedClone(context.Background(), log, url, baseSHA, headSHA, opts); ok {
		t.Fatal("expected targeted Depth:1 fetch to miss the deep base commit, but it succeeded")
	}

	// Black-box: CloneForPR must still succeed by falling back to a broad clone.
	result, err := client.CloneForPR(context.Background(), url, baseSHA, headSHA, opts)
	if err != nil {
		t.Fatalf("CloneForPR should succeed via broad fallback: %v", err)
	}
	if result.BaseSHA != baseSHA || result.HeadSHA != headSHA {
		t.Errorf("SHA mismatch: got base=%s head=%s", result.BaseSHA, result.HeadSHA)
	}
	repo, err := gogit.PlainOpen(result.LocalPath)
	if err != nil {
		t.Fatalf("PlainOpen result: %v", err)
	}
	if _, err := repo.CommitObject(plumbing.NewHash(baseSHA)); err != nil {
		t.Errorf("deep base commit missing after broad fallback: %v", err)
	}
	if _, err := repo.CommitObject(plumbing.NewHash(headSHA)); err != nil {
		t.Errorf("head commit missing after broad fallback: %v", err)
	}
}

// TestCloneForPR_RequiredSHAMissingFallsBack verifies that a RequiredSHA (e.g.
// an anchor-resolution comment commit) absent from the shallow window forces
// the targeted path to fall back even though base and head are present. The
// broad fallback then succeeds on base+head (it does not gate on RequiredSHAs —
// an unreachable comment commit must not fail a clone; anchoring simply skips
// it, exactly as in the pre-targeted behavior). The point of RequiredSHAs is
// that the targeted path is never *worse* than broad for anchoring.
func TestCloneForPR_RequiredSHAMissingFallsBack(t *testing.T) {
	remoteDir := t.TempDir()
	const prNumber = 13
	baseBranch, baseSHA, headSHA := buildRemoteWithPRRef(t, remoteDir, prNumber)
	url := "file://" + remoteDir

	t.Setenv("BISTRO_REPO_DIR", t.TempDir())

	cache := NewCache()
	client := NewClient(cache)
	log := logging.FromContext(context.Background())

	const bogusRequired = "1111111111111111111111111111111111111111"
	opts := &CloneOptions{
		PRNumber:     prNumber,
		BaseBranch:   baseBranch,
		Depth:        50,
		RequiredSHAs: []string{bogusRequired},
	}

	// White-box: base+head are present, but the bogus required SHA is not, so
	// the targeted path must report failure rather than caching a clone that
	// would silently degrade anchor resolution.
	if _, ok := client.targetedClone(context.Background(), log, url, baseSHA, headSHA, opts); ok {
		t.Fatal("expected targeted fetch to fail when a RequiredSHA is absent")
	}

	// Black-box: CloneForPR still succeeds via the broad fallback — base and
	// head resolve; the unreachable comment commit is left for anchoring to
	// skip, the same as without the targeted optimization.
	result, err := client.CloneForPR(context.Background(), url, baseSHA, headSHA, opts)
	if err != nil {
		t.Fatalf("CloneForPR should succeed via broad fallback on base+head: %v", err)
	}
	repo, err := gogit.PlainOpen(result.LocalPath)
	if err != nil {
		t.Fatalf("PlainOpen result: %v", err)
	}
	if _, err := repo.CommitObject(plumbing.NewHash(baseSHA)); err != nil {
		t.Errorf("base commit missing after broad fallback: %v", err)
	}
	if _, err := repo.CommitObject(plumbing.NewHash(headSHA)); err != nil {
		t.Errorf("head commit missing after broad fallback: %v", err)
	}
}

// TestCloneForPR_RequiredSHAPresentTargetedSucceeds verifies the targeted path
// succeeds when every RequiredSHA is reachable within the fetched window. Here
// the required SHA is the base commit itself (an intermediate ancestor of head)
// and Depth:50 brings the full short history, so the targeted fetch proves all
// required commits present and returns without a broad fallback.
func TestCloneForPR_RequiredSHAPresentTargetedSucceeds(t *testing.T) {
	remoteDir := t.TempDir()
	const prNumber = 15
	baseBranch, baseSHA, headSHA := buildDeepRemote(t, remoteDir, prNumber)
	url := "file://" + remoteDir

	t.Setenv("BISTRO_REPO_DIR", t.TempDir())

	cache := NewCache()
	client := NewClient(cache)
	log := logging.FromContext(context.Background())

	// Require the base commit explicitly (in addition to it being baseSHA) to
	// exercise the RequiredSHAs verification on a present commit.
	opts := &CloneOptions{
		PRNumber:     prNumber,
		BaseBranch:   baseBranch,
		Depth:        50,
		RequiredSHAs: []string{baseSHA, ""}, // empty entry must be tolerated
	}

	result, ok := client.targetedClone(context.Background(), log, url, baseSHA, headSHA, opts)
	if !ok {
		t.Fatal("expected targeted fetch to succeed when all RequiredSHAs are present")
	}
	if result.BaseSHA != baseSHA || result.HeadSHA != headSHA {
		t.Errorf("SHA mismatch: got base=%s head=%s", result.BaseSHA, result.HeadSHA)
	}
	if _, cok := cache.Get(url, baseSHA, headSHA); !cok {
		t.Error("expected successful targeted clone to be cached")
	}
}

// TestCloneForPR_CacheHitRequiredSHAPresent verifies that a cache hit is
// honored (returned directly, no re-clone) when every RequiredSHA is present in
// the cached repo on disk.
func TestCloneForPR_CacheHitRequiredSHAPresent(t *testing.T) {
	remoteDir := t.TempDir()
	const prNumber = 21
	_, baseSHA, headSHA := buildRemoteWithPRRef(t, remoteDir, prNumber)
	url := "file://" + remoteDir

	cache := NewCache()
	cache.Set(url, baseSHA, headSHA, &RepoEntry{
		URL: url, LocalPath: remoteDir, BaseSHA: baseSHA, HeadSHA: headSHA,
	})
	client := NewClient(cache)

	result, err := client.CloneForPR(context.Background(), url, baseSHA, headSHA,
		&CloneOptions{RequiredSHAs: []string{baseSHA, headSHA, ""}})
	if err != nil {
		t.Fatalf("CloneForPR: %v", err)
	}
	if result.LocalPath != remoteDir {
		t.Errorf("expected cached path %s honored, got %s", remoteDir, result.LocalPath)
	}
}

// TestCloneForPR_CacheHitMissingRequiredSHACleared verifies that a cache hit
// whose on-disk repo lacks a RequiredSHA is NOT returned: the stale entry is
// cleared and a fresh clone is performed (targeted then broad fallback). The
// returned path must differ from the stale cached directory.
func TestCloneForPR_CacheHitMissingRequiredSHACleared(t *testing.T) {
	remoteDir := t.TempDir()
	const prNumber = 22
	baseBranch, baseSHA, headSHA := buildRemoteWithPRRef(t, remoteDir, prNumber)
	url := "file://" + remoteDir

	// The cached repo is a separate, empty git repo that lacks the required
	// commit, forcing the missing-RequiredSHA branch.
	staleDir := t.TempDir()
	if _, err := gogit.PlainInit(staleDir, true); err != nil {
		t.Fatalf("PlainInit stale: %v", err)
	}
	t.Setenv("BISTRO_REPO_DIR", t.TempDir())

	cache := NewCache()
	cache.Set(url, baseSHA, headSHA, &RepoEntry{
		URL: url, LocalPath: staleDir, BaseSHA: baseSHA, HeadSHA: headSHA,
	})
	client := NewClient(cache)

	// RequiredSHAs includes a commit absent from the stale cached repo (baseSHA
	// is not in the empty staleDir), so the cache hit must be rejected. The
	// broad clone of the real remote then satisfies base+head.
	result, err := client.CloneForPR(context.Background(), url, baseSHA, headSHA,
		&CloneOptions{PRNumber: prNumber, BaseBranch: baseBranch, Depth: 50, RequiredSHAs: []string{baseSHA}})
	if err != nil {
		t.Fatalf("CloneForPR should re-clone successfully: %v", err)
	}
	if result.LocalPath == staleDir {
		t.Fatalf("returned stale cached path %s despite missing RequiredSHA", staleDir)
	}
	// Re-clone must yield a repo that actually contains base and head.
	repo, err := gogit.PlainOpen(result.LocalPath)
	if err != nil {
		t.Fatalf("PlainOpen re-clone: %v", err)
	}
	if _, err := repo.CommitObject(plumbing.NewHash(baseSHA)); err != nil {
		t.Errorf("base commit missing after re-clone: %v", err)
	}
	if _, err := repo.CommitObject(plumbing.NewHash(headSHA)); err != nil {
		t.Errorf("head commit missing after re-clone: %v", err)
	}
}

// TestCloneForPR_CacheHitNoRequiredSHAs verifies the cache-hit fast path is
// unchanged when no RequiredSHAs are supplied (returned without opening the repo).
func TestCloneForPR_CacheHitNoRequiredSHAs(t *testing.T) {
	cache := NewCache()
	cachedDir := t.TempDir()
	const url = "https://github.com/org/repo3"
	cache.Set(url, "b9", "h9", &RepoEntry{
		URL: url, LocalPath: cachedDir, BaseSHA: "b9", HeadSHA: "h9",
	})
	client := NewClient(cache)

	result, err := client.CloneForPR(context.Background(), url, "b9", "h9",
		&CloneOptions{PRNumber: 5, BaseBranch: "main"}) // no RequiredSHAs
	if err != nil {
		t.Fatalf("CloneForPR: %v", err)
	}
	if result.LocalPath != cachedDir {
		t.Errorf("expected cached path %s, got %s", cachedDir, result.LocalPath)
	}
}
