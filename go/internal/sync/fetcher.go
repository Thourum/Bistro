package sync

import (
	"context"
	"fmt"
	"log/slog"
	gosync "sync"
	"time"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

// perPRFetchTimeout caps how long the detail fetch (comments, reviews, CI,
// diff) may take for a single PR before it is abandoned with an error.
const perPRFetchTimeout = 2 * time.Minute

// slowSubstepThresholdMs is the per-PR substep duration (comments, reviews,
// threads, CI, clone, diff, anchor) above which a normally-Debug timing record
// is escalated to Warn so unusually slow steps surface in ordinary runs.
const slowSubstepThresholdMs = 5000

// logSubstep emits a per-PR substep timing record. It stays at Debug for
// ordinary durations and escalates to Warn once duration_ms crosses
// slowSubstepThresholdMs, so a slow comments/CI/clone/diff step is visible
// without enabling Debug logging globally. attrs are appended after the
// standard duration_ms field.
func logSubstep(log *slog.Logger, msg string, durMs int64, attrs ...any) {
	all := make([]any, 0, len(attrs)+2)
	all = append(all, "duration_ms", durMs)
	all = append(all, attrs...)
	if durMs >= slowSubstepThresholdMs {
		log.Warn(msg+" (slow)", all...)
		return
	}
	log.Debug(msg, all...)
}

// PRFetchResult holds the fetched data for a single PR, including any
// per-PR error encountered during concurrent fetching.
type PRFetchResult struct {
	PR       *provider.PR
	Comments []*provider.Comment
	Threads  []*provider.ReviewThread // populated when provider implements ThreadProvider
	CI       *provider.CIStatus
	DiffSize int64
	// DiffFiles holds the per-file diffs to be persisted atomically with the PR
	// row and patchset in WriteSyncResultsContext.
	// nil means diff was not attempted or failed (preserve existing pr_diffs cache).
	// non-nil (including empty slice) means diff was successfully computed and
	// the writer should replace pr_diffs — even an empty slice clears stale rows.
	DiffFiles []store.DiffFile
	// DiffComputed is true iff diff generation succeeded and DiffFiles is valid.
	// When false, the writer must not touch pr_diffs for this result.
	DiffComputed bool
	// AnchorUpdates carries per-comment anchor results computed from the clone.
	// Populated only when the clone was available and anchoring ran without a
	// fatal error. Each entry is written back to the comments table after the
	// per-PR savepoint releases (best-effort; never blocks PR/patchset commit).
	AnchorUpdates []git.AnchorResult
	Error         error
}

// FetchPRDetails fetches comments, CI status, and diff size for each PR
// using a pool of DefaultWorkers concurrent workers. Results are returned in the same
// order as the input prs slice. Per-PR errors are captured in PRFetchResult.Error
// and do not abort fetching of other PRs. The returned error is non-nil only for
// fatal pool failures; individual PR errors are stored in PRFetchResult.Error.
//
// Each PR's fetch emits Debug records at every sub-step (start/comments/ci/diff
// clone/diff generate) so a stuck or dropped PR can be diagnosed from the logs
// alone.
func (o *Orchestrator) FetchPRDetails(
	ctx context.Context,
	prs []*provider.PR,
	includeComments, includeCI, includeDiff bool,
) ([]*PRFetchResult, error) {
	log := logging.FromContext(ctx)
	log.Debug("FetchPRDetails start",
		"pr_count", len(prs),
		"workers", DefaultWorkers,
		"include_comments", includeComments,
		"include_ci", includeCI,
		"include_diff", includeDiff,
	)

	results := make([]*PRFetchResult, len(prs))
	var mu gosync.Mutex

	pool := NewWorkerPool(DefaultWorkers, o.log)

	for i, pr := range prs {
		idx := i
		prCopy := pr

		task := func() {
			taskStart := time.Now()
			prLog := log.With(
				"pr_number", prCopy.Number,
				"pr_id", prCopy.ID,
				"pr_state", prCopy.State,
				"pr_base_sha", truncateSHA(prCopy.BaseSHA),
				"pr_head_sha", truncateSHA(prCopy.HeadSHA),
			)
			// Hard ceiling per PR so one stuck fetch can never wedge the
			// whole sync pass — it becomes a logged per-PR error instead.
			boundedCtx, cancel := context.WithTimeout(ctx, perPRFetchTimeout)
			defer cancel()
			taskCtx := logging.WithLogger(boundedCtx, prLog)

			prLog.Debug("pr fetch start", "title", prCopy.Title)

			result := &PRFetchResult{PR: prCopy}

			var errs []string

			if includeComments && o.provider != nil {
				subStart := time.Now()
				comments, err := o.provider.GetComments(taskCtx, prCopy.ID)
				subDur := time.Since(subStart).Milliseconds()
				if err != nil {
					errs = append(errs, fmt.Sprintf("comments: %v", err))
					prLog.Error("comment fetch failed",
						"duration_ms", subDur,
						"error", err,
					)
				} else {
					result.Comments = comments
					logSubstep(prLog, "pr comments fetched", subDur,
						"count", len(comments),
					)
				}
			}

			// Reviews ride along with the details pass: triage (Needs Action /
			// FYI) needs review_status refreshed on the same cadence as comments.
			if includeComments && o.provider != nil {
				subStart := time.Now()
				reviews, err := o.provider.ListReviews(taskCtx, prCopy.ID)
				subDur := time.Since(subStart).Milliseconds()
				if err != nil {
					errs = append(errs, fmt.Sprintf("reviews: %v", err))
					prLog.Error("review fetch failed",
						"duration_ms", subDur,
						"error", err,
					)
				} else {
					status, approvedBy, changesBy := provider.DeriveReviewStatus(reviews)
					prCopy.ReviewStatus = status
					prCopy.ApprovedBy = approvedBy
					prCopy.ChangesRequestedBy = changesBy
					logSubstep(prLog, "pr reviews fetched", subDur,
						"count", len(reviews),
						"review_status", status,
					)
				}
			}

			// GetReviewThreads: optional capability — graceful skip if not implemented.
			// Errors here are NOT added to `errs`: thread data is best-effort and must
			// never prevent the PR row, comments, CI, or diff from being persisted.
			if includeComments && o.provider != nil {
				if tp, ok := o.provider.(provider.ThreadProvider); ok {
					subStart := time.Now()
					threads, threadErr := tp.GetReviewThreads(taskCtx, prCopy.ID)
					subDur := time.Since(subStart).Milliseconds()
					if threadErr != nil {
						// Log at warn; do NOT append to errs so result.Error stays nil.
						prLog.Warn("review thread fetch failed (non-fatal)",
							"duration_ms", subDur, "error", threadErr)
					} else {
						result.Threads = threads
						logSubstep(prLog, "pr review threads fetched", subDur,
							"count", len(threads))
					}
				}
			}

			if includeCI && o.provider != nil {
				subStart := time.Now()
				ci, err := o.provider.GetCIStatus(taskCtx, prCopy.ID)
				subDur := time.Since(subStart).Milliseconds()
				if err != nil {
					errs = append(errs, fmt.Sprintf("ci: %v", err))
					prLog.Error("ci fetch failed",
						"duration_ms", subDur,
						"error", err,
					)
				} else {
					result.CI = ci
					status := ""
					if ci != nil {
						status = ci.Status
					}
					logSubstep(prLog, "pr ci fetched", subDur,
						"status", status,
					)
				}
			}

			// Phase 6 production path: the diff clone is BASE/HEAD-ONLY.
			//
			// This CloneForPR is on the diff critical path — its only job is to
			// make the PR's current base and head commits available so the diff
			// can be generated and persisted quickly. It therefore passes only
			// PRNumber + BaseBranch (targeted shallow fetch) and NO RequiredSHAs.
			//
			// Why no anchor/historical RequiredSHAs here: folding per-comment
			// CommitSHA/OriginalSHA and historical patchset BaseSHAs into this
			// clone's RequiredSHAs could force a broad full clone (~50s) whenever
			// any of those commits sits outside the shallow window — penalizing
			// every PR's diff readiness for the sake of anchoring. Anchors are
			// best-effort metadata; they must not gate or slow the diff.
			//
			// Anchor handling (see below): anchor resolution runs opportunistically
			// against this same fast clone. If a comment's historical/commit SHA
			// is not present in the shallow fetch, that specific anchor is skipped
			// (resolveAnchors is already best-effort) — it does NOT trigger a broad
			// fallback. Backfilling anchors that need deeper history is future work
			// (a separate anchor-specific job). collectAnchorSHAs /
			// collectPatchsetBaseSHAs and the git RequiredSHAs primitive are
			// retained for that future job; they intentionally no longer feed this
			// initial diff clone.
			if includeDiff && o.git != nil {
				cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", prCopy.Owner, prCopy.Repo)
				subStart := time.Now()
				cloneResult, cloneErr := o.git.CloneForPR(taskCtx, cloneURL, prCopy.BaseSHA, prCopy.HeadSHA,
					diffCloneOptions(prCopy))
				cloneDur := time.Since(subStart).Milliseconds()
				if cloneErr != nil {
					errs = append(errs, fmt.Sprintf("diff clone: %v", cloneErr))
					prLog.Error("repo clone failed",
						"clone_url", cloneURL,
						"duration_ms", cloneDur,
						"error", cloneErr,
					)
				} else {
					if cloneDur >= slowSubstepThresholdMs {
						prLog.Warn("repo clone slow", "duration_ms", cloneDur)
					} else {
						prLog.Debug("repo cloned",
							"local_path", cloneResult.LocalPath,
							"duration_ms", cloneDur,
						)
					}
					// NOTE: do NOT remove cloneResult.LocalPath here. The git
					// Client owns this directory via its process-lifetime cache
					// (keyed on base+head SHA) and reuses it across sync cycles
					// and for range-diff. Deleting it leaves a live cache entry
					// pointing at a missing dir, which surfaces as the misleading
					// "repository does not exist" on the next diff.

					diffStart := time.Now()
					diffResult, err := o.git.GenerateDiff(cloneResult.LocalPath, prCopy.BaseSHA, prCopy.HeadSHA, git.DefaultDiffOptions())
					diffDur := time.Since(diffStart).Milliseconds()
					if err != nil {
						errs = append(errs, fmt.Sprintf("diff: %v", err))
						prLog.Error("diff generation failed",
							"duration_ms", diffDur,
							"error", err,
						)
					} else if diffResult != nil {
						result.DiffSize = diffResult.Size
						logSubstep(prLog, "pr diff generated", diffDur,
							"bytes", diffResult.Size,
							"files", len(diffResult.Files),
							"binary_files", diffResult.Profile.BinaryFileCount,
							"duration_ms", diffDur,
							"profile_total_ms", diffResult.Profile.TotalDuration.Milliseconds(),
							"profile_open_repo_ms", diffResult.Profile.OpenRepository.Milliseconds(),
							"profile_load_commits_ms", diffResult.Profile.LoadCommits.Milliseconds(),
							"profile_load_trees_ms", diffResult.Profile.LoadTrees.Milliseconds(),
							"profile_build_patch_ms", diffResult.Profile.BuildPatch.Milliseconds(),
							"profile_render_patches_ms", diffResult.Profile.RenderFilePatches.Milliseconds(),
						)

						// Build DiffFiles to be persisted atomically with the PR row
						// and patchset in WriteSyncResultsContext. Do NOT call SaveDiffs
						// here — at this point the PR row may not yet exist (new PR
						// initial sync) and the write must be transactional with PR/patchset.
						diffFiles := make([]store.DiffFile, 0, len(diffResult.Files))
						for _, f := range diffResult.Files {
							diffFiles = append(diffFiles, store.DiffFile{
								FilePath:    f.Path,
								Status:      f.Status,
								DiffStatus:  f.DiffStatus,
								OldPath:     f.OldPath,
								IsBinary:    f.Binary,
								IsTruncated: f.Truncated,
								PatchBytes:  f.PatchBytes,
								HunkCount:   f.HunkCount,
								Patch:       f.Patch,
								Additions:   f.AddedLines,
								Deletions:   f.RemovedLines,
							})
						}
						result.DiffFiles = diffFiles
						result.DiffComputed = true
						o.notifyExpertiseHook(taskCtx, prCopy, diffFiles)

						// Anchor resolution is OPPORTUNISTIC against the fast
						// base/head-only clone above. Comments whose historical or
						// commit SHA is absent from this shallow fetch are skipped
						// by resolveAnchors (best-effort) rather than forcing a
						// broad re-clone — an anchor-only miss must never slow the
						// diff critical path. Results are carried in AnchorUpdates
						// and written back to the DB by the writer (best-effort,
						// outside the savepoint).
						if o.anchorResolver != nil {
							anchorStart := time.Now()
							result.AnchorUpdates = o.resolveAnchors(
								taskCtx, result.Comments, result.Threads,
								prCopy.ID, cloneResult.LocalPath, prCopy.HeadSHA, prCopy.BaseSHA,
								prLog,
							)
							logSubstep(prLog, "anchor resolution complete",
								time.Since(anchorStart).Milliseconds(),
								"anchored", len(result.AnchorUpdates),
							)
						}
					}
				}
			}

			if len(errs) > 0 {
				combined := ""
				for i, e := range errs {
					if i > 0 {
						combined += "; "
					}
					combined += e
				}
				result.Error = fmt.Errorf("%s", combined)
				prLog.Warn("pr fetch finished with errors",
					"errors", len(errs),
					"duration_ms", time.Since(taskStart).Milliseconds(),
					"error", result.Error,
				)
			} else {
				prLog.Debug("pr fetch finished",
					"duration_ms", time.Since(taskStart).Milliseconds(),
				)
			}

			mu.Lock()
			results[idx] = result
			mu.Unlock()
		}

		if err := pool.Submit(ctx, task); err != nil {
			log.Warn("task submission cancelled",
				"pr_number", prCopy.Number,
				"pr_id", prCopy.ID,
				"error", err,
			)
			mu.Lock()
			results[idx] = &PRFetchResult{PR: prCopy, Error: fmt.Errorf("task submission cancelled: %w", err)}
			mu.Unlock()
			break
		}
	}

	if err := pool.Wait(ctx); err != nil {
		log.Warn("worker pool wait cancelled", "error", err)
		return results, err
	}
	log.Debug("FetchPRDetails complete", "pr_count", len(prs))
	return results, nil
}

func (o *Orchestrator) notifyExpertiseHook(ctx context.Context, pr *provider.PR, diffFiles []store.DiffFile) {
	if o.expertise == nil || o.resolveRepoPath == nil || pr == nil {
		return
	}
	repoID := pr.Provider + ":" + pr.Owner + "/" + pr.Repo
	repoPath, ok := o.resolveRepoPath(repoID)
	if !ok || repoPath == "" {
		return
	}
	files := make([]string, 0, len(diffFiles))
	for _, diffFile := range diffFiles {
		files = append(files, diffFile.FilePath)
	}
	o.expertise.OnPRSynced(ctx, repoID, repoPath, files)
}

// truncateSHA returns a short 7-char prefix of a git SHA for log readability.
func truncateSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// diffCloneOptions builds the CloneOptions for the Phase 6 diff critical path.
// It is intentionally BASE/HEAD-ONLY: it carries the PR number and base branch
// (to drive the targeted shallow fetch of the current base/head) and never sets
// RequiredSHAs. Anchor/historical SHAs are deliberately excluded so an anchor-
// only miss can never force a broad full clone before the diff is persisted.
// This is the single source of truth for the diff clone's options.
func diffCloneOptions(pr *provider.PR) *git.CloneOptions {
	if pr == nil {
		return &git.CloneOptions{}
	}
	return &git.CloneOptions{
		PRNumber:   pr.Number,
		BaseBranch: pr.BaseBranch,
		// RequiredSHAs intentionally left nil — see comment above.
	}
}

// collectAnchorSHAs returns the unique, non-empty commit SHAs referenced by the
// given comments and review threads that anchor resolution depends on
// (per-comment CommitSHA and OriginalSHA).
//
// As of Phase 6 this is NOT used by the initial diff clone (the diff clone is
// base/head-only — see diffCloneOptions). It is retained for a future
// anchor-specific backfill job that may pre-fetch the commits needed to resolve
// anchors that the fast shallow diff clone did not include.
func collectAnchorSHAs(comments []*provider.Comment, threads []*provider.ReviewThread) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(sha string) {
		if sha == "" {
			return
		}
		if _, ok := seen[sha]; ok {
			return
		}
		seen[sha] = struct{}{}
		out = append(out, sha)
	}
	addComment := func(c *provider.Comment) {
		if c == nil || c.Kind != "review" {
			return
		}
		add(c.CommitSHA)
		add(c.OriginalSHA)
	}
	for _, c := range comments {
		addComment(c)
	}
	for _, th := range threads {
		if th == nil {
			continue
		}
		for _, c := range th.Comments {
			addComment(c)
		}
	}
	return out
}

