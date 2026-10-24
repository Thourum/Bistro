package handler

import (
	"context"
	"time"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store"
)

// viewedBaselineStore records the all-viewed baseline. Satisfied by *store.DB.
type viewedBaselineStore interface {
	ListViewedFiles(ctx context.Context, prID, login string) ([]store.FileViewed, error)
	CurrentPatchset(ctx context.Context, prID string) (store.Patchset, bool, error)
	ListPatchsetFiles(ctx context.Context, prID string, number int) (map[string]string, error)
	RecordLastReviewed(ctx context.Context, event store.ReviewEvent) error
}

// recordAllViewedBaseline is best-effort because the viewed mark already succeeded.
func recordAllViewedBaseline(ctx context.Context, deps MarkFileViewedDeps, prID, login string) {
	if deps.Baseline == nil || deps.DiffStore == nil {
		return
	}
	logger := logging.FromContext(ctx)
	diffs, err := deps.DiffStore.GetDiffs(prID)
	if err != nil {
		logger.Warn("all-viewed baseline: diff lookup failed", "pr_id", prID, "error", err)
		return
	}
	viewed, err := deps.Baseline.ListViewedFiles(ctx, prID, login)
	if err != nil {
		logger.Warn("all-viewed baseline: viewed lookup failed", "pr_id", prID, "error", err)
		return
	}
	if !allFilesViewed(diffs, viewed) {
		return
	}
	patchset, found, err := deps.Baseline.CurrentPatchset(ctx, prID)
	if err != nil || !found {
		logger.Warn("all-viewed baseline: no current patchset", "pr_id", prID, "found", found, "error", err)
		return
	}
	if !patchset.HasFingerprints || !cachedDiffsMatchPatchset(ctx, deps.Baseline, patchset, diffs) {
		logger.Debug("all-viewed baseline: current patchset diffs pending", "pr_id", prID, "patchset", patchset.Number)
		return
	}
	if err := deps.Baseline.RecordLastReviewed(ctx, store.ReviewEvent{
		PRID: prID, Login: login, HeadSHA: patchset.HeadSHA, PatchsetNum: patchset.Number,
		ReviewedAt: time.Now(), Source: store.ReviewSourceAllViewed,
	}); err != nil {
		logger.Warn("all-viewed baseline: record failed", "pr_id", prID, "error", err)
	}
}

// Cached diffs can outlive their head when a list-only sync advances the PR.
func cachedDiffsMatchPatchset(ctx context.Context, baseline viewedBaselineStore, patchset store.Patchset, diffs []store.DiffFile) bool {
	fingerprints, err := baseline.ListPatchsetFiles(ctx, patchset.PRID, patchset.Number)
	if err != nil {
		logging.FromContext(ctx).Warn("all-viewed baseline: fingerprint lookup failed", "pr_id", patchset.PRID, "error", err)
		return false
	}
	if len(fingerprints) != len(diffs) {
		return false
	}
	for _, diff := range diffs {
		patchID, found := fingerprints[diff.FilePath]
		if !found || patchID != viewedHash(diff) {
			return false
		}
	}
	return true
}

func allFilesViewed(diffs []store.DiffFile, viewed []store.FileViewed) bool {
	hashes := make(map[string]string, len(viewed))
	for _, file := range viewed {
		hashes[file.FilePath] = file.ContentHash
	}
	for _, diff := range diffs {
		if hashes[diff.FilePath] != viewedHash(diff) {
			return false
		}
	}
	return len(diffs) > 0
}
