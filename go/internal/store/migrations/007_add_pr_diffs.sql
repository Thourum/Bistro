-- Migration: 007_add_pr_diffs
-- Description: Add pr_diffs table for storing per-file diff patches

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

CREATE INDEX IF NOT EXISTS idx_pr_diffs_pr_id ON pr_diffs(pr_id);
