-- name: CreateLabInstance :exec
INSERT INTO lab_instances (
    id,
    student_user_id,
    course_id,
    lab_template_id,
    project_id,
    state,
    state_reason,
    ki_resources,
    cleanup_at,
    unfreeze_at,
    frozen_by_user_id,
    frozen_reason,
    student_ssh_key_secret_id,
    checker_ssh_key_secret_id,
    created_at,
    updated_at
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('student_user_id'),
    sqlc.arg('course_id'),
    sqlc.arg('lab_template_id'),
    sqlc.narg('project_id'),
    sqlc.arg('state'),
    sqlc.arg('state_reason'),
    sqlc.arg('ki_resources'),
    sqlc.narg('cleanup_at'),
    sqlc.narg('unfreeze_at'),
    sqlc.narg('frozen_by_user_id'),
    sqlc.narg('frozen_reason'),
    sqlc.narg('student_ssh_key_secret_id'),
    sqlc.narg('checker_ssh_key_secret_id'),
    COALESCE(sqlc.narg('created_at'), now()),
    COALESCE(sqlc.narg('updated_at'), now())
);

-- name: UpdateLabInstance :one
UPDATE lab_instances
SET project_id = sqlc.narg('project_id'),
    state = sqlc.arg('state'),
    state_reason = sqlc.arg('state_reason'),
    ki_resources = sqlc.arg('ki_resources'),
    cleanup_at = sqlc.narg('cleanup_at'),
    unfreeze_at = sqlc.narg('unfreeze_at'),
    frozen_by_user_id = sqlc.narg('frozen_by_user_id'),
    frozen_reason = sqlc.narg('frozen_reason'),
    student_ssh_key_secret_id = sqlc.narg('student_ssh_key_secret_id'),
    checker_ssh_key_secret_id = sqlc.narg('checker_ssh_key_secret_id'),
    updated_at = sqlc.arg('updated_at')
WHERE id = sqlc.arg('id')
  AND updated_at <= sqlc.arg('updated_at')
RETURNING *;

-- name: GetLabInstanceByID :one
SELECT *
FROM lab_instances
WHERE id = $1;

-- name: FindActiveLabByStudentAndCourse :one
SELECT *
FROM lab_instances
WHERE student_user_id = $1
  AND course_id = $2
  AND state NOT IN ('done', 'rejected', 'failed')
ORDER BY created_at DESC, id
LIMIT 1;

-- name: ListPendingCleanupLabs :many
SELECT *
FROM lab_instances
WHERE state = 'ready'
  AND cleanup_at <= $1
ORDER BY cleanup_at, id
LIMIT $2;

-- name: ListPendingUnfreezeLabs :many
SELECT *
FROM lab_instances
WHERE state = 'frozen'
  AND unfreeze_at <= $1
ORDER BY unfreeze_at, id
LIMIT $2;
