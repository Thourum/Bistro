-- 029_add_dedup_and_llm_cache.sql
-- No extra indexes by design: cache lookups hit the body_hash PRIMARY KEY,
-- and dedup_group_id is only read on rows already narrowed by pr_id.
ALTER TABLE comments ADD COLUMN dedup_group_id TEXT;

CREATE TABLE llm_classification_cache (
  body_hash  TEXT PRIMARY KEY,
  severity   TEXT NOT NULL,
  provider   TEXT NOT NULL,
  model      TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
