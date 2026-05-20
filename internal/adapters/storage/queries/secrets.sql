-- name: InsertEncryptedSecret :one
INSERT INTO encrypted_secrets (
    id,
    kind,
    ref_id,
    dek_ciphertext,
    dek_nonce,
    payload_ciphertext,
    payload_nonce,
    aad,
    kek_version,
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
    $8,
    $9,
    COALESCE(sqlc.narg('created_at'), now())
)
RETURNING *;

-- name: GetEncryptedSecretByID :one
SELECT *
FROM encrypted_secrets
WHERE id = $1;

-- name: DeleteEncryptedSecret :exec
DELETE FROM encrypted_secrets
WHERE id = $1;
