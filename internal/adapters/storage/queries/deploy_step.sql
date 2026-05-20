-- name: GetDeployStep :one
SELECT *
FROM lab_deploy_steps
WHERE lab_instance_id = $1
  AND step_name = $2;

-- name: UpsertDeployStep :exec
INSERT INTO lab_deploy_steps (
    lab_instance_id,
    step_name,
    status,
    attempt,
    last_error,
    result,
    started_at,
    finished_at
)
VALUES (
    $1,
    $2,
    $3,
    $4,
    NULLIF($5, ''),
    sqlc.narg('result'),
    sqlc.narg('started_at'),
    sqlc.narg('finished_at')
)
ON CONFLICT (lab_instance_id, step_name) DO UPDATE
SET status = EXCLUDED.status,
    attempt = EXCLUDED.attempt,
    last_error = EXCLUDED.last_error,
    result = EXCLUDED.result,
    started_at = EXCLUDED.started_at,
    finished_at = EXCLUDED.finished_at;

-- name: ListDeployStepsByLab :many
SELECT *
FROM lab_deploy_steps
WHERE lab_instance_id = $1
ORDER BY started_at ASC NULLS LAST, step_name;
