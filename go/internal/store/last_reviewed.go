package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// LastReviewedRecord holds the full patchset identity stored for a last-reviewed entry.
// PatchsetNum == 0 means the record was written without a patchset number (legacy/unknown).
type LastReviewedRecord struct {
	SHA         string // head_sha at time of review
	PatchsetNum int    // 0 = legacy (NULL in DB); > 0 = authoritative patchset number
	BaseSHA     string // base_sha at time of review; empty for legacy rows
}

// SetLastReviewedPatchset upserts full patchset identity for (prID, login).
// PatchsetNum > 0 is authoritative for since-last-review resolution; 0 writes NULL.
// Supersedes SetLastReviewedSHA for new code — legacy callers may still use SetLastReviewedSHA.
func (db *DB) SetLastReviewedPatchset(ctx context.Context, prID, login string, patchsetNum int, headSHA, baseSHA string) error {
	var numPtr interface{}
	if patchsetNum > 0 {
		numPtr = patchsetNum
	} // else nil → SQL NULL (legacy/unknown)

	const q = `
		INSERT INTO last_reviewed_sha (pr_id, login, sha, patchset_number, base_sha, updated_at)
		VALUES (?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		ON CONFLICT(pr_id, login) DO UPDATE SET
			sha             = excluded.sha,
			patchset_number = excluded.patchset_number,
			base_sha        = excluded.base_sha,
			updated_at      = excluded.updated_at`
	if _, err := db.ExecContext(ctx, q, prID, login, headSHA, numPtr, baseSHA); err != nil {
		return fmt.Errorf("set last reviewed patchset (%s, %s): %w", prID, login, err)
	}
	return nil
}

// GetLastReviewedPatchset returns the full patchset identity for (prID, login).
// Returns (LastReviewedRecord{}, false, nil) when no record exists.
// PatchsetNum == 0 in the returned record means the number was not stored (legacy).
func (db *DB) GetLastReviewedPatchset(ctx context.Context, prID, login string) (LastReviewedRecord, bool, error) {
	var sha string
	var num int
	var baseSHA string
	err := db.QueryRowContext(ctx,
		`SELECT sha, COALESCE(patchset_number, 0), COALESCE(base_sha, '')
		 FROM last_reviewed_sha WHERE pr_id = ? AND login = ?`,
		prID, login,
	).Scan(&sha, &num, &baseSHA)
	if errors.Is(err, sql.ErrNoRows) {
		return LastReviewedRecord{}, false, nil
	}
	if err != nil {
		return LastReviewedRecord{}, false, fmt.Errorf("get last reviewed patchset (%s, %s): %w", prID, login, err)
	}
	return LastReviewedRecord{SHA: sha, PatchsetNum: num, BaseSHA: baseSHA}, true, nil
}

// SetLastReviewedSHA upserts the SHA that login last reviewed for pr_id.
// Writes patchset_number=NULL and base_sha=NULL (legacy path).
// Prefer SetLastReviewedPatchset for new code so full patchset identity is captured.
func (db *DB) SetLastReviewedSHA(ctx context.Context, prID, login, sha string) error {
	const q = `
		INSERT INTO last_reviewed_sha (pr_id, login, sha, updated_at)
		VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		ON CONFLICT(pr_id, login) DO UPDATE SET
			sha        = excluded.sha,
			updated_at = excluded.updated_at`
	if _, err := db.ExecContext(ctx, q, prID, login, sha); err != nil {
		return fmt.Errorf("set last reviewed sha (%s, %s): %w", prID, login, err)
	}
	return nil
}

// GetLastReviewedSHA returns the last-reviewed SHA for (prID, login).
// Returns ("", false, nil) when no record exists.
// Prefer GetLastReviewedPatchset for new code.
func (db *DB) GetLastReviewedSHA(ctx context.Context, prID, login string) (string, bool, error) {
	var sha string
	err := db.QueryRowContext(ctx,
		`SELECT sha FROM last_reviewed_sha WHERE pr_id = ? AND login = ?`,
		prID, login,
	).Scan(&sha)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get last reviewed sha (%s, %s): %w", prID, login, err)
	}
	return sha, true, nil
}
