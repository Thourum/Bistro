package git

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/itterno/bistro/internal/logging"
)

// LocalRepoResolver reports a registered local checkout for a repo URL.
// Implemented by an adapter over store.RepoLocationStore in package server.
type LocalRepoResolver interface {
	// LocalPath returns (path, true) only for source='local' rows.
	LocalPath(repoURL string) (string, bool)
	// OnLocalDegraded records a broken local repo (missing path / mismatch).
	OnLocalDegraded(repoURL, status, code, msg string)
	// OnUsed bumps last_used_at.
	OnUsed(repoURL string)
	// OnCacheCloned records a cache clone: source='cache', local_path=path,
	// last_used_at=now (wraps RepoLocationStore.SetCache). Called after every
	// successful cache clone AND on cache-hit reuse (idempotent upsert).
	OnCacheCloned(repoURL, path string)
}

// Client handles git operations
type Client struct {
	cache *Cache
	// tokenSource supplies the auth token for clone/fetch (e.g. from the OS
	// keychain). Queried per operation so token rotation is picked up.
	tokenSource   func() string
	localResolver LocalRepoResolver
	locks         *RepoLocks
}

// NewClient creates a new git client with cache
func NewClient(cache *Cache) *Client {
	return &Client{cache: cache, locks: NewRepoLocks()}
}

// SetTokenSource configures where clone/fetch auth tokens come from.
// Must be called before the client is shared across goroutines.
func (c *Client) SetTokenSource(fn func() string) {
	c.tokenSource = fn
}

// SetLocalResolver configures local repo resolution. Nil disables local lookup.
func (c *Client) SetLocalResolver(r LocalRepoResolver) {
	c.localResolver = r
}

// SetLocks configures repo-level operation locks.
func (c *Client) SetLocks(l *RepoLocks) {
	c.locks = l
}

// auth returns HTTP basic auth from the configured token source, falling
// back to the GITHUB_TOKEN env var. Returns nil (anonymous) when neither is
// set — public repos still work.
func (c *Client) auth() *http.BasicAuth {
	if c.tokenSource != nil {
		if token := c.tokenSource(); token != "" {
			return &http.BasicAuth{
				Username: "x-token", // GitHub ignores username when token is used
				Password: token,
			}
		}
	}
	return tokenFromEnv()
}

// defaultTargetedFetchDepth is the shallow depth used for a PR-targeted fetch.
// A PR head commit is the tip of refs/pull/<N>/head, so depth 1 would suffice
// for head — but the PR base SHA is usually an ancestor of the base branch tip
// (the merge-base at PR creation / last sync), not the tip itself. A modest
// depth gives the targeted fetch a reasonable chance of pulling the base SHA
// along the base branch's first-parent history before falling back to a broad
// fetch. The verify-both-commits-exist gate keeps this safe regardless of depth.
const defaultTargetedFetchDepth = 50

// CloneOptions carries PR-specific hints that enable a targeted shallow fetch.
// All fields are optional; a zero value disables the targeted path and behaves
// identically to a plain Clone (broad fetch / full clone).
type CloneOptions struct {
	// PRNumber, when > 0, enables fetching refs/pull/<PRNumber>/head directly
	// instead of the broad refs/pull/*/head glob.
	PRNumber int
	// BaseBranch, when non-empty, enables fetching refs/heads/<BaseBranch>
	// directly instead of the broad refs/heads/* glob.
	BaseBranch string
	// Depth sets the shallow fetch depth for the targeted attempt. When 0,
	// defaultTargetedFetchDepth is used.
	Depth int
	// RequiredSHAs lists additional commit SHAs (beyond baseSHA and headSHA)
	// that must be present locally for the clone to be usable — e.g. the
	// per-comment CommitSHA / OriginalSHA needed by anchor resolution. The
	// targeted fetch only succeeds when every one of these is proven present;
	// otherwise it falls back to the broad clone. Empty/duplicate entries are
	// tolerated by the verifier.
	RequiredSHAs []string
}

// Clone fetches both baseSHA and headSHA into a local clone using the broad
// fetch strategy (all heads + all PR refs). It is retained for callers that
// have no PR-specific hints and for backward compatibility with existing tests.
// Returns cached result when both SHAs already exist in the cache.
func (c *Client) Clone(ctx context.Context, repoURL, baseSHA, headSHA string) (*CloneResult, error) {
	return c.cloneWithOptions(ctx, repoURL, baseSHA, headSHA, nil)
}

