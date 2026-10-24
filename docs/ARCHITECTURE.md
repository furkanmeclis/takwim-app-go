# Architecture

This repo is a two-app monorepo plus Compose files. Dependencies point inward: HTTP handlers and Next.js pages call use cases / services; they do not own SQL or SMTP.

## Layout

```
.
├── backend/                      Go API + worker
│   ├── cmd/                      server, worker, create-super-admin, demo-seed
│   ├── internal/modules/         auth, notifications, exports, imports,
│   │                             bulk, settings, activity, logs, storage,
│   │                             access, search, integrations
│   ├── internal/platform/        jwt, rbac, events, storage, mail, outbox,
│   │                             ioengine, bulkengine, activity, resourcemeta,
│   │                             stepup, searchengine, crypto
│   ├── migrations/               sequential golang-migrate SQL
│   └── docs/openapi.yaml         OpenAPI 3.1 (Scalar at /docs/)
├── frontend/                     Next.js BFF
│   ├── src/app/(platform)        /platform admin
│   ├── src/app/(tenant)          /t/{slug} organization shell
│   ├── src/app/(account)         / (home router), profile, organization picker
│   ├── src/app/(guest)           login, reset, verify
│   ├── src/app/api/v1            BFF proxy → Go
│   └── src/features/             users, roles, notifications, io, bulk-engine,
│                                 storage, logs, nav-engine, step-up-engine,
│                                 search-engine, access, integrations, account, auth
├── compose.local.yml             infra only
└── compose.prod.yml              full stack
```

## Shells

| Surface | Path | Who |
|---------|------|-----|
| Platform | `/platform` | `super_admin` or any `platform.*` permission |
| Organization | `/t/{slug}` | members of that organization (`tenant.*` permissions from the membership role) |
| Account | `/profile`, `/organizations`, `/no-access` | any signed-in user |
| Guest | `/platform/login` (aliases also exist at `/login`) | signed-out users |

After sign-in `next` wins; otherwise platform users land on `/platform`, users with one organization on `/t/{slug}`, users with several on `/organizations`, and everyone else on `/no-access` (`defaultHomeForUser`). There is no workspace or `/business` shell. JWT claims are `sub`, `roles`, `is_super_admin`, `exp`, optional `imp` (impersonator) — no `tid` / `wid`.

## Request flow

```
Browser
  → same-origin /api/v1/*          Next.js BFF (HttpOnly session)
  → $API_URL/...                   Go API (Bearer JWT)
  → use case → repository (sqlc)   PostgreSQL
```

- Access and refresh tokens never enter JavaScript. The BFF stores them in HttpOnly cookies and strips them from JSON responses.
- NextAuth is the frontend session layer; the Go adapter talks to `/v1/internal/auth/*`.
- OpenAPI types are generated into `frontend/src/generated/api.d.ts` via `pnpm api:generate`.

## Backend layers

```
handler → usecase → repository interface → sqlc/pgx → PostgreSQL
```

Cross-module signals go through `internal/platform/events` (in-process, fail-soft). Handlers never call SMTP or SMS; they enqueue via the notification service (`app:notification:deliver` on the `notifications` queue).

Notifications of an organization (tenant export / import / bulk jobs) carry `EnqueueInput.OrganizationID` (`notifications.organization_id`). The inbox is scoped by shell: `/v1/tenant/notifications*` (with `X-Organization`) shows that organization's rows plus un-stamped ones, `/v1/notifications*` only un-stamped ones; live cards are filtered the same way by `organization_uuid`. A finished tenant export links to `/t/{slug}/exports/{uuid}/download`, a tenant page that downloads through `/v1/tenant/exports/{uuid}/download` (a link cannot carry the organization header).

Audit: `activity.Recorder.Record` (fail-soft). Application logs: `internal/modules/logs`.

## Platform engines

| Engine | Package | Job |
|--------|---------|-----|
| Resource meta | `internal/platform/resourcemeta` | `GET /v1/.../meta` — columns, filters, capabilities |
| I/O | `internal/platform/ioengine` | Export / import adapters; jobs on Asynq `exports` / `imports` |
| Bulk | `internal/platform/bulkengine` | Batch mutations; jobs on Asynq `bulk` |
| Search | `internal/platform/searchengine` | Meilisearch indexing + `GET /v1/search` for Cmd+K |
| Storage | `internal/platform/storage` + `modules/storage` | Object bytes in MinIO/S3; metadata in Postgres |
| Activity | `internal/platform/activity` | Audit events for mutations and I/O |
| Step-up | `internal/platform/stepup` | Re-auth grant (password/passkey) before sensitive routes |
| Secrets | `internal/platform/crypto` | AES-256-GCM box for GitHub App credentials |

Adapters register in `internal/httpserver/server.go` and `cmd/worker/main.go`.

## Data stores

| Store | Role |
|-------|------|
| PostgreSQL 18 | Source of truth |
| Redis | Cache, Asynq, ephemeral state |
| MinIO / S3 | Object bytes; metadata stays in Postgres |
| Meilisearch | Cmd+K record search; HMAC/master key stays on the API |
| Centrifugo | Websocket fan-out; HMAC stays on the API |

Media: the browser must only use API-returned `public_url` / stream paths. See `frontend/src/lib/media/urls.ts`.

## RBAC

Users are global. Permissions are the union of assigned roles.

| Seed role | Notes |
|-----------|--------|
| `super_admin` | System role (`is_system=true`). Created by `make create-super-admin`, not a user column. |
| `cms_user` | `auth.session` + `notifications.read` |

Custom roles and permission assignment live under `/v1/platform/roles`. New permission slugs are added only via migrations. Details: [backend/docs/auth.md](../backend/docs/auth.md).
