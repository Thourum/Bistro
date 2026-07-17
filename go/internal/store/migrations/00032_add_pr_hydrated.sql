-- +goose Up
-- Migration: 00032_add_pr_hydrated
-- Description: Track whether a PR row carries full details (comments, CI,
-- diff). List-only rows written for very active repos are hydrated=0 and get
-- their details fetched on demand when the user selects them. Existing rows
-- were all written by full syncs, so they default to hydrated.

ALTER TABLE prs ADD COLUMN hydrated INTEGER NOT NULL DEFAULT 1;

-- +goose Down
-- This migration is irreversible.
