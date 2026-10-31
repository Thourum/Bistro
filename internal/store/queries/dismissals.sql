-- name: InsertCommentDismissal :exec
INSERT OR IGNORE INTO comment_dismissals (comment_id, account_provider, account_login)
VALUES (?, ?, ?);

-- name: DeleteCommentDismissal :exec
DELETE FROM comment_dismissals
WHERE comment_id = ? AND account_provider = ? AND account_login = ?;

-- name: ListDismissedCommentIDsForPR :many
SELECT d.comment_id
FROM comment_dismissals d
JOIN comments c ON c.id = d.comment_id
WHERE c.pr_id = ? AND d.account_provider = ? AND d.account_login = ?
ORDER BY d.comment_id;
