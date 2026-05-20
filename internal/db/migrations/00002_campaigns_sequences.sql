-- +goose Up
-- +goose StatementBegin

-- =============================================================================
-- AI Personas (one per account: system prompt, scripts, communication style)
-- =============================================================================
CREATE TABLE ai_personas (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id    TEXT NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE,
    name          TEXT,
    system_prompt TEXT NOT NULL DEFAULT '',
    guiones_json  JSONB NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================================================
-- Client profiles (onboarding questionnaire + merged summary for AI)
-- =============================================================================
CREATE TABLE client_profiles (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id         TEXT NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE,
    client_provider_id TEXT,
    questionnaire      JSONB NOT NULL DEFAULT '{}',
    merged_summary     TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================================================
-- Campaigns
-- =============================================================================
CREATE TABLE campaigns (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id                  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name                        TEXT NOT NULL,
    status                      TEXT NOT NULL DEFAULT 'draft'
                                  CHECK (status IN ('draft', 'active', 'paused', 'completed', 'archived')),
    daily_invite_limit          INT NOT NULL DEFAULT 40 CHECK (daily_invite_limit BETWEEN 1 AND 100),
    tz                          TEXT NOT NULL DEFAULT 'America/Argentina/Buenos_Aires',
    auto_reply_enabled          BOOLEAN NOT NULL DEFAULT FALSE,
    ai_monthly_cap_usd          NUMERIC(8,2) NOT NULL DEFAULT 5.00,
    ab_promote_config           JSONB NOT NULL DEFAULT '{}',

    -- Working hours / scheduling
    working_hours_start         INT NOT NULL DEFAULT 9,
    working_hours_end           INT NOT NULL DEFAULT 19,
    lunch_break_start           INT,
    lunch_break_end             INT,
    skip_weekends               BOOLEAN NOT NULL DEFAULT TRUE,
    skip_holidays               TEXT[] NOT NULL DEFAULT '{}',
    ramp_up_enabled             BOOLEAN NOT NULL DEFAULT FALSE,
    auto_withdraw_after_days    INT,
    auto_resume_on_human_reply  BOOLEAN NOT NULL DEFAULT FALSE,

    -- Behavior toggles
    is_followup                 BOOLEAN NOT NULL DEFAULT FALSE,
    strict_template_validation  BOOLEAN NOT NULL DEFAULT FALSE,
    auto_prewarm_visit          BOOLEAN NOT NULL DEFAULT FALSE,
    auto_prewarm_delay_minutes  INT NOT NULL DEFAULT 60,
    simulate_human_typing       BOOLEAN NOT NULL DEFAULT FALSE,

    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_campaigns_account_status ON campaigns(account_id, status);

-- =============================================================================
-- Sequence steps (ordered list of actions per campaign)
-- =============================================================================
CREATE TABLE sequence_steps (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id     UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    step_index      INT NOT NULL CHECK (step_index >= 0),
    step_type       TEXT NOT NULL
                      CHECK (step_type IN (
                          'invite', 'message', 'send_message', 'wait', 'condition',
                          'visit_profile', 'follow', 'like_post', 'comment_post',
                          'withdraw_invite', 'voice_note', 'inmail', 'ab_test', 'end'
                      )),
    delay_hours     INT NOT NULL DEFAULT 24 CHECK (delay_hours >= 0),
    template        TEXT,
    ai_personalize  BOOLEAN NOT NULL DEFAULT FALSE,
    note_max_chars  INT NOT NULL DEFAULT 200,
    stage_label     TEXT,
    config_json     JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (campaign_id, step_index)
);

CREATE INDEX idx_seq_steps_campaign ON sequence_steps(campaign_id, step_index);
CREATE INDEX idx_seq_steps_stage ON sequence_steps(campaign_id, stage_label) WHERE stage_label IS NOT NULL;

-- =============================================================================
-- Campaign templates (per-kind, AI-adaptable copy)
-- =============================================================================
CREATE TABLE campaign_templates (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id   UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    template_kind TEXT NOT NULL
                    CHECK (template_kind IN (
                        'invite_note', 'nota_premium', 'msg1', 'propuesta',
                        'transicion_with_phone', 'transicion_ask_phone', 'post_wa_confirmation'
                    )),
    template_text TEXT NOT NULL,
    ai_adapt      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (campaign_id, template_kind)
);

-- =============================================================================
-- Stage follow-up routing (when prospect replies at stage X, route to campaign Y)
-- =============================================================================
CREATE TABLE stage_followup_routing (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_campaign_id       UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    stage_label              TEXT NOT NULL,
    target_followup_campaign UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    enabled                  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_campaign_id, stage_label)
);

CREATE INDEX idx_stage_routing_source ON stage_followup_routing(source_campaign_id) WHERE enabled = TRUE;

-- =============================================================================
-- Prospects (lead per campaign)
-- =============================================================================
CREATE TABLE prospects (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id              UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    account_id               TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    profile_url              TEXT NOT NULL,
    public_identifier        TEXT,
    linkedin_provider_id     TEXT,
    chat_id                  TEXT,
    full_name                TEXT,
    first_name               TEXT,
    headline                 TEXT,
    company                  TEXT,
    enrichment_json          JSONB,
    status                   TEXT NOT NULL DEFAULT 'queued'
                               CHECK (status IN (
                                   'queued', 'resolving', 'resolved', 'enriching', 'ready',
                                   'invited', 'connected', 'replied', 'won', 'lost',
                                   'opted_out', 'withdrawn', 'failed'
                               )),
    conversation_stage       TEXT NOT NULL DEFAULT 'awaiting_invite_acceptance'
                               CHECK (conversation_stage IN (
                                   'awaiting_invite_acceptance',
                                   'awaiting_msg1_send',
                                   'awaiting_msg1_response',
                                   'awaiting_propuesta_response',
                                   'awaiting_transicion_response',
                                   'awaiting_post_wa_confirmation',
                                   'done', 'opted_out', 'lost'
                               )),
    error_detail             TEXT,
    ai_persona_id            UUID REFERENCES ai_personas(id) ON DELETE SET NULL,

    -- Lifecycle timestamps
    invited_at               TIMESTAMPTZ,
    connected_at             TIMESTAMPTZ,
    invite_accepted_at       TIMESTAMPTZ,
    last_reply_at            TIMESTAMPTZ,
    replied_at               TIMESTAMPTZ,
    last_visited_at          TIMESTAMPTZ,
    withdrawn_at             TIMESTAMPTZ,

    -- Behavior flags
    paused_by_reply          BOOLEAN NOT NULL DEFAULT FALSE,
    do_not_retry_invite      BOOLEAN NOT NULL DEFAULT FALSE,

    -- WhatsApp capture
    whatsapp_number          TEXT,
    whatsapp_captured_at     TIMESTAMPTZ,

    -- Custom data
    tags                     TEXT[] NOT NULL DEFAULT '{}',
    variables                JSONB NOT NULL DEFAULT '{}',
    branch_path              INT[] NOT NULL DEFAULT '{}',

    -- Re-routing hooks
    next_step_index_override INT,
    resume_after             TIMESTAMPTZ,
    routed_to_followup_id    UUID,

    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (account_id, profile_url)
);

CREATE INDEX idx_prospects_status         ON prospects(status, account_id);
CREATE INDEX idx_prospects_provider       ON prospects(linkedin_provider_id, account_id);
CREATE INDEX idx_prospects_chat           ON prospects(chat_id) WHERE chat_id IS NOT NULL;
CREATE INDEX idx_prospects_invite_accept  ON prospects(invite_accepted_at) WHERE invite_accepted_at IS NOT NULL;
CREATE INDEX idx_prospects_last_reply     ON prospects(last_reply_at) WHERE last_reply_at IS NOT NULL;
CREATE INDEX idx_prospects_paused         ON prospects(paused_by_reply) WHERE paused_by_reply = TRUE;
CREATE INDEX idx_prospects_status_invited ON prospects(status, invited_at) WHERE status = 'invited';
CREATE INDEX idx_prospects_conv_stage     ON prospects(conversation_stage, account_id);

-- =============================================================================
-- Prospect steps (planned + executed actions per prospect)
-- =============================================================================
CREATE TABLE prospect_steps (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prospect_id            UUID NOT NULL REFERENCES prospects(id) ON DELETE CASCADE,
    step_id                UUID REFERENCES sequence_steps(id) ON DELETE SET NULL,
    step_type              TEXT NOT NULL,
    scheduled_at           TIMESTAMPTZ NOT NULL,
    sent_at                TIMESTAMPTZ,
    status                 TEXT NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending', 'processing', 'sent', 'skipped', 'failed', 'cancelled')),
    message_sent           TEXT,
    error_detail           TEXT,
    retry_count            INT NOT NULL DEFAULT 0,
    max_retries            INT NOT NULL DEFAULT 3,
    ab_variant_id          TEXT,
    branch                 TEXT,
    last_check_at          TIMESTAMPTZ,
    processing_started_at  TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT prospect_steps_sent_at_required CHECK (status <> 'sent' OR sent_at IS NOT NULL)
);

CREATE INDEX idx_prospect_steps_sched      ON prospect_steps(scheduled_at) WHERE status = 'pending';
CREATE INDEX idx_prospect_steps_wait      ON prospect_steps(scheduled_at) WHERE status = 'pending' AND step_type = 'wait';
CREATE INDEX idx_prospect_steps_pid       ON prospect_steps(prospect_id);
CREATE INDEX idx_prospect_steps_lease     ON prospect_steps(processing_started_at) WHERE status = 'processing';

CREATE UNIQUE INDEX uniq_prospect_steps_active
    ON prospect_steps (prospect_id, step_id)
    WHERE status NOT IN ('cancelled', 'failed') AND step_id IS NOT NULL;

-- =============================================================================
-- Campaign signals (webhook -> scheduler channel)
-- =============================================================================
CREATE TABLE campaign_signals (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type        TEXT NOT NULL,
    prospect_id UUID REFERENCES prospects(id) ON DELETE CASCADE,
    data        JSONB NOT NULL DEFAULT '{}',
    processed   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_signals_unprocessed ON campaign_signals(created_at) WHERE processed = FALSE;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS campaign_signals;
DROP TABLE IF EXISTS prospect_steps;
DROP TABLE IF EXISTS prospects;
DROP TABLE IF EXISTS stage_followup_routing;
DROP TABLE IF EXISTS campaign_templates;
DROP TABLE IF EXISTS sequence_steps;
DROP TABLE IF EXISTS campaigns;
DROP TABLE IF EXISTS client_profiles;
DROP TABLE IF EXISTS ai_personas;
-- +goose StatementEnd
