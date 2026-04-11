-- Migration 008: Add repos table and repo_id column to prs

CREATE TABLE IF NOT EXISTS repos (
    id       TEXT PRIMARY KEY,  -- "provider:owner/repo" e.g. "github:acme/api"
    provider TEXT NOT NULL,
    owner    TEXT NOT NULL,
    repo     TEXT NOT NULL,
    watched  INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

ALTER TABLE prs ADD COLUMN repo_id TEXT NOT NULL DEFAULT '';
