// Package commitstats caches per-commit list statistics for PR ranges.
package commitstats

import (
	"context"
	"database/sql"
	"fmt"

	storedb "github.com/itterno/bistro/internal/store/db"
)

// Stats contains the cached statistics for one commit.
type Stats struct {
	SHA                             string
	Additions, Deletions, FileCount int
	PatchID                         string
}

// Store persists commit statistics.
type Store struct{ db *sql.DB }

// New creates a commit statistics store backed by db.
func New(db *sql.DB) *Store { return &Store{db: db} }

// List returns statistics matching version, keyed by commit SHA.
func (s *Store) List(ctx context.Context, prID string, version int) (map[string]Stats, error) {
	rows, err := storedb.New(s.db).ListCommitStats(ctx, storedb.ListCommitStatsParams{
		PrID: prID, AlgoVersion: int64(version),
	})
	if err != nil {
		return nil, fmt.Errorf("list commit stats (%s): %w", prID, err)
	}
	out := make(map[string]Stats, len(rows))
	for _, row := range rows {
		out[row.Sha] = Stats{
			SHA: row.Sha, Additions: int(row.Additions), Deletions: int(row.Deletions),
			FileCount: int(row.FileCount), PatchID: row.PatchID,
		}
	}
	return out, nil
}

// Upsert atomically stores statistics without removing other commits.
func (s *Store) Upsert(ctx context.Context, prID string, version int, stats []Stats) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin commit stats upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertAll(ctx, storedb.New(tx), prID, version, stats); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit stats upsert (%s): %w", prID, err)
	}
	return nil
}

// ReplaceTx replaces all versions of a PR's statistics in the caller's transaction.
// Passing nil or an empty slice clears the cached range.
func ReplaceTx(ctx context.Context, tx *sql.Tx, prID string, version int, stats []Stats) error {
	queries := storedb.New(tx)
	if err := queries.DeleteCommitStatsForPR(ctx, prID); err != nil {
		return fmt.Errorf("clear commit stats (%s): %w", prID, err)
	}
	return upsertAll(ctx, queries, prID, version, stats)
}

func upsertAll(ctx context.Context, queries *storedb.Queries, prID string, version int, stats []Stats) error {
	for _, stat := range stats {
		if err := queries.UpsertCommitStats(ctx, storedb.UpsertCommitStatsParams{
			PrID: prID, Sha: stat.SHA, Additions: int64(stat.Additions),
			Deletions: int64(stat.Deletions), FileCount: int64(stat.FileCount),
			PatchID: stat.PatchID, AlgoVersion: int64(version),
		}); err != nil {
			return fmt.Errorf("upsert commit stats (%s, %s): %w", prID, stat.SHA, err)
		}
	}
	return nil
}
