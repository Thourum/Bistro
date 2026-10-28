package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	storedb "github.com/itterno/bistro/internal/store/db"
)

// Patchset is one observed patchset row, keyed by (pr_id, number).
type Patchset struct {
	PRID            string    `json:"pr_id"`
	Number          int       `json:"number"`
	HeadSHA         string    `json:"head_sha"`
	BaseSHA         string    `json:"base_sha"`
	MergeBaseSHA    string    `json:"merge_base_sha"`
	HasFingerprints bool      `json:"has_fingerprints"`
	PushedAt        time.Time `json:"pushed_at"`
}

// ListPatchsets returns all observed patchsets for pr_id ordered by number ascending.
func (db *DB) ListPatchsets(ctx context.Context, prID string) ([]Patchset, error) {
	rows, err := storedb.New(db).ListPatchsets(ctx, prID)
	if err != nil {
		return nil, fmt.Errorf("list patchsets (%s): %w", prID, err)
	}

	patchsets := make([]Patchset, 0, len(rows))
	for _, row := range rows {
		patchsets = append(patchsets, patchsetFromRow(row))
	}

	return patchsets, nil
}

// GetCommentPatchsetNumbers returns a map of commentID → patchset_number for
// the given comment IDs. Only rows where patchset_number IS NOT NULL are
// included. Missing or NULL entries are simply absent from the returned map.
// Used by the anchor resolver to recover exact patchset identity for outdated
// review comments that were originally stored with a patchset_number.
func (db *DB) GetCommentPatchsetNumbers(ctx context.Context, commentIDs []string) (map[string]int, error) {
	if len(commentIDs) == 0 {
		return map[string]int{}, nil
	}

	rows, err := storedb.New(db).ListCommentPatchsetNumbers(ctx, commentIDs)
	if err != nil {
		return nil, fmt.Errorf("get comment patchset numbers: %w", err)
	}

	out := make(map[string]int, len(commentIDs))
	for _, row := range rows {
		out[row.ID] = int(row.PatchsetNumber.Int64)
	}
	return out, nil
}

// ListPatchsetFiles returns path → patch_id for one patchset.
func (db *DB) ListPatchsetFiles(ctx context.Context, prID string, number int) (map[string]string, error) {
	rows, err := storedb.New(db).ListPatchsetFiles(ctx, storedb.ListPatchsetFilesParams{
		PrID: prID, PatchsetNumber: int64(number),
	})
	if err != nil {
		return nil, fmt.Errorf("list patchset files (%s P%d): %w", prID, number, err)
	}
	files := make(map[string]string, len(rows))
	for _, row := range rows {
		files[row.Path] = row.PatchID
	}
	return files, nil
}

// CurrentPatchset returns the latest patchset at the PR's current head. The
// base tip does not identify a patchset: it moves without the PR changing.
func (db *DB) CurrentPatchset(ctx context.Context, prID string) (Patchset, bool, error) {
	row, err := storedb.New(db).GetCurrentPatchset(ctx, prID)
	if errors.Is(err, sql.ErrNoRows) {
		return Patchset{}, false, nil
	}
	if err != nil {
		return Patchset{}, false, fmt.Errorf("get current patchset (%s): %w", prID, err)
	}
	return patchsetFromRow(row), true, nil
}

func patchsetFromRow(row storedb.Patchset) Patchset {
	return Patchset{
		PRID:            row.PrID,
		Number:          int(row.Number),
		HeadSHA:         row.HeadSha,
		BaseSHA:         row.BaseSha,
		MergeBaseSHA:    row.MergeBaseSha,
		HasFingerprints: row.HasFingerprints != 0,
		PushedAt:        row.PushedAt,
	}
}
