-- +goose Up
-- Migration: 014_add_last_reviewed_and_files_viewed
-- Description: Reviewer state for Phase 2.1/2.2 patchset timeline and since-last-review.

CREATE TABLE IF NOT EXISTS last_reviewed_sha (
    pr_id      TEXT      NOT NULL,
    login      TEXT      NOT NULL,
    sha        TEXT      NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (pr_id, login),
    FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS files_viewed (
    pr_id        TEXT      NOT NULL,
    file_path    TEXT      NOT NULL,
    login        TEXT      NOT NULL,
    content_hash TEXT      NOT NULL,
    viewed_at    TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (pr_id, file_path, login),
    FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_last_reviewed_pr ON last_reviewed_sha(pr_id);
CREATE INDEX IF NOT EXISTS idx_files_viewed_pr_login ON files_viewed(pr_id, login);

-- +goose Down
-- This migration is irreversible.
