-- name: InsertReviewDraft :one
INSERT INTO review_drafts (id, pr_id, login, path, side, start_line, line, body, commit_sha)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateReviewDraftBody :one
UPDATE review_drafts SET body = ?
WHERE id = ? AND login = ?
RETURNING *;

-- name: ListReviewDrafts :many
SELECT * FROM review_drafts
WHERE pr_id = ? AND login = ?
ORDER BY created_at, id;

-- name: DeleteReviewDraft :exec
DELETE FROM review_drafts WHERE id = ? AND login = ?;
