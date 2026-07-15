-- name: DeleteDiffsForPR :exec
DELETE FROM pr_diffs
WHERE pr_id = ?;

-- name: InsertDiff :exec
INSERT INTO pr_diffs (
    pr_id,
    file_path,
    status,
    old_path,
    is_binary,
    is_truncated,
    patch_bytes,
    hunk_count,
    patch,
    additions,
    deletions,
    diff_status
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListDiffsForPR :many
SELECT
    file_path,
    status,
    COALESCE(old_path, '') AS old_path,
    is_binary,
    is_truncated,
    patch_bytes,
    hunk_count,
    patch,
    additions,
    deletions,
    diff_status
FROM pr_diffs
WHERE pr_id = ?
ORDER BY file_path;
