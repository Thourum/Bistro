package git

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/itterno/bistro/internal/git/clonecache"
	"github.com/itterno/bistro/internal/logging"
)

func requiredSHAs(baseSHA, headSHA string, opts *CloneOptions) []string {
	required := []string{baseSHA, headSHA}
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
// broad clone path. A rollback failure is returned instead of falling through
// and potentially replacing a clone that could not be restored.
func (c *Client) targetedClone(ctx context.Context, log *slog.Logger, repoURL, baseSHA, headSHA string, opts *CloneOptions) (*CloneResult, bool, error) {
	start := time.Now()

	tmpDir, err := clonecache.PrepareTargetedDir(repoURL)
	if err != nil {
		log.Warn("targeted fetch fallback: cannot prepare clone dir", "error", err)
		return nil, false, nil
	}
	cleanup, err := clonecache.StageTargetedDir(repoURL, tmpDir)
	if err != nil {
		log.Warn("targeted fetch fallback: cannot stage clone dir", "error", err)
		return nil, false, nil
	}

	// Avoid PlainClone's default-branch download before fetching targeted refs.
	repo, err := initTargetedRepo(tmpDir, repoURL)
	if err != nil {
		if cleanupErr := cleanup.Rollback(); cleanupErr != nil {
			return nil, false, errors.Join(err, fmt.Errorf("restore previous clone: %w", cleanupErr))
		}
		log.Warn("targeted fetch fallback: cannot initialize repository", "error", err)
		return nil, false, nil
	}

	depth := targetedDepth(opts)
	if err := c.fetchTargetedRefs(ctx, repo, opts, depth); err != nil {
		if cleanupErr := cleanup.Rollback(); cleanupErr != nil {
			return nil, false, errors.Join(err, fmt.Errorf("restore previous clone: %w", cleanupErr))
		}
		log.Debug("targeted fetch fallback: fetch failed",
			"pr_number", opts.PRNumber,
			"base_branch", opts.BaseBranch,
			"depth", depth,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return nil, false, nil
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
		if cleanupErr := cleanup.Rollback(); cleanupErr != nil {
			return nil, false, fmt.Errorf("restore previous clone after missing commit %s: %w", missing, cleanupErr)
		}
		log.Debug("targeted fetch fallback: required commit missing in shallow fetch",
			"missing_sha", logging.ShortSHA(missing), "depth", depth)
		return nil, false, nil
	}

	if err := cleanup.Commit(); err != nil {
		log.Warn("failed to remove previous clone backup", "error", err)
	}

	c.cache.Set(repoURL, baseSHA, headSHA, &clonecache.RepoEntry{
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
	}, true, nil
}

func initTargetedRepo(path, repoURL string) (*gitlib.Repository, error) {
	repo, err := gitlib.PlainInit(path, true)
	if err != nil {
		return nil, fmt.Errorf("initialize targeted repository: %w", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{repoURL}}); err != nil {
		return nil, fmt.Errorf("create targeted remote: %w", err)
	}
	return repo, nil
}

func (c *Client) fetchTargetedRefs(ctx context.Context, repo *gitlib.Repository, opts *CloneOptions, depth int) error {
	refSpecs := targetedRefSpecs(opts)
	if len(refSpecs) == 0 {
		return errors.New("targeted fetch requires a PR number or base branch")
	}
	fetchOpts := &gitlib.FetchOptions{
		Auth:     c.auth(),
		RefSpecs: refSpecs,
		Depth:    depth,
		Tags:     gitlib.NoTags,
	}
	if err := repo.FetchContext(ctx, fetchOpts); err != nil && !errors.Is(err, gitlib.NoErrAlreadyUpToDate) {
		return fmt.Errorf("fetch targeted refs: %w", err)
	}
	return nil
}

// targetedRefSpecs builds the PR-specific refspecs for a targeted fetch.
// Destination refs live under a private refs/bistro/* namespace rather than
// refs/remotes/origin/* so a targeted fetch can never collide with or shadow
// refs written by the broad clone path (which owns refs/remotes/origin/*). The
// destination ref is irrelevant to commit reachability — commits are resolved
// by SHA, not by ref — so the isolated namespace is purely defensive. Shared by
// the targeted path. Returns an empty slice when opts supplies neither a PR
// number nor a base branch.
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
// missing SHA, or "" when all required commits are present. The targeted
// clone checks this before trusting a shallow fetch.
func verifyRequiredCommits(repo *gitlib.Repository, baseSHA, headSHA string, opts *CloneOptions) string {
	return firstMissingCommitInRepo(repo, requiredSHAs(baseSHA, headSHA, opts))
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
	return firstMissingCommitInRepo(repo, shas), nil
}

func firstMissingCommitInRepo(repo *gitlib.Repository, shas []string) string {
	for _, sha := range shas {
		if sha == "" {
			continue
		}
		if _, err := repo.CommitObject(plumbing.NewHash(sha)); err != nil {
			return sha
		}
	}
	return ""
}
