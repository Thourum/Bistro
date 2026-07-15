package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	storedb "github.com/itterno/bistro/internal/store/db"
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
	row, err := storedb.New(db).GetSyncCursor(ctx, scope)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return syncCursorFromRow(row), nil
}

// SaveSyncCursorSuccess atomically advances a scope after the corresponding
// endpoint pages and local hydration/write have completed successfully.
func (db *DB) SaveSyncCursorSuccess(ctx context.Context, scope, cursorValue, etag string) error {
	return storedb.New(db).SaveSyncCursorSuccess(ctx, storedb.SaveSyncCursorSuccessParams{
		Scope:       scope,
		CursorValue: nonNullString(cursorValue),
		Etag:        nonNullString(etag),
	})
}

// MarkSyncCursorError records a failed attempt without advancing cursor_value.
func (db *DB) MarkSyncCursorError(ctx context.Context, scope string, syncErr error) error {
	payload, err := json.Marshal(map[string]string{
		"error": fmt.Sprint(syncErr),
	})
	if err != nil {
		return err
	}

	return storedb.New(db).MarkSyncCursorError(ctx, storedb.MarkSyncCursorErrorParams{
		Scope:     scope,
		ErrorJson: nonNullString(string(payload)),
	})
}

func FormatSyncCursorTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func ParseSyncCursorTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func syncCursorFromRow(row storedb.GetSyncCursorRow) *SyncCursor {
	return &SyncCursor{
		Scope:         row.Scope,
		CursorValue:   row.CursorValue,
		ETag:          row.Etag,
		LastSuccessAt: row.LastSuccessAt,
		LastErrorAt:   row.LastErrorAt,
		ErrorJSON:     row.ErrorJson,
	}
}

func nonNullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}
