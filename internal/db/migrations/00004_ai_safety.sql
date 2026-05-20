-- +goose Up
-- +goose StatementBegin

-- =============================================================================
-- AI reply queue (incoming messages enqueued for AI processing with human delay)
-- =============================================================================
CREATE TABLE ai_reply_queue (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prospect_id     UUID REFERENCES prospects(id) ON DELETE CASCADE,
    chat_id         TEXT NOT NULL,
    account_id      TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    incoming_text   TEXT NOT NULL,
    incoming_msg_id TEXT NOT NULL UNIQUE,
    scheduled_for   TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'processing', 'sent', 'failed', 'cancelled')),
    template_kind   TEXT,
    ai_draft        TEXT,
    model_used      TEXT,
    tokens_in       INT,
    tokens_out      INT,
    cost_usd        NUMERIC(10,6),
    error_detail    TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ai_queue_sched ON ai_reply_queue(scheduled_for) WHERE status = 'pending';
CREATE INDEX idx_ai_queue_chat  ON ai_reply_queue(chat_id);

-- =============================================================================
-- Human review queue (low-confidence drafts pending human approval)
-- =============================================================================
CREATE TABLE human_review_queue (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prospect_id   UUID REFERENCES prospects(id) ON DELETE CASCADE,
    chat_id       TEXT NOT NULL,
    incoming_text TEXT NOT NULL,
    ai_draft      TEXT,
    intent        TEXT,
    confidence    NUMERIC(4,3),
    reason        TEXT,
    status        TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'approved', 'rejected', 'edited')),
    reviewed_by   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at   TIMESTAMPTZ,
    final_text    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_human_review_status ON human_review_queue(status, created_at);

-- =============================================================================
-- AI interactions log (per-call cost accounting for monthly cap enforcement)
-- =============================================================================
CREATE TABLE ai_interactions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prospect_id UUID,
    account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    stage       TEXT NOT NULL CHECK (stage IN ('enrich', 'classify', 'reply', 'adapt', 'preamble', 'deep_enrich')),
    model       TEXT NOT NULL,
    tokens_in   INT NOT NULL DEFAULT 0,
    tokens_out  INT NOT NULL DEFAULT 0,
    cost_usd    NUMERIC(10,6) NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ai_interactions_account_month ON ai_interactions(account_id, created_at DESC);

-- =============================================================================
-- AI safety log (audit trail for blocked / paused AI events)
-- =============================================================================
CREATE TABLE ai_safety_log (
    id          BIGSERIAL PRIMARY KEY,
    account_id  TEXT,
    prospect_id UUID,
    job_id      UUID,
    reason      TEXT NOT NULL
                  CHECK (reason IN ('kill_switch', 'pii_blocked', 'quiet_hours', 'cap_account', 'cap_campaign', 'opt_out')),
    detail      JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ai_safety_log_account_time ON ai_safety_log(account_id, created_at DESC);

-- =============================================================================
-- Enrichment cache (deep-enrich outputs keyed by profile fingerprint)
-- =============================================================================
CREATE TABLE enrichment_cache (
    fingerprint TEXT PRIMARY KEY,
    enrichment  JSONB NOT NULL,
    model       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL,
    hit_count   INT NOT NULL DEFAULT 0
);

CREATE INDEX idx_enrichment_cache_expires ON enrichment_cache(expires_at);

-- =============================================================================
-- External enrichment cache (DENUE, SAT, DOF, Wikipedia, etc.)
-- =============================================================================
CREATE TABLE enrichment_external_cache (
    id         BIGSERIAL PRIMARY KEY,
    source     TEXT NOT NULL,
    lookup_key TEXT NOT NULL,
    payload    JSONB NOT NULL,
    found      BOOLEAN NOT NULL DEFAULT TRUE,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    hit_count  INT NOT NULL DEFAULT 0,
    UNIQUE (source, lookup_key)
);

CREATE INDEX idx_enrichment_external_expires    ON enrichment_external_cache(expires_at);
CREATE INDEX idx_enrichment_external_source_key ON enrichment_external_cache(source, lookup_key);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS enrichment_external_cache;
DROP TABLE IF EXISTS enrichment_cache;
DROP TABLE IF EXISTS ai_safety_log;
DROP TABLE IF EXISTS ai_interactions;
DROP TABLE IF EXISTS human_review_queue;
DROP TABLE IF EXISTS ai_reply_queue;
-- +goose StatementEnd
