-- +goose Up
CREATE TABLE IF NOT EXISTS rate_limit_state (
    id TEXT PRIMARY KEY DEFAULT 'github',
    provider TEXT NOT NULL,
    reset_at TIMESTAMP NOT NULL,
    remaining_requests INTEGER DEFAULT 0,
    limit_requests INTEGER DEFAULT 5000,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_rate_limit_provider ON rate_limit_state(provider);

-- +goose Down
-- This migration is irreversible.