// hasAnchorableOriginalPosition reports whether any review comment in the given
// comments/threads is an anchorable original-position comment — i.e. anchorable
// but lacking a current position, so anchor resolution would route through
// selectDiffBaseSHA and need a historical patchset BaseSHA. Mirrors the
// isCurrentPos test in resolveAnchors.
func hasAnchorableOriginalPosition(comments []*provider.Comment, threads []*provider.ReviewThread) bool {
	check := func(c *provider.Comment) bool {
		if c == nil || c.Kind != "review" || c.FilePath == "" || !isAnchorable(c) {
			return false
		}
		isCurrentPos := c.Position != nil && *c.Position > 0
		return !isCurrentPos
	}
	for _, c := range comments {
		if check(c) {
			return true
		}
	}
	for _, th := range threads {
		if th == nil {
			continue
		}
		for _, c := range th.Comments {
			if check(c) {
				return true
			}
		}
	}
	return false
}

// collectPatchsetBaseSHAs returns the unique non-empty patchset BaseSHA values
// for the PR that anchor resolution may need beyond the PR's current BaseSHA.
//
// Original-position comments resolve their diff geometry against a historical
// patchset's BaseSHA (see selectDiffBaseSHA), which can differ from
// prCopy.BaseSHA after a retarget/rebase.
//
// As of Phase 6 this is NOT used by the initial diff clone (the diff clone is
// base/head-only — see diffCloneOptions); folding these deep historical bases
// into the diff clone's RequiredSHAs could force a broad full clone and slow
// every PR's diff. It is retained for a future anchor-specific backfill job.
// Safe over-approximation when called: when anchoring is enabled, there is at
// least one anchorable original-position comment, and the store has patchsets
// for the PR, include every non-empty patchset BaseSHA. SHAs already present in
// `existing` (the per-comment anchor SHAs) are skipped to avoid duplicates.
func (o *Orchestrator) collectPatchsetBaseSHAs(
	ctx context.Context,
	prID string,
	comments []*provider.Comment,
	threads []*provider.ReviewThread,
	existing []string,
) []string {
	if o.anchorResolver == nil || o.store == nil || prID == "" {
		return nil
	}
	if !hasAnchorableOriginalPosition(comments, threads) {
		return nil
	}
	patchsets, err := o.store.ListPatchsets(ctx, prID)
	if err != nil {
		logging.FromContext(ctx).Warn(
			"collectPatchsetBaseSHAs: could not load patchsets; anchor base SHAs may be missing from targeted clone",
			"pr_id", prID, "error", err)
		return nil
	}
	seen := make(map[string]struct{}, len(existing))
	for _, s := range existing {
		if s != "" {
			seen[s] = struct{}{}
		}
	}
	var out []string
	for _, ps := range patchsets {
		if ps.BaseSHA == "" {
			continue
		}
		if _, ok := seen[ps.BaseSHA]; ok {
			continue
		}
		seen[ps.BaseSHA] = struct{}{}
		out = append(out, ps.BaseSHA)
	}
	return out
}
