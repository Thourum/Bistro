-- name: GetSyncCursor :one
SELECT
    scope,
    COALESCE(cursor_value, '') AS cursor_value,
    COALESCE(etag, '') AS etag,
    last_success_at,
    last_error_at,
    error_json
FROM sync_cursors
WHERE scope = ?;

-- name: SaveSyncCursorSuccess :exec
INSERT INTO sync_cursors (
    scope,
    cursor_value,
    etag,
    last_success_at,
    last_error_at,
    error_json,
    updated_at
)
VALUES (?, ?, ?, CURRENT_TIMESTAMP, NULL, NULL, CURRENT_TIMESTAMP)
ON CONFLICT(scope) DO UPDATE SET
    cursor_value = excluded.cursor_value,
    etag = excluded.etag,
    last_success_at = CURRENT_TIMESTAMP,
    last_error_at = NULL,
    error_json = NULL,
    updated_at = CURRENT_TIMESTAMP;

-- name: MarkSyncCursorError :exec
INSERT INTO sync_cursors (
    scope,
    last_error_at,
    error_json,
    updated_at
)
VALUES (?, CURRENT_TIMESTAMP, ?, CURRENT_TIMESTAMP)
ON CONFLICT(scope) DO UPDATE SET
    last_error_at = CURRENT_TIMESTAMP,
    error_json = excluded.error_json,
    updated_at = CURRENT_TIMESTAMP;
