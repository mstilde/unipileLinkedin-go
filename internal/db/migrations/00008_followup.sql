-- +goose Up
-- +goose StatementBegin

-- =============================================================================
-- Follow-up tasks (scheduled auto-replies triggered by chat status)
-- =============================================================================
CREATE TABLE follow_up_tasks (
    id                       BIGSERIAL PRIMARY KEY,
    chat_id                  TEXT NOT NULL,
    account_id               TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    trigger_status           TEXT NOT NULL,
    scheduled_for            TIMESTAMPTZ NOT NULL,
    status                   TEXT NOT NULL DEFAULT 'pending'
                               CHECK (status IN ('pending', 'sent', 'cancelled', 'expired')),
    sent_at                  TIMESTAMPTZ,
    cancelled_at             TIMESTAMPTZ,
    cancel_reason            TEXT,
    message_sent             TEXT,
    last_message_timestamp   TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_followup_pending_scheduled ON follow_up_tasks(scheduled_for) WHERE status = 'pending';
CREATE INDEX idx_followup_account           ON follow_up_tasks(account_id);
CREATE INDEX idx_followup_chat              ON follow_up_tasks(chat_id);

-- Prevent duplicate pending tasks per (chat, trigger_status)
CREATE UNIQUE INDEX idx_followup_unique_pending
    ON follow_up_tasks(chat_id, trigger_status)
    WHERE status = 'pending';

-- =============================================================================
-- Follow-up configs (global config per trigger_status)
-- =============================================================================
CREATE TABLE follow_up_configs (
    id               BIGSERIAL PRIMARY KEY,
    trigger_status   TEXT NOT NULL UNIQUE,
    delay_days       INT NOT NULL DEFAULT 3,
    delay_hours      INT NOT NULL DEFAULT 0,
    message_template TEXT NOT NULL,
    is_active        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================================================
-- Follow-up logs (audit of every follow-up lifecycle event)
-- =============================================================================
CREATE TABLE follow_up_logs (
    id          BIGSERIAL PRIMARY KEY,
    task_id     BIGINT REFERENCES follow_up_tasks(id) ON DELETE SET NULL,
    account_id  TEXT,
    action      TEXT NOT NULL
                  CHECK (action IN ('created', 'sent', 'cancelled', 'expired', 'error')),
    details     JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_followup_logs_date    ON follow_up_logs(created_at);
CREATE INDEX idx_followup_logs_account ON follow_up_logs(account_id);

-- =============================================================================
-- Default follow-up configs (seed data)
-- =============================================================================
INSERT INTO follow_up_configs (trigger_status, delay_days, message_template, is_active) VALUES
    ('m1',          3, 'Hola {nombre}, quería dar seguimiento a mi mensaje anterior. ¿Tuviste oportunidad de verlo?', TRUE),
    ('propuesta',   3, 'Hola {nombre}, ¿pudiste revisar la propuesta que te envié? Me encantaría conocer tu opinión.',  TRUE),
    ('aceptado',    5, 'Hola {nombre}, ¡genial que aceptaste! ¿Cuándo te quedaría bien agendar una llamada?',           TRUE);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS follow_up_logs;
DROP TABLE IF EXISTS follow_up_configs;
DROP TABLE IF EXISTS follow_up_tasks;
-- +goose StatementEnd
