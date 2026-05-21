-- name: ResolveSetting :one
SELECT *
FROM settings
WHERE key = sqlc.arg('key')
  AND (
    (scope = 'per_lab_template' AND scope_id = COALESCE(sqlc.narg('lab_template_id')::text, '__none__'))
    OR (scope = 'per_course' AND scope_id = COALESCE(sqlc.narg('course_id')::text, '__none__'))
    OR (scope = 'global' AND scope_id = '')
  )
ORDER BY CASE scope
    WHEN 'per_lab_template' THEN 1
    WHEN 'per_course' THEN 2
    ELSE 3
END
LIMIT 1;

-- name: ListSettings :many
SELECT *
FROM settings
WHERE (sqlc.narg('scope')::text IS NULL OR scope = sqlc.narg('scope')::text)
  AND (sqlc.narg('scope_id')::text IS NULL OR scope_id = sqlc.narg('scope_id')::text)
ORDER BY scope, scope_id, key;

-- name: UpsertSetting :one
INSERT INTO settings (
    key,
    scope,
    scope_id,
    value,
    updated_by_user_id,
    updated_at
)
VALUES (
    sqlc.arg('key'),
    sqlc.arg('scope'),
    sqlc.arg('scope_id'),
    sqlc.arg('value'),
    sqlc.arg('updated_by_user_id'),
    COALESCE(sqlc.narg('updated_at'), now())
)
ON CONFLICT (key, scope, scope_id) DO UPDATE
SET value = EXCLUDED.value,
    updated_by_user_id = EXCLUDED.updated_by_user_id,
    updated_at = EXCLUDED.updated_at
RETURNING *;
