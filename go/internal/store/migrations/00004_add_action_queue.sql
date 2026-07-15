-- +goose Up
-- Description: Add action queue for offline action persistence

CREATE TABLE IF NOT EXISTS action_queue (
    id TEXT PRIMARY KEY,
    action_type TEXT NOT NULL,
    pr_id TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    retry_count INTEGER DEFAULT 0,
    last_error TEXT,
    status TEXT DEFAULT 'pending' -- pending, processing, completed, failed
);

CREATE INDEX IF NOT EXISTS idx_action_queue_status ON action_queue(status);
CREATE INDEX IF NOT EXISTS idx_action_queue_created_at ON action_queue(created_at);

-- +goose Down
-- This migration is irreversible.
