-- name: GetUserByUsername :one
SELECT id, username, password_hash, display_name, role, is_active, created_at, updated_at
FROM users
WHERE username = $1
  AND is_active = TRUE
LIMIT 1;

-- name: GetUserByID :one
SELECT id, username, password_hash, display_name, role, is_active, created_at, updated_at
FROM users
WHERE id = $1
LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (username, password_hash, display_name, role)
VALUES ($1, $2, $3, $4)
RETURNING id, username, display_name, role, is_active, created_at, updated_at;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $2, updated_at = NOW()
WHERE id = $1;

-- name: DeactivateUser :exec
UPDATE users
SET is_active = FALSE, updated_at = NOW()
WHERE id = $1;

-- name: ListUsers :many
SELECT id, username, display_name, role, is_active, created_at, updated_at
FROM users
ORDER BY created_at DESC;
