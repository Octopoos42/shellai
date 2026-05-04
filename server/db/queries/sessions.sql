-- name: CreateSession :one
INSERT INTO sessions (api_key_id, title) VALUES ($1, $2) RETURNING *;

-- name: GetSession :one
SELECT * FROM sessions WHERE id = $1;

-- name: ListSessions :many
SELECT * FROM sessions WHERE api_key_id = $1 ORDER BY updated_at DESC;

-- name: UpdateSessionTitle :one
UPDATE sessions SET title = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: TouchSession :exec
UPDATE sessions SET updated_at = NOW() WHERE id = $1;
