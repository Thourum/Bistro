-- +goose Up
-- Migration: 00033_add_pr_mentioned
-- Description: Phase 5 inbox — whether the current viewer's @login appears in
-- the PR body or any synced comment. Computed locally at classify time from
-- already-synced data; no extra GitHub API calls.

ALTER TABLE prs ADD COLUMN mentioned INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- This migration is irreversible.
