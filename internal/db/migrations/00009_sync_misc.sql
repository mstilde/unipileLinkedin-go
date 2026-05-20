-- +goose Up
-- +goose StatementBegin

-- =============================================================================
-- Sync status (delta-sync state per account)
-- =============================================================================
CREATE TABLE sync_status (
    account_id     TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    total_chats    INT NOT NULL DEFAULT 0,
    synced_chats   INT NOT NULL DEFAULT 0,
    last_cursor    TEXT,
    last_sync_at   TIMESTAMPTZ,
    sync_status    TEXT NOT NULL DEFAULT 'pending',
    error_message  TEXT,
    retry_count    INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================================================
-- System health events (Unipile API health, latency, errors)
-- =============================================================================
CREATE TABLE system_health_events (
    id           BIGSERIAL PRIMARY KEY,
    event_type   TEXT NOT NULL,
    account_id   TEXT,
    status       TEXT NOT NULL CHECK (status IN ('ok', 'fail', 'degraded')),
    latency_ms   INT,
    error_type   TEXT,
    error_detail TEXT,
    metadata     JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_health_events_type    ON system_health_events(event_type, created_at DESC);
CREATE INDEX idx_health_events_account ON system_health_events(account_id, created_at DESC);

-- =============================================================================
-- Scheduler heartbeats (one row per scheduler component)
-- =============================================================================
CREATE TABLE scheduler_health (
    component    TEXT PRIMARY KEY,
    last_tick_at TIMESTAMPTZ NOT NULL,
    metadata     JSONB
);

-- =============================================================================
-- Webhook log (audit trail of incoming webhooks)
-- =============================================================================
CREATE TABLE webhook_log (
    id            BIGSERIAL PRIMARY KEY,
    provider      TEXT NOT NULL,
    event_type    TEXT NOT NULL,
    event_id      TEXT,
    payload       JSONB NOT NULL,
    received_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at  TIMESTAMPTZ,
    process_ok    BOOLEAN,
    process_error TEXT,
    needs_review  BOOLEAN NOT NULL DEFAULT FALSE,
    review_reason TEXT
);

CREATE INDEX idx_webhook_log_received    ON webhook_log(received_at DESC);
CREATE INDEX idx_webhook_log_unprocessed ON webhook_log(received_at) WHERE processed_at IS NULL;
CREATE INDEX idx_webhook_log_review      ON webhook_log(received_at DESC) WHERE needs_review = TRUE;

-- =============================================================================
-- Webhook dedup (idempotency for (provider, event_type, event_id))
-- =============================================================================
CREATE TABLE webhook_dedup (
    provider   TEXT NOT NULL,
    event_type TEXT NOT NULL,
    event_id   TEXT NOT NULL,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (provider, event_type, event_id)
);

-- =============================================================================
-- System state (global key/value store: maintenance flags, feature flags)
-- =============================================================================
CREATE TABLE system_state (
    key        TEXT PRIMARY KEY,
    value      JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================================================
-- Daily metrics snapshots (frozen daily counters per (date, user, account))
-- =============================================================================
CREATE TABLE daily_metrics_snapshots (
    metric_date         DATE NOT NULL,
    user_id             BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id          TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    chats_totales       INT NOT NULL DEFAULT 0,
    no_iniciado         INT NOT NULL DEFAULT 0,
    m1                  INT NOT NULL DEFAULT 0,
    propuesta           INT NOT NULL DEFAULT 0,
    aceptada            INT NOT NULL DEFAULT 0,
    numero_conseguido   INT NOT NULL DEFAULT 0,
    eliminado           INT NOT NULL DEFAULT 0,
    no_leidos           INT NOT NULL DEFAULT 0,
    is_finalized        BOOLEAN NOT NULL DEFAULT FALSE,
    finalized_at        TIMESTAMPTZ,
    last_calculated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (metric_date, user_id, account_id)
);

CREATE INDEX idx_daily_metrics_user    ON daily_metrics_snapshots(user_id, metric_date DESC);
CREATE INDEX idx_daily_metrics_account ON daily_metrics_snapshots(account_id, metric_date DESC);

-- =============================================================================
-- Bot feedback (supervised learning signal for AI persona / templates)
-- =============================================================================
CREATE TABLE bot_feedback (
    id                BIGSERIAL PRIMARY KEY,
    user_id           BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_id           TEXT NOT NULL,
    account_id        TEXT,
    etapa_fase        TEXT,
    estado_actual     TEXT,
    estado_sugerido   TEXT,
    razon_estado      TEXT,
    accion_sugerida   TEXT,
    tipo_sugerido     TEXT,
    template_kind     TEXT,
    draft_message     TEXT,
    final_message     TEXT,
    feedback_type     TEXT NOT NULL,
    feedback_reason   TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bot_feedback_chat_created ON bot_feedback(chat_id, created_at DESC);
CREATE INDEX idx_bot_feedback_user_created ON bot_feedback(user_id, created_at DESC);

-- =============================================================================
-- Universal messages (shared template library, tree-structured)
-- =============================================================================
CREATE TABLE universal_messages (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type        TEXT,
    title       TEXT NOT NULL,
    content     TEXT,
    sort_order  INT NOT NULL DEFAULT 0,
    parent_id   UUID REFERENCES universal_messages(id) ON DELETE CASCADE,
    media_url   TEXT,
    media_type  TEXT,
    account_id  TEXT REFERENCES accounts(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_universal_messages_parent  ON universal_messages(parent_id, sort_order);
CREATE INDEX idx_universal_messages_account ON universal_messages(account_id);
CREATE INDEX idx_universal_messages_type    ON universal_messages(type);

-- =============================================================================
-- Account configs (per-account settings like quick_replies, follow_ups_enabled)
-- =============================================================================
CREATE TABLE account_configs (
    account_id           TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    quick_replies        JSONB NOT NULL DEFAULT '{}',
    follow_ups_enabled   BOOLEAN NOT NULL DEFAULT TRUE,
    extra                JSONB NOT NULL DEFAULT '{}',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS account_configs;
DROP TABLE IF EXISTS universal_messages;
DROP TABLE IF EXISTS bot_feedback;
DROP TABLE IF EXISTS daily_metrics_snapshots;
DROP TABLE IF EXISTS system_state;
DROP TABLE IF EXISTS webhook_dedup;
DROP TABLE IF EXISTS webhook_log;
DROP TABLE IF EXISTS scheduler_health;
DROP TABLE IF EXISTS system_health_events;
DROP TABLE IF EXISTS sync_status;
-- +goose StatementEnd
