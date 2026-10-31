-- name: UpsertQueuedAction :exec
INSERT INTO action_queue (
    id,
    action_type,
    pr_id,
    payload,
    account_provider,
    account_login,
    status,
    retry_count
)
VALUES (?, ?, ?, ?, ?, ?, 'pending', 0)
ON CONFLICT(id) DO UPDATE SET
    payload = excluded.payload,
    account_provider = excluded.account_provider,
    account_login = excluded.account_login,
    status = 'pending',
    retry_count = 0;

-- name: GetQueuedActionByID :one
SELECT
    id,
    action_type,
    pr_id,
    payload,
    account_provider,
    account_login,
    CAST(created_at AS TEXT) AS created_at,
    retry_count,
    COALESCE(last_error, '') AS last_error,
    status
FROM action_queue
WHERE id = ?;

-- name: ListPendingQueuedActions :many
SELECT
    id,
    action_type,
    pr_id,
    payload,
    account_provider,
    account_login,
    CAST(created_at AS TEXT) AS created_at,
    retry_count,
    COALESCE(last_error, '') AS last_error,
    status
FROM action_queue
WHERE status = 'pending' AND retry_count < ?
ORDER BY created_at ASC
LIMIT ?;

-- name: ListPendingQueuedActionsForAccount :many
SELECT
    id,
    action_type,
    pr_id,
    payload,
    account_provider,
    account_login,
    CAST(created_at AS TEXT) AS created_at,
    retry_count,
    COALESCE(last_error, '') AS last_error,
    status
FROM action_queue
WHERE status = 'pending'
  AND retry_count < ?
  AND account_provider = ?
  AND account_login = ?
ORDER BY created_at ASC
LIMIT ?;

-- name: ResetProcessingQueuedActions :execrows
UPDATE action_queue
SET status = 'pending'
WHERE status = 'processing';

-- name: MarkQueuedActionProcessing :execrows
UPDATE action_queue
SET status = 'processing'
WHERE id = ? AND status = 'pending';

-- name: MarkQueuedActionCompleted :execrows
UPDATE action_queue
SET status = 'completed'
WHERE id = ?;

-- name: MarkQueuedActionFailed :execrows
UPDATE action_queue
SET status = ?, last_error = ?, retry_count = ?
WHERE id = ?;

-- name: DeleteCompletedQueuedActionsBefore :execrows
DELETE FROM action_queue
WHERE status = 'completed' AND created_at <= CAST(? AS TEXT);

-- name: DeleteFailedQueuedActionsBefore :exec
DELETE FROM action_queue
WHERE status = 'failed' AND created_at <= CAST(? AS TEXT);
