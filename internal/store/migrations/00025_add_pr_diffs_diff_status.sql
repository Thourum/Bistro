-- +goose Up
-- Migration: 025_add_pr_diffs_diff_status
-- Description: Add diff_status column to pr_diffs to track per-file budget enforcement.
-- Values: 'ok' (full patch stored), 'truncated' (per-file budget exceeded),
--         'too_large' (PR-level budget exhausted).

ALTER TABLE pr_diffs ADD COLUMN diff_status TEXT NOT NULL DEFAULT 'ok';

-- +goose Down
-- This migration is irreversible.
