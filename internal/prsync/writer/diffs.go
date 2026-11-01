package writer

import (
	"context"
	"database/sql"

	"github.com/itterno/bistro/internal/git/anchor"
	"github.com/itterno/bistro/internal/store/prdiff"
)

// writeDiffs replaces a PR's diff cache in the same savepoint as the PR row and
// patchset, preserving the old cache if any critical insert fails.
func (w *Writer) writeDiffs(ctx context.Context, tx *sql.Tx, prID string, diffs []prdiff.File) error {
	return prdiff.ReplaceTx(ctx, tx, prID, diffs)
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
