-- name: UpsertFileViewed :exec
INSERT INTO files_viewed (
    pr_id,
    file_path,
    account_provider,
    account_login,
    login,
    content_hash,
    viewed_at
)
VALUES (
    sqlc.arg(pr_id),
    sqlc.arg(file_path),
    sqlc.arg(account_provider),
    sqlc.arg(account_login),
    sqlc.arg(login),
    sqlc.arg(content_hash),
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
)
ON CONFLICT(pr_id, file_path, account_provider, account_login, login) DO UPDATE SET
    content_hash = excluded.content_hash,
    viewed_at = excluded.viewed_at;

-- name: GetViewedFileContentHash :one
SELECT content_hash
FROM files_viewed
WHERE pr_id = sqlc.arg(pr_id)
  AND file_path = sqlc.arg(file_path)
  AND account_provider = sqlc.arg(account_provider)
  AND account_login = sqlc.arg(account_login)
  AND login = sqlc.arg(login);

-- name: ListViewedFilesForLogin :many
SELECT file_path, content_hash
FROM files_viewed
WHERE pr_id = sqlc.arg(pr_id)
  AND account_provider = sqlc.arg(account_provider)
  AND account_login = sqlc.arg(account_login)
  AND login = sqlc.arg(login)
ORDER BY file_path;
