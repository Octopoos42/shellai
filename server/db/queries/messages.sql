-- name: CreateMessage :one
INSERT INTO messages (session_id, role, content) VALUES ($1, $2, $3) RETURNING *;

-- name: ListMessagesBySession :many
SELECT * FROM messages WHERE session_id = $1 ORDER BY created_at ASC;

-- name: DeleteMessagesBySession :exec
DELETE FROM messages WHERE session_id = $1;
