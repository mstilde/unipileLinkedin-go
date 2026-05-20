-- +goose Up
-- +goose StatementBegin

-- =============================================================================
-- campaign_metrics — pre-computed funnel counts per campaign.
-- Refresh manually: REFRESH MATERIALIZED VIEW CONCURRENTLY campaign_metrics;
-- =============================================================================
CREATE MATERIALIZED VIEW campaign_metrics AS
SELECT
    c.id AS campaign_id,
    c.name AS campaign_name,
    c.status AS campaign_status,
    COUNT(p.id)::INT AS total_prospects,
    COUNT(p.id) FILTER (WHERE p.status = 'invited')::INT AS invited,
    COUNT(p.id) FILTER (WHERE p.invite_accepted_at IS NOT NULL OR p.status = 'connected')::INT AS connected,
    COUNT(p.id) FILTER (WHERE p.last_reply_at IS NOT NULL)::INT AS replied,
    COUNT(p.id) FILTER (WHERE p.status = 'opted_out')::INT AS opted_out,
    COUNT(p.id) FILTER (WHERE p.status = 'withdrawn')::INT AS withdrawn,
    COUNT(p.id) FILTER (WHERE p.status = 'failed')::INT AS failed,
    (SELECT COUNT(*)::INT FROM prospect_steps ps
        JOIN prospects pp ON pp.id = ps.prospect_id
        WHERE pp.campaign_id = c.id AND ps.step_type = 'invite' AND ps.status = 'sent') AS invites_sent,
    (SELECT COUNT(*)::INT FROM prospect_steps ps
        JOIN prospects pp ON pp.id = ps.prospect_id
        WHERE pp.campaign_id = c.id AND ps.step_type IN ('message', 'send_message') AND ps.status = 'sent') AS messages_sent,
    NOW() AS computed_at
FROM campaigns c
LEFT JOIN prospects p ON p.campaign_id = c.id
GROUP BY c.id;

CREATE UNIQUE INDEX idx_campaign_metrics_id ON campaign_metrics(campaign_id);

-- +goose StatementEnd

-- +goose StatementBegin

-- =============================================================================
-- TRIGGER 1: ensure_account_safety_defaults
-- BEFORE INSERT on accounts: fill nulls with safe defaults (idempotent via COALESCE).
-- =============================================================================
CREATE OR REPLACE FUNCTION ensure_account_safety_defaults()
RETURNS TRIGGER AS $$
BEGIN
    NEW.engagement_noise_enabled := COALESCE(NEW.engagement_noise_enabled, TRUE);
    NEW.vacation_auto_schedule   := COALESCE(NEW.vacation_auto_schedule, TRUE);
    NEW.silent_days_per_month    := COALESCE(NEW.silent_days_per_month, 2);
    NEW.ai_replies_enabled       := COALESCE(NEW.ai_replies_enabled, FALSE);
    NEW.status                   := COALESCE(NEW.status, 'OK');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_accounts_safety_defaults
BEFORE INSERT ON accounts
FOR EACH ROW
EXECUTE FUNCTION ensure_account_safety_defaults();
-- +goose StatementEnd

-- +goose StatementBegin

-- =============================================================================
-- TRIGGER 2: seed_account_daily_limits
-- AFTER INSERT on accounts: if provider='LINKEDIN', seed 11 rows in
-- account_daily_limits with tier='premium' caps. ON CONFLICT DO NOTHING.
-- =============================================================================
CREATE OR REPLACE FUNCTION seed_account_daily_limits()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.provider IS DISTINCT FROM 'LINKEDIN' THEN
        RETURN NEW;
    END IF;

    INSERT INTO account_daily_limits (
        account_id, action_type, daily_cap, weekly_cap, tier,
        current_day_count, last_reset_at, ramp_up_enabled, ramp_up_curve
    )
    SELECT
        NEW.id,
        t.action_type,
        t.daily_cap,
        t.weekly_cap,
        'premium',
        0,
        CURRENT_DATE,
        FALSE,
        '{"type":"linear","start":2,"increment":2}'::JSONB
    FROM (VALUES
        ('invite',           15,  80),
        ('message',          40,  700),
        ('send_message',     40,  700),
        ('visit_profile',    80,  NULL),
        ('follow',           30,  NULL),
        ('like_post',        50,  NULL),
        ('comment_post',     15,  NULL),
        ('inmail',           20,  200),
        ('voice_note',       10,  50),
        ('withdraw_invite',  100, NULL),
        ('engagement_noise', 5,   NULL)
    ) AS t(action_type, daily_cap, weekly_cap)
    ON CONFLICT (account_id, action_type) DO NOTHING;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_accounts_seed_limits
