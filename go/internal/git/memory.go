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

// fetchForPRInMemoryRaw performs the PR-specific targeted shallow fetch into a
// fresh bare in-memory repository (memory.Storage, nil worktree) and returns the
// repo WITHOUT verifying that any particular commit is present. It isolates the
// transport/object-transfer cost (Init + CreateRemote + FetchContext) so
// benchmarks can measure fetch-only time and depth sensitivity independently of
// commit verification and diff generation.
//
// Pure go-git v5 + in-memory storer: no disk writes, no BISTRO_REPO_DIR, no
// on-disk cache, no GitHub raw diff, no native git CLI.
func (c *Client) fetchForPRInMemoryRaw(ctx context.Context, repoURL string, opts *CloneOptions) (*gitlib.Repository, error) {
	if repoURL == "" {
		return nil, fmt.Errorf("repository URL cannot be empty")
	}
	refSpecs := targetedRefSpecs(opts)
	if len(refSpecs) == 0 {
		return nil, fmt.Errorf("in-memory PR fetch requires a PR number or base branch")
	}
	depth := targetedDepth(opts)

	// Bare in-memory repository: object store lives entirely in RAM, no working
	// tree (nil billy.Filesystem). Diff generation reads commit/tree objects
	// straight from this store, so a worktree would be pure overhead.
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
	return repo, nil
}

// fetchForPRInMemory performs the targeted in-memory fetch (fetchForPRInMemoryRaw)
// and then enforces the same correctness gate as the disk targeted path: every
// required commit (baseSHA, headSHA, and any opts.RequiredSHAs) must be present
// before the repository is returned. Unlike the disk path there is no broad
// fallback — callers that need full history must use the disk CloneForPR.
func (c *Client) fetchForPRInMemory(ctx context.Context, repoURL, baseSHA, headSHA string, opts *CloneOptions) (*gitlib.Repository, error) {
	if baseSHA == "" {
		return nil, fmt.Errorf("baseSHA cannot be empty")
	}
	if headSHA == "" {
		return nil, fmt.Errorf("headSHA cannot be empty")
	}
	repo, err := c.fetchForPRInMemoryRaw(ctx, repoURL, opts)
	if err != nil {
		return nil, err
	}
	if missing := verifyRequiredCommits(repo, baseSHA, headSHA, opts); missing != "" {
		return nil, fmt.Errorf("in-memory targeted fetch missing required commit %s (depth=%d)",
			truncateSHAForLog(missing), targetedDepth(opts))
	}
	return repo, nil
}

// GenerateDiffForPRInMemory is a Phase 4/5 PROTOTYPE: it performs a PR-specific,
// targeted shallow fetch entirely in memory (no disk clone, no working tree)
// and computes the per-file diff between baseSHA and headSHA directly from the
// in-memory object store. It delegates to fetchForPRInMemory and then reuses the
// shared generateDiffFromRepo core.
//
// It mirrors the disk targeted path (targetedClone) exactly for fetch shape:
//   - go-git Init over memory.NewStorage() (bare; nil worktree)
//   - CreateRemote("origin") + FetchContext with the same private refs/bistro/*
//     refspecs, Depth, and Tags: NoTags
//   - RequiredSHAs verification (base, head, and anchor SHAs) before trusting
//     the result — identical to verifyRequiredCommits on the disk path
//
// SCOPE / CAVEAT — diff-only prototype, NOT wired into production FetchPRDetails.
// See the "Phase 5 production-wiring decision" note in internal/sync/fetcher.go
// for the full rationale; in short: anchor resolution requires an on-disk
// localPath, and a single PRFetchResult couples the diff with AnchorUpdates
// (both persisted together after FetchPRDetails returns), so a memory-first diff
// cannot be delivered to the user before anchors and would not reduce the
// user-visible latency of a result that still needs a disk clone for anchoring.
//
// No GitHub raw diff and no native git CLI are used; this is pure go-git v5 with
// an in-memory storer. It does not touch BISTRO_REPO_DIR or the on-disk cache.
func (c *Client) GenerateDiffForPRInMemory(ctx context.Context, repoURL, baseSHA, headSHA string, opts *CloneOptions) (*DiffResult, error) {
	log := logging.FromContext(ctx).With(
		"repo_url", repoURL,
		"base_sha", truncateSHAForLog(baseSHA),
		"head_sha", truncateSHAForLog(headSHA),
	)
	start := time.Now()

	repo, err := c.fetchForPRInMemory(ctx, repoURL, baseSHA, headSHA, opts)
	if err != nil {
		return nil, err
	}

	result, err := generateDiffFromRepo(repo, baseSHA, headSHA, DefaultDiffOptions())
	if err != nil {
		return nil, fmt.Errorf("in-memory diff generation failed: %w", err)
	}

	log.Debug("in-memory PR fetch+diff complete",
		"pr_number", prNumberOf(opts),
		"base_branch", baseBranchOf(opts),
		"depth", targetedDepth(opts),
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
