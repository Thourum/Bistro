-- 026_add_comment_classification.sql
-- Phase 3.1: classification columns + fingerprint side-table.
-- No CHECK constraints: the Go engine is the single writer and validates values.
ALTER TABLE comments ADD COLUMN author_type TEXT;
ALTER TABLE comments ADD COLUMN bot_vendor TEXT;
ALTER TABLE comments ADD COLUMN severity TEXT;
ALTER TABLE comments ADD COLUMN category TEXT;
ALTER TABLE comments ADD COLUMN classification_source TEXT;

CREATE TABLE comment_classification_state (
  comment_id  TEXT PRIMARY KEY,
  fingerprint TEXT NOT NULL,
  FOREIGN KEY(comment_id) REFERENCES comments(id) ON DELETE CASCADE
);
