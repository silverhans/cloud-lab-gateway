-- name: FetchOutbox :many
SELECT id, topic, payload, occurred_at, attempts
FROM outbox
WHERE published_at IS NULL
  AND attempts < sqlc.arg('max_attempts')
ORDER BY id ASC
LIMIT sqlc.arg('limit');

-- name: MarkOutboxPublished :exec
UPDATE outbox
SET published_at = now()
WHERE id = ANY(sqlc.arg('ids')::bigint[]);

-- name: BumpOutboxAttempts :exec
UPDATE outbox
SET attempts = attempts + 1
WHERE id = $1;
