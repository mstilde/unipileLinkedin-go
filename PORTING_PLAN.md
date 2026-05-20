# Plan de Portado: Node/Express → Go

Documento que captura el plan global para clonar `unipileLinkedin/unipile-test`
(~85k LOC en Node) a este repo Go.

## Principio rector

**Sistema autónomo.** Este clon no comparte:

- Cuenta Unipile (DSN, API key)
- Claves de IA (Anthropic, OpenAI)
- Base de datos
- JWT secret
- Webhooks
- Deploy/dominio

Todas las credenciales son nuevas y se cargan vía `.env` separado.

---

## Sistema original — mapa de entendimiento

### Backend Node/Express
- ~150 endpoints HTTP en `server.js` (10.9k LOC).
- 13 grupos de features: auth, accounts, inbox, campaigns, templates, LinkedIn
  search, safety, follow-ups, AI ops, admin, sync, config, multimedia.
- 3 schedulers en intervalos (`setInterval`):
  - `campaign-scheduler` 15min — pasos, ramp-up, A/B promote
  - `ai-queue-worker` 30s — semáforo de 5 concurrentes
  - `follow-up-scheduler` 15min — working hours + TZ
- 3 webhooks Unipile: messages, relations, account.status (dedup en
  `webhook_dedup`).
- ~60 módulos `lib/` + 8 `ai/`.

### DB
- ~40 tablas en 11 dominios. Multi-tenancy por `account_id` (TEXT).
- RLS solo en `prospect_funnel_events`.
- Triggers: `safety_defaults` + `seed_account_daily_limits`.
- Vista materializada: `campaign_metrics`.

### Sequence-builder UI
- Slots fijos: `invite → wait → msg_1 → wait → msg_2 → wait → msg_3`.
- 13 tipos de paso. Template engine con variables, género, spintax,
  condicionales. Delay engine TZ-aware con holidays.

---

## Stack del clon

| Capa | Elección |
|---|---|
| Router | chi v5 |
| Templating | templ |
| Interactividad | HTMX 2.x + Alpine.js mínimo |
| ORM | sqlc |
| Migrations | goose |
| Postgres | 16 (Docker local) |
| Validation | go-playground/validator/v10 |
| Auth | golang-jwt/jwt/v5 + bcrypt |
| Config | env + godotenv |
| Logging | log/slog (stdlib) |
| HTTP client (Unipile) | net/http puro |
| Anthropic | anthropics/anthropic-sdk-go |
| OpenAI | openai/openai-go |
| Realtime UI | SSE |
| Tests | testing + testcontainers-go |

---

## Fases (las 12 tareas del tracker)

| # | Fase | Estado |
|---|---|---|
| 1 | Plan + arquitectura | ✅ |
| 2 | Scaffolding repo + go.mod + Makefile | 🔄 en curso |
| 3 | Migrations DB (goose) | ⬜ |
| 4 | Dominio core + tests (template, delay, sequence-validator) | ⬜ |
| 5 | Auth (JWT) + middleware base + ownership | ⬜ |
| 6 | API endpoints campañas + sequence CRUD | ⬜ |
| 7 | UI sequence-builder en templ + HTMX | ⬜ |
| 8 | Schedulers (campaign + follow-up + ai-queue) | ⬜ |
| 9 | Wrapper Unipile HTTP + actions + webhooks | ⬜ |
| 10 | Integración IA (Anthropic + OpenAI) | ⬜ |
| 11 | UI resto (inbox + métricas + admin + onboarding) | ⬜ |
| 12 | Features secundarios (safety, A/B, search, hybrid) | ⬜ |

## Lo que NO se porta (o se decide después)

- 3 edge functions de Supabase (Deno/TS) → endpoints normales en Go.
- Socket.io → SSE.
- ngrok integrado → manual.
- WhatsApp linked accounts → fuera de scope si el clon es solo LinkedIn.
- Imports de LinkedHelper en formato propietario → fase tardía.
- Cualquier código en `_archive/` del repo original.

---

## Notas

- El repo original sigue siendo la fuente de verdad para entender
  comportamiento sutil (template rendering, delay calc, classify). Cuando
  haya dudas durante el porte, leer el archivo correspondiente en
  `C:\Users\Matias\Proyectos\unipileLinkedin\unipile-test\`.
- Cada fase deja el sistema en estado funcional incremental (no
  half-finished).
- Tests verdes son condición para marcar una fase completa, no opcional.
