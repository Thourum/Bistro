package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"log/slog"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

// Writer handles database writes for sync results.
type Writer struct {
	log *slog.Logger
	db  *store.DB
}

// NewWriter creates a new database writer. If log is nil, slog.Default() is
// used so the writer can always log unconditionally.
func NewWriter(log *slog.Logger, db *store.DB) *Writer {
	if log == nil {
		log = slog.Default()
	}
	return &Writer{
		log: log,
		db:  db,
	}
}

// WriteSyncResults persists all sync results to the database in a single transaction.
// Per-item errors are logged and skipped; they do not abort the transaction.
func (w *Writer) WriteSyncResults(results []*PRFetchResult) error {
	return w.WriteSyncResultsContext(context.Background(), results)
}

// WriteSyncResultsContext persists all sync results to the database in a single transaction,
// respecting the provided context for cancellation.
// Per-item errors are logged and skipped; they do not abort the transaction.
func (w *Writer) WriteSyncResultsContext(ctx context.Context, results []*PRFetchResult) error {
	// Prefer the request-scoped logger from ctx (carries sync_id, repo, etc.)
	// over the writer's own logger; fall back to the writer's logger when ctx
	// doesn't carry one.
	log := logging.FromContext(ctx)
	if log == slog.Default() && w.log != nil {
		log = w.log
	}

	if len(results) == 0 {
		log.Debug("WriteSyncResults: nothing to write")
		return nil
	}

	if w.db == nil {
		log.Debug("WriteSyncResults: no DB configured, skipping", "results", len(results))
		return nil
	}

	overallStart := time.Now()

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		log.Error("failed to start transaction", "error", err)
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	prCount := 0
	prSkipped := 0
	prErrors := 0
	commentCount := 0
	diffCount := 0

	for _, result := range results {
		if result == nil {
			prSkipped++
			continue
		}

		if result.Error != nil {
			prErrors++
			prID := ""
			if result.PR != nil {
				prID = result.PR.ID
			}
			log.Warn("skipping pr write due to upstream error",
				"pr_id", prID,
				"error", result.Error,
			)
			continue
		}

		if result.PR == nil {
			prSkipped++
			log.Debug("skipping nil PR in result")
			continue
		}

		writeStart := time.Now()
		if err := w.writePR(ctx, tx, result.PR); err != nil {
			prErrors++
			log.Error("failed to write PR",
				"pr_id", result.PR.ID,
				"pr_number", result.PR.Number,
				"duration_ms", time.Since(writeStart).Milliseconds(),
				"error", err,
			)
			continue
		}
		prCount++
		log.Debug("pr written",
			"pr_id", result.PR.ID,
			"pr_number", result.PR.Number,
			"state", result.PR.State,
			"duration_ms", time.Since(writeStart).Milliseconds(),
		)

		for _, comment := range result.Comments {
			if comment == nil {
				continue
			}
			commentStart := time.Now()
			if err := w.writeComment(ctx, tx, result.PR.ID, comment); err != nil {
				log.Error("failed to write comment",
					"comment_id", comment.ID,
					"pr_id", result.PR.ID,
					"duration_ms", time.Since(commentStart).Milliseconds(),
					"error", err,
				)
				continue
			}
			commentCount++
		}

		if len(result.Diffs) > 0 {
			diffStart := time.Now()
			written, err := w.writeDiffs(ctx, tx, result.PR.ID, result.Diffs)
			if err != nil {
				log.Error("failed to write diffs",
					"pr_id", result.PR.ID,
					"files", len(result.Diffs),
					"duration_ms", time.Since(diffStart).Milliseconds(),
					"error", err,
				)
				continue
			}
			diffCount += written
			log.Debug("diffs written",
				"pr_id", result.PR.ID,
				"files", written,
				"duration_ms", time.Since(diffStart).Milliseconds(),
			)
		}
	}

	commitStart := time.Now()
	if err := tx.Commit(); err != nil {
		log.Error("failed to commit transaction",
			"duration_ms", time.Since(commitStart).Milliseconds(),
			"error", err,
		)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	totalDur := time.Since(overallStart).Milliseconds()
	logFn := log.Info
	if totalDur > 500 {
		// Writing >500ms for a single sync batch is unusual for SQLite — flag it.
		logFn = log.Warn
	}
	logFn("sync results written",
		"prs", prCount,
		"prs_skipped", prSkipped,
		"prs_errors", prErrors,
		"comments", commentCount,
		"diffs", diffCount,
		"duration_ms", totalDur,
	)
	return nil
}

// writePR inserts or updates a PR in the database using an upsert.
func (w *Writer) writePR(ctx context.Context, tx *sql.Tx, pr *provider.PR) error {
	if pr.ID == "" {
		return fmt.Errorf("PR ID cannot be empty")
	}

	const query = `
		INSERT INTO prs (
			id, provider, owner, repo, number, title, body, author, state,
			created_at, updated_at, merged_at,
			base_branch, head_branch, base_sha, head_sha,
			comment_count, ci_status, draft,
			review_status, approved_by, changes_requested_by,
			requested_reviewers,
			repo_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title                  = excluded.title,
			body                   = excluded.body,
			state                  = excluded.state,
			updated_at             = excluded.updated_at,
			merged_at              = excluded.merged_at,
			base_branch            = excluded.base_branch,
			head_branch            = excluded.head_branch,
			base_sha               = excluded.base_sha,
			head_sha               = excluded.head_sha,
			comment_count          = excluded.comment_count,
			ci_status              = excluded.ci_status,
			draft                  = excluded.draft,
			review_status          = excluded.review_status,
			approved_by            = excluded.approved_by,
			changes_requested_by   = excluded.changes_requested_by,
			requested_reviewers    = excluded.requested_reviewers,
			repo_id                = excluded.repo_id
	`

	repoID := pr.Provider + ":" + pr.Owner + "/" + pr.Repo

	draft := 0
	if pr.Draft {
		draft = 1
	}

	reviewers := pr.RequestedReviewers
	if reviewers == nil {
		reviewers = []string{}
	}
	requestedReviewers, err := json.Marshal(reviewers)
	if err != nil {
		return fmt.Errorf("failed to marshal requested reviewers for %s: %w", pr.ID, err)
	}

	res, err := tx.ExecContext(ctx, query,
		pr.ID,
		pr.Provider,
		pr.Owner,
		pr.Repo,
		pr.Number,
		pr.Title,
		pr.Description,
		pr.Author,
		pr.State,
		pr.CreatedAt,
		pr.UpdatedAt,
		pr.MergedAt,
		pr.BaseBranch,
		pr.HeadBranch,
		pr.BaseSHA,
		pr.HeadSHA,
		pr.CommentCount,
		pr.CIStatus,
		draft,
		pr.ReviewStatus,
		pr.ApprovedBy,
		pr.ChangesRequestedBy,
		string(requestedReviewers),
		repoID,
	)
	if err != nil {
		return fmt.Errorf("failed to write PR %s: %w", pr.ID, err)
	}

	if logging.FromContext(ctx) != slog.Default() {
		if affected, err := res.RowsAffected(); err == nil {
			logging.FromContext(ctx).Debug("pr upsert rows affected",
				"pr_id", pr.ID,
				"rows_affected", affected,
			)
		}
	}

	return nil
}

// writeComment inserts or updates a comment in the database using an upsert.
func (w *Writer) writeComment(ctx context.Context, tx *sql.Tx, prID string, comment *provider.Comment) error {
	if comment.ID == "" {
		return fmt.Errorf("comment ID cannot be empty")
	}
	if prID == "" {
		return fmt.Errorf("PR ID cannot be empty for comment %s", comment.ID)
	}

	const query = `
		INSERT INTO comments (id, pr_id, author, body, created_at, updated_at, line_number, file_path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			body       = excluded.body,
			updated_at = excluded.updated_at
	`

	_, err := tx.ExecContext(ctx, query,
		comment.ID,
		prID,
		comment.Author,
		comment.Body,
		comment.CreatedAt,
		comment.UpdatedAt,
		comment.LineNo,
		comment.FilePath,
	)
	if err != nil {
		return fmt.Errorf("failed to write comment %s: %w", comment.ID, err)
	}

	return nil
}

// writeDiffs inserts or updates per-file diffs for a PR inside the writer's
// batch transaction. It must run after writePR so pr_diffs' foreign key to prs
// is satisfied in the same transaction.
func (w *Writer) writeDiffs(ctx context.Context, tx *sql.Tx, prID string, diffs []store.DiffFile) (int, error) {
	if len(diffs) == 0 {
		return 0, nil
	}
	if prID == "" {
		return 0, fmt.Errorf("PR ID cannot be empty for diffs")
	}

	const query = `
		INSERT INTO pr_diffs (pr_id, file_path, patch, additions, deletions)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(pr_id, file_path) DO UPDATE SET
			patch      = excluded.patch,
			additions  = excluded.additions,
			deletions  = excluded.deletions
	`

	written := 0
	for _, d := range diffs {
		if _, err := tx.ExecContext(ctx, query, prID, d.FilePath, d.Patch, d.Additions, d.Deletions); err != nil {
			return written, fmt.Errorf("failed to write diff %s/%s: %w", prID, d.FilePath, err)
		}
		written++
	}

	return written, nil
}
