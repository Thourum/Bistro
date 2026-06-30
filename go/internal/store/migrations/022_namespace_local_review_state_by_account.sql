-- Migration: 022_namespace_local_review_state_by_account
-- Description: Namespace local-only review progress and queued actions by GitHub account.

ALTER TABLE last_reviewed_sha RENAME TO last_reviewed_sha_legacy_022;

CREATE TABLE last_reviewed_sha (
    pr_id            TEXT      NOT NULL,
    account_provider TEXT      NOT NULL DEFAULT 'github',
    account_login    TEXT      NOT NULL DEFAULT '',
    login            TEXT      NOT NULL,
    sha              TEXT      NOT NULL,
    updated_at       TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    patchset_number  INTEGER,
    base_sha         TEXT,
    PRIMARY KEY (pr_id, account_provider, account_login, login),
    FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

INSERT INTO last_reviewed_sha (
    pr_id, account_provider, account_login, login, sha, updated_at, patchset_number, base_sha
)
SELECT pr_id, 'github', login, login, sha, updated_at, patchset_number, base_sha
FROM last_reviewed_sha_legacy_022;

DROP TABLE last_reviewed_sha_legacy_022;

CREATE INDEX IF NOT EXISTS idx_last_reviewed_pr ON last_reviewed_sha(pr_id);
CREATE INDEX IF NOT EXISTS idx_last_reviewed_account ON last_reviewed_sha(account_provider, account_login);

ALTER TABLE files_viewed RENAME TO files_viewed_legacy_022;

CREATE TABLE files_viewed (
    pr_id            TEXT      NOT NULL,
    file_path        TEXT      NOT NULL,
    account_provider TEXT      NOT NULL DEFAULT 'github',
    account_login    TEXT      NOT NULL DEFAULT '',
    login            TEXT      NOT NULL,
    content_hash     TEXT      NOT NULL,
    viewed_at        TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (pr_id, file_path, account_provider, account_login, login),
    FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

INSERT INTO files_viewed (
    pr_id, file_path, account_provider, account_login, login, content_hash, viewed_at
)
SELECT pr_id, file_path, 'github', login, login, content_hash, viewed_at
FROM files_viewed_legacy_022;

DROP TABLE files_viewed_legacy_022;

CREATE INDEX IF NOT EXISTS idx_files_viewed_pr_login ON files_viewed(pr_id, login);
CREATE INDEX IF NOT EXISTS idx_files_viewed_account ON files_viewed(account_provider, account_login);

CREATE TABLE IF NOT EXISTS current_user (
    provider TEXT PRIMARY KEY,
    login TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS action_queue (
    id TEXT PRIMARY KEY,
    action_type TEXT NOT NULL,
    pr_id TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    retry_count INTEGER DEFAULT 0,
    last_error TEXT,
    status TEXT DEFAULT 'pending'
);

ALTER TABLE action_queue ADD COLUMN account_provider TEXT NOT NULL DEFAULT 'github';
ALTER TABLE action_queue ADD COLUMN account_login TEXT NOT NULL DEFAULT '';

UPDATE action_queue
SET account_login = COALESCE((SELECT login FROM current_user WHERE provider = 'github'), '')
WHERE account_provider = 'github' AND account_login = '';

CREATE INDEX IF NOT EXISTS idx_action_queue_account_status
    ON action_queue(account_provider, account_login, status, created_at);