// CloneForPR is like Clone but accepts PR-specific hints (PR number, base
// branch, depth) and first attempts a targeted shallow fetch of only the PR
// head ref and base branch. If the targeted fetch cannot prove that both
// baseSHA and headSHA are present locally, it transparently falls back to the
// broad full-clone behavior, logging the fallback reason. A nil opts is
// equivalent to Clone.
func (c *Client) CloneForPR(ctx context.Context, repoURL, baseSHA, headSHA string, opts *CloneOptions) (*CloneResult, error) {
	return c.cloneWithOptions(ctx, repoURL, baseSHA, headSHA, opts)
}

func repoCacheDir(repoURL string) (string, error) {
	root, err := CacheRoot()
	if err != nil {
		return "", err
	}
	if os.Getenv("BISTRO_REPO_DIR") != "" {
		return root, nil
	}
	identity, err := ParseRemoteURL(repoURL)
	if err != nil {
		return root, nil
	}
	dir := filepath.Join(root, identity.Owner, identity.Repo)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func prepareCacheCloneDir(repoURL string) (string, error) {
	return prepareCacheCloneDirWithMode(repoURL, true)
}

func prepareTargetedCloneDir(repoURL string) (string, error) {
	return prepareCacheCloneDirWithMode(repoURL, false)
}

func prepareCacheCloneDirWithMode(repoURL string, shouldReplace bool) (string, error) {
	repoDir, err := repoCacheDir(repoURL)
	if err != nil {
		return "", err
	}
	if os.Getenv("BISTRO_REPO_DIR") != "" {
		return os.MkdirTemp(repoDir, "repo-*")
	}
	if _, err := ParseRemoteURL(repoURL); err != nil {
		return os.MkdirTemp(repoDir, "repo-*")
	}
	if shouldReplace {
		if err := os.RemoveAll(repoDir); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(repoDir), 0755); err != nil {
		return "", err
	}
	return repoDir, nil
}

func stageTargetedCloneDir(repoURL, repoDir string) (func(bool), error) {
	removeOnFailure := func(success bool) {
		if !success {
			_ = os.RemoveAll(repoDir)
		}
	}
	if os.Getenv("BISTRO_REPO_DIR") != "" {
		return removeOnFailure, nil
	}
	if _, err := ParseRemoteURL(repoURL); err != nil {
		return removeOnFailure, nil
	}
	if _, err := os.Stat(repoDir); err != nil {
		if os.IsNotExist(err) {
			return removeOnFailure, nil
		}
		return nil, err
	}

	backupDir, err := os.MkdirTemp(filepath.Dir(repoDir), filepath.Base(repoDir)+"-backup-*")
	if err != nil {
		return nil, err
	}
	if err := os.RemoveAll(backupDir); err != nil {
		return nil, err
	}
	if err := os.Rename(repoDir, backupDir); err != nil {
		return nil, err
	}

	return func(success bool) {
		if success {
			_ = os.RemoveAll(backupDir)
			return
		}
		_ = os.RemoveAll(repoDir)
		_ = os.Rename(backupDir, repoDir)
	}, nil
}

func (c *Client) recordCacheCloned(repoURL, path string) {
	if os.Getenv("BISTRO_REPO_DIR") != "" || c.localResolver == nil {
		return
	}
	c.localResolver.OnCacheCloned(repoURL, path)
}

func (c *Client) resolveLocal(ctx context.Context, repoURL, baseSHA, headSHA string, opts *CloneOptions) (*CloneResult, error, bool) {
	if os.Getenv("BISTRO_REPO_DIR") != "" || c.localResolver == nil {
		return nil, nil, false
	}
	localPath, ok := c.localResolver.LocalPath(repoURL)
	if !ok {
		return nil, nil, false
	}
	identity, err := ParseRemoteURL(repoURL)
	if err != nil {
		locationErr := &LocationError{RepoID: repoURL, Operation: "fetch", Code: "invalid_url", Message: err.Error()}
		return nil, locationErr, true
	}
	if !localRepoExists(localPath) {
		msg := fmt.Sprintf("folder not found: %s", localPath)
		c.localResolver.OnLocalDegraded(repoURL, "missing", "path_missing", msg)
		return nil, &LocationError{RepoID: identity.ID(), Operation: "fetch", Code: "path_missing", Message: msg}, true
	}
	if err := c.FetchIntoLocal(ctx, localPath, identity, baseSHA, headSHA, opts); err != nil {
		var fetchErr *LocalFetchError
		if errors.As(err, &fetchErr) && fetchErr.Code == remoteMismatchCode {
			msg := err.Error()
			c.localResolver.OnLocalDegraded(repoURL, remoteMismatchCode, remoteMismatchCode, msg)
			return nil, &LocationError{RepoID: identity.ID(), Operation: "fetch", Code: remoteMismatchCode, Message: msg}, true
		}
		return nil, err, true
	}
	c.localResolver.OnUsed(repoURL)
	return &CloneResult{LocalPath: localPath, BaseSHA: baseSHA, HeadSHA: headSHA}, nil, true
}

func (c *Client) cloneWithOptions(ctx context.Context, repoURL, baseSHA, headSHA string, opts *CloneOptions) (*CloneResult, error) {
	log := logging.FromContext(ctx).With(
		"repo_url", repoURL,
		"base_sha", truncateSHAForLog(baseSHA),
		"head_sha", truncateSHAForLog(headSHA),
	)
	if repoURL == "" {
		return nil, fmt.Errorf("repository URL cannot be empty")
	}
	if baseSHA == "" {
		return nil, fmt.Errorf("baseSHA cannot be empty")
	}
	if headSHA == "" {
		return nil, fmt.Errorf("headSHA cannot be empty")
	}

	var result *CloneResult
	operation := func() error {
		var err error
		result, err = c.cloneWithOptionsLocked(ctx, log, repoURL, baseSHA, headSHA, opts)
		return err
	}
	if c.locks == nil {
		return result, operation()
	}
	if err := c.locks.WithRepo(repoLockKey(repoURL), operation); err != nil {
		return nil, err
	}
	return result, nil
}

func repoLockKey(repoURL string) string {
	if os.Getenv("BISTRO_REPO_DIR") != "" {
		return repoURL
	}
	identity, err := ParseRemoteURL(repoURL)
	if err != nil {
		return repoURL
	}
	return identity.ID()
}

func (c *Client) cloneWithOptionsLocked(ctx context.Context, log *slog.Logger, repoURL, baseSHA, headSHA string, opts *CloneOptions) (*CloneResult, error) {
	if result, err, ok := c.resolveLocal(ctx, repoURL, baseSHA, headSHA, opts); ok {
		return result, err
	}

	// Check composite cache — keyed on both SHAs to avoid stale hits after force-pushes.
	// A hit is only honored when the clone still exists on disk: the in-memory cache
	// outlives the directory, which can be removed by the OS cache reaper or a user
	// clearing ~/Library/Caches. Returning a path that no longer exists would surface
	// go-git's misleading "repository does not exist" on the next diff. If the dir is
	// gone, drop the stale entry and fall through to a fresh clone.
	if cached, ok := c.cache.Get(repoURL, baseSHA, headSHA); ok {
		if c.validateCacheHit(log, repoURL, baseSHA, headSHA, cached, opts) {
			return &CloneResult{
				LocalPath: cached.LocalPath,
				BaseSHA:   cached.BaseSHA,
				HeadSHA:   cached.HeadSHA,
			}, nil
		}
	}

	// Targeted shallow fetch: only attempted when caller supplied a PR number
	// or base branch. On success it caches and returns directly; on any failure
	// it logs the reason and falls through to the broad clone below.
	if opts != nil && (opts.PRNumber > 0 || opts.BaseBranch != "") {
		if result, ok := c.targetedClone(ctx, log, repoURL, baseSHA, headSHA, opts); ok {
			return result, nil
		}
	}

	log.Debug("git clone start")
	overallStart := time.Now()

	tmpDir, err := prepareCacheCloneDir(repoURL)
	if err != nil {
		log.Error("failed to prepare clone dir", "error", err)
		return nil, fmt.Errorf("failed to prepare clone directory: %w", err)
	}

	cloneOpts := &gitlib.CloneOptions{
		URL:  repoURL,
		Auth: c.auth(),
	}

	cloneStart := time.Now()
	// Clone bare (no working tree): diff generation reads commit objects and
	// trees directly via the object store, so a checkout is pure overhead —
	// it doubles disk I/O and writes files we never open. GenerateDiff and
	// FetchRefs both go through PlainOpen, which works against bare repos.
	repo, err := gitlib.PlainCloneContext(ctx, tmpDir, true, cloneOpts)
	cloneDur := time.Since(cloneStart).Milliseconds()
	if err != nil {
		os.RemoveAll(tmpDir)
		log.Error("git clone failed",
			"duration_ms", cloneDur,
			"error", err,
		)
		return nil, fmt.Errorf("failed to clone repository %s: %w", repoURL, err)
	}
	log.Debug("git clone done", "duration_ms", cloneDur, "local_path", tmpDir)

	// Fetch both branch heads and PR refs so base and head commits are locally available
	fetchOpts := &gitlib.FetchOptions{
		Auth: c.auth(),
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
			config.RefSpec("+refs/pull/*/head:refs/remotes/origin/pr/*"),
		},
	}

	fetchStart := time.Now()
	if err := repo.FetchContext(ctx, fetchOpts); err != nil && err != gitlib.NoErrAlreadyUpToDate {
		os.RemoveAll(tmpDir)
		log.Error("git fetch refs failed",
			"duration_ms", time.Since(fetchStart).Milliseconds(),
			"error", err,
		)
		return nil, fmt.Errorf("failed to fetch refs: %w", err)
	}
	log.Debug("git fetch refs done", "duration_ms", time.Since(fetchStart).Milliseconds())

	// Verify both commits are present — fail fast before writing to cache
	if _, err := repo.CommitObject(plumbing.NewHash(baseSHA)); err != nil {
		os.RemoveAll(tmpDir)
		log.Warn("base commit missing after fetch", "error", err)
		return nil, fmt.Errorf("base commit %s not found after fetch: %w", baseSHA, err)
	}
	if _, err := repo.CommitObject(plumbing.NewHash(headSHA)); err != nil {
		os.RemoveAll(tmpDir)
		log.Warn("head commit missing after fetch", "error", err)
		return nil, fmt.Errorf("head commit %s not found after fetch: %w", headSHA, err)
	}

	result := &CloneResult{
		LocalPath: tmpDir,
		BaseSHA:   baseSHA,
		HeadSHA:   headSHA,
	}

	c.cache.Set(repoURL, baseSHA, headSHA, &RepoEntry{
		URL:       repoURL,
		LocalPath: tmpDir,
		BaseSHA:   baseSHA,
		HeadSHA:   headSHA,
		FetchedAt: time.Now(),
	})
	c.recordCacheCloned(repoURL, tmpDir)

	log.Debug("git clone complete",
		"total_duration_ms", time.Since(overallStart).Milliseconds(),
		"local_path", tmpDir,
	)
	return result, nil
}

