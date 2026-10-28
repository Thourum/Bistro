package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	storedb "github.com/itterno/bistro/internal/store/db"
)

// LocalReviewAccountProvider identifies the provider used for local review state.
const LocalReviewAccountProvider = "github"

// reviewedAtLayout is UTC at second precision so stored strings sort
// chronologically; it matches the strftime format migration 035 backfills.
const reviewedAtLayout = "2006-01-02T15:04:05Z"

// ReviewSource names the event that set a since-last-review baseline.
type ReviewSource string

const (
	ReviewSourceGitHubReview ReviewSource = "github_review"
	ReviewSourceBistroAction ReviewSource = "bistro_action"
	ReviewSourceAllViewed    ReviewSource = "all_viewed"
)

// ReviewEvent is one candidate since-last-review baseline.
// PatchsetNum 0 means unknown; readers then match the patchset by HeadSHA.
type ReviewEvent struct {
	PRID        string
	Login       string
	HeadSHA     string
	PatchsetNum int
	ReviewedAt  time.Time
	Source      ReviewSource
}

// LastReviewedRecord is the stored baseline for one PR and login.
type LastReviewedRecord struct {
	SHA         string
	PatchsetNum int
	ReviewedAt  time.Time
	Source      ReviewSource
}

// RecordLastReviewed stores event as the baseline unless a newer one is
// already stored. q is a *DB or the sync writer's *sql.Tx.
func RecordLastReviewed(ctx context.Context, q storedb.DBTX, event ReviewEvent) error {
	const query = `
		INSERT INTO last_reviewed_sha (
			pr_id, account_provider, account_login, login, sha,
			patchset_number, reviewed_at, source, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		ON CONFLICT(pr_id, account_provider, account_login, login) DO UPDATE SET
			sha             = excluded.sha,
			patchset_number = excluded.patchset_number,
			reviewed_at     = excluded.reviewed_at,
			source          = excluded.source,
			updated_at      = excluded.updated_at
		WHERE last_reviewed_sha.reviewed_at IS NULL
		   OR excluded.reviewed_at > last_reviewed_sha.reviewed_at`
	if _, err := q.ExecContext(ctx, query,
		event.PRID, LocalReviewAccountProvider, event.Login, event.Login, event.HeadSHA,
		nullablePositiveInt(event.PatchsetNum), event.ReviewedAt.UTC().Format(reviewedAtLayout),
		string(event.Source),
	); err != nil {
		return fmt.Errorf("record last reviewed (%s, %s, %s): %w", event.PRID, event.Login, event.Source, err)
	}
	return nil
}

// RecordLastReviewed stores event as the baseline unless a newer one exists.
func (db *DB) RecordLastReviewed(ctx context.Context, event ReviewEvent) error {
	return RecordLastReviewed(ctx, db, event)
}

// GetLastReviewed returns the stored baseline for (prID, login).
func (db *DB) GetLastReviewed(ctx context.Context, prID, login string) (LastReviewedRecord, bool, error) {
	row, err := storedb.New(db).GetLastReviewed(ctx, storedb.GetLastReviewedParams{
		PrID: prID, AccountProvider: LocalReviewAccountProvider, AccountLogin: login, Login: login,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return LastReviewedRecord{}, false, nil
	}
	if err != nil {
		return LastReviewedRecord{}, false, fmt.Errorf("get last reviewed (%s, %s): %w", prID, login, err)
	}
	reviewedAt, err := time.Parse(reviewedAtLayout, row.ReviewedAt.String)
	if err != nil {
		return LastReviewedRecord{}, false, fmt.Errorf("parse last reviewed time (%s, %s): %w", prID, login, err)
	}
	return LastReviewedRecord{
		SHA: row.Sha, PatchsetNum: int(row.PatchsetNumber.Int64),
		ReviewedAt: reviewedAt, Source: ReviewSource(row.Source),
	}, true, nil
}

func nullablePositiveInt(n int) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(n), Valid: n > 0}
}
