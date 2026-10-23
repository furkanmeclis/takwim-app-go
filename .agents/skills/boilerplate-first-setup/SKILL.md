---
name: boilerplate-first-setup
description: >-
  First-time setup for this Next.js + Go boilerplate: root .env, NAME_PREFIX,
  compose local/prod, Dokploy network, VAPID, media URL rules, super admin,
  and make local-dev. Use when initializing the repo, configuring Docker naming,
  or onboarding a new clone.
---

# Boilerplate first setup

Human docs: `docs/DEVELOPMENT.md`. Canonical env is the **repo-root** `.env`.

## Layout

- `backend/` — Go API + worker (`:8080`, `/v1/...`)
- `frontend/` — Next.js BFF (`/platform` + organizations `/t/{slug}`)
- `compose.local.yml` — infra only (Postgres, Redis, MinIO, MailHog, Adminer, Centrifugo, Meilisearch)
- `compose.prod.yml` — full stack on its own app network; frontend + centrifugo also join `dokploy-network`

Do **not** keep a long-lived `frontend/.env.local` for API URLs. Do **not** treat `backend/.env` as canonical.

## 1. Copy env

```sh
cp .env.example .env
# production later:
cp .env.example .env.server   # then replace every secret
```

`API_URL` **must** include `/v1` (default `http://127.0.0.1:8080/v1`).

## 2. Stack identity (required before sharing a host)

| Variable | Purpose |
|----------|---------|
| `NAME_PREFIX` | **Required in prod.** Container, volume and network names (`${NAME_PREFIX}-postgres`, `${NAME_PREFIX}-app`, …) |
| `COMPOSE_PROJECT_NAME` | Compose project name |
| `LOCAL_NETWORK_NAME` | Local bridge |
| `APP_NETWORK_NAME` | Prod private bridge (default `${NAME_PREFIX}-app`) |
| `POSTGRES_VOLUME_NAME` / `REDIS_VOLUME_NAME` / `MINIO_VOLUME_NAME` / `MEILI_VOLUME_NAME` | Named volumes so data does not collide |

In `compose.prod.yml` every internal host is qualified as `<service>.${APP_NETWORK_NAME}` (`postgres.myapp-app`, `backend.myapp-app`, …). Never use a bare service name: on the shared `dokploy-network` it can resolve to another stack's container.

Change secrets in `.env` even for local if the machine is shared: `JWT_*`, `AUTH_SECRET`, `CENTRIFUGO_*`, `S3_SECRET_KEY`, `DB_PASSWORD`, `MEILI_MASTER_KEY`, `APP_ENCRYPTION_KEY`.

## 3. Local run

```sh
make local-dev
make create-super-admin
# optional: make create-super-admin SA_EMAIL=you@example.com SA_PASSWORD='Secret1' SA_NAME=Ada SA_SURNAME=Admin
```

`make local-dev` starts Docker infra, migrates, then **air** + **pnpm dev** on the host. It does **not** build app images.

| Helper | What |
|--------|------|
| `make infra` / `make infra-down` | Compose infra only |
| `make backend-dev` | air on `:8080` |
| `make frontend-dev` | pnpm on `:3000` |
| `make migrate-up` | SQL migrations |
| `make search-reindex` | Rebuild Meilisearch indexes |

### URLs

| Service | URL |
|---------|-----|
| Frontend / login | http://localhost:3000/platform/login |
| API / Scalar | http://127.0.0.1:8080 · `/docs/` |
| Health | `/healthz` · `/readyz` |
| Adminer | http://127.0.0.1:8081 |
| MinIO console | http://127.0.0.1:9001 |
| MailHog | http://127.0.0.1:8025 |
| Meilisearch | http://127.0.0.1:7700 |

Sign-in: `super_admin` / any `platform.*` permission → `/platform`; one organization → `/t/{slug}`; several → `/organizations`; none → `/no-access`.

## 4. Shells & RBAC

- Platform (`/platform`): `super_admin` or any `platform.*` permission
- Organization (`/t/{slug}`): members of that organization
- Account pages (`/profile`, `/organizations`, `/no-access`): any signed-in user
- Public register creates a user with **no** roles
- Assign roles from platform admin
- No tenants / workspaces / `/business`

Brand: `frontend/src/config/brand.ts` + `NAME_PREFIX`.

## 5. VAPID (Web Push)

Required for closed-browser notifications; optional otherwise.

```sh
npx web-push generate-vapid-keys
```

Set `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, `VAPID_SUBJECT` in `.env`.

Frontend: `/app-push-sw.js` → BFF → `/v1/notifications/vapid-public-key` + push-subscriptions.

Optional public vars (already in `.env.example`): `NEXT_PUBLIC_BFF_BASE_URL`, `NEXT_PUBLIC_REALTIME_ENABLED`, `NEXT_PUBLIC_CENTRIFUGO_URL`, `NEXT_PUBLIC_DEFAULT_LOCALE`, `NEXT_PUBLIC_FALLBACK_LOCALE`.

## 6. Media URLs

Browser must **never** assemble MinIO/S3 URLs from object keys. Use API-returned `public_url` / `logo_url`. Guard: `frontend/src/lib/media/urls.ts`. Skill: `.agents/skills/storage-media`.

## 7. Production / Dokploy

1. Fill `.env.server` with a unique `NAME_PREFIX` — every `${VAR:?…}` is required
2. `make prod-config` then `make prod-up`
3. Plain Docker host only: `docker network create dokploy-network` (once; Dokploy creates it itself)

Compose creates the app's own private network (`${NAME_PREFIX}-app`). Only **frontend** and **centrifugo** join `dokploy-network` (set their domains in Dokploy: service + container port). Backend, MinIO, Postgres, Redis, Meilisearch and the worker stay private.

`SEARCH_*` / `MEILI_*` enable Cmd+K. `APP_ENCRYPTION_KEY` (32-byte raw or base64) encrypts GitHub App secrets. Generate: `openssl rand -base64 32`.

## 8. After clone checklist

- [ ] Unique `NAME_PREFIX` + volume names on a shared Docker host
- [ ] `API_URL` ends with `/v1`
- [ ] `make local-dev` then `make create-super-admin`
- [ ] Login at `/platform/login`
- [ ] MailHog receives verify/reset mail
- [ ] VAPID keys if push is in scope
- [ ] `APP_ENCRYPTION_KEY` replaced if GitHub OAuth will be used
- [ ] Brand tokens swapped if this is a named product
