# CLAUDE.md — Context for future Claude sessions

This file is loaded automatically by Claude Code at the start of every session in this repo. Read it end-to-end before doing work.

---

## What this repo is

**`unipile-linkedin-go`** is a Go port of the Node.js system at `C:\Users\Matias\Proyectos\unipileLinkedin\unipile-test\` (~85k LOC). It's a LinkedIn campaign automation platform: schedule sequences (invite → wait → message → follow-up), manage prospects, auto-reply with AI, route conversations across stages, enforce per-account safety caps. Built across one long session in May 2026.

**Hard constraint from the user (verbatim, do not violate):**
> "necesito que esté desligado de la cuenta actual de Unipile, y las IAs correspondientes. Es decir, que el sistema actual sea autonomo en servicios y gastos del otro."

Concretely:
- **Different Unipile account** (different `UNIPILE_DSN` / `UNIPILE_API_KEY`).
- **Different AI API keys** (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`).
- **Different Postgres** (this one runs locally in Docker — see `docker-compose.yml`).
- **Different JWT secret.**
- The two systems must never share runtime credentials. If you ever see code reading env from the original `unipile-test/` repo, that's a bug — fix it.

---

## Stack

| Layer | Choice |
|---|---|
| Language | Go 1.26 |
| Router | `github.com/go-chi/chi/v5` |
| Logging | `log/slog` (JSON handler) |
| Auth | `github.com/golang-jwt/jwt/v5` + `golang.org/x/crypto/bcrypt`, HS256, cookie-based for UI, Bearer for API |
| DB | Postgres 16 in Docker, `pgxpool` driver, **`sqlc` generates typed Go from `internal/db/queries/*.sql`** |
| Migrations | `goose` (files in `internal/db/migrations/`, also embedded via `//go:embed` for production) |
| UI | `templ` (compiled `.templ` → `_templ.go`) + HTMX 2.0 over a chi-served HTML response |
| Static assets | `./static/css/app.css`, `./static/js/sequence-builder.js`, served at `/static/*` |
| Config | `internal/config/config.go` reads env (with `.env` via `godotenv`), validates in production |

No ORMs, no codegen frameworks beyond sqlc/templ, no SDKs for Unipile/Anthropic/OpenAI — everything is hand-rolled `net/http` clients in `internal/{unipile,ai}/`.

---

## Repo layout

