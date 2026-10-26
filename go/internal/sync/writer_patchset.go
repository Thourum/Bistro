package sync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

type patchsetWrite struct {
	pr        *provider.PR
	mergeBase string           // "" when unknown
	files     []store.DiffFile // nil when the diff was not computed this sync
}

type latestPatchset struct {
	number    int
	headSHA   string
	mergeBase string
}

// recordPatchset returns the PR's current patchset number, recording a new
// patchset when the head or a known merge base changed. A base tip that moves
// without changing the merge base is the same patchset.
func (w *Writer) recordPatchset(ctx context.Context, tx *sql.Tx, write patchsetWrite) (int, error) {
	latest, found, err := readLatestPatchset(ctx, tx, write.pr.ID)
	if err != nil {
		return 0, err
	}
	number := latest.number
	if !found || isNewPatchset(latest, write) {
		number, err = insertPatchset(ctx, tx, write, latest.number+1)
		if err != nil {
			return 0, err
		}
	}
	if err := backfillMergeBase(ctx, tx, write, number); err != nil {
		return 0, err
	}
	// Base-tip fallback diffs can include upstream-only files; leave their
	// fingerprints pending until the PR's merge base is known.
	if write.files == nil || write.mergeBase == "" {
		return number, nil
	}
	return number, writeFingerprintsOnce(ctx, tx, write, number)
}

func isNewPatchset(latest latestPatchset, write patchsetWrite) bool {
	if latest.headSHA != write.pr.HeadSHA {
		return true
	}
	return write.mergeBase != "" && latest.mergeBase != "" && latest.mergeBase != write.mergeBase
}

func readLatestPatchset(ctx context.Context, tx *sql.Tx, prID string) (latestPatchset, bool, error) {
	var latest latestPatchset
	err := tx.QueryRowContext(ctx,
		`SELECT number, head_sha, merge_base_sha FROM patchsets WHERE pr_id = ? ORDER BY number DESC LIMIT 1`,
		prID,
	).Scan(&latest.number, &latest.headSHA, &latest.mergeBase)
	if errors.Is(err, sql.ErrNoRows) {
		return latestPatchset{}, false, nil
	}
	if err != nil {
		return latestPatchset{}, false, fmt.Errorf("query latest patchset for PR %s: %w", prID, err)
	}
	return latest, true, nil
}

// insertPatchset inserts P<number>. UNIQUE(pr_id, base_sha, head_sha) ignores
// a return to an exact earlier (base tip, head) pair; the highest-numbered
// patchset at that head stays current.
func insertPatchset(ctx context.Context, tx *sql.Tx, write patchsetWrite, number int) (int, error) {
	pr := write.pr
	inserted, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO patchsets (pr_id, number, head_sha, base_sha, merge_base_sha, pushed_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		pr.ID, number, pr.HeadSHA, pr.BaseSHA, write.mergeBase, time.Now())
	if err != nil {
		return 0, fmt.Errorf("insert patchset %s P%d: %w", pr.ID, number, err)
	}
	rows, err := inserted.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read inserted patchset count %s P%d: %w", pr.ID, number, err)
	}
	if rows == 1 {
		return number, nil
	}
	var existing int
	if err := tx.QueryRowContext(ctx,
		`SELECT number FROM patchsets WHERE pr_id = ? AND head_sha = ? ORDER BY number DESC LIMIT 1`,
		pr.ID, pr.HeadSHA,
	).Scan(&existing); err != nil {
		return 0, fmt.Errorf("read existing patchset %s at %s: %w", pr.ID, logging.ShortSHA(pr.HeadSHA), err)
	}
	return existing, nil
}

func backfillMergeBase(ctx context.Context, tx *sql.Tx, write patchsetWrite, number int) error {
	if write.mergeBase == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE patchsets SET merge_base_sha = ? WHERE pr_id = ? AND number = ? AND merge_base_sha = ''`,
		write.mergeBase, write.pr.ID, number,
	); err != nil {
		return fmt.Errorf("backfill merge base %s P%d: %w", write.pr.ID, number, err)
	}
	return nil
}

// writeFingerprintsOnce stores per-file patch IDs the first time a diff is
// computed against a known merge base; the same head and merge base yield the same
// diff, so later syncs have nothing to add.
func writeFingerprintsOnce(ctx context.Context, tx *sql.Tx, write patchsetWrite, number int) error {
	marked, err := tx.ExecContext(ctx,
		`UPDATE patchsets SET has_fingerprints = 1 WHERE pr_id = ? AND number = ? AND has_fingerprints = 0`,
		write.pr.ID, number)
	if err != nil {
		return fmt.Errorf("mark fingerprints %s P%d: %w", write.pr.ID, number, err)
	}
	rows, err := marked.RowsAffected()
	if err != nil {
		return fmt.Errorf("read marked fingerprint count %s P%d: %w", write.pr.ID, number, err)
	}
	if rows == 0 {
		return nil
	}
	for _, file := range write.files {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO patchset_files (pr_id, patchset_number, path, patch_id) VALUES (?, ?, ?, ?)`,
			write.pr.ID, number, file.FilePath, file.PatchID,
		); err != nil {
			return fmt.Errorf("insert fingerprint %s P%d %s: %w", write.pr.ID, number, file.FilePath, err)
		}
	}
	return nil
}

// recordGitHubReviewBaseline offers the current user's latest submitted
// review (browser, gh, or Bistro) as the since-last-review baseline.
// Best-effort: the PR write never fails because of it.
func (w *Writer) recordGitHubReviewBaseline(ctx context.Context, batch *writerBatch, result *PRFetchResult) {
	if w.currentUser == nil || len(result.Reviews) == 0 {
		return
	}
	login, ok, err := w.currentUser.Get(ctx, "github")
	if err != nil || !ok || login == "" {
		return // No user means no baseline to attribute; mentions handle the logging.
	}
	review := latestSubmittedReview(result.Reviews, login)
	if review == nil {
		return
	}
	if err := store.RecordLastReviewed(ctx, batch.tx, store.ReviewEvent{
		PRID: result.PR.ID, Login: login, HeadSHA: review.CommitID,
		ReviewedAt: review.SubmittedAt, Source: store.ReviewSourceGitHubReview,
	}); err != nil {
		batch.log.Warn("record GitHub review baseline failed", "pr_id", result.PR.ID, "error", err)
	}
}

func latestSubmittedReview(reviews []*provider.Review, login string) *provider.Review {
	var latest *provider.Review
	for _, review := range reviews {
		isSubmitted := review != nil && review.State != "pending" && review.CommitID != "" && !review.SubmittedAt.IsZero()
		if !isSubmitted || !strings.EqualFold(review.Author, login) {
			continue
		}
		if latest == nil || review.SubmittedAt.After(latest.SubmittedAt) {
			latest = review
		}
	}
	return latest
}
