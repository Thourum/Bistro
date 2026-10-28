-- +goose Up
-- Migration 009: Backfill repo_id for existing prs rows
-- Description: Backfill repo_id from provider/owner/repo columns; FK enforcement requires table rebuild (deferred)

-- Backfill any rows that were inserted before migration 008 (repo_id = '').
UPDATE prs
SET    repo_id = provider || ':' || owner || '/' || repo
WHERE  repo_id = '';

-- NOTE: SQLite does not support ALTER TABLE ... ADD FOREIGN KEY.
-- Enforcing REFERENCES repos(id) on prs.repo_id requires a full table rebuild
-- (CREATE new table, INSERT ... SELECT, DROP, RENAME) and must be deferred until
-- all existing prs rows are guaranteed to have a matching repos row, to avoid
-- FK violations on the copy step.

-- +goose Down
-- This migration is irreversible.
