package store

// schema defines all tables and indexes for the bistro sync store.
// Uses IF NOT EXISTS so Migrate is idempotent.
const schema = `
CREATE TABLE IF NOT EXISTS prs (
	id           TEXT     PRIMARY KEY,
	provider     TEXT     NOT NULL,
	owner        TEXT     NOT NULL,
	repo         TEXT     NOT NULL,
	number       INTEGER  NOT NULL,
	title        TEXT     NOT NULL,
	body         TEXT,
	author       TEXT     NOT NULL,
	state        TEXT     NOT NULL,
	created_at   DATETIME NOT NULL,
	updated_at   DATETIME NOT NULL,
	merged_at    DATETIME,
	base_branch  TEXT     NOT NULL,
	head_branch  TEXT     NOT NULL,
	base_sha     TEXT     NOT NULL,
	head_sha     TEXT     NOT NULL,
	comment_count INTEGER DEFAULT 0,
	ci_status    TEXT,
	draft        INTEGER  DEFAULT 0,
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
	FOREIGN KEY(pr_id) REFERENCES prs(id)
);

CREATE TABLE IF NOT EXISTS files (
	id         TEXT    PRIMARY KEY,
	pr_id      TEXT    NOT NULL,
	file_path  TEXT    NOT NULL,
	status     TEXT    NOT NULL,
	additions  INTEGER DEFAULT 0,
	deletions  INTEGER DEFAULT 0,
	patch_path TEXT,
	FOREIGN KEY(pr_id) REFERENCES prs(id),
	UNIQUE(pr_id, file_path)
);

CREATE INDEX IF NOT EXISTS idx_prs_updated_at  ON prs(updated_at);
CREATE INDEX IF NOT EXISTS idx_comments_pr_id  ON comments(pr_id);
CREATE INDEX IF NOT EXISTS idx_files_pr_id     ON files(pr_id);
`

// Migrate applies the schema to the database.
// It is safe to call multiple times (idempotent).
func Migrate(db *DB) error {
	_, err := db.Exec(schema)
	return err
}
