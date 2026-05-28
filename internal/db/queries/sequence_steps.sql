-- name: ListStepsByCampaign :many
SELECT *
FROM sequence_steps
WHERE campaign_id = $1
ORDER BY step_index;

-- name: GetStep :one
SELECT *
FROM sequence_steps
WHERE id = $1
LIMIT 1;

-- name: CreateStep :one
INSERT INTO sequence_steps (
    campaign_id, step_index, step_type, delay_hours,
    template, ai_personalize, note_max_chars, stage_label, config_json
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateStep :one
UPDATE sequence_steps SET
    step_type      = COALESCE($2, step_type),
    delay_hours    = COALESCE($3, delay_hours),
    template       = COALESCE($4, template),
    ai_personalize = COALESCE($5, ai_personalize),
    note_max_chars = COALESCE($6, note_max_chars),
    stage_label    = COALESCE($7, stage_label),
    config_json    = COALESCE($8, config_json)
WHERE id = $1
RETURNING *;

-- name: DeleteStep :exec
DELETE FROM sequence_steps WHERE id = $1;

-- name: DeleteAllStepsForCampaign :exec
DELETE FROM sequence_steps WHERE campaign_id = $1;

-- name: ListTemplatesByCampaign :many
SELECT *
FROM campaign_templates
WHERE campaign_id = $1
ORDER BY template_kind;

-- name: UpsertTemplate :one
INSERT INTO campaign_templates (campaign_id, template_kind, template_text, ai_adapt)
VALUES ($1, $2, $3, $4)
ON CONFLICT (campaign_id, template_kind) DO UPDATE
SET template_text = EXCLUDED.template_text,
    ai_adapt      = EXCLUDED.ai_adapt,
    updated_at    = NOW()
RETURNING *;

-- name: DeleteTemplate :exec
DELETE FROM campaign_templates
WHERE campaign_id = $1 AND template_kind = $2;
