package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SyncCursor is local-only sync bookkeeping for provider endpoints. It is kept
// separate from GitHub-synced rows such as prs/comments/repos.
type SyncCursor struct {
	Scope         string
	CursorValue   string
	ETag          string
	LastSuccessAt sql.NullTime
	LastErrorAt   sql.NullTime
	ErrorJSON     sql.NullString
}

// GetSyncCursor returns persisted local-only cursor state for a scope.
func (db *DB) GetSyncCursor(ctx context.Context, scope string) (*SyncCursor, error) {
	row := db.QueryRowContext(ctx, `
		SELECT scope, COALESCE(cursor_value, ''), COALESCE(etag, ''),
		       last_success_at, last_error_at, error_json
		FROM sync_cursors
		WHERE scope = ?`, scope)

	var cur SyncCursor
	if err := row.Scan(&cur.Scope, &cur.CursorValue, &cur.ETag, &cur.LastSuccessAt, &cur.LastErrorAt, &cur.ErrorJSON); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &cur, nil
}

// SaveSyncCursorSuccess atomically advances a scope after the corresponding
// endpoint pages and local hydration/write have completed successfully.
func (db *DB) SaveSyncCursorSuccess(ctx context.Context, scope, cursorValue, etag string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO sync_cursors
			(scope, cursor_value, etag, last_success_at, last_error_at, error_json, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, NULL, NULL, CURRENT_TIMESTAMP)
		ON CONFLICT(scope) DO UPDATE SET
			cursor_value = excluded.cursor_value,
			etag = excluded.etag,
			last_success_at = CURRENT_TIMESTAMP,
			last_error_at = NULL,
			error_json = NULL,
			updated_at = CURRENT_TIMESTAMP`, scope, cursorValue, etag)
	return err
}

// MarkSyncCursorError records a failed attempt without advancing cursor_value.
func (db *DB) MarkSyncCursorError(ctx context.Context, scope string, syncErr error) error {
	payload, err := json.Marshal(map[string]string{
		"error": fmt.Sprint(syncErr),
	})
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO sync_cursors
			(scope, last_error_at, error_json, updated_at)
		VALUES (?, CURRENT_TIMESTAMP, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(scope) DO UPDATE SET
			last_error_at = CURRENT_TIMESTAMP,
			error_json = excluded.error_json,
			updated_at = CURRENT_TIMESTAMP`, scope, string(payload))
	return err
}

func FormatSyncCursorTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func ParseSyncCursorTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}
