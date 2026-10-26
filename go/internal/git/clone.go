package git

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	gitlib "github.com/go-git/go-git/v5"

	"github.com/itterno/bistro/internal/git/remote"
	"github.com/itterno/bistro/internal/git/repolock"
	"github.com/itterno/bistro/internal/logging"
)

// defaultTargetedFetchDepth is the shallow depth used for a PR-targeted fetch.
// A PR head commit is the tip of refs/pull/<N>/head, so depth 1 would suffice
// for head — but the PR base SHA is usually an ancestor of the base branch tip
// (the merge-base at PR creation / last sync), not the tip itself. A modest
// depth gives the targeted fetch a reasonable chance of pulling the base SHA
// along the base branch's first-parent history before falling back to a broad
// fetch. Required-commit verification keeps this safe regardless of depth.
const (
	defaultTargetedFetchDepth    = 50
	locationFetchOperation       = "fetch"
	locationInvalidURLCode       = "invalid_url"
	locationMissingStatus        = "missing"
	locationPathMissingCode      = "path_missing"
	locationPermissionDeniedCode = "permission_denied"
	locationCloneFailedCode      = "clone_failed"
)

// CloneResult tracks a successful clone
type CloneResult struct {
	LocalPath string // Where the repo is stored
	BaseSHA   string // Base commit SHA
	HeadSHA   string // Head commit SHA
}

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

// CloneRepoToPath clones repoURL into destination. Bare clones are used for
// Bistro-managed cache storage; non-bare clones are used for user-owned dev
// folder destinations so the user gets a normal checkout.
// It is used by repo-location handlers that already resolved the exact cache/dev
// folder path and need the same auth/locking behavior as the production git client.
func (c *Client) CloneRepoToPath(ctx context.Context, repoURL, destination string, bare bool) error {
	operation := func() error {
		cloneOpts := &gitlib.CloneOptions{URL: repoURL, Auth: c.auth(), Tags: gitlib.NoTags}
		if _, err := gitlib.PlainCloneContext(ctx, destination, bare, cloneOpts); err != nil {
			return fmt.Errorf("clone repository %s: %w", repoURL, err)
		}
		if bare {
			c.recordCacheCloned(repoURL, destination)
		}
		return nil
	}
	if c.locks == nil {
		return operation()
	}
	return c.locks.WithRepo(repolock.Key(repoURL), operation)
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

func (c *Client) recordCacheCloned(repoURL, path string) {
	if os.Getenv("BISTRO_REPO_DIR") != "" || c.localResolver == nil {
		return
	}
	c.localResolver.OnCacheCloned(repoURL, path)
}

func (c *Client) resolveLocal(ctx context.Context, repoURL, baseSHA, headSHA string, opts *CloneOptions) (*CloneResult, bool, error) {
	if os.Getenv("BISTRO_REPO_DIR") != "" || c.localResolver == nil {
		return nil, false, nil
	}
	localPath, ok := c.localResolver.LocalPath(repoURL)
	if !ok {
		return nil, false, nil
	}
	identity, err := remote.ParseURL(repoURL)
	if err != nil {
		return nil, true, &remote.LocationError{RepoID: repoURL, Operation: locationFetchOperation, Code: locationInvalidURLCode, Message: err.Error(), Err: err}
	}
	if !localRepoExists(localPath) {
		msg := fmt.Sprintf("folder not found: %s", localPath)
		c.localResolver.OnLocalDegraded(repoURL, locationMissingStatus, locationPathMissingCode, msg)
		return nil, true, &remote.LocationError{RepoID: identity.ID(), Operation: locationFetchOperation, Code: locationPathMissingCode, Message: msg}
	}
	if err := c.FetchIntoLocal(ctx, localPath, identity, baseSHA, headSHA, opts); err != nil {
		msg := err.Error()
		var fetchErr *LocalFetchError
		if errors.As(err, &fetchErr) && fetchErr.Code == remoteMismatchCode {
			c.localResolver.OnLocalDegraded(repoURL, remoteMismatchCode, remoteMismatchCode, msg)
			return nil, true, &remote.LocationError{RepoID: identity.ID(), Operation: locationFetchOperation, Code: remoteMismatchCode, Message: msg, Err: err}
		}
		if errors.Is(err, os.ErrPermission) {
			c.localResolver.OnLocalDegraded(repoURL, locationPermissionDeniedCode, locationPermissionDeniedCode, msg)
			return nil, true, &remote.LocationError{RepoID: identity.ID(), Operation: locationFetchOperation, Code: locationPermissionDeniedCode, Message: msg, Err: err}
		}
		// Context cancellation stays bare so callers can detect it directly.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, true, err
		}
		// Transient failures (network, missing commits) are surfaced as a
		// structured remote.LocationError without marking the repo degraded.
		return nil, true, &remote.LocationError{RepoID: identity.ID(), Operation: locationFetchOperation, Code: locationCloneFailedCode, Message: msg, Err: err}
	}
	c.localResolver.OnUsed(repoURL)
	return &CloneResult{LocalPath: localPath, BaseSHA: baseSHA, HeadSHA: headSHA}, true, nil
}

