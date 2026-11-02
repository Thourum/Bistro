-- +goose Up
-- Migration: 00038_add_commits_viewed
-- Description: Local per-account "viewed" marks for commits in the
-- commit-by-commit review view. patch_key is a content fingerprint plus an
-- occurrence ordinal; occurrences is the fingerprint's count in the PR range
-- when marked (a mismatch means a rewrite touched duplicates: unviewed).
CREATE TABLE IF NOT EXISTS commits_viewed (
    pr_id            TEXT      NOT NULL,
    patch_key        TEXT      NOT NULL,
    occurrences      INTEGER   NOT NULL CHECK (occurrences > 0),
    account_provider TEXT      NOT NULL DEFAULT 'github',
    account_login    TEXT      NOT NULL DEFAULT '',
    login            TEXT      NOT NULL,
    viewed_at        TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (pr_id, patch_key, account_provider, account_login, login),
    FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE IF EXISTS commits_viewed;
