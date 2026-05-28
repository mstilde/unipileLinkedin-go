-- name: GetCampaign :one
SELECT *
FROM campaigns
WHERE id = $1
LIMIT 1;

-- name: GetCampaignForAccount :one
SELECT *
FROM campaigns
WHERE id = $1 AND account_id = $2
LIMIT 1;

-- name: ListCampaignsByAccount :many
SELECT *
FROM campaigns
WHERE account_id = $1
ORDER BY created_at DESC;

-- name: ListActiveCampaigns :many
SELECT *
FROM campaigns
WHERE status = 'active'
ORDER BY account_id, created_at;

-- name: CreateCampaign :one
INSERT INTO campaigns (
    account_id, name, status, daily_invite_limit, tz,
    auto_reply_enabled, ai_monthly_cap_usd, working_hours_start, working_hours_end,
    skip_weekends, is_followup
)
VALUES ($1, $2, 'draft', $3, $4, $5, COALESCE($6, 5.00), $7, $8, $9, $10)
RETURNING *;

-- name: UpdateCampaign :one
UPDATE campaigns SET
    name                       = COALESCE($2, name),
    daily_invite_limit         = COALESCE($3, daily_invite_limit),
    tz                         = COALESCE($4, tz),
    auto_reply_enabled         = COALESCE($5, auto_reply_enabled),
    ai_monthly_cap_usd         = COALESCE($6, ai_monthly_cap_usd),
    working_hours_start        = COALESCE($7, working_hours_start),
    working_hours_end          = COALESCE($8, working_hours_end),
    lunch_break_start          = COALESCE($9, lunch_break_start),
    lunch_break_end            = COALESCE($10, lunch_break_end),
    skip_weekends              = COALESCE($11, skip_weekends),
    skip_holidays              = COALESCE($12, skip_holidays),
    ramp_up_enabled            = COALESCE($13, ramp_up_enabled),
    auto_withdraw_after_days   = COALESCE($14, auto_withdraw_after_days),
    auto_resume_on_human_reply = COALESCE($15, auto_resume_on_human_reply),
    is_followup                = COALESCE($16, is_followup),
    strict_template_validation = COALESCE($17, strict_template_validation),
    auto_prewarm_visit         = COALESCE($18, auto_prewarm_visit),
    auto_prewarm_delay_minutes = COALESCE($19, auto_prewarm_delay_minutes),
    simulate_human_typing      = COALESCE($20, simulate_human_typing),
    updated_at                 = NOW()
WHERE id = $1
RETURNING *;

-- name: SetCampaignStatus :one
UPDATE campaigns
SET status = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteCampaign :exec
DELETE FROM campaigns WHERE id = $1;

-- name: DuplicateCampaign :one
INSERT INTO campaigns (
    account_id, name, status, daily_invite_limit, tz,
    auto_reply_enabled, ai_monthly_cap_usd,
    working_hours_start, working_hours_end, lunch_break_start, lunch_break_end,
    skip_weekends, skip_holidays, ramp_up_enabled,
    auto_withdraw_after_days, auto_resume_on_human_reply,
    is_followup, strict_template_validation, auto_prewarm_visit,
    auto_prewarm_delay_minutes, simulate_human_typing
)
SELECT
    account_id, $2 AS name, 'draft' AS status, daily_invite_limit, tz,
    auto_reply_enabled, ai_monthly_cap_usd,
    working_hours_start, working_hours_end, lunch_break_start, lunch_break_end,
    skip_weekends, skip_holidays, ramp_up_enabled,
    auto_withdraw_after_days, auto_resume_on_human_reply,
    is_followup, strict_template_validation, auto_prewarm_visit,
    auto_prewarm_delay_minutes, simulate_human_typing
FROM campaigns c
WHERE c.id = $1
RETURNING *;
