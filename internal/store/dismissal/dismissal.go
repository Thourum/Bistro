package dismissal

import (
	"context"
	"database/sql"
	"fmt"

	storedb "github.com/itterno/bistro/internal/store/db"
)

// Store provides persistence for per-account comment dismissals.
type Store struct {
	db      *sql.DB
	queries *storedb.Queries
}

// New creates a new Store store backed by the given database.
func New(db *sql.DB) *Store {
	return &Store{db: db, queries: storedb.New(db)}
}

// Dismiss inserts comment dismissal rows for the given account in one transaction.
// Existing rows are ignored, making repeated dismiss commands idempotent.
func (d *Store) Dismiss(ctx context.Context, commentIDs []string, provider string, login string) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin dismiss comments: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := d.queries.WithTx(tx)
	for _, commentID := range commentIDs {
		if err := qtx.InsertCommentDismissal(ctx, storedb.InsertCommentDismissalParams{
			CommentID:       commentID,
			AccountProvider: provider,
			AccountLogin:    login,
		}); err != nil {
			return fmt.Errorf("dismiss comments (%s, %s/%s): %w", commentID, provider, login, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dismiss comments: %w", err)
	}
	return nil
}

// Undismiss deletes comment dismissal rows for the given account in one transaction.
func (d *Store) Undismiss(ctx context.Context, commentIDs []string, provider string, login string) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin undismiss comments: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := d.queries.WithTx(tx)
	for _, commentID := range commentIDs {
		if err := qtx.DeleteCommentDismissal(ctx, storedb.DeleteCommentDismissalParams{
			CommentID:       commentID,
			AccountProvider: provider,
			AccountLogin:    login,
		}); err != nil {
			return fmt.Errorf("undismiss comments (%s, %s/%s): %w", commentID, provider, login, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit undismiss comments: %w", err)
	}
	return nil
}

// DismissedIDs returns the set of dismissed comment IDs for a PR and account.
func (d *Store) DismissedIDs(ctx context.Context, prID string, provider string, login string) (map[string]bool, error) {
	commentIDs, err := d.queries.ListDismissedCommentIDsForPR(ctx, storedb.ListDismissedCommentIDsForPRParams{
		PrID:            prID,
		AccountProvider: provider,
		AccountLogin:    login,
	})
	if err != nil {
		return nil, fmt.Errorf("list dismissed comments (%s, %s/%s): %w", prID, provider, login, err)
	}

	ids := make(map[string]bool, len(commentIDs))
	for _, commentID := range commentIDs {
		ids[commentID] = true
	}
	return ids, nil
}