AFTER INSERT ON accounts
FOR EACH ROW
EXECUTE FUNCTION seed_account_daily_limits();
-- +goose StatementEnd

-- +goose StatementBegin

-- =============================================================================
-- TRIGGER 3: updated_at touch on common tables
-- Generic trigger to keep updated_at fresh on UPDATE.
-- =============================================================================
CREATE OR REPLACE FUNCTION touch_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_users_updated_at         BEFORE UPDATE ON users         FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_accounts_updated_at      BEFORE UPDATE ON accounts      FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_campaigns_updated_at     BEFORE UPDATE ON campaigns     FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_ai_personas_updated_at   BEFORE UPDATE ON ai_personas   FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_client_profiles_upd      BEFORE UPDATE ON client_profiles FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_campaign_templates_upd   BEFORE UPDATE ON campaign_templates FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_stage_routing_upd        BEFORE UPDATE ON stage_followup_routing FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_prospects_updated_at     BEFORE UPDATE ON prospects     FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_chats_cache_updated_at   BEFORE UPDATE ON chats_cache   FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_account_state_upd        BEFORE UPDATE ON account_state FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_account_configs_upd      BEFORE UPDATE ON account_configs FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_followup_configs_upd     BEFORE UPDATE ON follow_up_configs FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_universal_messages_upd   BEFORE UPDATE ON universal_messages FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_chat_states_upd          BEFORE UPDATE ON chat_states FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_profiles_upd             BEFORE UPDATE ON profiles FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_system_state_upd         BEFORE UPDATE ON system_state FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_daily_metrics_upd        BEFORE UPDATE ON daily_metrics_snapshots FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE TRIGGER trg_adl_upd                  BEFORE UPDATE ON account_daily_limits FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_users_updated_at         ON users;
DROP TRIGGER IF EXISTS trg_accounts_updated_at      ON accounts;
DROP TRIGGER IF EXISTS trg_campaigns_updated_at     ON campaigns;
DROP TRIGGER IF EXISTS trg_ai_personas_updated_at   ON ai_personas;
DROP TRIGGER IF EXISTS trg_client_profiles_upd      ON client_profiles;
DROP TRIGGER IF EXISTS trg_campaign_templates_upd   ON campaign_templates;
DROP TRIGGER IF EXISTS trg_stage_routing_upd        ON stage_followup_routing;
DROP TRIGGER IF EXISTS trg_prospects_updated_at     ON prospects;
DROP TRIGGER IF EXISTS trg_chats_cache_updated_at   ON chats_cache;
DROP TRIGGER IF EXISTS trg_account_state_upd        ON account_state;
DROP TRIGGER IF EXISTS trg_account_configs_upd      ON account_configs;
DROP TRIGGER IF EXISTS trg_followup_configs_upd     ON follow_up_configs;
DROP TRIGGER IF EXISTS trg_universal_messages_upd   ON universal_messages;
DROP TRIGGER IF EXISTS trg_chat_states_upd          ON chat_states;
DROP TRIGGER IF EXISTS trg_profiles_upd             ON profiles;
DROP TRIGGER IF EXISTS trg_system_state_upd         ON system_state;
DROP TRIGGER IF EXISTS trg_daily_metrics_upd        ON daily_metrics_snapshots;
DROP TRIGGER IF EXISTS trg_adl_upd                  ON account_daily_limits;
DROP FUNCTION IF EXISTS touch_updated_at();

DROP TRIGGER IF EXISTS trg_accounts_seed_limits     ON accounts;
DROP FUNCTION IF EXISTS seed_account_daily_limits();

DROP TRIGGER IF EXISTS trg_accounts_safety_defaults ON accounts;
DROP FUNCTION IF EXISTS ensure_account_safety_defaults();

DROP MATERIALIZED VIEW IF EXISTS campaign_metrics;
-- +goose StatementEnd
