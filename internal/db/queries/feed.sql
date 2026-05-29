-- name: ListDueFeedSearches :many
SELECT * FROM feed_searches
WHERE enabled = TRUE
  AND (last_run_at IS NULL OR last_run_at < NOW() - $1::interval)
ORDER BY last_run_at ASC NULLS FIRST
LIMIT $2;

-- name: ListFeedSearchesByAccount :many
SELECT * FROM feed_searches
WHERE account_id = $1
ORDER BY created_at;

-- name: ListEnabledFeedSearchAccounts :many
SELECT DISTINCT account_id FROM feed_searches WHERE enabled = TRUE;

-- name: TouchFeedSearch :exec
UPDATE feed_searches
SET last_run_at = NOW(), last_seen_count = $2
WHERE id = $1;

-- name: CreateFeedSearch :one
INSERT INTO feed_searches (account_id, name, keywords, search_url, max_results)
VALUES ($1, $2, $3, $4, COALESCE($5, 20))
ON CONFLICT (account_id, name) DO UPDATE
SET keywords    = EXCLUDED.keywords,
    search_url  = EXCLUDED.search_url,
    max_results = EXCLUDED.max_results,
    enabled     = TRUE
RETURNING *;

-- name: InsertFeedPost :one
-- On conflict (already seen) returns no rows; caller treats pgx.ErrNoRows as skip.
INSERT INTO feed_posts (
    account_id, search_id, linkedin_post_id, post_url, text,
    author_name, author_headline, author_provider_id, author_profile_url,
    reactions_count, comments_count, posted_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (account_id, linkedin_post_id) DO NOTHING
RETURNING *;

-- name: ListUnscoredFeedPosts :many
SELECT * FROM feed_posts
WHERE account_id = $1
  AND status = 'new'
  AND ai_relevant IS NULL
ORDER BY first_seen_at
LIMIT $2;

-- name: SetFeedPostClassification :exec
-- Records the AI classification. status should be 'relevant' or 'irrelevant'.
UPDATE feed_posts
SET ai_relevant  = $2,
    ai_score     = $3,
    ai_reasoning = $4,
    ai_role      = $5,
    ai_company   = $6,
    ai_tags      = COALESCE($7, '[]'::jsonb),
    ai_model     = $8,
    status       = $9,
    scored_at    = NOW()
WHERE id = $1;

-- name: MarkFeedPostClassifyFailed :exec
UPDATE feed_posts
SET status       = 'dismissed',
    ai_reasoning = $2,
    scored_at    = NOW()
WHERE id = $1;

-- name: ListFeedPostsByAccount :many
SELECT * FROM feed_posts
WHERE account_id = $1
ORDER BY ai_score DESC NULLS LAST, first_seen_at DESC
LIMIT $2;

-- name: GetFeedPost :one
SELECT * FROM feed_posts WHERE id = $1 AND account_id = $2 LIMIT 1;

-- name: SetFeedPostStatus :exec
UPDATE feed_posts
SET status = $2
WHERE id = $1 AND account_id = $3;

-- name: SetFeedPostImported :exec
UPDATE feed_posts
SET status = 'imported', imported_prospect_id = $2
WHERE id = $1 AND account_id = $3;
