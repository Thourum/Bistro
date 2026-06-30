-- Migration: 021_add_sync_cursors
-- Description: Add local-only durable sync cursor and ETag state.

-- Local-only operational metadata. These rows describe the last successful
-- sync boundary for GitHub endpoints; they are not GitHub-synced PR/comment
-- state and must only advance after the corresponding hydration/write succeeds.
CREATE TABLE IF NOT EXISTS sync_cursors (
    scope          TEXT PRIMARY KEY,
    cursor_value   TEXT,
    etag           TEXT,
    last_success_at DATETIME,
    last_error_at   DATETIME,
    error_json      TEXT,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sync_cursors_updated_at ON sync_cursors(updated_at);
