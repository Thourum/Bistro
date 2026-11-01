-- name: ListPatchsets :many
SELECT
    pr_id,
    number,
    head_sha,
    base_sha,
    pushed_at,
    merge_base_sha,
    has_fingerprints
FROM patchsets
WHERE pr_id = ?
ORDER BY number ASC;

-- name: GetCurrentPatchset :one
SELECT ps.pr_id, ps.number, ps.head_sha, ps.base_sha, ps.pushed_at, ps.merge_base_sha, ps.has_fingerprints
FROM patchsets ps
JOIN prs p ON p.id = ps.pr_id AND p.head_sha = ps.head_sha
WHERE ps.pr_id = ?
ORDER BY ps.number DESC
LIMIT 1;

-- name: ListPatchsetFiles :many
SELECT path, patch_id
FROM patchset_files
WHERE pr_id = ? AND patchset_number = ?
ORDER BY path;

-- name: ListCommentPatchsetNumbers :many
SELECT
    id,
    patchset_number
FROM comments
WHERE id IN (sqlc.slice('comment_ids'))
  AND patchset_number IS NOT NULL;
