-- name: GetPRCheckoutInfo :one
-- Minimal PR fields needed to resolve a local checkout: which repo it
-- belongs to (repo_id, for the repos.local_path lookup) and the PR's head
-- SHA (the commit a checkout should land on).
SELECT
    repo_id,
    head_sha
FROM prs
WHERE id = ?;
