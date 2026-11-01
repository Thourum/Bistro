-- +goose Up
-- Migration: 006_add_pr_review_status
-- Description: Add review_status, approved_by, changes_requested_by to prs table

ALTER TABLE prs ADD COLUMN review_status TEXT;
ALTER TABLE prs ADD COLUMN approved_by TEXT;
ALTER TABLE prs ADD COLUMN changes_requested_by TEXT;

-- +goose Down
-- This migration is irreversible.
