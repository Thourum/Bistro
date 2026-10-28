-- name: ListReviewThreads :many
SELECT
    id,
    pr_id,
    COALESCE(file_path, '') AS file_path,
    COALESCE(line, 0) AS line,
    resolved,
    outdated,
    state_known
FROM review_threads
WHERE pr_id = ?
ORDER BY file_path, line;

-- name: GetReviewThread :one
SELECT
    id,
    pr_id,
    COALESCE(file_path, '') AS file_path,
    COALESCE(line, 0) AS line,
    resolved,
    outdated,
    state_known
FROM review_threads
WHERE pr_id = ? AND id = ?;

-- name: SetReviewThreadResolved :execrows
UPDATE review_threads
SET resolved = ?
WHERE pr_id = ? AND id = ?;

-- name: GetReplyTargetCommentIDForThread :one
SELECT id
FROM comments
WHERE pr_id = ? AND thread_id = ? AND kind = 'review'
ORDER BY CASE WHEN in_reply_to IS NULL OR in_reply_to = '' THEN 0 ELSE 1 END,
         created_at ASC,
         id ASC
LIMIT 1;
