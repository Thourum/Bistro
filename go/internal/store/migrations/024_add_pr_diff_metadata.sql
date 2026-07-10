-- Migration: 024_add_pr_diff_metadata
-- Description: Persist per-file diff metadata for local diffs

CREATE TABLE IF NOT EXISTS pr_diffs (
    id         INTEGER  PRIMARY KEY AUTOINCREMENT,
    pr_id      TEXT     NOT NULL,
    file_path  TEXT     NOT NULL,
    patch      TEXT     NOT NULL,
    additions  INTEGER  NOT NULL DEFAULT 0,
    deletions  INTEGER  NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(pr_id, file_path),
    FOREIGN KEY(pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

ALTER TABLE pr_diffs ADD COLUMN status TEXT NOT NULL DEFAULT 'modified';
ALTER TABLE pr_diffs ADD COLUMN old_path TEXT;
ALTER TABLE pr_diffs ADD COLUMN is_binary INTEGER NOT NULL DEFAULT 0;
ALTER TABLE pr_diffs ADD COLUMN is_truncated INTEGER NOT NULL DEFAULT 0;
ALTER TABLE pr_diffs ADD COLUMN patch_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE pr_diffs ADD COLUMN hunk_count INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_pr_diffs_pr_id ON pr_diffs(pr_id);
