-- name: UpsertCurrentUser :exec
INSERT INTO current_user (provider, login)
VALUES (?, ?)
ON CONFLICT(provider) DO UPDATE SET
    login = excluded.login;

-- name: DeleteCurrentUser :exec
DELETE FROM current_user
WHERE provider = ?;

-- name: GetCurrentUserLogin :one
SELECT login
FROM current_user
WHERE provider = ?;
