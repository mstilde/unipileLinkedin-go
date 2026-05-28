-- name: ListProspectsByCampaign :many
SELECT *
FROM prospects
WHERE campaign_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountProspectsByCampaign :one
SELECT COUNT(*)::BIGINT AS total FROM prospects WHERE campaign_id = $1;

-- name: GetProspect :one
SELECT *
FROM prospects
WHERE id = $1
LIMIT 1;

-- name: CreateProspect :one
INSERT INTO prospects (
    campaign_id, account_id, profile_url, public_identifier, full_name, first_name,
    headline, company, status, conversation_stage, tags, variables
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE($9, 'queued'), COALESCE($10, 'awaiting_invite_acceptance'), $11, COALESCE($12, '{}'::jsonb))
ON CONFLICT (account_id, profile_url) DO UPDATE
SET full_name  = COALESCE(EXCLUDED.full_name, prospects.full_name),
    first_name = COALESCE(EXCLUDED.first_name, prospects.first_name),
    headline   = COALESCE(EXCLUDED.headline, prospects.headline),
    company    = COALESCE(EXCLUDED.company, prospects.company),
    tags       = EXCLUDED.tags,
    variables  = EXCLUDED.variables,
    updated_at = NOW()
RETURNING *;

-- name: SetProspectStatus :one
UPDATE prospects
SET status = $2, error_detail = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteProspect :exec
DELETE FROM prospects WHERE id = $1;

-- name: ProspectFunnelByCampaign :many
SELECT status::TEXT AS status, COUNT(*)::BIGINT AS count
FROM prospects
WHERE campaign_id = $1
GROUP BY status;

-- name: ProspectStageDistribution :many
SELECT conversation_stage::TEXT AS stage, COUNT(*)::BIGINT AS count
FROM prospects
WHERE campaign_id = $1
GROUP BY conversation_stage;
