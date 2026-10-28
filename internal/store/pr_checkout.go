package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	storedb "github.com/itterno/bistro/internal/store/db"
)

// PRCheckoutInfo is the minimal PR data needed to resolve a local checkout.
type PRCheckoutInfo struct {
	RepoID  string
	HeadSHA string
}

// GetPRCheckoutInfo returns the repo_id and head_sha for prID.
// Returns (nil, nil) when no row matches — callers should treat that as
// "PR not found" rather than an error.
func (db *DB) GetPRCheckoutInfo(ctx context.Context, prID string) (*PRCheckoutInfo, error) {
	row, err := storedb.New(db).GetPRCheckoutInfo(ctx, prID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get PR checkout info (%s): %w", prID, err)
	}
	return &PRCheckoutInfo{RepoID: row.RepoID, HeadSHA: row.HeadSha}, nil
}
