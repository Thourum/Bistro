-- Migration: 001_init
-- Description: Initial schema - create prs, comments, files tables

CREATE TABLE IF NOT EXISTS prs (
    id            TEXT     PRIMARY KEY,
    provider      TEXT     NOT NULL,
    owner         TEXT     NOT NULL,
    repo          TEXT     NOT NULL,
    number        INTEGER  NOT NULL,
    title         TEXT     NOT NULL,
    body          TEXT,
    author        TEXT     NOT NULL,
    state         TEXT     NOT NULL,
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL,
    merged_at     DATETIME,
    base_branch   TEXT     NOT NULL,
    head_branch   TEXT     NOT NULL,
    base_sha      TEXT     NOT NULL,
    head_sha      TEXT     NOT NULL,
    comment_count INTEGER  DEFAULT 0,
    ci_status     TEXT,
    draft         INTEGER  DEFAULT 0,
    UNIQUE(provider, owner, repo, number)
);

CREATE TABLE IF NOT EXISTS comments (
    id          TEXT     PRIMARY KEY,
    pr_id       TEXT     NOT NULL,
    author      TEXT     NOT NULL,
    body        TEXT     NOT NULL,
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL,
    line_number INTEGER,
    file_path   TEXT,
    FOREIGN KEY(pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS files (
    id         TEXT    PRIMARY KEY,
    pr_id      TEXT    NOT NULL,
    file_path  TEXT    NOT NULL,
    status     TEXT    NOT NULL,
    additions  INTEGER DEFAULT 0,
    deletions  INTEGER DEFAULT 0,
    patch_path TEXT,
    FOREIGN KEY(pr_id) REFERENCES prs(id) ON DELETE CASCADE,
    UNIQUE(pr_id, file_path)
);
