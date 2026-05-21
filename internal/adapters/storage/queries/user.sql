-- name: GetUserByID :one
SELECT id, display_name, email, role, password_hash, created_at
FROM users
WHERE id = $1;

-- name: GetUserByLTI :one
SELECT u.id, u.display_name, u.email, u.role, u.password_hash, u.created_at
FROM users u
JOIN lti_identities li ON li.user_id = u.id
WHERE li.iss = $1
  AND li.sub = $2;

-- name: InsertUserFromLaunch :one
INSERT INTO users (
    id,
    display_name,
    email,
    role,
    created_at
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('display_name'),
    sqlc.narg('email'),
    sqlc.arg('role'),
    COALESCE(sqlc.narg('created_at'), now())
)
RETURNING id, display_name, email, role, password_hash, created_at;

-- name: UpdateUserFromLaunch :one
UPDATE users
SET display_name = sqlc.arg('display_name'),
    email = sqlc.narg('email'),
    role = sqlc.arg('role')
WHERE id = sqlc.arg('id')
RETURNING id, display_name, email, role, password_hash, created_at;

-- name: UpsertLTIIdentity :exec
INSERT INTO lti_identities (
    user_id,
    iss,
    sub,
    created_at
)
VALUES (
    sqlc.arg('user_id'),
    sqlc.arg('iss'),
    sqlc.arg('sub'),
    COALESCE(sqlc.narg('created_at'), now())
)
ON CONFLICT (iss, sub) DO UPDATE
SET user_id = EXCLUDED.user_id;

-- name: ListCourseRolesByUser :many
SELECT course_id, role_in_course
FROM enrollments
WHERE user_id = $1;
