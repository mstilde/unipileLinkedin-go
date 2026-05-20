-- +goose Up
-- +goose StatementBegin

-- =============================================================================
-- Chats cache (Unipile chat metadata)
-- =============================================================================
CREATE TABLE chats_cache (
    id                     TEXT PRIMARY KEY,
    account_id             TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    account_name           TEXT,
    attendee_name          TEXT,
    attendee_provider_id   TEXT,
    profile_picture_url    TEXT,
    last_message_text      TEXT,
    last_message_time      TIMESTAMPTZ,
    last_message_is_sender BOOLEAN NOT NULL DEFAULT FALSE,
    provider_id            TEXT,
    archived               BOOLEAN NOT NULL DEFAULT FALSE,
    timestamp              TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chats_cache_account   ON chats_cache(account_id);
CREATE INDEX idx_chats_cache_timestamp ON chats_cache(timestamp DESC);
CREATE INDEX idx_chats_cache_attendee  ON chats_cache(attendee_provider_id) WHERE attendee_provider_id IS NOT NULL;

-- =============================================================================
-- Messages (canonical, write-once)
-- =============================================================================
CREATE TABLE messages (
    id           BIGSERIAL PRIMARY KEY,
    chat_id      TEXT NOT NULL,
    sender_id    TEXT NOT NULL,
    text         TEXT,
    timestamp    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_sender    BOOLEAN NOT NULL DEFAULT FALSE,
    attachments  JSONB NOT NULL DEFAULT '[]',
    status       TEXT NOT NULL DEFAULT 'sent',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_messages_chat      ON messages(chat_id);
CREATE INDEX idx_messages_timestamp ON messages(chat_id, timestamp DESC);

-- =============================================================================
-- Messages cache (optimized cache keyed by Unipile message id)
-- =============================================================================
CREATE TABLE messages_cache (
    id                 TEXT PRIMARY KEY,
    chat_id            TEXT NOT NULL,
    text               TEXT,
    timestamp          TIMESTAMPTZ,
    is_sender          BOOLEAN NOT NULL DEFAULT FALSE,
    sender_name        TEXT,
    sender_picture_url TEXT,
    attachments        JSONB NOT NULL DEFAULT '[]',
    cached_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_messages_cache_chat       ON messages_cache(chat_id);
CREATE INDEX idx_messages_cache_chat_ts    ON messages_cache(chat_id, timestamp ASC);

-- =============================================================================
-- Per-user chat state (read flags, pin, tags, custom status)
-- =============================================================================
CREATE TABLE chat_states (
    id                 BIGSERIAL PRIMARY KEY,
    chat_id            TEXT NOT NULL,
    user_id            BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    is_read            BOOLEAN NOT NULL DEFAULT FALSE,
    last_read_at       TIMESTAMPTZ,
    marked_unread_at   TIMESTAMPTZ,
    unread_count       INT NOT NULL DEFAULT 0,
    is_pinned          BOOLEAN NOT NULL DEFAULT FALSE,
    custom_status      TEXT,
    tags               JSONB NOT NULL DEFAULT '[]',
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (chat_id, user_id)
);

CREATE INDEX idx_chat_states_user ON chat_states(user_id);
CREATE INDEX idx_chat_states_chat ON chat_states(chat_id);

-- =============================================================================
-- Status changes audit (every chat status transition)
-- =============================================================================
CREATE TABLE status_changes (
    id          BIGSERIAL PRIMARY KEY,
    chat_id     TEXT NOT NULL,
    account_id  TEXT NOT NULL,
    old_status  TEXT,
    new_status  TEXT NOT NULL,
    changed_by  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_status_changes_chat            ON status_changes(chat_id);
CREATE INDEX idx_status_changes_account         ON status_changes(account_id);
CREATE INDEX idx_status_changes_time            ON status_changes(changed_at);
CREATE INDEX idx_status_changes_account_status  ON status_changes(account_id, new_status, changed_at);

-- =============================================================================
-- Account connection log (track Unipile (re)connections / backfills)
-- =============================================================================
CREATE TABLE account_connection_log (
    id                   BIGSERIAL PRIMARY KEY,
    account_id           TEXT NOT NULL,
    status               TEXT NOT NULL,
    detected_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_good_sync_at    TIMESTAMPTZ,
    backfill_triggered   BOOLEAN NOT NULL DEFAULT FALSE,
    backfill_job_id      TEXT
);

CREATE INDEX idx_conn_log_account ON account_connection_log(account_id, detected_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS account_connection_log;
DROP TABLE IF EXISTS status_changes;
DROP TABLE IF EXISTS chat_states;
DROP TABLE IF EXISTS messages_cache;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS chats_cache;
-- +goose StatementEnd
