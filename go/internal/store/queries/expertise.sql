-- name: UpsertFileExpertise :exec
INSERT INTO file_expertise (
    repo_id, file_path, author_email, author_login, surviving_lines,
    total_lines, commit_score, is_original, score, last_commit_at,
    dormant, computed_at, computed_at_sha
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(repo_id, file_path, author_email) DO UPDATE SET
    author_login = excluded.author_login,
    surviving_lines = excluded.surviving_lines,
    total_lines = excluded.total_lines,
    commit_score = excluded.commit_score,
    is_original = excluded.is_original,
    score = excluded.score,
    last_commit_at = excluded.last_commit_at,
    dormant = excluded.dormant,
    computed_at = excluded.computed_at,
    computed_at_sha = excluded.computed_at_sha;

-- name: DeleteFileExpertiseForFile :exec
DELETE FROM file_expertise WHERE repo_id = ? AND file_path = ?;

-- name: DeleteFileExpertiseForRepo :exec
DELETE FROM file_expertise WHERE repo_id = ?;

-- name: GetFileExpertise :many
SELECT * FROM file_expertise
WHERE repo_id = ? AND file_path = ?
ORDER BY score DESC;

-- name: ListExpertiseFilePathsForRepo :many
SELECT DISTINCT file_path FROM file_expertise WHERE repo_id = ?;

-- name: GetExpertiseRepoSource :one
SELECT source FROM repos WHERE id = ?;

-- name: ListReadyExpertisePRIDs :many
SELECT p.id
FROM prs p
JOIN expertise_repo_state ers ON ers.repo_id = p.repo_id
WHERE p.id IN (sqlc.slice('pr_ids'))
  AND ers.enabled = 1 AND ers.status = 'ready';

-- name: GetExpertiseRepoState :one
SELECT * FROM expertise_repo_state WHERE repo_id = ?;

-- name: UpsertExpertiseRepoState :exec
INSERT INTO expertise_repo_state (
    repo_id, enabled, status, indexed_sha, indexed_at, error_code, error,
    files_total, files_done, nudge_dismissed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(repo_id) DO UPDATE SET
    enabled = excluded.enabled,
    status = excluded.status,
    indexed_sha = excluded.indexed_sha,
    indexed_at = excluded.indexed_at,
    error_code = excluded.error_code,
    error = excluded.error,
    files_total = excluded.files_total,
    files_done = excluded.files_done,
    nudge_dismissed_at = excluded.nudge_dismissed_at;

-- name: UpdateExpertiseIndexProgress :exec
UPDATE expertise_repo_state SET files_done = ?, files_total = ? WHERE repo_id = ?;

-- name: ListInterruptedExpertiseRepos :many
SELECT repo_id FROM expertise_repo_state WHERE enabled = 1 AND status = 'indexing';

-- name: GetExpertiseSetting :one
SELECT value FROM expertise_settings WHERE key = ?;

-- name: UpsertExpertiseSetting :exec
INSERT INTO expertise_settings (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value;

-- name: CountReviewedPRsForRepo :one
-- last_reviewed_sha is account-namespaced (migration 022); filter by the
-- current account, not just login, or another account on the same machine
-- inflates the count.
SELECT COUNT(DISTINCT lrs.pr_id)
FROM last_reviewed_sha lrs
JOIN prs p ON p.id = lrs.pr_id
WHERE p.repo_id = ?
  AND lrs.account_provider = ?
  AND lrs.account_login = ?
  AND lrs.login = ?;

-- name: GetExpertiseForPRs :many
SELECT
    p.id AS pr_id,
    fe.repo_id,
    fe.file_path,
    fe.author_email,
    fe.author_login,
    fe.surviving_lines,
    fe.total_lines,
    fe.commit_score,
    fe.is_original,
    fe.score,
    fe.dormant
FROM prs p
JOIN pr_diffs pd ON pd.pr_id = p.id
JOIN file_expertise fe ON fe.repo_id = p.repo_id AND fe.file_path = pd.file_path
WHERE p.id IN (sqlc.slice('pr_ids'))
  AND fe.dormant = 0
ORDER BY p.id, fe.file_path, fe.score DESC;
