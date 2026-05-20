# `internal/db` — schema, migrations, queries

## Migrations

Files in `migrations/*.sql` are written for [goose](https://github.com/pressly/goose)
and are embedded into the binary via `//go:embed` (see `migrations.go`).

### Order

1. **00001_users_accounts** — users, accounts, account_assignments, account_state, profiles
2. **00002_campaigns_sequences** — ai_personas, client_profiles, campaigns, sequence_steps, campaign_templates, stage_followup_routing, prospects, prospect_steps, campaign_signals
3. **00003_chats_messages** — chats_cache, messages, messages_cache, chat_states, status_changes, account_connection_log
4. **00004_ai_safety** — ai_reply_queue, human_review_queue, ai_interactions, ai_safety_log, enrichment_cache, enrichment_external_cache
5. **00005_safety_limits** — account_daily_limits, account_weekly_actions, global_blacklist, account_vacations
6. **00006_funnel_ab** — prospect_funnel_events, ab_promotion_log, ab_test_assignments, daily_invite_counts
7. **00007_linkedin_search** — linkedin_lookup_cache, linkedin_search_jobs, linkedin_saved_searches, linkedin_account_quota, linkedin_import_runs, saas_owners, hybrid_search_audit
8. **00008_followup** — follow_up_tasks, follow_up_configs (seeded), follow_up_logs
9. **00009_sync_misc** — sync_status, system_health_events, scheduler_health, webhook_log, webhook_dedup, system_state, daily_metrics_snapshots, bot_feedback, universal_messages, account_configs
10. **00010_views_triggers** — campaign_metrics matview, `ensure_account_safety_defaults` + `seed_account_daily_limits` + `touch_updated_at` triggers

### Running

The `Makefile` `migrate` target uses the goose CLI:

```sh
go install github.com/pressly/goose/v3/cmd/goose@latest
make db-up       # docker compose up -d postgres
export DATABASE_URL='postgres://unipile:unipile@localhost:5432/unipile_go?sslmode=disable'
make migrate     # goose -dir internal/db/migrations postgres "$DATABASE_URL" up
```

For production we will instead apply migrations from the embedded FS via
`goose.SetBaseFS(db.Migrations())` so the binary is self-contained.

## Queries (sqlc)

`queries/*.sql` will contain hand-written SQL with sqlc annotations. Run
`make sqlc` to regenerate `gen/` (typed Go code).

Not yet populated — added during task #4 (domain core).

## Multi-tenancy

All tenant-scoped tables include `account_id TEXT` with FK to `accounts(id)`.
Ownership is enforced in HTTP middleware (`requireOwnedAccount`), not at the DB
level via RLS, to keep the schema portable across Postgres flavors.