```
cmd/
  server/       Entry point. Wires config → DB pool → JWT signer → API/UI routers → scheduler manager → http.Server with graceful shutdown.
  hashgen/      Tiny CLI: `go run ./cmd/hashgen <password>` → prints bcrypt hash. Used for seeding test users.

internal/
  ai/           Anthropic + OpenAI clients (hand-rolled HTTP), classify.go, reply.go, enrich.go, cost.go (PricingTable).
                NEVER swap to an SDK without asking — current code does cache-control + retry-on-429 + cost accounting deliberately.
  auth/         Signer (HS256), Authenticate / RequireAdmin / RequireOwnedAccount middleware, Claims = uid+role+RegisteredClaims.
  config/       Config struct, env loading, prod validation (rejects JWT_SECRET < 32 chars).
  db/
    migrations/ 10 goose files (00001 → 00010). Order matters; do not renumber.
    queries/    Hand-written SQL with sqlc annotations (`-- name: Foo :one|:many|:exec`).
    gen/        sqlc OUTPUT — DO NOT EDIT BY HAND. Regenerate with `sqlc generate`.
    conn.go     pgxpool helper (MaxConns=25, ping on open).
    migrations.go  Embeds migrations/*.sql for production self-contained binary.
  domain/       Pure-Go logic, no DB:
    delay/      delay-engine port: working hours, lunch break, holidays, exponential backoff with FNV jitter.
    sequence/   step-type validation (invite/message/wait/condition/ab_test/voice_note/inmail/end + branching).
    template/   liquid-style template engine: conditionals (stack-based; RE2 has no lookaheads), spin blocks, {{var|filter|fallback}}, legacy {nombre}, gender {H|M|N}, spintax {A|B|C}.
  http/
    api/        JSON REST routes mounted at /api/v1. Files: auth.go (login + /me), campaigns.go (CRUD + start/pause/duplicate/funnel), sequence.go (PUT replace-all in tx + templates upsert), prospects.go (list + CSV upload), account_store.go (impl of auth.AccountOwnershipStore), helpers.go, router.go.
    ui/         Server-rendered HTML routes (templ). Files: handlers.go (login/dashboard/campaigns/sequence), extra_handlers.go (metrics/inbox/onboarding/admin), session.go (cookie + requireSession middleware), helpers.go, router.go.
    views/      .templ files + their compiled _templ.go. Regenerate with `templ generate`.
  pkg/          (empty placeholder)
  safety/       Per-tier daily caps, weekly caps, ramp curves (staircase + LemList linear), A/B variant picker (FNV deterministic), URL/email normalization for blacklist, ShortenFullName ("Juan Carlos Pérez Hernández" → "Juan Pérez"), NextMondayMorning with FNV jitter [08:30, 11:30).
  scheduler/    Manager with 3 goroutines (campaign, follow-up, ai-queue), ctx-driven shutdown, panic-safe tick wrapper. **Dispatch is stubbed**: dry-run marks rows "sent"/"done" with synthetic notes; non-dry-run marks "failed: dispatch not implemented yet". Wiring real Unipile actions is the next big chunk of work.
  unipile/      HTTP client over X-API-KEY, error classifier (IsLinkedInRateLimit / IsWeeklyCap / IsThrottled / IsTransient / IsPermanent). `SendInvitation`, `SendMessage`, `StartNewChat` (multipart) are wired; 7 other actions are stubs returning ErrNotImplemented.

static/         CSS + JS for the UI (dark theme, minimalist).
templates/      (empty — UI lives in internal/http/views/)
docs/           UNIPILE_SUPPORT_TICKET_v3aIjee.md (carried over).
sqlc.yaml       sqlc config (pgx/v5 driver, emit_pointers_for_null_types, rename rules for ID/URL/PII/TZ etc).
docker-compose.yml  Postgres 16-alpine on :5432 with persistent volume.
Makefile        run, build, test, fmt, vet, tidy, db-up, db-down, migrate, sqlc, templ.
PORTING_PLAN.md Original 12-phase plan from the porting session.
```

---

## Local dev setup

```powershell
# 1. Start Postgres
docker compose up -d

# 2. Apply migrations (one-time, or after pulling new migrations)
$env:DATABASE_URL = "postgres://unipile:unipile@localhost:5432/unipile_go?sslmode=disable"
goose -dir internal/db/migrations postgres "$env:DATABASE_URL" up

# 3. Run server (DRY_RUN=true means scheduler logs but doesn't invoke Unipile)
$env:DRY_RUN = "true"
go run ./cmd/server
# → http://localhost:8080
```

