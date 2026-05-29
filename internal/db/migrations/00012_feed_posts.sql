-- +goose Up
-- +goose StatementBegin

-- =============================================================================
-- Front 4 — Post-feed scanner (Buscalaburos 3000)
--
-- Poll LinkedIn for "hiring / contratando / vacante" posts, AI-classify them
-- for relevance to the job search, and surface the authors as candidate
-- prospects (closing the loop back to front 1's recruiter outreach).
--
-- Same shape as front 2B (job_searches / job_postings): a handful of keyword
-- searches feeding a classified posts table. Decoupled from the SaaS-oriented
-- linkedin_* tables in 00007 on purpose.
-- =============================================================================

CREATE TABLE feed_searches (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    keywords        TEXT NOT NULL,
    -- Optional full LinkedIn content-search URL; overrides keywords when set.
    search_url      TEXT,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    max_results     INT NOT NULL DEFAULT 20,
    last_run_at     TIMESTAMPTZ,
    last_seen_count INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (account_id, name)
);

CREATE INDEX idx_feed_searches_due ON feed_searches(enabled, last_run_at) WHERE enabled = TRUE;

-- Discovered posts, one per (account, linkedin post id). ai_* columns are NULL
-- until classified. author_provider_id + author_profile_url let us import the
-- author as a prospect for front 1.
CREATE TABLE feed_posts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id          TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    search_id           UUID REFERENCES feed_searches(id) ON DELETE SET NULL,
    linkedin_post_id    TEXT NOT NULL,
    post_url            TEXT,
    text                TEXT NOT NULL DEFAULT '',
    author_name         TEXT,
    author_headline     TEXT,
    author_provider_id  TEXT,
    author_profile_url  TEXT,
    reactions_count     INT,
    comments_count      INT,
    posted_at           TIMESTAMPTZ,
    ai_relevant         BOOLEAN,            -- NULL = not classified yet
    ai_score            INT,                -- 0-100 relevance
    ai_reasoning        TEXT,
    ai_role             TEXT,               -- extracted role being hired for, if any
    ai_company          TEXT,
    ai_tags             JSONB NOT NULL DEFAULT '[]',
    ai_model            TEXT,
    status              TEXT NOT NULL DEFAULT 'new'
                          CHECK (status IN ('new', 'relevant', 'irrelevant', 'imported', 'dismissed')),
    imported_prospect_id UUID REFERENCES prospects(id) ON DELETE SET NULL,
    first_seen_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    scored_at           TIMESTAMPTZ,
    UNIQUE (account_id, linkedin_post_id)
);

CREATE INDEX idx_feed_posts_unscored ON feed_posts(account_id, first_seen_at)
    WHERE status = 'new' AND ai_relevant IS NULL;
CREATE INDEX idx_feed_posts_report ON feed_posts(account_id, ai_score DESC NULLS LAST, first_seen_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS feed_posts;
DROP TABLE IF EXISTS feed_searches;
-- +goose StatementEnd
