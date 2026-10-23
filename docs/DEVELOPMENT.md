# Development

## Prerequisites

- Go 1.22+ (match `backend/go.mod`)
- Node 20.9+ and pnpm
- Docker + Compose
- golang-migrate, sqlc, air, golangci-lint, gofumpt — see [backend/TOOLCHAIN.md](../backend/TOOLCHAIN.md)

## First clone

```sh
cp .env.example .env
```

Set these **before** sharing a Docker host with another stack:

| Variable | Purpose |
|----------|---------|
| `NAME_PREFIX` | **Required in prod.** Container, volume and network names (`${NAME_PREFIX}-postgres`, `${NAME_PREFIX}-app`, …) |
| `COMPOSE_PROJECT_NAME` | Compose project name |
| `LOCAL_NETWORK_NAME` | Local bridge |
| `APP_NETWORK_NAME` | Prod private bridge (default `${NAME_PREFIX}-app`) |
| `*_VOLUME_NAME` | Named volumes so MinIO/Postgres do not collide |

In `compose.prod.yml` every internal host is qualified as `<service>.${APP_NETWORK_NAME}` (`postgres.myapp-app`, `backend.myapp-app`, …). Never use a bare service name: on the shared `dokploy-network` it can resolve to another stack's container.

Root `.env` is canonical. `backend/.env` is a fallback only. Do not keep a long-lived `frontend/.env.local` for API URLs; `API_URL` comes from the root env.

`API_URL` **must** include `/v1` (Go routes are `/v1/...`). Default: `http://127.0.0.1:8080/v1`.

## Local

```sh
make local-dev
make create-super-admin
```

Helpers:

| Target | What it does |
|--------|----------------|
| `make infra` / `make infra-down` | Compose infra only |
| `make backend-dev` | air on `:8080` |
| `make frontend-dev` | `pnpm dev` on `:3000` |
| `make migrate-up` | Apply SQL migrations |
| `make create-super-admin` | Upsert platform admin + `super_admin` role |
| `make search-reindex` | Rebuild Meilisearch indexes (users, roles, …) |
| `make check-i18n` | Find missing / unwired translation keys (en+tr, activity, RBAC, exports) |

### Local URLs

| Service | URL |
|---------|-----|
| Frontend | http://localhost:3000 |
| API | http://127.0.0.1:8080 |
| Scalar docs | http://127.0.0.1:8080/docs/ |
| Health / ready | http://127.0.0.1:8080/healthz · `/readyz` |
| Adminer | http://127.0.0.1:8081 |
| MinIO console | http://127.0.0.1:9001 |
| MailHog | http://127.0.0.1:8025 |
| Meilisearch | http://127.0.0.1:7700 |
| Centrifugo WS | ws://127.0.0.1:8000/connection/websocket |

Login: http://localhost:3000/platform/login

## Branding

Swap name and colors in `frontend/src/config/brand.ts` and the CSS tokens in the frontend theme. Default product name is **App**.

## VAPID (optional Web Push)

```sh
npx web-push generate-vapid-keys
```

Set `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, and `VAPID_SUBJECT` in `.env`. The frontend registers via `/app-push-sw.js` → BFF → `/v1/notifications/vapid-public-key` + push-subscriptions.

Closed-browser push does not work until these three are set.

Optional frontend public vars (root `.env`): `NEXT_PUBLIC_BFF_BASE_URL`, `NEXT_PUBLIC_REALTIME_ENABLED`, `NEXT_PUBLIC_CENTRIFUGO_URL`, `NEXT_PUBLIC_DEFAULT_LOCALE`, `NEXT_PUBLIC_FALLBACK_LOCALE`.

## Production / Dokploy

1. `cp .env.example .env.server`, set a unique `NAME_PREFIX` and replace every secret (`${VAR:?…}` is required)
2. `make prod-config` then `make prod-up`
3. Plain Docker host only (Dokploy creates it for you): `docker network create dokploy-network` once

Networks: Compose creates a private bridge network per app (`${APP_NETWORK_NAME:-${NAME_PREFIX}-app}`) and every service joins it. Only **frontend** (`:3000`) and **centrifugo** (`:8000`) also join the shared `dokploy-network`, so the proxy can route their domains. The backend is reached through the frontend's `/api` proxy, and MinIO, Postgres, Redis, Meilisearch and the worker stay private (files reach browsers through API endpoints, never a bucket URL). No service publishes host ports.

`APP_ENCRYPTION_KEY` (32-byte, raw or base64) encrypts GitHub App secrets at rest. Generate: `openssl rand -base64 32`. Replace the example key in production.

## Error tracking (optional, Sentry protocol)

The tracker is a self-hosted, Sentry-protocol endpoint (events only; it has no `sentry-cli` API, so nothing is uploaded at build time: no source maps, no releases, no auth token). Every variable is optional; an empty DSN turns reporting off.

| Env (Dokploy / `.env.server`) | Passed as | Read by |
|---|---|---|
| `BACKEND_SENTRY_DSN` | `SENTRY_DSN` | backend API + worker |
| `FRONTEND_SENTRY_DSN` | `SENTRY_DSN` | Next.js server, edge, browser |
| `SENTRY_ENVIRONMENT` (default `production`) | same | both |
| `SENTRY_TRACES_SAMPLE_RATE` (default `0`) | same | backend only |

Local dev reads `SENTRY_DSN` from the root `.env` directly.

- **Backend**: 5xx responses and panics (route pattern, status, `request_id` tag), auth misconfiguration warnings (adapter secret mismatch, provider disabled, id_token audience/issuer), failed queue tasks (`task_type` tag), and `ERROR` log lines (one event per message per minute). Requests the client cancelled are logged as `499` at INFO and never reported. One `http_request` INFO line is written per 4xx and per `/v1/auth/*` / `/v1/internal/auth/*` request (method, route, status, duration, request_id, error_code, scrubbed reason).
- **Frontend**: `instrumentation.ts` (server/edge, `onRequestError`), `instrumentation-client.ts` (browser), `global-error.tsx`, route error boundaries, the `/api/v1` BFF proxy, and the NextAuth `logger.error` hook (user errors like `CredentialsSignin` are logged, not reported). The browser gets its DSN at runtime from `/api/client-env` (loaded `beforeInteractive` in the root layout), so the image needs no build arg.
- **Never sent**: request bodies, cookies, query strings, `Authorization` / adapter headers, tokens, secrets, emails, phone numbers. The user is the internal user uuid only. Code: `backend/internal/observability`, `frontend/src/lib/observability`.

## Super admin

From the repo root (loads `.env`):

```sh
make create-super-admin
make create-super-admin SA_EMAIL=you@example.com SA_PASSWORD='Secret1' SA_NAME=Ada SA_SURNAME=Admin
```

The same target exists under `backend/` (`make -C backend create-super-admin …`).

## After a backend contract change

```sh
make -C backend sqlc
make -C backend openapi-lint
cd frontend && pnpm api:generate && pnpm typecheck
```
