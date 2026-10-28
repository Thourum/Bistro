-- +goose Up
-- Migration: 00035_add_patchset_fingerprints
-- Description: Since-last-review triage. A patchset records the merge base it
-- was diffed against and whether per-file patch IDs were captured;
-- patchset_files holds them; pr_diffs keeps the patch ID used as the viewed
-- hash; last_reviewed_sha records which event set the baseline and when.
-- reviewed_at is TEXT in '%Y-%m-%dT%H:%M:%SZ' (UTC, second precision) so string
-- comparison is chronological.

ALTER TABLE patchsets ADD COLUMN merge_base_sha TEXT NOT NULL DEFAULT '';
ALTER TABLE patchsets ADD COLUMN has_fingerprints INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS patchset_files (
    pr_id           TEXT    NOT NULL,
    patchset_number INTEGER NOT NULL,
    path            TEXT    NOT NULL,
    patch_id        TEXT    NOT NULL,
    PRIMARY KEY (pr_id, patchset_number, path),
    FOREIGN KEY (pr_id, patchset_number) REFERENCES patchsets(pr_id, number) ON DELETE CASCADE
);

ALTER TABLE pr_diffs ADD COLUMN patch_id TEXT NOT NULL DEFAULT '';

ALTER TABLE last_reviewed_sha ADD COLUMN reviewed_at TEXT;
ALTER TABLE last_reviewed_sha ADD COLUMN source TEXT NOT NULL DEFAULT 'bistro_action';
UPDATE last_reviewed_sha SET reviewed_at = updated_at WHERE reviewed_at IS NULL;

-- +goose Down
-- This migration is irreversible.