func (c *Client) validateCacheHit(
	log *slog.Logger,
	repoURL string,
	baseSHA string,
	headSHA string,
	cached *RepoEntry,
	opts *CloneOptions,
) bool {
	if !localRepoExists(cached.LocalPath) {
		log.Warn("git clone cache entry points at a missing directory; re-cloning",
			"local_path", cached.LocalPath)
		c.cache.Clear(repoURL, baseSHA, headSHA)
		return false
	}

	required := cacheHitRequiredSHAs(baseSHA, headSHA, opts)
	missing, err := firstMissingCommit(cached.LocalPath, required)
	if err != nil {
		log.Warn("git clone cache hit unverifiable; re-cloning",
			"local_path", cached.LocalPath, "error", err)
		c.cache.Clear(repoURL, baseSHA, headSHA)
		return false
	}
	if missing != "" {
		log.Debug("git clone cache hit missing a required commit; re-cloning",
			"local_path", cached.LocalPath, "missing_sha", truncateSHAForLog(missing))
		c.cache.Clear(repoURL, baseSHA, headSHA)
		return false
	}

	log.Debug("git clone cache hit (commits verified)", "local_path", cached.LocalPath)
	c.recordCacheCloned(repoURL, cached.LocalPath)
	return true
}

func cacheHitRequiredSHAs(baseSHA, headSHA string, opts *CloneOptions) []string {
	required := make([]string, 0, 2)
	required = append(required, baseSHA, headSHA)
	if opts != nil {
		required = append(required, opts.RequiredSHAs...)
	}
	return required
}

