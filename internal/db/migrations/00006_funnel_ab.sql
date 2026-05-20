-- +goose Up
-- +goose StatementBegin

-- =============================================================================
-- Prospect funnel events (immutable cohort + A/B funnel log)
-- account_id is account text id (cast in app, no RLS here — enforced by handler)
-- =============================================================================
CREATE TABLE prospect_funnel_events (
    id                   BIGSERIAL PRIMARY KEY,
    prospect_id          UUID NOT NULL REFERENCES prospects(id) ON DELETE CASCADE,
    campaign_id          UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    account_id           TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    stage                TEXT NOT NULL
                           CHECK (stage IN (
                               'invite_accepted', 'msg1_replied', 'propuesta_sent',
                               'propuesta_accepted', 'phone_captured'
                           )),
    is_first_occurrence  BOOLEAN NOT NULL,
    out_of_order         BOOLEAN NOT NULL DEFAULT FALSE,
    cohort_week          DATE NOT NULL,
    ab_step_id           UUID,
    ab_variant_id        TEXT,
    template_kind        TEXT,
    source               TEXT NOT NULL
                           CHECK (source IN (
                               'webhook_relations', 'webhook_message', 'scheduler',
                               'reply_handler', 'reconciliation', 'polling', 'manual'
                           )),
    occurred_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata             JSONB NOT NULL DEFAULT '{}'
);

-- One "first occurrence" per (prospect, stage)
CREATE UNIQUE INDEX uq_funnel_first_per_stage
    ON prospect_funnel_events (prospect_id, stage)
    WHERE is_first_occurrence = TRUE;

CREATE INDEX idx_funnel_cohort            ON prospect_funnel_events (cohort_week, campaign_id, stage);
CREATE INDEX idx_funnel_ab                ON prospect_funnel_events (ab_step_id, ab_variant_id, stage) WHERE ab_step_id IS NOT NULL;
CREATE INDEX idx_funnel_out_of_order      ON prospect_funnel_events (out_of_order, occurred_at) WHERE out_of_order = TRUE;
CREATE INDEX idx_funnel_camp_stage_occ    ON prospect_funnel_events (campaign_id, stage, occurred_at DESC);

-- =============================================================================
-- A/B promotion log (auditable record of automatic variant promotions)
-- =============================================================================
CREATE TABLE ab_promotion_log (
    id           BIGSERIAL PRIMARY KEY,
    step_id      UUID NOT NULL,
    campaign_id  UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    decided_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decision     TEXT NOT NULL
                   CHECK (decision IN ('ramp_up', 'promoted', 'rolled_back', 'inconclusive')),
    metric       TEXT NOT NULL,
    prev_weights JSONB NOT NULL,
    new_weights  JSONB NOT NULL,
    sample_sizes JSONB NOT NULL,
    p_value      NUMERIC,
    lift_pct     NUMERIC,
    notes        TEXT
);

CREATE INDEX idx_ab_promo_step ON ab_promotion_log(step_id, decided_at DESC);
CREATE INDEX idx_ab_promo_camp ON ab_promotion_log(campaign_id, decided_at DESC);

-- =============================================================================
-- A/B test assignments (per-prospect variant assignment + outcomes)
-- =============================================================================
CREATE TABLE ab_test_assignments (
    campaign_id  UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    step_id      UUID NOT NULL,
    prospect_id  UUID NOT NULL REFERENCES prospects(id) ON DELETE CASCADE,
    variant_id   TEXT NOT NULL,
    sent_at      TIMESTAMPTZ,
    accepted_at  TIMESTAMPTZ,
    replied_at   TIMESTAMPTZ,
    opened_at    TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (campaign_id, step_id, prospect_id)
);

CREATE INDEX idx_ab_assign_step ON ab_test_assignments(campaign_id, step_id);

-- =============================================================================
-- Daily invite counts (denormalized counter, kept in sync by scheduler)
-- =============================================================================
CREATE TABLE daily_invite_counts (
    account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    date        DATE NOT NULL DEFAULT CURRENT_DATE,
    count       INT NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, date)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS daily_invite_counts;
DROP TABLE IF EXISTS ab_test_assignments;
DROP TABLE IF EXISTS ab_promotion_log;
DROP TABLE IF EXISTS prospect_funnel_events;
-- +goose StatementEnd
