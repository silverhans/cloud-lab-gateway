-- name: QueryAuditEvents :many
SELECT *
FROM audit_events
WHERE (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind')::text)
  AND (sqlc.narg('actor_user_id')::uuid IS NULL OR actor_user_id = sqlc.narg('actor_user_id')::uuid)
  AND (sqlc.narg('since')::timestamptz IS NULL OR occurred_at >= sqlc.narg('since')::timestamptz)
ORDER BY occurred_at DESC, id DESC
LIMIT sqlc.arg('limit');
