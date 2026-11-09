-- name: UpsertCommitViewed :exec
INSERT INTO commits_viewed (pr_id, patch_key, occurrences, account_provider, account_login, login, viewed_at)
VALUES (sqlc.arg(pr_id), sqlc.arg(patch_key), sqlc.arg(occurrences), sqlc.arg(account_provider), sqlc.arg(account_login), sqlc.arg(login), strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
ON CONFLICT(pr_id, patch_key, account_provider, account_login, login) DO UPDATE SET
    occurrences = excluded.occurrences,
    viewed_at = excluded.viewed_at;

-- name: DeleteCommitViewed :exec
DELETE FROM commits_viewed
WHERE pr_id = sqlc.arg(pr_id) AND patch_key = sqlc.arg(patch_key)
  AND account_provider = sqlc.arg(account_provider) AND account_login = sqlc.arg(account_login) AND login = sqlc.arg(login);

-- name: ListCommitsViewed :many
SELECT patch_key, occurrences
FROM commits_viewed
WHERE pr_id = sqlc.arg(pr_id)
  AND account_provider = sqlc.arg(account_provider) AND account_login = sqlc.arg(account_login) AND login = sqlc.arg(login)
ORDER BY patch_key;