func (c *Client) cloneWithOptions(ctx context.Context, repoURL, baseSHA, headSHA string, opts *CloneOptions) (*CloneResult, error) {
	var result *CloneResult
	err := c.WithPRClone(ctx, repoURL, baseSHA, headSHA, opts, func(clone *CloneResult) error {
		result = clone
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// WithPRClone resolves the repo clone for the SHAs and passes it to use while
// still holding the repo's exclusive lock. Clones replace the shared cache
// directory, so anything that reads it (diffs, anchor lookups) must run inside
// use, not after WithPRClone returns.
func (c *Client) WithPRClone(ctx context.Context, repoURL, baseSHA, headSHA string, opts *CloneOptions, use func(*CloneResult) error) error {
	log := logging.FromContext(ctx).With(
		"repo_url", repoURL,
		"base_sha", logging.ShortSHA(baseSHA),
		"head_sha", logging.ShortSHA(headSHA),
	)
	if repoURL == "" {
		return errors.New("repository URL cannot be empty")
	}
	if baseSHA == "" {
		return errors.New("baseSHA cannot be empty")
	}
	if headSHA == "" {
		return errors.New("headSHA cannot be empty")
	}

	operation := func() error {
		clone, err := c.cloneWithOptionsLocked(ctx, log, repoURL, baseSHA, headSHA, opts)
		if err != nil {
			return err
		}
		pinPRHead(log, clone.LocalPath, repoURL, opts, headSHA)
		return use(clone)
	}
	if c.locks == nil {
		return operation()
	}
	return c.locks.WithRepo(repolock.Key(repoURL), operation)
}

func (c *Client) cloneWithOptionsLocked(ctx context.Context, log *slog.Logger, repoURL, baseSHA, headSHA string, opts *CloneOptions) (*CloneResult, error) {
	if result, handled, err := c.resolveLocal(ctx, repoURL, baseSHA, headSHA, opts); handled {
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

	if result, ok := c.reuseOnDiskClone(log, repoURL, baseSHA, headSHA, opts); ok {
		return result, nil
	}
	if result, ok := c.refreshOnDiskClone(ctx, log, repoURL, baseSHA, headSHA, opts); ok {
		return result, nil
	}

	// Targeted shallow fetch: only attempted when caller supplied a PR number
	// or base branch. On success it caches and returns directly; on any failure
	// it logs the reason and falls through to the broad clone below.
	if opts != nil && (opts.PRNumber > 0 || opts.BaseBranch != "") {
		result, ok, err := c.targetedClone(ctx, log, repoURL, baseSHA, headSHA, opts)
		if err != nil {
			return nil, err
		}
		if ok {
			return result, nil
		}
	}

	return c.broadClone(ctx, log, repoURL, baseSHA, headSHA)
}
