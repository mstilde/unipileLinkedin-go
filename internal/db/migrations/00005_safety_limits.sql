-- +goose Up
-- +goose StatementBegin

-- =============================================================================
-- Account daily limits (per-action caps with optional ramp-up curve)
-- =============================================================================
CREATE TABLE account_daily_limits (
    account_id          TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    action_type         TEXT NOT NULL,
    daily_cap           INT NOT NULL DEFAULT 20,
    current_day_count   INT NOT NULL DEFAULT 0,
    last_reset_at       DATE NOT NULL DEFAULT CURRENT_DATE,
    weekly_cap          INT,
    tier                TEXT NOT NULL DEFAULT 'premium'
                          CHECK (tier IN ('free', 'premium', 'sales_nav', 'recruiter')),
    ramp_up_enabled     BOOLEAN NOT NULL DEFAULT FALSE,
    ramp_up_started_at  TIMESTAMPTZ,
    ramp_up_curve       JSONB,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, action_type)
);

CREATE INDEX idx_adl_account ON account_daily_limits(account_id);

-- =============================================================================
-- Weekly actions log (rolling 7d log to enforce weekly_cap)
-- Cleanup older than 14 days via cron from app side.
-- =============================================================================
CREATE TABLE account_weekly_actions (
    id          BIGSERIAL PRIMARY KEY,
    account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    action_type TEXT NOT NULL,
    action_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_awa_account_action_at ON account_weekly_actions(account_id, action_type, action_at DESC);
CREATE INDEX idx_awa_old_cleanup       ON account_weekly_actions(action_at);

-- =============================================================================
-- Global blacklist (prospects we never want to contact again)
-- =============================================================================
CREATE TABLE global_blacklist (
    id                  BIGSERIAL PRIMARY KEY,
    account_id          TEXT REFERENCES accounts(id) ON DELETE CASCADE,
    lead_provider_id    TEXT,
    lead_linkedin_url   TEXT,
    lead_email          TEXT,
    reason              TEXT,
    source_campaign_id  UUID REFERENCES campaigns(id) ON DELETE SET NULL,
    added_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_blacklist_provider ON global_blacklist(lead_provider_id) WHERE lead_provider_id IS NOT NULL;
CREATE INDEX idx_blacklist_url      ON global_blacklist(lower(lead_linkedin_url)) WHERE lead_linkedin_url IS NOT NULL;
CREATE INDEX idx_blacklist_email    ON global_blacklist(lower(lead_email)) WHERE lead_email IS NOT NULL;
CREATE INDEX idx_blacklist_account  ON global_blacklist(account_id);

-- =============================================================================
-- Account vacations (manual + auto-scheduled pause windows)
-- =============================================================================
CREATE TABLE account_vacations (
    id              BIGSERIAL PRIMARY KEY,
    account_id      TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    starts_at       DATE NOT NULL,
    ends_at         DATE NOT NULL,
    reason          TEXT,
    auto_scheduled  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    applied_at      TIMESTAMPTZ,
    released_at     TIMESTAMPTZ
);

CREATE INDEX idx_av_account_dates ON account_vacations(account_id, starts_at, ends_at);
CREATE INDEX idx_av_active        ON account_vacations(starts_at, ends_at) WHERE released_at IS NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS account_vacations;
DROP TABLE IF EXISTS global_blacklist;
DROP TABLE IF EXISTS account_weekly_actions;
DROP TABLE IF EXISTS account_daily_limits;
-- +goose StatementEnd