// targetedClone attempts a bare init + remote + shallow fetch of only the PR
// head ref and base branch, avoiding the broad "all heads + all PR refs" fetch.
// It returns (result, true) only when both baseSHA and headSHA are proven to
// exist locally after the fetch. On any failure — fetch error, or either commit
// missing (e.g. base SHA deeper than the shallow window) — it cleans up, logs
// the fallback reason, and returns (nil, false) so the caller proceeds with the
// broad clone path. This keeps fallback correctness regardless of fetch depth.
func (c *Client) targetedClone(ctx context.Context, log *slog.Logger, repoURL, baseSHA, headSHA string, opts *CloneOptions) (*CloneResult, bool) {
	start := time.Now()

	tmpDir, err := prepareTargetedCloneDir(repoURL)
	if err != nil {
		log.Warn("targeted fetch fallback: cannot prepare clone dir", "error", err)
		return nil, false
	}
	cleanup, err := stageTargetedCloneDir(repoURL, tmpDir)
	if err != nil {
		log.Warn("targeted fetch fallback: cannot stage clone dir", "error", err)
		return nil, false
	}

	// PlainInit (bare) + CreateRemote, rather than PlainClone, so we never
	// download the default branch up front — PlainClone always fetches it
	// before any extra refspec, which is exactly the broad cost we want to
	// avoid on the targeted path.
	repo, err := gitlib.PlainInit(tmpDir, true)
	if err != nil {
		cleanup(false)
		log.Warn("targeted fetch fallback: plain init failed", "error", err)
		return nil, false
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{repoURL},
	}); err != nil {
		cleanup(false)
		log.Warn("targeted fetch fallback: create remote failed", "error", err)
		return nil, false
	}

	refSpecs := targetedRefSpecs(opts)
	if len(refSpecs) == 0 {
		cleanup(false)
		return nil, false
	}

	depth := targetedDepth(opts)

	fetchOpts := &gitlib.FetchOptions{
		Auth:     c.auth(),
		RefSpecs: refSpecs,
		Depth:    depth,
		Tags:     gitlib.NoTags,
	}

	if err := repo.FetchContext(ctx, fetchOpts); err != nil && err != gitlib.NoErrAlreadyUpToDate {
		cleanup(false)
		log.Debug("targeted fetch fallback: fetch failed",
			"pr_number", opts.PRNumber,
			"base_branch", opts.BaseBranch,
			"depth", depth,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return nil, false
	}

	// Verify every commit the caller depends on exists before trusting the
	// shallow result. baseSHA and headSHA are always required; RequiredSHAs
	// carries anchor-resolution SHAs (per-comment CommitSHA / OriginalSHA) that
	// must also be reachable so anchoring does not silently degrade on a
	// targeted clone. A shallow base-branch fetch frequently stops short of the
	// PR base SHA (which is usually an ancestor, not the tip) or an older
	// comment commit; when any required commit is missing we fall back to the
	// broad clone rather than cache an incomplete repo.
	if missing := verifyRequiredCommits(repo, baseSHA, headSHA, opts); missing != "" {
		cleanup(false)
		log.Debug("targeted fetch fallback: required commit missing in shallow fetch",
			"missing_sha", truncateSHAForLog(missing), "depth", depth)
		return nil, false
	}

	cleanup(true)

	c.cache.Set(repoURL, baseSHA, headSHA, &RepoEntry{
		URL:       repoURL,
		LocalPath: tmpDir,
		BaseSHA:   baseSHA,
		HeadSHA:   headSHA,
		FetchedAt: time.Now(),
	})
	c.recordCacheCloned(repoURL, tmpDir)

	log.Debug("targeted shallow fetch complete",
		"pr_number", opts.PRNumber,
		"base_branch", opts.BaseBranch,
		"depth", depth,
		"duration_ms", time.Since(start).Milliseconds(),
		"local_path", tmpDir,
	)
	return &CloneResult{
		LocalPath: tmpDir,
		BaseSHA:   baseSHA,
		HeadSHA:   headSHA,
	}, true
}

