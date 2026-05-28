-- name: GetAIPersonaByAccount :one
SELECT * FROM ai_personas WHERE account_id = $1 LIMIT 1;

-- name: GetClientProfileByAccount :one
SELECT * FROM client_profiles WHERE account_id = $1 LIMIT 1;

-- name: UpsertAIPersona :one
INSERT INTO ai_personas (account_id, system_prompt, guiones_json)
VALUES ($1, $2, COALESCE($3, '{}'::jsonb))
ON CONFLICT (account_id) DO UPDATE
SET system_prompt = EXCLUDED.system_prompt,
    guiones_json  = EXCLUDED.guiones_json,
    updated_at    = NOW()
RETURNING *;

-- name: UpsertClientProfile :one
INSERT INTO client_profiles (account_id, questionnaire)
VALUES ($1, COALESCE($2, '{}'::jsonb))
ON CONFLICT (account_id) DO UPDATE
SET questionnaire = EXCLUDED.questionnaire,
    updated_at    = NOW()
RETURNING *;

-- name: ListProspectsByAccount :many
SELECT * FROM prospects
WHERE account_id = $1
ORDER BY COALESCE(last_reply_at, created_at) DESC
LIMIT $2;
