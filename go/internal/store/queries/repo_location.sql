-- name: UpsertLocalRepoLocation :exec
INSERT INTO repos (
    id,
    provider,
    owner,
    repo,
    watched,
    local_path,
    source,
    last_used_at,
    location_status,
    location_error_code,
    location_error
)
VALUES (?, ?, ?, ?, 1, ?, 'local', ?, 'ok', NULL, NULL)
ON CONFLICT(id) DO UPDATE SET
    provider = excluded.provider,
    owner = excluded.owner,
    repo = excluded.repo,
    watched = 1,
    local_path = excluded.local_path,
    source = excluded.source,
    last_used_at = excluded.last_used_at,
    location_status = excluded.location_status,
    location_error_code = NULL,
    location_error = NULL;

-- name: SetCacheRepoLocation :exec
UPDATE repos
SET
    local_path = ?,
    source = 'cache',
    last_used_at = ?,
    location_status = 'ok',
    location_error_code = NULL,
    location_error = NULL
WHERE id = ?;

-- name: TouchRepoLocation :exec
UPDATE repos
SET
    last_used_at = ?,
    location_status = 'ok',
    location_error_code = NULL,
    location_error = NULL
WHERE id = ?;

-- name: MarkRepoLocationDegraded :exec
UPDATE repos
SET
    location_status = ?,
    location_error_code = ?,
    location_error = ?
WHERE id = ?;

-- name: ClearRepoLocationDegraded :exec
UPDATE repos
SET
    location_status = 'ok',
    location_error_code = NULL,
    location_error = NULL
WHERE id = ?;

-- name: ClearCacheRepoLocation :exec
UPDATE repos
SET
    local_path = NULL,
    source = NULL,
    last_used_at = NULL,
    location_status = NULL,
    location_error_code = NULL,
    location_error = NULL
WHERE id = ?;

-- name: GetRepoLocation :one
SELECT
    id,
    provider,
    owner,
    repo,
    COALESCE(local_path, '') AS local_path,
    COALESCE(source, '') AS source,
    last_used_at,
    COALESCE(location_status, '') AS location_status,
    COALESCE(location_error_code, '') AS location_error_code,
    COALESCE(location_error, '') AS location_error
FROM repos
WHERE id = ?;

-- name: ListRepoLocationsBySource :many
SELECT
    id,
    provider,
    owner,
    repo,
    COALESCE(local_path, '') AS local_path,
    COALESCE(source, '') AS source,
    last_used_at,
    COALESCE(location_status, '') AS location_status,
    COALESCE(location_error_code, '') AS location_error_code,
    COALESCE(location_error, '') AS location_error
FROM repos
WHERE source = ?
ORDER BY id;
