package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	storedb "github.com/itterno/bistro/internal/store/db"
)

const localReviewAccountProvider = "github"

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
	if err := storedb.New(db).UpsertLastReviewedPatchset(ctx, storedb.UpsertLastReviewedPatchsetParams{
		PrID:            prID,
		AccountProvider: localReviewAccountProvider,
		AccountLogin:    login,
		Login:           login,
		Sha:             headSHA,
		PatchsetNumber:  nullablePositiveInt(patchsetNum),
		BaseSha:         sql.NullString{String: baseSHA, Valid: true},
	}); err != nil {
		return fmt.Errorf("set last reviewed patchset (%s, %s): %w", prID, login, err)
	}
	return nil
}

// GetLastReviewedPatchset returns the full patchset identity for (prID, login).
// Returns (LastReviewedRecord{}, false, nil) when no record exists.
// PatchsetNum == 0 in the returned record means the number was not stored (legacy).
func (db *DB) GetLastReviewedPatchset(ctx context.Context, prID, login string) (LastReviewedRecord, bool, error) {
	row, err := storedb.New(db).GetLastReviewedPatchset(ctx, storedb.GetLastReviewedPatchsetParams{
		PrID:            prID,
		AccountProvider: localReviewAccountProvider,
		AccountLogin:    login,
		Login:           login,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return LastReviewedRecord{}, false, nil
	}
	if err != nil {
		return LastReviewedRecord{}, false, fmt.Errorf("get last reviewed patchset (%s, %s): %w", prID, login, err)
	}
	return LastReviewedRecord{SHA: row.Sha, PatchsetNum: int(row.PatchsetNumber), BaseSHA: row.BaseSha}, true, nil
}

// SetLastReviewedSHA upserts the SHA that login last reviewed for pr_id.
// Writes patchset_number=NULL and base_sha=NULL (legacy path).
// Prefer SetLastReviewedPatchset for new code so full patchset identity is captured.
func (db *DB) SetLastReviewedSHA(ctx context.Context, prID, login, sha string) error {
	if err := storedb.New(db).UpsertLastReviewedSHA(ctx, storedb.UpsertLastReviewedSHAParams{
		PrID:            prID,
		AccountProvider: localReviewAccountProvider,
		AccountLogin:    login,
		Login:           login,
		Sha:             sha,
	}); err != nil {
		return fmt.Errorf("set last reviewed sha (%s, %s): %w", prID, login, err)
	}
	return nil
}

// GetLastReviewedSHA returns the last-reviewed SHA for (prID, login).
// Returns ("", false, nil) when no record exists.
// Prefer GetLastReviewedPatchset for new code.
func (db *DB) GetLastReviewedSHA(ctx context.Context, prID, login string) (string, bool, error) {
	sha, err := storedb.New(db).GetLastReviewedSHA(ctx, storedb.GetLastReviewedSHAParams{
		PrID:            prID,
		AccountProvider: localReviewAccountProvider,
		AccountLogin:    login,
		Login:           login,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get last reviewed sha (%s, %s): %w", prID, login, err)
	}
	return sha, true, nil
}

func nullablePositiveInt(n int) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(n), Valid: n > 0}
}
