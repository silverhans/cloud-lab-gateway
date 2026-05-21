-- name: GetCourseByID :one
SELECT *
FROM courses
WHERE id = $1;

-- name: GetCourseByExternalID :one
SELECT *
FROM courses
WHERE external_id = $1;

-- name: UpsertCourse :one
INSERT INTO courses (
    id,
    external_id,
    name,
    ki_domain_id,
    created_at
)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('external_id'),
    sqlc.arg('name'),
    sqlc.arg('ki_domain_id'),
    COALESCE(sqlc.narg('created_at'), now())
)
ON CONFLICT (external_id) DO UPDATE
SET name = EXCLUDED.name,
    ki_domain_id = EXCLUDED.ki_domain_id
RETURNING id;

-- name: EnrollCourseUser :exec
INSERT INTO enrollments (
    user_id,
    course_id,
    role_in_course,
    created_at
)
VALUES (
    sqlc.arg('user_id'),
    sqlc.arg('course_id'),
    sqlc.arg('role_in_course'),
    COALESCE(sqlc.narg('created_at'), now())
)
ON CONFLICT (user_id, course_id) DO UPDATE
SET role_in_course = EXCLUDED.role_in_course;
