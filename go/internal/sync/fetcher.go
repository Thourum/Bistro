package sync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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
	// CommentsFetched and ThreadsFetched distinguish a successful empty refresh
	// from a write where the corresponding source was not fetched. Mention state
	// may only be cleared once every source has been refreshed.
	CommentsFetched bool
	ThreadsFetched  bool
	CI              *provider.CIStatus
	DiffSize        int64
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
	// ListOnly marks a result written without a detail fetch (very active
	// repo over the hydration threshold). The PR row is stored hydrated=0 and
	// details are fetched on demand; an already-hydrated row is never
	// downgraded.
	ListOnly bool
	Error    error
}

// FetchPRDetails fetches comments, reviews, threads, CI, and diffs concurrently.
// Results preserve input order; per-PR errors are stored in PRFetchResult.Error.
// Only a worker-pool failure is returned as an error.
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

	options := prFetchOptions{comments: includeComments, ci: includeCI, diff: includeDiff}
	results := make([]*PRFetchResult, len(prs))
	pool := NewWorkerPool(DefaultWorkers, o.log)
	for i, pr := range prs {
		if err := pool.Submit(ctx, func() {
			results[i] = o.fetchPR(ctx, pr, options)
		}); err != nil {
			log.Warn("task submission cancelled", "pr_number", pr.Number, "pr_id", pr.ID, "error", err)
			results[i] = &PRFetchResult{PR: pr, Error: fmt.Errorf("task submission cancelled: %w", err)}
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

type prFetchOptions struct {
	comments bool
	ci       bool
	diff     bool
}

func (o *Orchestrator) fetchPR(ctx context.Context, pr *provider.PR, options prFetchOptions) *PRFetchResult {
	started := time.Now()
	log := logging.FromContext(ctx).With(
		"pr_number", pr.Number,
		"pr_id", pr.ID,
		"pr_state", pr.State,
		"pr_base_sha", truncateSHA(pr.BaseSHA),
		"pr_head_sha", truncateSHA(pr.HeadSHA),
	)
	boundedCtx, cancel := context.WithTimeout(ctx, perPRFetchTimeout)
	defer cancel()
	taskCtx := logging.WithLogger(boundedCtx, log)
	log.Debug("pr fetch start", "title", pr.Title)

	result := &PRFetchResult{PR: pr}
	var fetchErrors []error
	if options.comments && o.provider != nil {
		if err := o.fetchComments(taskCtx, result, log); err != nil {
			fetchErrors = append(fetchErrors, err)
		}
		if err := o.fetchReviews(taskCtx, result, log); err != nil {
			fetchErrors = append(fetchErrors, err)
		}
		o.fetchThreads(taskCtx, result, log)
	}
	if options.ci && o.provider != nil {
		if err := o.fetchCI(taskCtx, result, log); err != nil {
			fetchErrors = append(fetchErrors, err)
		}
	}
	if options.diff && o.git != nil {
		if err := o.fetchDiff(taskCtx, result, log); err != nil {
			fetchErrors = append(fetchErrors, err)
		}
	}

	result.Error = joinFetchErrors(fetchErrors)
	if len(fetchErrors) != 0 {
		log.Warn("pr fetch finished with errors", "errors", len(fetchErrors),
			"duration_ms", time.Since(started).Milliseconds(), "error", result.Error)
	} else {
		log.Debug("pr fetch finished", "duration_ms", time.Since(started).Milliseconds())
	}
	return result
}

func joinFetchErrors(substepErrors []error) error {
	if len(substepErrors) == 0 {
		return nil
	}
	messages := make([]string, 0, len(substepErrors))
	for _, err := range substepErrors {
		messages = append(messages, err.Error())
	}
	return &fetchErrors{message: strings.Join(messages, "; "), causes: substepErrors}
}

type fetchErrors struct {
	message string
	causes  []error
}

func (errs *fetchErrors) Error() string   { return errs.message }
func (errs *fetchErrors) Unwrap() []error { return errs.causes }

func (o *Orchestrator) fetchComments(ctx context.Context, result *PRFetchResult, log *slog.Logger) error {
	started := time.Now()
	comments, err := o.provider.GetComments(ctx, result.PR.ID)
	if err != nil {
		return fmt.Errorf("comments: %w", err)
	}
	result.Comments = comments
	result.CommentsFetched = true
	logSubstep(log, "pr comments fetched", time.Since(started).Milliseconds(), "count", len(comments))
	return nil
}

func (o *Orchestrator) fetchReviews(ctx context.Context, result *PRFetchResult, log *slog.Logger) error {
	started := time.Now()
	reviews, err := o.provider.ListReviews(ctx, result.PR.ID)
	if err != nil {
		return fmt.Errorf("reviews: %w", err)
	}
	status, approvedBy, changesBy := provider.DeriveReviewStatus(reviews)
	result.PR.ReviewStatus = status
	result.PR.ApprovedBy = approvedBy
	result.PR.ChangesRequestedBy = changesBy
	logSubstep(log, "pr reviews fetched", time.Since(started).Milliseconds(),
		"count", len(reviews), "review_status", status)
	return nil
}

func (o *Orchestrator) fetchThreads(ctx context.Context, result *PRFetchResult, log *slog.Logger) {
	threadProvider, ok := o.provider.(provider.ThreadProvider)
	if !ok {
		result.ThreadsFetched = true
		return
	}
	started := time.Now()
	threads, err := threadProvider.GetReviewThreads(ctx, result.PR.ID)
	if err != nil {
		// Thread data is best-effort and must not prevent other details being persisted.
		log.Warn("review thread fetch failed (non-fatal)",
			"duration_ms", time.Since(started).Milliseconds(), "error", err)
		return
	}
	result.Threads = threads
	result.ThreadsFetched = true
	logSubstep(log, "pr review threads fetched", time.Since(started).Milliseconds(), "count", len(threads))
}

func (o *Orchestrator) fetchCI(ctx context.Context, result *PRFetchResult, log *slog.Logger) error {
	started := time.Now()
	ci, err := o.provider.GetCIStatus(ctx, result.PR.ID)
	if err != nil {
		return fmt.Errorf("ci: %w", err)
	}
	result.CI = ci
	status := ""
	if ci != nil {
		status = ci.Status
	}
	logSubstep(log, "pr ci fetched", time.Since(started).Milliseconds(), "status", status)
	return nil
}

func (o *Orchestrator) fetchDiff(ctx context.Context, result *PRFetchResult, log *slog.Logger) error {
	pr := result.PR
	cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", pr.Owner, pr.Repo)
	// The diff and anchor reads must happen while the repo lock is held: another
	// PR's clone of the same repo replaces the shared cache directory.
	err := o.git.WithPRClone(ctx, cloneURL, pr.BaseSHA, pr.HeadSHA, diffCloneOptions(pr),
		func(clone *git.CloneResult) error {
			return o.generateDiff(ctx, result, clone, log)
		})
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}
	return nil
}

func (o *Orchestrator) generateDiff(ctx context.Context, result *PRFetchResult, clone *git.CloneResult, log *slog.Logger) error {
	pr := result.PR
	started := time.Now()
	diff, err := o.git.GenerateDiff(clone.LocalPath, pr.BaseSHA, pr.HeadSHA, git.DefaultDiffOptions())
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}
	if diff == nil {
		return nil
	}
	result.DiffSize = diff.Size
	logSubstep(log, "pr diff generated", time.Since(started).Milliseconds(),
		"bytes", diff.Size,
		"files", len(diff.Files),
		"binary_files", diff.Profile.BinaryFileCount,
		"profile_total_ms", diff.Profile.TotalDuration.Milliseconds(),
		"profile_open_repo_ms", diff.Profile.OpenRepository.Milliseconds(),
		"profile_load_commits_ms", diff.Profile.LoadCommits.Milliseconds(),
		"profile_load_trees_ms", diff.Profile.LoadTrees.Milliseconds(),
		"profile_build_patch_ms", diff.Profile.BuildPatch.Milliseconds(),
		"profile_render_patches_ms", diff.Profile.RenderFilePatches.Milliseconds(),
	)

	// Diffs must be written atomically with the PR row and patchset by the writer.
	result.DiffFiles = storeDiffFiles(diff)
	result.DiffComputed = true
	o.notifyExpertiseHook(ctx, pr, result.DiffFiles)
	o.resolveDiffAnchors(ctx, result, clone, log)
	return nil
}