// targetedRefSpecs builds the PR-specific refspecs for a targeted fetch.
// Destination refs live under a private refs/bistro/* namespace rather than
// refs/remotes/origin/* so a targeted fetch can never collide with or shadow
// refs written by the broad clone path (which owns refs/remotes/origin/*). The
// destination ref is irrelevant to commit reachability — commits are resolved
// by SHA, not by ref — so the isolated namespace is purely defensive. Shared by
// the disk targeted path and the in-memory PR-specific path. Returns an empty
// slice when opts supplies neither a PR number nor a base branch.
func targetedRefSpecs(opts *CloneOptions) []config.RefSpec {
	refSpecs := make([]config.RefSpec, 0, 2)
	if opts == nil {
		return refSpecs
	}
	if opts.PRNumber > 0 {
		refSpecs = append(refSpecs, config.RefSpec(fmt.Sprintf(
			"+refs/pull/%d/head:refs/bistro/pull/%d/head", opts.PRNumber, opts.PRNumber)))
	}
	if opts.BaseBranch != "" {
		refSpecs = append(refSpecs, config.RefSpec(fmt.Sprintf(
			"+refs/heads/%s:refs/bistro/base/%s", opts.BaseBranch, opts.BaseBranch)))
	}
	return refSpecs
}

// targetedDepth returns the shallow fetch depth for a targeted fetch, applying
// defaultTargetedFetchDepth when opts is nil or specifies no positive depth.
func targetedDepth(opts *CloneOptions) int {
	if opts == nil || opts.Depth <= 0 {
		return defaultTargetedFetchDepth
	}
	return opts.Depth
}

