-- +goose Up
-- Migration: 00036_add_review_drafts
-- Description: Pending line comments, per account, submitted as one GitHub
-- review. No FK to prs: a cache eviction must never drop user-written text.
CREATE TABLE IF NOT EXISTS review_drafts (
    id         TEXT PRIMARY KEY,
    pr_id      TEXT NOT NULL,
    login      TEXT NOT NULL,
    path       TEXT NOT NULL,
    side       TEXT NOT NULL CHECK (side IN ('LEFT', 'RIGHT')),
    start_line INTEGER CHECK (start_line IS NULL OR start_line > 0),
    line       INTEGER NOT NULL CHECK (line > 0),
    body       TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    CHECK (start_line IS NULL OR start_line < line)
);
CREATE INDEX IF NOT EXISTS idx_review_drafts_pr_login ON review_drafts(pr_id, login);

-- +goose Down
-- This migration is irreversible.
