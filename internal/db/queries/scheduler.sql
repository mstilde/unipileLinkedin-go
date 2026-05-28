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
