-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys WHERE key_hash = $1 AND revoked_at IS NULL;

-- name: CreateAPIKey :one
INSERT INTO api_keys (key_hash, label) VALUES ($1, $2) RETURNING *;

-- name: ListAPIKeys :many
SELECT * FROM api_keys ORDER BY created_at DESC;

-- name: RevokeAPIKey :one
-- Returns the revoked row, or zero rows if the key was already revoked or does not exist.
UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL RETURNING *;
