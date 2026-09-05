-- +goose Up
-- Migration: 00034_add_pr_read_state
-- Description: Phase 5 inbox read state. A PR is unread for a login when no
-- row exists or prs.updated_at > last_read_at.

CREATE TABLE IF NOT EXISTS pr_read_state (
    pr_id        TEXT NOT NULL,
    login        TEXT NOT NULL,
    last_read_at TIMESTAMP NOT NULL,
    PRIMARY KEY (pr_id, login)
);

-- +goose Down
-- This migration is irreversible.
