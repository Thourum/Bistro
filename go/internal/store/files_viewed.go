package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	storedb "github.com/itterno/bistro/internal/store/db"
)

// FileViewed is one row from files_viewed.
type FileViewed struct {
	FilePath    string
	ContentHash string
}

// MarkFileViewed upserts a viewed record for (pr_id, file_path, login).
// The content_hash is updated on conflict — viewing after a rebase replaces
// the old hash so IsFileViewed correctly returns false for stale content.
func (db *DB) MarkFileViewed(ctx context.Context, prID, filePath, login, contentHash string) error {
	if err := storedb.New(db).UpsertFileViewed(ctx, storedb.UpsertFileViewedParams{
		PrID:            prID,
		FilePath:        filePath,
		AccountProvider: LocalReviewAccountProvider,
		AccountLogin:    login,
		Login:           login,
		ContentHash:     contentHash,
	}); err != nil {
		return fmt.Errorf("mark file viewed (%s, %s, %s): %w", prID, filePath, login, err)
	}
	return nil
}

// IsFileViewed returns true only when a record exists for (pr_id, file_path, login)
// AND the stored content_hash matches the given hash.
// A different hash returns false — the content changed and needs re-review.
func (db *DB) IsFileViewed(ctx context.Context, prID, filePath, login, contentHash string) (bool, error) {
	stored, err := storedb.New(db).GetViewedFileContentHash(ctx, storedb.GetViewedFileContentHashParams{
		PrID:            prID,
		FilePath:        filePath,
		AccountProvider: LocalReviewAccountProvider,
		AccountLogin:    login,
		Login:           login,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("is file viewed (%s, %s, %s): %w", prID, filePath, login, err)
	}
	return stored == contentHash, nil
}

// ListViewedFiles returns all files the given login has viewed for pr_id,
// ordered by file_path, with the stored content_hash for each.
func (db *DB) ListViewedFiles(ctx context.Context, prID, login string) ([]FileViewed, error) {
	rows, err := storedb.New(db).ListViewedFilesForLogin(ctx, storedb.ListViewedFilesForLoginParams{
		PrID:            prID,
		AccountProvider: LocalReviewAccountProvider,
		AccountLogin:    login,
		Login:           login,
	})
	if err != nil {
		return nil, fmt.Errorf("list viewed files (%s, %s): %w", prID, login, err)
	}

	out := make([]FileViewed, 0, len(rows))
	for _, row := range rows {
		out = append(out, FileViewed{
			FilePath:    row.FilePath,
			ContentHash: row.ContentHash,
		})
	}
	return out, nil
}
