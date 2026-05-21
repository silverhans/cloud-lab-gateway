-- name: CreateCheckRun :exec
INSERT INTO check_runs (
    id,
    lab_instance_id,
    check_template_id,
    triggered_by_user_id,
    state,
    summary,
    ansible_stdout,
    started_at,
    finished_at
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('lab_instance_id'),
    sqlc.arg('check_template_id'),
    sqlc.narg('triggered_by_user_id'),
    sqlc.arg('state'),
    sqlc.arg('summary'),
    sqlc.arg('ansible_stdout'),
    sqlc.narg('started_at'),
    sqlc.narg('finished_at')
);

-- name: UpdateCheckRun :exec
UPDATE check_runs
SET state = sqlc.arg('state'),
    summary = sqlc.arg('summary'),
    ansible_stdout = sqlc.arg('ansible_stdout'),
    started_at = sqlc.narg('started_at'),
    finished_at = sqlc.narg('finished_at')
WHERE id = sqlc.arg('id');

-- name: GetCheckRunByID :one
SELECT *
FROM check_runs
WHERE id = $1;

-- name: ListCheckRunsByLab :many
SELECT *
FROM check_runs
WHERE lab_instance_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: DeleteCheckRunSteps :exec
DELETE FROM check_run_steps
WHERE check_run_id = $1;

-- name: InsertCheckRunStep :exec
INSERT INTO check_run_steps (
    check_run_id,
    seq,
    task_name,
    status,
    expected,
    actual,
    message
)
VALUES (
    sqlc.arg('check_run_id'),
    sqlc.arg('seq'),
    sqlc.arg('task_name'),
    sqlc.arg('status'),
    sqlc.narg('expected'),
    sqlc.narg('actual'),
    NULLIF(sqlc.arg('message'), '')
);

-- name: ListCheckRunSteps :many
SELECT *
FROM check_run_steps
WHERE check_run_id = $1
ORDER BY seq ASC;

-- name: GetCheckTemplateByID :one
SELECT *
FROM check_templates
WHERE id = $1;
