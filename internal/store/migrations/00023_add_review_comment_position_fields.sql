-- +goose Up
-- Migration: 023_add_review_comment_position_fields
-- Description: Add GitHub review-comment line range and subject positioning fields.

-- These columns are GitHub-synced review-comment state. They are NULL for
-- issue comments and for existing local cache rows until the next sync.
ALTER TABLE comments ADD COLUMN original_line INTEGER;
ALTER TABLE comments ADD COLUMN start_line INTEGER;
ALTER TABLE comments ADD COLUMN original_start_line INTEGER;
ALTER TABLE comments ADD COLUMN start_side TEXT;
ALTER TABLE comments ADD COLUMN subject_type TEXT;

-- +goose Down
-- This migration is irreversible.
