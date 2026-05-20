-- name: UpsertQuotaCache :exec
INSERT INTO quota_cache (id, snapshot, fetched_at)
VALUES (1, $1, now())
ON CONFLICT (id) DO UPDATE
SET snapshot = EXCLUDED.snapshot,
    fetched_at = now();

-- name: GetQuotaCache :one
SELECT snapshot, fetched_at
FROM quota_cache
WHERE id = 1;
