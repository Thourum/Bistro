-- Migration: 016_rebuild_patchsets_base_head_identity
-- Description: Repair Phase 2 Groundwork patchsets identity from head-only to base+head.

-- Migration 012 originally used UNIQUE(pr_id, head_sha) while Phase 2 patchset
-- identity must include both diff endpoints. Rebuild the table so DBs that
-- already applied the earlier working-tree 012 get the corrected constraint.
ALTER TABLE patchsets RENAME TO patchsets_old;

CREATE TABLE patchsets (
    pr_id     TEXT    NOT NULL,
    number    INTEGER NOT NULL,
    head_sha  TEXT    NOT NULL,
    base_sha  TEXT    NOT NULL,
    pushed_at TIMESTAMP NOT NULL,
    PRIMARY KEY (pr_id, number),
    UNIQUE (pr_id, base_sha, head_sha),
    FOREIGN KEY(pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
SELECT pr_id, number, head_sha, base_sha, pushed_at
FROM patchsets_old;

DROP TABLE patchsets_old;

CREATE INDEX IF NOT EXISTS idx_patchsets_pr_id ON patchsets(pr_id);
