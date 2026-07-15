-- name: ListPatchsets :many
SELECT
    pr_id,
    number,
    head_sha,
    base_sha,
    pushed_at
FROM patchsets
WHERE pr_id = ?
ORDER BY number ASC;

-- name: ListCommentPatchsetNumbers :many
SELECT
    id,
    patchset_number
FROM comments
WHERE id IN (sqlc.slice('comment_ids'))
  AND patchset_number IS NOT NULL;
