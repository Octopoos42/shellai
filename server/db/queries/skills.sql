-- name: CreateSkill :one
INSERT INTO skills (api_key_id, name, description, content, is_public)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetSkill :one
SELECT * FROM skills WHERE id = $1;

-- name: ListSkillsByOwner :many
SELECT * FROM skills WHERE api_key_id = $1 ORDER BY updated_at DESC;

-- name: ListPublicSkills :many
SELECT * FROM skills WHERE is_public = true ORDER BY updated_at DESC;

-- name: UpdateSkill :one
UPDATE skills
SET name = $2, description = $3, content = $4, is_public = $5, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteSkill :exec
DELETE FROM skills WHERE id = $1;
