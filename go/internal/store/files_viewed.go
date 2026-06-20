package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	const q = `
		INSERT INTO files_viewed (pr_id, file_path, login, content_hash, viewed_at)
		VALUES (?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		ON CONFLICT(pr_id, file_path, login) DO UPDATE SET
			content_hash = excluded.content_hash,
			viewed_at    = excluded.viewed_at`
	if _, err := db.ExecContext(ctx, q, prID, filePath, login, contentHash); err != nil {
		return fmt.Errorf("mark file viewed (%s, %s, %s): %w", prID, filePath, login, err)
	}
	return nil
}

// IsFileViewed returns true only when a record exists for (pr_id, file_path, login)
// AND the stored content_hash matches the given hash.
// A different hash returns false — the content changed and needs re-review.
func (db *DB) IsFileViewed(ctx context.Context, prID, filePath, login, contentHash string) (bool, error) {
	var stored string
	err := db.QueryRowContext(ctx,
		`SELECT content_hash FROM files_viewed WHERE pr_id = ? AND file_path = ? AND login = ?`,
		prID, filePath, login,
	).Scan(&stored)
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
	rows, err := db.QueryContext(ctx,
		`SELECT file_path, content_hash FROM files_viewed
		 WHERE pr_id = ? AND login = ?
		 ORDER BY file_path`,
		prID, login,
	)
	if err != nil {
		return nil, fmt.Errorf("list viewed files (%s, %s): %w", prID, login, err)
	}
	defer rows.Close()
	var out []FileViewed
	for rows.Next() {
		var fv FileViewed
		if err := rows.Scan(&fv.FilePath, &fv.ContentHash); err != nil {
			return nil, fmt.Errorf("scan file viewed: %w", err)
		}
		out = append(out, fv)
	}
	return out, rows.Err()
}
