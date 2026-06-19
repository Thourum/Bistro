-- Migration: 011_fix_rate_limit_state_provider_key
-- Description: Rebuild rate_limit_state so the PRIMARY KEY is the provider name, removing the DEFAULT 'github' landmine that would silently overwrite rows when a second provider is added

-- Recreate without the unsafe DEFAULT so every insert must name the provider explicitly.
CREATE TABLE IF NOT EXISTS rate_limit_state_new (
    id                  TEXT     PRIMARY KEY,
    provider            TEXT     NOT NULL,
    reset_at            TIMESTAMP NOT NULL,
    remaining_requests  INTEGER  DEFAULT 0,
    limit_requests      INTEGER  DEFAULT 5000,
    updated_at          TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Preserve any existing row (the single 'github' row inserted by the old schema).
INSERT INTO rate_limit_state_new (id, provider, reset_at, remaining_requests, limit_requests, updated_at)
SELECT id, provider, reset_at, remaining_requests, limit_requests, updated_at
FROM rate_limit_state;

DROP TABLE rate_limit_state;

ALTER TABLE rate_limit_state_new RENAME TO rate_limit_state;

-- Recreate the index that was on the old table.
CREATE INDEX IF NOT EXISTS idx_rate_limit_provider ON rate_limit_state(provider);
