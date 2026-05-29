-- name: ListDueJobSearches :many
-- Enabled job searches never run, or last run longer ago than the given
-- interval. The scheduler discovery phase iterates these.
SELECT * FROM job_searches
WHERE enabled = TRUE
  AND (last_run_at IS NULL OR last_run_at < NOW() - $1::interval)
ORDER BY last_run_at ASC NULLS FIRST
LIMIT $2;

-- name: ListJobSearchesByAccount :many
SELECT * FROM job_searches
WHERE account_id = $1
ORDER BY created_at;

-- name: TouchJobSearch :exec
UPDATE job_searches
SET last_run_at     = NOW(),
    last_seen_count = $2
WHERE id = $1;

-- name: CreateJobSearch :one
INSERT INTO job_searches (account_id, name, keywords, geo_id, search_url, max_results)
VALUES ($1, $2, $3, COALESCE($4, '92000000'), $5, COALESCE($6, 20))
ON CONFLICT (account_id, name) DO UPDATE
SET keywords    = EXCLUDED.keywords,
    geo_id      = EXCLUDED.geo_id,
    search_url  = EXCLUDED.search_url,
    max_results = EXCLUDED.max_results,
    enabled     = TRUE
RETURNING *;

-- name: InsertJobPosting :one
-- Insert a freshly-discovered posting. On conflict (already seen) returns no
-- rows, so the caller treats pgx.ErrNoRows as "skip, already have it".
INSERT INTO job_postings (
    account_id, search_id, linkedin_job_id, title, company, location, url
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (account_id, linkedin_job_id) DO NOTHING
RETURNING *;

-- name: ListUnscoredJobPostings :many
SELECT * FROM job_postings
WHERE account_id = $1
  AND status = 'new'
  AND ai_score IS NULL
ORDER BY first_seen_at
LIMIT $2;

-- name: SetJobPostingScore :exec
-- Records the AI score plus the detail we fetched while scoring (JD, applicants,
-- posted_at). Flips status new -> scored.
UPDATE job_postings
SET ai_score         = $2,
    ai_reasoning     = $3,
    ai_tags          = COALESCE($4, '[]'::jsonb),
    ai_model         = $5,
    raw_jd           = COALESCE($6, raw_jd),
    applicants_count = COALESCE($7, applicants_count),
    posted_at        = COALESCE($8, posted_at),
    status           = 'scored',
    scored_at        = NOW()
WHERE id = $1;

-- name: MarkJobPostingScoreFailed :exec
-- Could not score (detail fetch or LLM error). Park it as 'dismissed' with the
-- reason so it leaves the unscored queue instead of looping forever.
UPDATE job_postings
SET status       = 'dismissed',
    ai_reasoning = $2,
    scored_at    = NOW()
WHERE id = $1;

-- name: ListJobPostingsByAccount :many
-- Report ordering: best score first, then most recently seen.
SELECT * FROM job_postings
WHERE account_id = $1
ORDER BY ai_score DESC NULLS LAST, first_seen_at DESC
LIMIT $2;

-- name: SetJobPostingStatus :exec
UPDATE job_postings
SET status = $2
WHERE id = $1 AND account_id = $3;

-- name: ListEnabledJobSearchAccounts :many
-- Distinct accounts that have at least one enabled job search; the scoring
-- phase iterates these to drain their unscored postings.
SELECT DISTINCT account_id FROM job_searches WHERE enabled = TRUE;

-- name: CountUnscoredJobPostings :one
SELECT COUNT(*)::BIGINT AS count
FROM job_postings
WHERE account_id = $1 AND status = 'new' AND ai_score IS NULL;
