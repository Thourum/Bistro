// Package commitviewed stores local per-account viewed marks for PR commits.
package commitviewed

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/itterno/bistro/internal/store"
	storedb "github.com/itterno/bistro/internal/store/db"
)

// Store persists commit viewed marks.
type Store struct{ queries *storedb.Queries }

// New creates a commit viewed store backed by db.
func New(db *sql.DB) *Store { return &Store{queries: storedb.New(db)} }

// List returns patch_key → occurrences recorded for login on prID.
func (s *Store) List(ctx context.Context, prID, login string) (map[string]int, error) {
	rows, err := s.queries.ListCommitsViewed(ctx, storedb.ListCommitsViewedParams{
		PrID: prID, AccountProvider: store.LocalReviewAccountProvider, AccountLogin: login, Login: login,
	})
	if err != nil {
		return nil, fmt.Errorf("list commits viewed (%s, %s): %w", prID, login, err)
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.PatchKey] = int(row.Occurrences)
	}
	return out, nil
}

// Mark records patchKey viewed with the fingerprint's current occurrence count.
func (s *Store) Mark(ctx context.Context, prID, login, patchKey string, occurrences int) error {
	if err := s.queries.UpsertCommitViewed(ctx, storedb.UpsertCommitViewedParams{
		PrID: prID, PatchKey: patchKey, Occurrences: int64(occurrences),
		AccountProvider: store.LocalReviewAccountProvider, AccountLogin: login, Login: login,
	}); err != nil {
		return fmt.Errorf("mark commit viewed (%s, %s): %w", prID, patchKey, err)
	}
	return nil
}

// Unmark removes the viewed mark; a missing mark is not an error.
func (s *Store) Unmark(ctx context.Context, prID, login, patchKey string) error {
	if err := s.queries.DeleteCommitViewed(ctx, storedb.DeleteCommitViewedParams{
		PrID: prID, PatchKey: patchKey,
		AccountProvider: store.LocalReviewAccountProvider, AccountLogin: login, Login: login,
	}); err != nil {
		return fmt.Errorf("unmark commit viewed (%s, %s): %w", prID, patchKey, err)
	}
	return nil
}