func storeDiffFiles(diff *git.DiffResult) []store.DiffFile {
	files := make([]store.DiffFile, 0, len(diff.Files))
	for _, file := range diff.Files {
		files = append(files, store.DiffFile{
			FilePath:    file.Path,
			Status:      file.Status,
			DiffStatus:  file.DiffStatus,
			OldPath:     file.OldPath,
			IsBinary:    file.Binary,
			IsTruncated: file.Truncated,
			PatchBytes:  file.PatchBytes,
			HunkCount:   file.HunkCount,
			Patch:       file.Patch,
			Additions:   file.AddedLines,
			Deletions:   file.RemovedLines,
		})
	}
	return files
}

func (o *Orchestrator) resolveDiffAnchors(ctx context.Context, result *PRFetchResult, clone *git.CloneResult, log *slog.Logger) {
	if o.anchorResolver == nil {
		return
	}
	// Anchors are best-effort against the same shallow clone; missing historical
	// commits must not force a broader clone before the diff is persisted.
	started := time.Now()
	pr := result.PR
	result.AnchorUpdates = o.resolveAnchors(ctx, anchorResolutionOptions{
		comments:  result.Comments,
		threads:   result.Threads,
		prID:      pr.ID,
		localPath: clone.LocalPath,
		headSHA:   pr.HeadSHA,
		prBaseSHA: pr.BaseSHA,
		log:       log,
	})
	logSubstep(log, "anchor resolution complete", time.Since(started).Milliseconds(),
		"anchored", len(result.AnchorUpdates))
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

const shaLogPrefixLength = 7 // Git's conventional abbreviated SHA length.

func truncateSHA(sha string) string {
	if len(sha) > shaLogPrefixLength {
		return sha[:shaLogPrefixLength]
	}
	return sha
}

// diffCloneOptions excludes anchor-only historical commits from the shallow diff clone.
func diffCloneOptions(pr *provider.PR) *git.CloneOptions {
	if pr == nil {
		return &git.CloneOptions{}
	}
	return &git.CloneOptions{PRNumber: pr.Number, BaseBranch: pr.BaseBranch}
}
