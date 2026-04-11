package sync

import (
	"context"
	"database/sql"
	"fmt"

	"log/slog"

	"github.com/samuelasvanyi/bistro/internal/provider"
	"github.com/samuelasvanyi/bistro/internal/store"
)

// Writer handles database writes for sync results.
type Writer struct {
	log *slog.Logger
	db  *store.DB
}

// NewWriter creates a new database writer.
func NewWriter(log *slog.Logger, db *store.DB) *Writer {
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
	if len(results) == 0 {
		return nil
	}

	// If no DB is configured (e.g. in tests), skip writes.
	if w.db == nil {
		return nil
	}

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	prCount := 0
	commentCount := 0

	for _, result := range results {
		if result == nil {
			continue
		}

		if result.Error != nil {
			if w.log != nil {
				w.log.Error("skipping result with error", "err", result.Error)
			}
			continue
		}

		if result.PR == nil {
			continue
		}

		if err := w.writePR(ctx, tx, result.PR); err != nil {
			if w.log != nil {
				w.log.Error("failed to write PR", "pr_id", result.PR.ID, "err", err)
			}
			continue
		}
		prCount++

		for _, comment := range result.Comments {
			if comment == nil {
				continue
			}
			if err := w.writeComment(ctx, tx, result.PR.ID, comment); err != nil {
				if w.log != nil {
					w.log.Error("failed to write comment", "comment_id", comment.ID, "err", err)
				}
				continue
			}
			commentCount++
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	if w.log != nil {
		w.log.Info("sync results written", "prs", prCount, "comments", commentCount)
	}
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
			repo_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			repo_id                = excluded.repo_id
	`

	repoID := pr.Provider + ":" + pr.Owner + "/" + pr.Repo

	draft := 0
	if pr.Draft {
		draft = 1
	}

	_, err := tx.ExecContext(ctx, query,
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
		repoID,
	)
	if err != nil {
		return fmt.Errorf("failed to write PR %s: %w", pr.ID, err)
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
