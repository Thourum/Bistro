-- +goose Up
-- Migration: 00036_fold_repo_id_case
-- Description: GitHub repo and PR identities are lowercase. Fold old mixed-case
-- rows and references into their canonical identities.

-- Foreign keys lack ON UPDATE CASCADE; parents and children change in one tx.
PRAGMA defer_foreign_keys = ON;

-- Preserve watch flags and whichever row has a local/cache location.
INSERT INTO repos (id, provider, owner, repo, watched, created_at, local_path, source,
                   last_used_at, location_status, location_error_code, location_error)
SELECT lower(id), lower(provider), lower(owner), lower(repo), watched, created_at, local_path, source,
       last_used_at, location_status, location_error_code, location_error
FROM repos
WHERE id <> lower(id)
ON CONFLICT(id) DO UPDATE SET
    owner               = excluded.owner,
    repo                = excluded.repo,
    watched             = MAX(repos.watched, excluded.watched),
    created_at          = MIN(repos.created_at, excluded.created_at),
    local_path          = CASE WHEN repos.source IS NULL THEN excluded.local_path ELSE repos.local_path END,
    last_used_at        = CASE WHEN repos.source IS NULL THEN excluded.last_used_at ELSE repos.last_used_at END,
    location_status     = CASE WHEN repos.source IS NULL THEN excluded.location_status ELSE repos.location_status END,
    location_error_code = CASE WHEN repos.source IS NULL THEN excluded.location_error_code ELSE repos.location_error_code END,
    location_error      = CASE WHEN repos.source IS NULL THEN excluded.location_error ELSE repos.location_error END,
    source              = COALESCE(repos.source, excluded.source);

DELETE FROM repos WHERE id <> lower(id);
UPDATE repos SET owner = lower(owner), repo = lower(repo);

-- On collisions the lowercase row wins. Delete duplicate parents before folding
-- children so ON DELETE CASCADE cannot delete the canonical row's children.
-- ponytail: lossy on real collisions (a duplicate PR's local review state,
-- newer pending actions and newer read state are dropped). Accepted pre-release;
-- reparent/merge children before deleting parents if duplicates ever matter.
UPDATE OR IGNORE prs
SET id = lower(id), owner = lower(owner), repo = lower(repo), repo_id = lower(repo_id);
DELETE FROM prs WHERE id <> lower(id);
-- A duplicate's owner/repo UNIQUE key may have blocked the canonical row above.
UPDATE prs SET owner = lower(owner), repo = lower(repo), repo_id = lower(repo_id);

UPDATE OR IGNORE comments SET pr_id = lower(pr_id) WHERE pr_id <> lower(pr_id);
DELETE FROM comments WHERE pr_id <> lower(pr_id);
UPDATE OR IGNORE files SET pr_id = lower(pr_id) WHERE pr_id <> lower(pr_id);
DELETE FROM files WHERE pr_id <> lower(pr_id);
-- Queue IDs embed the PR ID (sync.queuedActionID) and dedupe by ON CONFLICT(id).
UPDATE OR IGNORE action_queue
SET id = CASE WHEN instr(id, pr_id) > 0
              THEN substr(id, 1, instr(id, pr_id) - 1) || lower(pr_id) || substr(id, instr(id, pr_id) + length(pr_id))
              ELSE id END,
    pr_id = lower(pr_id)
WHERE pr_id <> lower(pr_id);
DELETE FROM action_queue WHERE pr_id <> lower(pr_id);
UPDATE OR IGNORE pr_diffs SET pr_id = lower(pr_id) WHERE pr_id <> lower(pr_id);
DELETE FROM pr_diffs WHERE pr_id <> lower(pr_id);
UPDATE OR IGNORE review_threads SET pr_id = lower(pr_id) WHERE pr_id <> lower(pr_id);
DELETE FROM review_threads WHERE pr_id <> lower(pr_id);
UPDATE OR IGNORE patchsets SET pr_id = lower(pr_id) WHERE pr_id <> lower(pr_id);
DELETE FROM patchsets WHERE pr_id <> lower(pr_id);
UPDATE OR IGNORE patchset_files SET pr_id = lower(pr_id) WHERE pr_id <> lower(pr_id);
DELETE FROM patchset_files WHERE pr_id <> lower(pr_id);
UPDATE OR IGNORE last_reviewed_sha SET pr_id = lower(pr_id) WHERE pr_id <> lower(pr_id);
DELETE FROM last_reviewed_sha WHERE pr_id <> lower(pr_id);
UPDATE OR IGNORE files_viewed SET pr_id = lower(pr_id) WHERE pr_id <> lower(pr_id);
DELETE FROM files_viewed WHERE pr_id <> lower(pr_id);
UPDATE OR IGNORE pr_read_state SET pr_id = lower(pr_id) WHERE pr_id <> lower(pr_id);
DELETE FROM pr_read_state WHERE pr_id <> lower(pr_id);

-- Only thread payloads carry a PR identity; other actions can contain arbitrary
-- user text (including JSON). Preserve that text and opaque thread IDs.
UPDATE action_queue
SET payload = json_set(payload, '$.pr_id', lower(json_extract(payload, '$.pr_id')))
WHERE action_type IN ('resolve_thread', 'unresolve_thread')
  AND CASE WHEN json_valid(payload) THEN json_type(payload, '$.pr_id') = 'text' ELSE 0 END;

-- sync_cursors.scope is github:inbox:<login>:search:involves, not a repo identity.

-- file_expertise is a recomputable cache: on a collision keep the lowercase row.
UPDATE OR IGNORE file_expertise SET repo_id = lower(repo_id) WHERE repo_id <> lower(repo_id);
DELETE FROM file_expertise WHERE repo_id <> lower(repo_id);

-- Keep the lowercase row's index state, but never drop a user's opt-in or dismissal.
INSERT INTO expertise_repo_state (repo_id, enabled, status, indexed_sha, indexed_at, error_code, error,
                                  files_total, files_done, nudge_dismissed_at)
SELECT lower(repo_id), enabled, status, indexed_sha, indexed_at, error_code, error,
       files_total, files_done, nudge_dismissed_at
FROM expertise_repo_state
WHERE repo_id <> lower(repo_id)
ON CONFLICT(repo_id) DO UPDATE SET
    enabled            = MAX(expertise_repo_state.enabled, excluded.enabled),
    nudge_dismissed_at = COALESCE(expertise_repo_state.nudge_dismissed_at, excluded.nudge_dismissed_at);
DELETE FROM expertise_repo_state WHERE repo_id <> lower(repo_id);

-- Per-repo exclude globs are keyed excludes:<repoID>; on a collision keep the lowercase key.
UPDATE OR IGNORE expertise_settings SET key = lower(key) WHERE key LIKE 'excludes:%' AND key <> lower(key);
DELETE FROM expertise_settings WHERE key LIKE 'excludes:%' AND key <> lower(key);

-- +goose Down
-- This migration is irreversible.
