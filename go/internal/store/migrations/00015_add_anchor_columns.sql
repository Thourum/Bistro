-- +goose Up
-- Migration: 015_add_anchor_columns
-- Description: Cache columns for AnchorResolver results on kind='review' comments.

ALTER TABLE comments ADD COLUMN anchored_line      INTEGER;
ALTER TABLE comments ADD COLUMN anchor_head_sha    TEXT;
ALTER TABLE comments ADD COLUMN outdated           INTEGER NOT NULL DEFAULT 0;
ALTER TABLE comments ADD COLUMN outdated_near_line INTEGER;

-- +goose Down
-- This migration is irreversible.
