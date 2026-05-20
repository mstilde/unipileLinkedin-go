-- +goose Up
-- +goose StatementBegin

-- =============================================================================
-- LinkedIn lookup cache (industry / location / company / job title IDs)
-- Keyed by "type:locale:keywords-normalized" — IDs differ by account locale.
-- =============================================================================
CREATE TABLE linkedin_lookup_cache (
    cache_key       TEXT PRIMARY KEY,
    account_locale  TEXT NOT NULL,
    type            TEXT NOT NULL,
    keywords        TEXT,
    items_json      JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_lookup_expires ON linkedin_lookup_cache(expires_at);
CREATE INDEX idx_lookup_type    ON linkedin_lookup_cache(type, account_locale);

-- =============================================================================
-- LinkedIn search jobs (async-paginated SN/Recruiter/relations/post_engagers/hybrid)
-- =============================================================================
CREATE TABLE linkedin_search_jobs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id        TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    user_id           BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source            TEXT NOT NULL DEFAULT 'search'
                        CHECK (source IN ('search', 'relations', 'post_engagers', 'hybrid')),
    job_type          TEXT NOT NULL DEFAULT 'search',
    status            TEXT NOT NULL DEFAULT 'queued'
                        CHECK (status IN ('queued', 'running', 'partial', 'completed', 'failed', 'cancelled')),
    payload           JSONB NOT NULL,
    schema_version    SMALLINT NOT NULL DEFAULT 1,
    cursor_state      JSONB NOT NULL DEFAULT '{}',
    results_json      JSONB NOT NULL DEFAULT '[]',
    results_count     INT NOT NULL DEFAULT 0,
    total_estimated   INT,
    error_code        TEXT,
    error_detail      TEXT,
    anchor_account_id TEXT,
    target_distance   INT,
    top_n             INT,
    max_final_results INT,
    hybrid_stats      JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ,
    cancelled_at      TIMESTAMPTZ,
    last_progress_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_jobs_account_status ON linkedin_search_jobs(account_id, status, created_at DESC);
CREATE INDEX idx_jobs_user           ON linkedin_search_jobs(user_id, created_at DESC);
CREATE INDEX idx_jobs_running        ON linkedin_search_jobs(status, last_progress_at) WHERE status IN ('queued', 'running');
CREATE INDEX idx_jobs_type           ON linkedin_search_jobs(job_type);

-- =============================================================================
-- Saved searches (with optional auto-sync)
-- =============================================================================
CREATE TABLE linkedin_saved_searches (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id        TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    user_id           BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    source            TEXT NOT NULL DEFAULT 'search'
                        CHECK (source IN ('search', 'relations', 'post_engagers', 'hybrid')),
    payload           JSONB NOT NULL,
    schema_version    SMALLINT NOT NULL DEFAULT 1,
    auto_sync         BOOLEAN NOT NULL DEFAULT FALSE,
    last_run_job_id   UUID REFERENCES linkedin_search_jobs(id) ON DELETE SET NULL,
    last_run_count    INT,
    last_seen_ids     JSONB NOT NULL DEFAULT '[]',
    last_run_at       TIMESTAMPTZ,
    anchor_account_id TEXT,
    target_distance   INT,
    top_n             INT,
    deleted_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_saved_account  ON linkedin_saved_searches(account_id, deleted_at, created_at DESC);
CREATE INDEX idx_saved_autosync ON linkedin_saved_searches(auto_sync, last_run_at) WHERE auto_sync = TRUE AND deleted_at IS NULL;

-- =============================================================================
-- Per-account daily quota (defensive rate limit independent of LinkedIn's)
-- =============================================================================
CREATE TABLE linkedin_account_quota (
    account_id        TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    date              DATE NOT NULL,
    searches_used     INT NOT NULL DEFAULT 0,
    profiles_fetched  INT NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, date)
);

-- =============================================================================
-- Import runs (idempotency for "import this job into this campaign")
-- =============================================================================
CREATE TABLE linkedin_import_runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id          UUID NOT NULL REFERENCES linkedin_search_jobs(id) ON DELETE CASCADE,
    campaign_id     UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    imported_count  INT NOT NULL DEFAULT 0,
    skipped_count   INT NOT NULL DEFAULT 0,
    blocked_count   INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (job_id, campaign_id)
);

-- =============================================================================
-- SaaS owners (users authorized to run hybrid search)
-- =============================================================================
CREATE TABLE saas_owners (
    user_id    BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes      TEXT
);

-- =============================================================================
-- Hybrid search audit (every hybrid run logged for abuse / anomaly detection)
-- =============================================================================
CREATE TABLE hybrid_search_audit (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id            UUID NOT NULL,
    owner_user_id     BIGINT NOT NULL,
    anchor_account_id TEXT NOT NULL,
    owner_account_id  TEXT NOT NULL,
    target_distance   INT,
    top_n             INT,
    payload_summary   JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_hybrid_audit_owner  ON hybrid_search_audit(owner_user_id, created_at DESC);
CREATE INDEX idx_hybrid_audit_anchor ON hybrid_search_audit(anchor_account_id, created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS hybrid_search_audit;
DROP TABLE IF EXISTS saas_owners;
DROP TABLE IF EXISTS linkedin_import_runs;
DROP TABLE IF EXISTS linkedin_account_quota;
DROP TABLE IF EXISTS linkedin_saved_searches;
DROP TABLE IF EXISTS linkedin_search_jobs;
DROP TABLE IF EXISTS linkedin_lookup_cache;
-- +goose StatementEnd
