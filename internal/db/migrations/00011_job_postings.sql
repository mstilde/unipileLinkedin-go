-- +goose Up
-- +goose StatementBegin

-- =============================================================================
-- Front 2B — Job postings ranker (Buscalaburos 3000)
--
-- Two tables, deliberately decoupled from the linkedin_saved_searches /
-- linkedin_search_jobs machinery in 00007 (that one is the people-search SaaS
-- engine, with hybrid/anchor/SaaS-owner columns we don't want here). This is a
-- small single-user job-hunt feature: a handful of keyword searches feeding a
-- ranked postings table.
-- =============================================================================

-- Saved job searches. Each row is a keyword query we poll on a cadence. geo_id
-- defaults to worldwide (92000000) so LinkedIn doesn't silently scope results
-- to the account's own location.
CREATE TABLE job_searches (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    keywords        TEXT NOT NULL,
    geo_id          TEXT NOT NULL DEFAULT '92000000',
    -- Optional full LinkedIn jobs-search URL; when set it overrides keywords
    -- (url-mode search, lets us pin filters LinkedIn doesn't expose by keyword).
    search_url      TEXT,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    max_results     INT NOT NULL DEFAULT 20,
    last_run_at     TIMESTAMPTZ,
    last_seen_count INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (account_id, name)
);

CREATE INDEX idx_job_searches_due ON job_searches(enabled, last_run_at) WHERE enabled = TRUE;

-- Discovered job postings, one per (account, linkedin job id). ai_* columns are
-- NULL until the ranker scores them; status drives the report UI.
CREATE TABLE job_postings (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id       TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    search_id        UUID REFERENCES job_searches(id) ON DELETE SET NULL,
    linkedin_job_id  TEXT NOT NULL,
    title            TEXT NOT NULL DEFAULT '',
    company          TEXT NOT NULL DEFAULT '',
    location         TEXT,
    url              TEXT,
    applicants_count INT,
    raw_jd           TEXT,                 -- full job description (filled on detail fetch)
    posted_at        TIMESTAMPTZ,
    ai_score         INT,                  -- 0-100; NULL = not scored yet
    ai_reasoning     TEXT,
    ai_tags          JSONB NOT NULL DEFAULT '[]',
    ai_model         TEXT,
    status           TEXT NOT NULL DEFAULT 'new'
                       CHECK (status IN ('new', 'scored', 'applied', 'dismissed', 'saved')),
    first_seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    scored_at        TIMESTAMPTZ,
    UNIQUE (account_id, linkedin_job_id)
);

-- Scoring queue: postings that still need an AI score.
CREATE INDEX idx_job_postings_unscored ON job_postings(account_id, first_seen_at)
    WHERE status = 'new' AND ai_score IS NULL;
-- Report ordering: best scored first.
CREATE INDEX idx_job_postings_report ON job_postings(account_id, ai_score DESC NULLS LAST, first_seen_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS job_postings;
DROP TABLE IF EXISTS job_searches;
-- +goose StatementEnd
