-- 028_add_comment_dismissals.sql
-- Phase 3.5: local dismissals, namespaced per account (pattern of migration 022).
CREATE TABLE comment_dismissals (
  comment_id       TEXT NOT NULL,
  account_provider TEXT NOT NULL DEFAULT 'github',
  account_login    TEXT NOT NULL DEFAULT '',
  dismissed_at     TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  PRIMARY KEY (comment_id, account_provider, account_login),
  FOREIGN KEY(comment_id) REFERENCES comments(id) ON DELETE CASCADE
);
