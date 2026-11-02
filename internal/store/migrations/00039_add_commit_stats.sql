-- +goose Up
-- Migration: 00039_add_commit_stats
-- Description: Cached list statistics for commits in a PR's range. A commit
-- SHA fixes its diff, so rows are only replaced by sync or invalidated by a
-- changed algo_version (diff options fingerprint changed).
CREATE TABLE IF NOT EXISTS commit_stats (
    pr_id        TEXT    NOT NULL,
    sha          TEXT    NOT NULL,
    additions    INTEGER NOT NULL,
    deletions    INTEGER NOT NULL,
    file_count   INTEGER NOT NULL,
    patch_id     TEXT    NOT NULL,
    algo_version INTEGER NOT NULL,
    PRIMARY KEY (pr_id, sha),
    FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE IF EXISTS commit_stats;
