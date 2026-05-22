-- name: AllocateOneFreeProject :one
WITH candidate AS (
    SELECT id
    FROM projects AS p
    WHERE p.ki_domain_id = $1
      AND p.state = 'free'
    ORDER BY p.last_state_change_at, p.id
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
UPDATE projects
SET state = 'allocated',
    allocated_to_lab_id = $2,
    last_state_change_at = now()
WHERE id = (SELECT id FROM candidate)
RETURNING *;

-- name: UpdateProject :one
UPDATE projects
SET state = $2,
    allocated_to_lab_id = $3,
    cleanup_failures = $4,
    last_state_change_at = $5
WHERE id = $1
  AND last_state_change_at <= $5
RETURNING *;

-- name: GetProjectByID :one
SELECT *
FROM projects
WHERE id = $1;

-- name: ListProjectsByDomain :many
SELECT *
FROM projects
WHERE (sqlc.narg('ki_domain_id')::text IS NULL OR ki_domain_id = sqlc.narg('ki_domain_id')::text)
  AND (sqlc.narg('state')::text IS NULL OR state = sqlc.narg('state')::text)
ORDER BY created_at, id;

-- name: SeedInsertProject :execrows
INSERT INTO projects (
    id,
    ki_project_id,
    ki_domain_id,
    name,
    state,
    allocated_to_lab_id,
    cleanup_failures,
    last_state_change_at,
    created_at
)
VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    COALESCE(sqlc.narg('last_state_change_at'), now()),
    COALESCE(sqlc.narg('created_at'), now())
)
ON CONFLICT (ki_project_id) DO NOTHING;

-- name: InsertOutbox :exec
INSERT INTO outbox (topic, payload, occurred_at)
VALUES ($1, $2, COALESCE(sqlc.narg('occurred_at'), now()));

-- name: InsertAuditEvent :exec
INSERT INTO audit_events (
    id,
    kind,
    actor_user_id,
    subject_type,
    subject_id,
    payload,
    request_id,
    occurred_at
)
VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    NULLIF($7, ''),
    COALESCE(sqlc.narg('occurred_at'), now())
);
