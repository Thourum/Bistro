package writer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/itterno/bistro/internal/git/anchor"
	"github.com/itterno/bistro/internal/store/prdiff"
)

// writeDiffs replaces a PR's diff cache in the same savepoint as the PR row and
// patchset, preserving the old cache if any critical insert fails.
func (w *Writer) writeDiffs(ctx context.Context, tx *sql.Tx, prID string, diffs []prdiff.File) (writeErr error) {
	if _, err := tx.ExecContext(ctx, `DELETE FROM pr_diffs WHERE pr_id = ?`, prID); err != nil {
		return fmt.Errorf("clear pr_diffs for %s: %w", prID, err)
	}
	if len(diffs) == 0 {
		return nil // An empty diff clears stale cached files.
	}
	const query = `
		INSERT INTO pr_diffs (
			pr_id, file_path, status, old_path, is_binary, is_truncated,
			patch_bytes, hunk_count, patch, additions, deletions, diff_status, patch_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare pr_diffs insert for %s: %w", prID, err)
	}
	defer func() {
		if closeErr := stmt.Close(); closeErr != nil {
			// A failed statement close must not advance an incompletely written
			// diff cache; the caller rolls back this PR's savepoint.
			writeErr = errors.Join(writeErr, fmt.Errorf("close pr_diffs insert for %s: %w", prID, closeErr))
		}
	}()
	for _, diff := range diffs {
		if _, err := stmt.ExecContext(ctx, prID, diff.FilePath, diffStatus(diff.Status),
			nullableString(diff.OldPath), diff.IsBinary, diff.IsTruncated,
			diff.PatchBytes, diff.HunkCount, diff.Patch, diff.Additions, diff.Deletions,
			storedDiffStatus(diff.DiffStatus), diff.PatchID); err != nil {
			return fmt.Errorf("insert pr_diff %s/%s: %w", prID, diff.FilePath, err)
		}
	}
	return nil
}

func diffStatus(status string) string {
	if status == "" {
		return "modified"
	}
	return status
}

func storedDiffStatus(status string) string {
	if status == "" {
		return "ok"
	}
	return status
}

// writeAnchorUpdates runs after releasing the PR savepoint: anchor failures
// cannot roll back the PR row, patchset, or diff. Each comment is best-effort.
func (w *Writer) writeAnchorUpdates(ctx context.Context, tx *sql.Tx, updates []anchor.Result, currentPatchsetNum int) {
	const query = `
		UPDATE comments
		SET anchored_line                  = ?,
		    anchor_head_sha                = ?,
		    outdated                       = ?,
		    outdated_near_line             = ?,
		    anchor_patchset_number         = ?,
		    anchor_current_patchset_number = ?
		WHERE id = ?`
	for _, update := range updates {
		var anchoredLine any
		if update.Anchored && update.AnchoredLine > 0 {
			anchoredLine = update.AnchoredLine
		}
		var nearLine any
		if update.Outdated && update.OutdatedNearLine > 0 {
			nearLine = update.OutdatedNearLine
		}
		// Source and current patchset differ for rebased comments. A missing
		// number from fetch is filled with the patchset recorded in this batch.
		sourceNumber := update.PatchsetNumber
		if sourceNumber == 0 {
			sourceNumber = currentPatchsetNum
		}
		currentNumber := update.CurrentPatchsetNumber
		if currentNumber == 0 {
			currentNumber = currentPatchsetNum
		}
		if _, err := tx.ExecContext(ctx, query, anchoredLine, update.HeadSHA,
			update.Outdated, nearLine, nullableInt(sourceNumber),
			nullableInt(currentNumber), update.CommentID,
		); err != nil {
			w.log.Warn("writeAnchorUpdates: failed to update anchor", "comment_id", update.CommentID, "error", err)
		}
	}
}
