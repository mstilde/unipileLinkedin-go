-- name: GetAccountByID :one
SELECT *
FROM accounts
WHERE id = $1
LIMIT 1;

-- name: ListAccounts :many
SELECT *
FROM accounts
ORDER BY created_at DESC;

-- name: ListAccountsByOwner :many
SELECT a.*
FROM accounts a
LEFT JOIN account_assignments aa ON aa.account_id = a.id
WHERE a.owner_user_id = $1
   OR aa.user_id = $1
GROUP BY a.id
ORDER BY a.created_at DESC;

-- name: IsAccountOwned :one
SELECT EXISTS (
    SELECT 1 FROM accounts WHERE id = $1 AND owner_user_id = $2
    UNION
    SELECT 1 FROM account_assignments WHERE account_id = $1 AND user_id = $2
) AS owned;

-- name: UpsertAccount :one
INSERT INTO accounts (id, account_id, provider, name, owner_user_id, status)
VALUES ($1, $1, $2, $3, $4, COALESCE($5, 'OK'))
ON CONFLICT (id) DO UPDATE
SET provider       = EXCLUDED.provider,
    name           = COALESCE(EXCLUDED.name, accounts.name),
    owner_user_id  = COALESCE(EXCLUDED.owner_user_id, accounts.owner_user_id),
    status         = EXCLUDED.status,
    last_status_at = NOW(),
    updated_at     = NOW()
RETURNING *;

-- name: AssignAccountToUser :exec
INSERT INTO account_assignments (user_id, account_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: UnassignAccountFromUser :exec
DELETE FROM account_assignments
WHERE user_id = $1 AND account_id = $2;
