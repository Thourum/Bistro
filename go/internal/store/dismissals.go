package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Dismissals provides persistence for per-account comment dismissals.
type Dismissals struct {
	db *sql.DB
}

// NewDismissals creates a new Dismissals store backed by the given database.
func NewDismissals(db *sql.DB) *Dismissals {
	return &Dismissals{db: db}
}

// Dismiss inserts comment dismissal rows for the given account in one transaction.
// Existing rows are ignored, making repeated dismiss commands idempotent.
func (d *Dismissals) Dismiss(ctx context.Context, commentIDs []string, provider string, login string) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin dismiss comments: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	const query = `
		INSERT OR IGNORE INTO comment_dismissals (comment_id, account_provider, account_login)
		VALUES (?, ?, ?)
	`
	for _, commentID := range commentIDs {
		if _, err := tx.ExecContext(ctx, query, commentID, provider, login); err != nil {
			return fmt.Errorf("dismiss comments (%s, %s/%s): %w", commentID, provider, login, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dismiss comments: %w", err)
	}
	committed = true
	return nil
}

// Undismiss deletes comment dismissal rows for the given account in one transaction.
func (d *Dismissals) Undismiss(ctx context.Context, commentIDs []string, provider string, login string) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin undismiss comments: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	const query = `
		DELETE FROM comment_dismissals
		WHERE comment_id = ? AND account_provider = ? AND account_login = ?
	`
	for _, commentID := range commentIDs {
		if _, err := tx.ExecContext(ctx, query, commentID, provider, login); err != nil {
			return fmt.Errorf("undismiss comments (%s, %s/%s): %w", commentID, provider, login, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit undismiss comments: %w", err)
	}
	committed = true
	return nil
}

// DismissedIDs returns the set of dismissed comment IDs for a PR and account.
func (d *Dismissals) DismissedIDs(ctx context.Context, prID string, provider string, login string) (map[string]bool, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT d.comment_id
		FROM comment_dismissals d
		JOIN comments c ON c.id = d.comment_id
		WHERE c.pr_id = ? AND d.account_provider = ? AND d.account_login = ?
		ORDER BY d.comment_id
	`, prID, provider, login)
	if err != nil {
		return nil, fmt.Errorf("list dismissed comments (%s, %s/%s): %w", prID, provider, login, err)
	}
	defer rows.Close()

	ids := make(map[string]bool)
	for rows.Next() {
		var commentID string
		if err := rows.Scan(&commentID); err != nil {
			return nil, fmt.Errorf("scan dismissed comment: %w", err)
		}
		ids[commentID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dismissed comments (%s, %s/%s): %w", prID, provider, login, err)
	}
	return ids, nil
}
