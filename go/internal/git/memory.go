package git

import (
	"context"
	"fmt"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/itterno/bistro/internal/logging"
)

// GenerateDiffForPRInMemory is a Phase 4 PROTOTYPE: it performs a PR-specific,
// targeted shallow fetch entirely in memory (no disk clone, no working tree)
// and computes the per-file diff between baseSHA and headSHA directly from the
// in-memory object store.
//
// It mirrors the disk targeted path (targetedClone) exactly for fetch shape:
//   - go-git Init over memory.NewStorage() (bare; nil worktree)
//   - CreateRemote("origin") + FetchContext with the same private refs/bistro/*
//     refspecs, Depth, and Tags: NoTags
//   - RequiredSHAs verification (base, head, and anchor SHAs) before trusting
//     the result — identical to verifyRequiredCommits on the disk path
//
// SCOPE / CAVEAT — diff-only prototype, NOT wired into production FetchPRDetails:
// anchor resolution (resolveAnchors → AnchorResolver) currently requires an
// on-disk localPath to open the repository. This in-memory path produces no
// localPath, so it cannot feed anchoring. Production sync therefore continues to
// use the disk-backed CloneForPR + GenerateDiff(localPath, ...) so anchors keep
// working. This method exists so Phase 4 benchmarks can measure the in-memory
// fetch+diff cost against the disk targeted path; promoting it to production
// requires either an in-memory-capable anchor resolver or a hybrid (memory diff
// + lazy disk materialization for anchoring), which is out of scope here.
//
// No GitHub raw diff and no native git CLI are used; this is pure go-git v5 with
// an in-memory storer. It does not touch BISTRO_REPO_DIR or the on-disk cache.
func (c *Client) GenerateDiffForPRInMemory(ctx context.Context, repoURL, baseSHA, headSHA string, opts *CloneOptions) (*DiffResult, error) {
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

	refSpecs := targetedRefSpecs(opts)
	if len(refSpecs) == 0 {
		return nil, fmt.Errorf("in-memory PR fetch requires a PR number or base branch")
	}
	depth := targetedDepth(opts)
	start := time.Now()

	// Bare in-memory repository: object store lives entirely in RAM, no
	// working tree (nil billy.Filesystem). Diff generation reads commit/tree
	// objects straight from this store, so a worktree would be pure overhead.
	repo, err := gitlib.Init(memory.NewStorage(), nil)
	if err != nil {
		return nil, fmt.Errorf("in-memory init failed: %w", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{repoURL},
	}); err != nil {
		return nil, fmt.Errorf("in-memory create remote failed: %w", err)
	}

	fetchOpts := &gitlib.FetchOptions{
		Auth:     c.auth(),
		RefSpecs: refSpecs,
		Depth:    depth,
		Tags:     gitlib.NoTags,
	}
	if err := repo.FetchContext(ctx, fetchOpts); err != nil && err != gitlib.NoErrAlreadyUpToDate {
		return nil, fmt.Errorf("in-memory targeted fetch failed (pr=%d base_branch=%q depth=%d): %w",
			prNumberOf(opts), baseBranchOf(opts), depth, err)
	}

	// Same correctness gate as the disk targeted path: every required commit
	// must be present before we trust the shallow result.
	if missing := verifyRequiredCommits(repo, baseSHA, headSHA, opts); missing != "" {
		return nil, fmt.Errorf("in-memory targeted fetch missing required commit %s (depth=%d)",
			truncateSHAForLog(missing), depth)
	}

	result, err := generateDiffFromRepo(repo, baseSHA, headSHA)
	if err != nil {
		return nil, fmt.Errorf("in-memory diff generation failed: %w", err)
	}

	log.Debug("in-memory PR fetch+diff complete",
		"pr_number", prNumberOf(opts),
		"base_branch", baseBranchOf(opts),
		"depth", depth,
		"files", len(result.Files),
		"bytes", result.Size,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return result, nil
}

func prNumberOf(opts *CloneOptions) int {
	if opts == nil {
		return 0
	}
	return opts.PRNumber
}

func baseBranchOf(opts *CloneOptions) string {
	if opts == nil {
		return ""
	}
	return opts.BaseBranch
}
