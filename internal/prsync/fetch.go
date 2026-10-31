package prsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/itterno/bistro/internal/git"
	gitdiff "github.com/itterno/bistro/internal/git/diff"
	"github.com/itterno/bistro/internal/git/remote"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/prsync/workerpool"
	"github.com/itterno/bistro/internal/prsync/writer"
	"github.com/itterno/bistro/internal/store/prdiff"
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

// FetchPRDetails fetches comments, reviews, threads, CI, and diffs concurrently.
// Results preserve input order; per-PR errors are stored in writer.FetchedPR.Error.
// Only a worker-pool failure is returned as an error.
func (o *Orchestrator) FetchPRDetails(
	ctx context.Context,
	prs []*provider.PR,
	includeComments, includeCI, includeDiff bool,
) ([]*writer.FetchedPR, error) {
	log := logging.FromContext(ctx)
	log.Debug("FetchPRDetails start",
		"pr_count", len(prs),
		"workers", workerpool.DefaultWorkers,
		"include_comments", includeComments,
		"include_ci", includeCI,
		"include_diff", includeDiff,
	)

	options := prFetchOptions{comments: includeComments, ci: includeCI, diff: includeDiff}
	results := make([]*writer.FetchedPR, len(prs))
	pool := workerpool.New(workerpool.DefaultWorkers, o.log, workerpool.Options{OnPanic: o.onWorkerPanic})
	// Cancellation must not leave tasks writing results or using stores after return.
	defer func() { _ = pool.Wait(context.Background()) }()
	for i, pr := range prs {
		if err := pool.Submit(ctx, func() {
			results[i] = o.fetchPR(ctx, pr, options)
		}); err != nil {
			log.Warn("task submission cancelled", "pr_number", pr.Number, "pr_id", pr.ID, "error", err)
			results[i] = &writer.FetchedPR{PR: pr, Error: fmt.Errorf("task submission cancelled: %w", err)}
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

func (o *Orchestrator) fetchPR(ctx context.Context, pr *provider.PR, options prFetchOptions) *writer.FetchedPR {
	started := time.Now()
	log := logging.FromContext(ctx).With(
		"pr_number", pr.Number,
		"pr_id", pr.ID,
		"pr_state", pr.State,
		"pr_base_sha", logging.ShortSHA(pr.BaseSHA),
		"pr_head_sha", logging.ShortSHA(pr.HeadSHA),
	)
	boundedCtx, cancel := context.WithTimeout(ctx, perPRFetchTimeout)
	defer cancel()
	taskCtx := logging.WithLogger(boundedCtx, log)
	log.Debug("pr fetch start", "title", pr.Title)

	result := &writer.FetchedPR{PR: pr}
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

func (o *Orchestrator) fetchComments(ctx context.Context, result *writer.FetchedPR, log *slog.Logger) error {
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

func (o *Orchestrator) fetchReviews(ctx context.Context, result *writer.FetchedPR, log *slog.Logger) error {
	started := time.Now()
	reviews, err := o.provider.ListReviews(ctx, result.PR.ID)
	if err != nil {
		return fmt.Errorf("reviews: %w", err)
	}
	result.Reviews = reviews
	status, approvedBy, changesBy := provider.DeriveReviewStatus(reviews)
	result.PR.ReviewStatus = status
	result.PR.ApprovedBy = approvedBy
	result.PR.ChangesRequestedBy = changesBy
	logSubstep(log, "pr reviews fetched", time.Since(started).Milliseconds(),
		"count", len(reviews), "review_status", status)
	return nil
}

func (o *Orchestrator) fetchThreads(ctx context.Context, result *writer.FetchedPR, log *slog.Logger) {
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

func (o *Orchestrator) fetchCI(ctx context.Context, result *writer.FetchedPR, log *slog.Logger) error {
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

func (o *Orchestrator) fetchDiff(ctx context.Context, result *writer.FetchedPR, log *slog.Logger) error {
	pr := result.PR
	cloneURL := remote.GitHubCloneURL(pr.Owner, pr.Repo)
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

func (o *Orchestrator) generateDiff(ctx context.Context, result *writer.FetchedPR, clone *git.CloneResult, log *slog.Logger) error {
	pr := result.PR
	started := time.Now()
	mergeBase := o.resolveMergeBase(ctx, clone, pr, log)
	diffBase := pr.BaseSHA
	if mergeBase != "" {
		diffBase = mergeBase
	}
	diff, err := gitdiff.Generate(clone.LocalPath, diffBase, pr.HeadSHA, gitdiff.DefaultOptions())
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
		"profile_load_commits_ms", diff.Profile.LoadCommits.Milliseconds(),
		"profile_load_trees_ms", diff.Profile.LoadTrees.Milliseconds(),
		"profile_build_patch_ms", diff.Profile.BuildPatch.Milliseconds(),
		"profile_render_patches_ms", diff.Profile.RenderFilePatches.Milliseconds(),
	)

	// Diffs must be written atomically with the PR row and patchset by the writer.
	result.DiffFiles = storeDiffFiles(diff)
	result.DiffComputed = true
	result.MergeBaseSHA = mergeBase
	o.notifyExpertiseHook(ctx, pr, result.DiffFiles)
	o.resolveDiffAnchors(ctx, result, clone, diffBase, log)
	return nil
}

// resolveMergeBase returns the PR's merge base, deepening a shallow clone once
// when history stops short of it. "" means it stayed unknown; the caller then
// diffs against the base tip.
func (o *Orchestrator) resolveMergeBase(ctx context.Context, clone *git.CloneResult, pr *provider.PR, log *slog.Logger) string {
	mergeBase, err := git.MergeBase(clone.LocalPath, pr.BaseSHA, pr.HeadSHA)
	if errors.Is(err, git.ErrMergeBaseNotFound) {
		if deepenErr := o.git.Deepen(ctx, clone.LocalPath, diffCloneOptions(pr)); deepenErr != nil {
			log.Debug("merge base deepen failed", "error", deepenErr)
		} else {
			mergeBase, err = git.MergeBase(clone.LocalPath, pr.BaseSHA, pr.HeadSHA)
		}
	}
	if err != nil {
		log.Warn("merge base unavailable; diffing against base tip", "error", err)
		return ""
	}
	return mergeBase
}

func storeDiffFiles(diff *gitdiff.Result) []prdiff.File {
	files := make([]prdiff.File, 0, len(diff.Files))
	for _, file := range diff.Files {
		files = append(files, prdiff.File{
			FilePath:    file.Path,
			Status:      file.Status,
			DiffStatus:  file.DiffStatus,
			OldPath:     file.OldPath,
			IsBinary:    file.Binary,
			IsTruncated: file.Truncated,
			PatchBytes:  file.PatchBytes,
			HunkCount:   file.HunkCount,
			Patch:       file.Patch,
			PatchID:     file.PatchID,
			Additions:   file.AddedLines,
			Deletions:   file.RemovedLines,
		})
	}
	return files
}

func (o *Orchestrator) resolveDiffAnchors(ctx context.Context, result *writer.FetchedPR, clone *git.CloneResult, diffBase string, log *slog.Logger) {
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
		prBaseSHA: diffBase,
		log:       log,
	})
	logSubstep(log, "anchor resolution complete", time.Since(started).Milliseconds(),
		"anchored", len(result.AnchorUpdates))
}

func (o *Orchestrator) notifyExpertiseHook(ctx context.Context, pr *provider.PR, diffFiles []prdiff.File) {
	if o.expertise == nil || o.resolveRepoPath == nil || pr == nil {
		return
	}
	repoID := remote.Identity{Provider: pr.Provider, Owner: pr.Owner, Repo: pr.Repo}.ID()
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

// diffCloneOptions excludes anchor-only historical commits from the shallow diff clone.
func diffCloneOptions(pr *provider.PR) *git.CloneOptions {
	if pr == nil {
		return &git.CloneOptions{}
	}
	return &git.CloneOptions{PRNumber: pr.Number, BaseBranch: pr.BaseBranch}
}
