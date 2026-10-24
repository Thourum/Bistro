-- name: GetLastReviewed :one
SELECT
    sha,
    patchset_number,
    reviewed_at,
    source
FROM last_reviewed_sha
WHERE pr_id = sqlc.arg(pr_id)
  AND account_provider = sqlc.arg(account_provider)
  AND account_login = sqlc.arg(account_login)
  AND login = sqlc.arg(login);
