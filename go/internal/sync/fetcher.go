package sync

import (
	"context"
	"fmt"
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
					prLog.Debug("pr comments fetched",
						"count", len(comments),
						"duration_ms", subDur,
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
					prLog.Debug("pr reviews fetched",
						"count", len(reviews),
						"review_status", status,
						"duration_ms", subDur,
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
						prLog.Debug("pr review threads fetched",
							"count", len(threads), "duration_ms", subDur)
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
					prLog.Debug("pr ci fetched",
						"status", status,
						"duration_ms", subDur,
					)
				}
			}

			if includeDiff && o.git != nil {
				cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", prCopy.Owner, prCopy.Repo)
				subStart := time.Now()
				cloneResult, cloneErr := o.git.Clone(taskCtx, cloneURL, prCopy.BaseSHA, prCopy.HeadSHA)
				cloneDur := time.Since(subStart).Milliseconds()
				if cloneErr != nil {
					errs = append(errs, fmt.Sprintf("diff clone: %v", cloneErr))
					prLog.Error("repo clone failed",
						"clone_url", cloneURL,
						"duration_ms", cloneDur,
						"error", cloneErr,
					)
				} else {
					prLog.Debug("repo cloned",
						"local_path", cloneResult.LocalPath,
						"duration_ms", cloneDur,
					)
					// NOTE: do NOT remove cloneResult.LocalPath here. The git
					// Client owns this directory via its process-lifetime cache
					// (keyed on base+head SHA) and reuses it across sync cycles
					// and for range-diff. Deleting it leaves a live cache entry
					// pointing at a missing dir, which surfaces as the misleading
					// "repository does not exist" on the next diff.

					diffStart := time.Now()
					diffResult, err := o.git.GenerateDiff(cloneResult.LocalPath, prCopy.BaseSHA, prCopy.HeadSHA)
					diffDur := time.Since(diffStart).Milliseconds()
					if err != nil {
						errs = append(errs, fmt.Sprintf("diff: %v", err))
						prLog.Error("diff generation failed",
							"duration_ms", diffDur,
							"error", err,
						)
					} else if diffResult != nil {
						result.DiffSize = diffResult.Size
						prLog.Debug("pr diff generated",
							"bytes", diffResult.Size,
							"files", len(diffResult.Files),
							"duration_ms", diffDur,
						)

						// Build DiffFiles to be persisted atomically with the PR row
						// and patchset in WriteSyncResultsContext. Do NOT call SaveDiffs
						// here — at this point the PR row may not yet exist (new PR
						// initial sync) and the write must be transactional with PR/patchset.
						diffFiles := make([]store.DiffFile, 0, len(diffResult.Files))
						for _, f := range diffResult.Files {
							diffFiles = append(diffFiles, store.DiffFile{
								FilePath:  f.Path,
								Patch:     f.Patch,
								Additions: f.AddedLines,
								Deletions: f.RemovedLines,
							})
						}
						result.DiffFiles = diffFiles
						result.DiffComputed = true

						// Run anchor resolution while the clone is still on disk.
						// Results are carried in AnchorUpdates and written back to
						// the DB by the writer (best-effort, outside the savepoint).
						if o.anchorResolver != nil {
							anchorStart := time.Now()
							result.AnchorUpdates = o.resolveAnchors(
								taskCtx, result.Comments, result.Threads,
								prCopy.ID, cloneResult.LocalPath, prCopy.HeadSHA, prCopy.BaseSHA,
								prLog,
							)
							prLog.Debug("anchor resolution complete",
								"anchored", len(result.AnchorUpdates),
								"duration_ms", time.Since(anchorStart).Milliseconds(),
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

// truncateSHA returns a short 7-char prefix of a git SHA for log readability.
func truncateSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
