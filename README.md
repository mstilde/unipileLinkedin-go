# unipile-linkedin-go

Clon en Go del sistema de automatización LinkedIn (originalmente en Node.js).
Sistema **autónomo** — cuenta Unipile, claves de IA y base de datos completamente separadas del repo original.

## Stack

- **Go** 1.26+
- **chi** (router HTTP)
- **templ + HTMX** (UI server-side)
- **sqlc** (SQL → Go tipado)
- **goose** (migrations)
- **Postgres 16** (Docker local)
- **Anthropic + OpenAI SDKs** (Go oficiales)
- Cliente Unipile HTTP propio (no hay SDK Go oficial)

## Estado actual

Scaffolding inicial. Ver [`PORTING_PLAN.md`](PORTING_PLAN.md) para el plan completo y fases.

## Quick start (cuando esté listo)

```bash
# 1. Levantar Postgres local (requiere Docker Desktop)
docker compose up -d

# 2. Copiar env y completar
cp .env.example .env
# editar .env con tus credenciales

# 3. Migrar DB
make migrate

# 4. Correr
make run
```

Por ahora solo:

```bash
go run ./cmd/server
# -> http://localhost:8080/health
```

## Layout

```
cmd/server/          entrypoint
internal/
  config/            carga .env
  auth/              JWT + middleware
  db/                migrations, queries, sqlc gen
  domain/            tipos + validators (campaign, sequence, template, delay, safety)
  unipile/           cliente HTTP + actions + webhooks
  ai/                classify, adapt, enrich, reply, queue
  scheduler/         3 schedulers en goroutines
  http/              routes, middleware, handlers, views (templ)
  pkg/               utils reusables
static/              css, htmx, alpine
templates/           *.templ
migrations/          *.sql (goose)
```

## Nombre del módulo

`github.com/mstilde/unipile-linkedin-go` — si lo querés publicar bajo otro path,
hacé find/replace global del module path antes del primer push.
