-- +goose Up
-- Migration: 010_add_pr_requested_reviewers
-- Description: Store logins whose review is currently requested, as a JSON
-- array (e.g. '["alice","bob"]'). Powers the Needs Action / FYI triage.

ALTER TABLE prs ADD COLUMN requested_reviewers TEXT;

-- +goose Down
-- This migration is irreversible.
