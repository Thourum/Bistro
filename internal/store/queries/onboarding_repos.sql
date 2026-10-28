-- name: InsertDiscoveredRepo :exec
INSERT INTO repos (id, provider, owner, repo)
VALUES (?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING;

-- name: ListOnboardingRepos :many
SELECT id, provider, owner, repo, watched, local_path, source,
       location_status, location_error_code, location_error
FROM repos
ORDER BY watched DESC, owner ASC, repo ASC;
