-- Migration: 012_add_review_comments_and_patchsets
-- Description: Inline review comments (unified comments table), review threads, observed patchsets, current_user

-- review_threads MUST be created BEFORE the comments.thread_id FK column is added,
-- or the ALTER TABLE ... REFERENCES fails at apply time (foreign key mismatch).
CREATE TABLE IF NOT EXISTS review_threads (
    id          TEXT    PRIMARY KEY,
    pr_id       TEXT    NOT NULL,
    file_path   TEXT,
    line        INTEGER,
    resolved    INTEGER NOT NULL DEFAULT 0,
    outdated    INTEGER NOT NULL DEFAULT 0,
    state_known INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY(pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_review_threads_pr_id ON review_threads(pr_id);

-- Extend the comments table with kind discriminator + anchoring fields.
-- kind defaults to 'issue' so all existing rows are untouched.
ALTER TABLE comments ADD COLUMN kind              TEXT    NOT NULL DEFAULT 'issue';
ALTER TABLE comments ADD COLUMN commit_sha        TEXT;
ALTER TABLE comments ADD COLUMN original_sha      TEXT;
ALTER TABLE comments ADD COLUMN side              TEXT;
ALTER TABLE comments ADD COLUMN position          INTEGER;
ALTER TABLE comments ADD COLUMN original_position INTEGER;
ALTER TABLE comments ADD COLUMN in_reply_to       TEXT;
-- thread_id FK: ON DELETE SET NULL so deleting a thread orphans its comments
-- rather than cascading (comments survive for history).
ALTER TABLE comments ADD COLUMN thread_id         TEXT REFERENCES review_threads(id) ON DELETE SET NULL;
-- patchset_number is filled by Phase 2.3; NULL for now.
ALTER TABLE comments ADD COLUMN patchset_number   INTEGER;

CREATE INDEX IF NOT EXISTS idx_comments_thread_id ON comments(thread_id);
CREATE INDEX IF NOT EXISTS idx_comments_pr_kind ON comments(pr_id, kind);

-- Observed patchsets: one row per unique (pr_id, base_sha, head_sha) tuple.
-- Both SHAs matter: a base-only move changes the effective PR diff.
-- pushed_at = observation time (named deliberately: Phase 2 server will backfill
-- true push times into this column — do not rename to observed_at).
CREATE TABLE IF NOT EXISTS patchsets (
    pr_id     TEXT    NOT NULL,
    number    INTEGER NOT NULL,
    head_sha  TEXT    NOT NULL,
    base_sha  TEXT    NOT NULL,
    pushed_at TIMESTAMP NOT NULL,
    PRIMARY KEY (pr_id, number),
    UNIQUE (pr_id, base_sha, head_sha),
    FOREIGN KEY(pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_patchsets_pr_id ON patchsets(pr_id);

-- Current authenticated user, keyed by provider slug.
CREATE TABLE IF NOT EXISTS current_user (
    provider TEXT PRIMARY KEY,
    login TEXT NOT NULL
);
