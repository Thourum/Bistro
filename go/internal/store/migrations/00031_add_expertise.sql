-- +goose Up
-- Migration: 00031_add_expertise
-- Description: Phase 4 local expertise scoring — per-file scores, per-repo
-- indexing state, and global/per-repo expertise settings.

CREATE TABLE file_expertise (
    repo_id         TEXT NOT NULL,            -- 'provider:owner/repo'
    file_path       TEXT NOT NULL,
    author_email    TEXT NOT NULL,
    author_login    TEXT,                     -- resolved when matchable, else NULL
    surviving_lines INTEGER NOT NULL,
    total_lines     INTEGER NOT NULL,
    commit_score    REAL NOT NULL,
    is_original     INTEGER NOT NULL,
    score           REAL NOT NULL,
    last_commit_at  TIMESTAMP,
    dormant         INTEGER NOT NULL DEFAULT 0,
    computed_at     TIMESTAMP NOT NULL,
    computed_at_sha TEXT NOT NULL,
    PRIMARY KEY (repo_id, file_path, author_email)
);
CREATE INDEX idx_file_expertise_repo_path ON file_expertise(repo_id, file_path);

CREATE TABLE expertise_repo_state (
    repo_id            TEXT PRIMARY KEY,
    enabled            INTEGER NOT NULL DEFAULT 0,
    status             TEXT NOT NULL DEFAULT 'idle',
    indexed_sha        TEXT,
    indexed_at         TIMESTAMP,
    error_code         TEXT,
    error              TEXT,
    files_total        INTEGER,
    files_done         INTEGER,
    nudge_dismissed_at TIMESTAMP
);

CREATE TABLE expertise_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- +goose Down
DROP TABLE expertise_settings;
DROP TABLE expertise_repo_state;
DROP INDEX idx_file_expertise_repo_path;
DROP TABLE file_expertise;
