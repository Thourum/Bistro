-- name: ListCommitStats :many
SELECT sha, additions, deletions, file_count, patch_id
FROM commit_stats
WHERE pr_id = sqlc.arg(pr_id) AND algo_version = sqlc.arg(algo_version);

-- name: UpsertCommitStats :exec
INSERT INTO commit_stats (pr_id, sha, additions, deletions, file_count, patch_id, algo_version)
VALUES (sqlc.arg(pr_id), sqlc.arg(sha), sqlc.arg(additions), sqlc.arg(deletions), sqlc.arg(file_count), sqlc.arg(patch_id), sqlc.arg(algo_version))
ON CONFLICT(pr_id, sha) DO UPDATE SET
    additions = excluded.additions,
    deletions = excluded.deletions,
    file_count = excluded.file_count,
    patch_id = excluded.patch_id,
    algo_version = excluded.algo_version;

-- name: DeleteCommitStatsForPR :exec
DELETE FROM commit_stats WHERE pr_id = sqlc.arg(pr_id);
