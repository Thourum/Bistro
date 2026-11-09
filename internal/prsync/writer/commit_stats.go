package writer

import (
	"context"
	"database/sql"

	gitdiff "github.com/itterno/bistro/internal/git/diff"
	"github.com/itterno/bistro/internal/store/commitstats"
)

const commitStatsSavepoint = "commit_stats"

// writeCommitStats nests a savepoint so a failure discards only the stats
// rows, never the PR row, patchset, or diff written in the PR savepoint.
func (w *Writer) writeCommitStats(ctx context.Context, tx *sql.Tx, prID string, stats []commitstats.Stats) {
	if _, err := tx.ExecContext(ctx, "SAVEPOINT "+commitStatsSavepoint); err != nil {
		w.log.Warn("commit stats: savepoint failed", "pr_id", prID, "error", err)
		return
	}
	if err := commitstats.ReplaceTx(ctx, tx, prID, gitdiff.CommitStatsVersion, stats); err != nil {
		w.log.Warn("commit stats: write failed", "pr_id", prID, "error", err)
		if _, rbErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+commitStatsSavepoint); rbErr != nil {
			w.log.Warn("commit stats: rollback failed", "pr_id", prID, "error", rbErr)
		}
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+commitStatsSavepoint); err != nil {
		w.log.Warn("commit stats: release failed", "pr_id", prID, "error", err)
	}
}
