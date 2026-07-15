-- name: UpsertLastReviewedPatchset :exec
INSERT INTO last_reviewed_sha (
    pr_id,
    account_provider,
    account_login,
    login,
    sha,
    patchset_number,
    base_sha,
    updated_at
)
VALUES (
    sqlc.arg(pr_id),
    sqlc.arg(account_provider),
    sqlc.arg(account_login),
    sqlc.arg(login),
    sqlc.arg(sha),
    sqlc.narg(patchset_number),
    sqlc.arg(base_sha),
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
)
ON CONFLICT(pr_id, account_provider, account_login, login) DO UPDATE SET
    sha = excluded.sha,
    patchset_number = excluded.patchset_number,
    base_sha = excluded.base_sha,
    updated_at = excluded.updated_at;

-- name: GetLastReviewedPatchset :one
SELECT
    sha,
    COALESCE(patchset_number, 0) AS patchset_number,
    COALESCE(base_sha, '') AS base_sha
FROM last_reviewed_sha
WHERE pr_id = sqlc.arg(pr_id)
  AND account_provider = sqlc.arg(account_provider)
  AND account_login = sqlc.arg(account_login)
  AND login = sqlc.arg(login);

-- name: UpsertLastReviewedSHA :exec
INSERT INTO last_reviewed_sha (
    pr_id,
    account_provider,
    account_login,
    login,
    sha,
    updated_at
)
VALUES (
    sqlc.arg(pr_id),
    sqlc.arg(account_provider),
    sqlc.arg(account_login),
    sqlc.arg(login),
    sqlc.arg(sha),
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
)
ON CONFLICT(pr_id, account_provider, account_login, login) DO UPDATE SET
    sha = excluded.sha,
    updated_at = excluded.updated_at;

-- name: GetLastReviewedSHA :one
SELECT sha
FROM last_reviewed_sha
WHERE pr_id = sqlc.arg(pr_id)
  AND account_provider = sqlc.arg(account_provider)
  AND account_login = sqlc.arg(account_login)
  AND login = sqlc.arg(login);
