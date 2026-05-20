-- +goose Up
-- +goose StatementBegin

-- =============================================================================
-- Users (auth)
-- =============================================================================
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name  TEXT,
    role          TEXT NOT NULL DEFAULT 'worker' CHECK (role IN ('admin', 'worker')),
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_username_active ON users(username) WHERE is_active = TRUE;

-- =============================================================================
-- Accounts (LinkedIn / WhatsApp accounts onboarded via Unipile)
-- =============================================================================
CREATE TABLE accounts (
    id                       TEXT PRIMARY KEY,                   -- Unipile account id
    account_id               TEXT,                                -- duplicate of id (legacy compat in app code)
    provider                 TEXT NOT NULL DEFAULT 'LINKEDIN',
    name                     TEXT,
    owner_user_id            BIGINT REFERENCES users(id) ON DELETE SET NULL,
    status                   TEXT NOT NULL DEFAULT 'OK',          -- OK | CREDENTIALS | ERROR | STOPPED | RECONNECTED
    last_status_at           TIMESTAMPTZ,
    last_alarm_at            TIMESTAMPTZ,
    last_alarm_reason        TEXT,
    last_alarm_severity      TEXT,

    -- AI safety (kill-switch + caps)
    ai_replies_enabled       BOOLEAN NOT NULL DEFAULT FALSE,
    ai_monthly_cap_usd       NUMERIC(8,2),
    ai_paused_reason         TEXT,
    ai_paused_at             TIMESTAMPTZ,

    -- Human-pattern safety
    engagement_noise_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    vacation_auto_schedule   BOOLEAN NOT NULL DEFAULT TRUE,
    silent_days_per_month    INT NOT NULL DEFAULT 2 CHECK (silent_days_per_month BETWEEN 0 AND 7),

    -- Health score
    health_score             INT,
    health_severity          TEXT,
    health_updated_at        TIMESTAMPTZ,

    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_accounts_status_alarm ON accounts(status, last_alarm_at DESC) WHERE last_alarm_at IS NOT NULL;
CREATE INDEX idx_accounts_health_score ON accounts(health_score);
CREATE INDEX idx_accounts_owner ON accounts(owner_user_id);

-- =============================================================================
-- Account assignments (N:M users <-> accounts)
-- =============================================================================
CREATE TABLE account_assignments (
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, account_id)
);

CREATE INDEX idx_account_assignments_account ON account_assignments(account_id);

-- =============================================================================
-- Per-account runtime state (scheduler pause, dry-run, cooldown)
-- =============================================================================
CREATE TABLE account_state (
    account_id             TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    scheduler_paused       BOOLEAN NOT NULL DEFAULT FALSE,
    paused_reason          TEXT,
    paused_at              TIMESTAMPTZ,
    paused_by              TEXT,
    dry_run                BOOLEAN NOT NULL DEFAULT FALSE,
    consecutive_fail_count INT NOT NULL DEFAULT 0,
    cooldown_until         TIMESTAMPTZ,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================================================
-- LinkedIn profiles cache (Unipile profile data, keyed by provider_id)
-- =============================================================================
CREATE TABLE profiles (
    provider_id                TEXT PRIMARY KEY,
    public_identifier          TEXT,
    first_name                 TEXT,
    last_name                  TEXT,
    maiden_name                TEXT,
    pronoun                    TEXT,
    headline                   TEXT,
    summary                    TEXT,
    location                   TEXT,
    profile_picture_url        TEXT,
    profile_picture_url_large  TEXT,
    background_picture_url     TEXT,
    public_profile_url         TEXT,
    contact_info               JSONB NOT NULL DEFAULT '{}',
    birthdate                  JSONB NOT NULL DEFAULT '{}',
    primary_locale             JSONB NOT NULL DEFAULT '{}',
    websites                   JSONB NOT NULL DEFAULT '[]',
    industry                   JSONB NOT NULL DEFAULT '[]',
    work_experience            JSONB NOT NULL DEFAULT '[]',
    education                  JSONB NOT NULL DEFAULT '[]',
    volunteering_experience    JSONB NOT NULL DEFAULT '[]',
    skills                     JSONB NOT NULL DEFAULT '[]',
    languages                  JSONB NOT NULL DEFAULT '[]',
    certifications             JSONB NOT NULL DEFAULT '[]',
    projects                   JSONB NOT NULL DEFAULT '[]',
    recommendations            JSONB NOT NULL DEFAULT '{}',
    is_premium                 BOOLEAN,
    is_influencer              BOOLEAN,
    is_creator                 BOOLEAN,
    is_hiring                  BOOLEAN,
    is_open_to_work            BOOLEAN,
    is_open_profile            BOOLEAN,
    follower_count             INT,
    connections_count          INT,
    shared_connections_count   INT,
    network_distance           TEXT CHECK (network_distance IN ('FIRST_DEGREE', 'SECOND_DEGREE', 'THIRD_DEGREE', 'OUT_OF_NETWORK')),
    is_relationship            BOOLEAN,
    raw_data                   JSONB,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_profiles_public_identifier ON profiles(public_identifier);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS profiles;
DROP TABLE IF EXISTS account_state;
DROP TABLE IF EXISTS account_assignments;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
