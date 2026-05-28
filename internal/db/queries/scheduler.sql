-- name: ListPendingProspectSteps :many
SELECT ps.id, ps.prospect_id, ps.step_id, ps.step_type, ps.scheduled_at,
       ps.sent_at, ps.status, ps.message_sent, ps.error_detail,
       ps.retry_count, ps.max_retries, ps.ab_variant_id, ps.branch,
       ps.last_check_at, ps.processing_started_at, ps.created_at,
       p.account_id::TEXT       AS account_id,
       p.campaign_id::UUID      AS campaign_id,
       p.profile_url::TEXT      AS profile_url,
       p.linkedin_provider_id   AS linkedin_provider_id
FROM prospect_steps ps
JOIN prospects p ON p.id = ps.prospect_id
WHERE ps.status = 'pending'
  AND ps.scheduled_at <= NOW()
ORDER BY ps.scheduled_at
LIMIT $1;

-- name: LeaseProspectStep :one
UPDATE prospect_steps
SET status = 'processing',
    processing_started_at = NOW(),
    last_check_at = NOW()
WHERE id = $1
  AND status = 'pending'
RETURNING *;

-- name: MarkProspectStepSent :exec
UPDATE prospect_steps
SET status = 'sent',
    sent_at = NOW(),
    message_sent = $2
WHERE id = $1;

-- name: MarkProspectStepFailed :exec
UPDATE prospect_steps
SET status = 'failed',
    error_detail = $2,
    retry_count = retry_count + 1
WHERE id = $1;

-- name: ReleaseStaleLeases :exec
UPDATE prospect_steps
SET status = 'pending',
    processing_started_at = NULL
WHERE status = 'processing'
  AND processing_started_at < NOW() - $1::interval;

-- name: ListPendingAIReplies :many
SELECT * FROM ai_reply_queue
WHERE status = 'pending'
  AND scheduled_for <= NOW()
ORDER BY scheduled_for
LIMIT $1;

-- name: MarkAIReplyDone :exec
UPDATE ai_reply_queue
SET status = 'sent',
    ai_draft = $2
WHERE id = $1;

-- name: MarkAIReplyFailed :exec
UPDATE ai_reply_queue
SET status = 'failed',
    error_detail = $2
WHERE id = $1;

-- name: ListDueFollowUpTasks :many
SELECT * FROM follow_up_tasks
WHERE status = 'pending'
  AND scheduled_for <= NOW()
ORDER BY scheduled_for
LIMIT $1;

-- name: MarkFollowUpTaskSent :exec
UPDATE follow_up_tasks
SET status = 'sent',
    sent_at = NOW(),
    message_sent = $2
WHERE id = $1;

-- name: MarkFollowUpTaskCancelled :exec
UPDATE follow_up_tasks
SET status = 'cancelled',
    cancelled_at = NOW(),
    cancel_reason = $2
WHERE id = $1;

-- name: GetNextSequenceStep :one
-- Given the just-dispatched step (by sequence_step id), find the next one in the
-- same campaign (step_index + 1). Returns no rows when there is no next step.
SELECT next.*
FROM sequence_steps cur
JOIN sequence_steps next
  ON next.campaign_id = cur.campaign_id
 AND next.step_index = cur.step_index + 1
WHERE cur.id = $1
LIMIT 1;

-- name: CreateProspectStep :one
-- Insert a pending prospect_step. scheduled_at should already include any delay.
INSERT INTO prospect_steps (
    prospect_id, step_id, step_type, scheduled_at, status
)
VALUES ($1, $2, $3, $4, 'pending')
RETURNING *;

-- name: SetProspectInvited :exec
-- Bookkeeping after a successful invite: set status, invited_at, chat_id, and
-- the LinkedIn provider_id if we discovered it from the response.
UPDATE prospects
SET status               = 'invited',
    invited_at           = COALESCE(invited_at, NOW()),
    linkedin_provider_id = COALESCE($2, linkedin_provider_id),
    chat_id              = COALESCE($3, chat_id),
    updated_at           = NOW()
WHERE id = $1;

-- name: SetProspectChatID :exec
-- After StartNewChat we learn the chat_id for the prospect.
UPDATE prospects
SET chat_id    = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: CountInvitesSentToday :one
-- Defensive in-app daily cap. Counts prospect_steps marked 'sent' today for the
-- account, filtered to invite step types.
SELECT COUNT(*)::BIGINT AS count
FROM prospect_steps ps
JOIN prospects p ON p.id = ps.prospect_id
WHERE p.account_id = $1
  AND ps.step_type = 'invite'
  AND ps.status = 'sent'
  AND ps.sent_at >= NOW() - INTERVAL '24 hours';