**Seeded test data** (created during the porting session — verify it's still there with `docker exec -i unipile-go-postgres psql -U unipile -d unipile_go -c "SELECT id, username, role FROM users;"`):
- User: `matias / test123` (admin)
- Account: `acct-demo` (LINKEDIN, owned by matias)
- Campaign: `Demo Campaign` (active), with 3-step sequence (invite → wait 48h → message)

If the DB is wiped, regenerate them with:
```powershell
$hash = go run ./cmd/hashgen test123
docker exec -i unipile-go-postgres psql -U unipile -d unipile_go -c "INSERT INTO users (username, password_hash, display_name, role) VALUES ('matias', '$hash', 'Matias', 'admin'); INSERT INTO accounts (id, account_id, provider, owner_user_id) SELECT 'acct-demo', 'acct-demo', 'LINKEDIN', id FROM users WHERE username='matias';"
```

---

## Regeneration workflow

Every time you change SQL or templ files, regenerate:

```powershell
sqlc generate    # internal/db/queries/*.sql → internal/db/gen/*.go
templ generate   # internal/http/views/*.templ → *_templ.go
go build ./...   # sanity check
go test ./...    # 180+ tests across 8 packages
```

The `make sqlc` / `make templ` targets do the same.

---

## What's done vs. stubbed (state at end of porting session)

**✅ Fully done:**
- Schema + migrations (10 files, applied locally)
- Auth (JWT for API + cookie session for UI)
- Domain: template engine, sequence validator, delay engine
- Safety: tier caps, ramp curves, A/B picker, blacklist, short-name
- Unipile HTTP client with error classification
- Anthropic + OpenAI clients with cost accounting + retries
- AI classify/reply/enrich pipelines (Haiku → Sonnet fallback, PII validator)
- REST API: login, /me, campaigns CRUD, sequence replace-all, templates upsert, prospects list + CSV upload, funnel
- UI: login, dashboard, sequence-builder (collapsible cards + HTMX save), inbox, metrics, admin (create users), onboarding (system prompt + questionnaire)
- Scheduler manager + 3 loops with graceful shutdown

**🟡 Stubbed — TODOs for the next session:**
- `internal/scheduler/campaign.go::dispatchStep` — currently just marks rows "sent" in dry-run. Real implementation needs to: load the sequence step, render template via `internal/domain/template`, call the correct `internal/unipile/*` action, check `internal/safety` caps before sending, handle errors with the existing error classifier.
- `internal/scheduler/aiqueue.go` — drains the queue but writes placeholder draft. Should call `internal/ai/reply.go::GenerateReply` with the actual chat history + system prompt + safety/PII check.
- `internal/scheduler/followup.go` — same: load message_template from `follow_up_configs`, render, send via Unipile.
- 7 of 10 Unipile actions return `ErrNotImplemented` (VisitProfile, Follow, LikePost, CommentPost, WithdrawInvite, SendVoiceNote, SendInMail). Implement them as needed.
- Webhook handler for inbound LinkedIn events (the Node.js version has it; we haven't ported it yet).
- Per-account ramp-up startup (we have the curve logic in `internal/safety/limits.go`, but no scheduler integration yet).

---

## Coding conventions specific to this repo

- **sqlc gotcha**: when SQL has `COALESCE($N, default)` and `$N` isn't typed, sqlc emits `Column9` etc. as `interface{}`. Use those exact field names (you'll see `Column6: nil` in `CreateCampaignParams`). If you want a real typed nullable, restructure the query or add explicit casts.
- **pgtype.UUID**: use `parseUUID(s)` from `internal/http/api/helpers.go` (or the equivalent in `ui/`) and `uuidString(u)` for the inverse. Don't hand-roll.
- **No emojis in code/comments unless the user asks.**
- **No backward-compatibility shims**: this is a fresh repo, just delete or rewrite.
- **CRLF warnings** from git on Windows are normal — `.gitattributes` is not set yet. Ignore them.
- **Errors carry context**: every db error path returns the message via `writeError(w, status, "db error")` for the API or `http.Error` for UI. Don't expose raw `err.Error()` to UI consumers in production paths (it leaks SQL).

---

## Useful commands

```powershell
# Run a single package's tests
go test ./internal/scheduler/... -v

# Inspect the DB
docker exec -i unipile-go-postgres psql -U unipile -d unipile_go

# Tail server logs (when running in background)
# Logs are slog JSON to stdout — pipe through jq if needed

# Run the API end-to-end smoke test
$login = Invoke-RestMethod -Uri http://localhost:8080/api/v1/login -Method POST -Body '{"username":"matias","password":"test123"}' -ContentType "application/json"
$hdrs = @{ Authorization = "Bearer $($login.token)" }
Invoke-RestMethod -Uri http://localhost:8080/api/v1/accounts/acct-demo/campaigns -Headers $hdrs
```

---

## Where to look for prior session decisions

- `PORTING_PLAN.md` has the original 12-phase plan and rationale.
- Recent commits (`git log --oneline -15`) walk through the porting in order: schema → domain → auth → unipile → ai → safety → API → schedulers → UI → extras.
- The user is `matiasbuttari@gmail.com` (git user `mstilde`). They speak Spanish but commit messages are in English. Mirror that.

---

## The original Node.js repo

`C:\Users\Matias\Proyectos\unipileLinkedin\unipile-test\` is the source-of-truth for product behavior. When porting something new, **read the JS file first**, understand exactly what it does (including subtle quirks like timezone handling or PII regex normalization), then write Go that matches. Don't improvise — the user has fine-tuned business rules over months.

But: **do not import code, configs, or credentials from it.** The autonomy constraint at the top of this file is non-negotiable.