// verifyRequiredCommits checks that baseSHA, headSHA, and every non-empty
// opts.RequiredSHA resolve to a commit object in repo. It returns the first
// missing SHA, or "" when all required commits are present. Shared by the disk
// targeted path and the in-memory path so both enforce identical commit
// availability before trusting a shallow fetch.
func verifyRequiredCommits(repo *gitlib.Repository, baseSHA, headSHA string, opts *CloneOptions) string {
	required := make([]string, 0, 2)
	required = append(required, baseSHA, headSHA)
	if opts != nil {
		required = append(required, opts.RequiredSHAs...)
	}
	for _, sha := range required {
		if sha == "" {
			continue
		}
		if _, err := repo.CommitObject(plumbing.NewHash(sha)); err != nil {
			return sha
		}
	}
	return ""
}

// firstMissingCommit opens the repository at localPath and returns the first
// SHA in shas whose commit object is not present locally. Empty SHAs are
// skipped. Returns ("", nil) when every non-empty SHA is present. A non-nil
// error means the repo could not be opened (the caller should treat the cache
// entry as unusable and re-clone).
func firstMissingCommit(localPath string, shas []string) (string, error) {
	repo, err := gitlib.PlainOpen(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to open repository at %s: %w", localPath, err)
	}
	for _, sha := range shas {
		if sha == "" {
			continue
		}
		if _, err := repo.CommitObject(plumbing.NewHash(sha)); err != nil {
			return sha, nil
		}
	}
	return "", nil
}

// localRepoExists reports whether path is a non-empty string naming an existing
// filesystem entry. Used to validate a cache hit before trusting its clone path.
func localRepoExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// truncateSHAForLog returns a 7-char prefix of a SHA for compact log output.
func truncateSHAForLog(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

// FetchRefs updates an existing local repository by fetching all branch and PR
// refs, then verifies both baseSHA and headSHA are locally available.
func (c *Client) FetchRefs(ctx context.Context, localPath, baseSHA, headSHA string) error {
	if localPath == "" {
		return fmt.Errorf("local path cannot be empty")
	}
	if baseSHA == "" {
		return fmt.Errorf("baseSHA cannot be empty")
	}
	if headSHA == "" {
		return fmt.Errorf("headSHA cannot be empty")
	}

	repo, err := gitlib.PlainOpen(localPath)
	if err != nil {
		return fmt.Errorf("failed to open repository at %s: %w", localPath, err)
	}

	fetchOpts := &gitlib.FetchOptions{
		Auth: c.auth(),
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
			config.RefSpec("+refs/pull/*/head:refs/remotes/origin/pr/*"),
		},
	}

	if err := repo.FetchContext(ctx, fetchOpts); err != nil && err != gitlib.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch refs: %w", err)
	}

	if _, err := repo.CommitObject(plumbing.NewHash(baseSHA)); err != nil {
		return fmt.Errorf("base commit %s not available after fetch: %w", baseSHA, err)
	}
	if _, err := repo.CommitObject(plumbing.NewHash(headSHA)); err != nil {
		return fmt.Errorf("head commit %s not available after fetch: %w", headSHA, err)
	}

	return nil
}

// tokenFromEnv returns HTTP basic auth from GITHUB_TOKEN env var, or nil if unset.
func tokenFromEnv() *http.BasicAuth {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil
	}
	return &http.BasicAuth{
		Username: "x-token", // GitHub ignores username when token is used
		Password: token,
	}
}
