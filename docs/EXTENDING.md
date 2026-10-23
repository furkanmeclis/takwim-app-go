# Extending the boilerplate

Add product modules on top of auth, RBAC, notifications, and the two shells. Do not reintroduce tenants or workspaces unless that is an explicit product decision.

For a full agent checklist, use `.agents/skills/add-platform-resource`. Narrative is here; checklists are in skills.

## Backend module

1. Create `backend/internal/modules/<name>/` with `handler`, `usecase`, `repository`, `model`, and `routes.go`.
2. Register routes from `internal/httpserver/server.go`.
3. SQL: `make -C backend migrate-create NAME=…` → write up + down → `make -C backend migrate-up` → `make -C backend sqlc`.
4. Update `backend/docs/openapi.yaml` in the **same** change. Live Go routes win if the spec still lists a stripped path.
5. Lists use `pkg/apiquery` (`limit`, `offset`, `q`, `sort`). Envelope via `pkg/response`. See [backend/API_CONVENTIONS.md](../backend/API_CONVENTIONS.md).
6. If the resource is listed, add `GET /v1/{resource}/meta` via `internal/platform/resourcemeta` and register `/meta` **before** `/{uuid}`.
7. Side effects: publish on `internal/platform/events` or `notifications.Service.Enqueue`. Never SMTP from a handler.
8. Mutations that matter: `activity.Recorder.Record` (fail-soft).
9. New permission slugs: migration + `internal/platform/rbac` + `frontend/src/config/permissions.ts` + `permissions.labels.*` in `en`/`tr`.

## Frontend feature

1. Add `frontend/src/features/<name>/` (`components`, `hooks`, `services`, `schemas`).
2. Add App Router pages under `(platform)` and/or `(cms)`.
3. Register routes in `frontend/src/config/routes.ts`, sidebar items in `frontend/src/config/nav.ts`, and gate with permissions from `frontend/src/config/permissions.ts`.
4. Call the BFF (`/api/v1/...`), not the Go host, from the browser.
5. Dates: shared `DatePicker` / `AppDatePicker` — see `.agents/skills/admin-date-picker`.
6. After OpenAPI changes: `cd frontend && pnpm api:generate`.

## Sidebar (nav-engine)

Declarative catalog: `frontend/src/config/nav.ts`. Renderer: `frontend/src/features/nav-engine`. Skill: `.agents/skills/nav-engine`.

Canonical live-badge example: `frontend/src/features/users/nav/`.

## Import / export (ioengine)

Package: `internal/platform/ioengine`. HTTP: `internal/modules/{exports,imports}`. Skill: `.agents/skills/io-engine`.

1. Implement `ioengine.ResourceAdapter` in `internal/platform/ioengine/adapters/`.
2. Register in `ioengine.NewRegistry(...)` from `internal/httpserver/server.go` **and** `cmd/worker/main.go`.
3. Set `resourcemeta` `capabilities.export` / `import`. Expose `GET /v1/.../meta`.
4. RBAC: `platform.<resource>.export` / `.import` (migration + rbac + frontend permissions).
5. Mount `POST /v1/platform/<resource>/export` (and import + sample if create-capable).
6. Frontend: `ResourceIOToolbar` from `@/features/io` on the list page.

Formats: export PDF / XLSX / CSV / JSON. Import JSON / XLSX / CSV / TSV. Letterhead comes from `app_settings`. Source files land in MinIO under `imports/{job_uuid}/`.

## Bulk actions (bulkengine)

Package: `internal/platform/bulkengine`. Skill: `.agents/skills/bulk-engine`.

1. Implement `bulkengine.BulkAdapter` in `internal/platform/bulkengine/adapters/`.
2. Register in `bulkengine.NewRegistry(...)` from `httpserver/server.go` and `cmd/worker/main.go`.
3. RBAC: `platform.<resource>.bulk.<action>`.
4. `resourcemeta` `capabilities.bulk = true` and `bulk_actions` from the adapter.
5. Mount `POST /v1/platform/<resource>/bulk`.
6. Frontend: `rowSelection`, `useBulkSelection`, `SelectionBanner`, `BulkActionMenu`.

Jobs above `BULK_SYNC_MAX` (default 50) return **202** and run on Asynq queue `bulk`. Rollback window: `BULK_ROLLBACK_HOURS` (default 24). Undo: `POST /v1/platform/bulk/{uuid}/rollback`.

## Command palette / search (search-engine)

Package: `internal/platform/searchengine`. HTTP: `internal/modules/search`. Skill: `.agents/skills/search-engine`.

1. Implement `searchengine.Adapter` in `internal/platform/searchengine/adapters/`.
2. Register in `searchengine.NewRegistry(...)` from `httpserver/server.go` and `cmd/worker/main.go`.
3. Enqueue `Indexer.EnqueueUpsert` / `EnqueueDelete` after mutations (fail-soft).
4. Frontend: optional `defineSearchSpec` in `frontend/src/features/<name>/search/`; pages come from `nav-engine` automatically.
5. `GET /v1/search?q=&spec=` and `GET /v1/search/specs` — browser uses BFF only; Meilisearch stays on the private network.
6. Reindex: `make search-reindex`.

## GitHub OAuth (integrations)

Skill: `.agents/skills/github-integration`. Super-admin settings at `/platform/integrations/github`.

- Credentials (`client_secret`, private key) encrypted with `APP_ENCRYPTION_KEY` via `internal/platform/crypto`. API never returns plaintext secrets.
- Users **link** GitHub from profile (`GET/DELETE /v1/auth/identities`). Unlinked GitHub accounts cannot self-register.
- NextAuth talks to internal adapter routes (`/v1/internal/auth/oauth/github`) — not in OpenAPI.

## Impersonation

`POST /v1/platform/users/{uuid}/impersonate` (`platform.users.impersonate`, step-up). Stop: `POST /v1/auth/impersonation/stop`. JWT optional claim `imp`. Cannot impersonate yourself or a super admin; stop the current session before starting another.

## Storage / media

Skill: `.agents/skills/storage-media`.

- Object bytes in MinIO/S3; rows in Postgres. Browser never builds object URLs from keys.
- Use only API-returned `public_url` / `logo_url` / stream paths. Helper: `frontend/src/lib/media/urls.ts`.
- Key helpers: `internal/platform/storage/keys.go` (`app/branding/`, `exports/{job}/`, `imports/{job}/`).
- Explorer API lives under `/v1/platform/storage/*` (`platform.storage.read` / `.write`).

## Activity and logs

- **Activity** (who did what): `activity.Recorder.Record(ctx, actorID, action, resource, resourceUUID, payload, req)`. List: `GET /v1/platform/activity`. Permission: `platform.activity.read`.
- New `action` / `resource` strings need `activity.actions.*` and `activity.resources.*` in `frontend/src/locales/{en,tr}/activity.json`, plus an entry in `RESOURCE_LABEL_KEYS`. `make check-i18n` flags gaps.
- **Logs** (application log viewer + purge rules): `/v1/platform/logs`, `/v1/platform/log-rules`. Permissions: `platform.logs.read` / `.write`.
- Do not log secrets (tokens, passwords, VAPID private key, SMTP password).

## Step-up (re-auth)

Sensitive mutations can require a fresh password or passkey confirmation even when the session is valid. Skill: `.agents/skills/step-up-engine`.

- Backend: `middleware.RequireStepUp` after `Authenticate`. `403` + `STEP_UP_REQUIRED` until grant TTL.
- Frontend: `StepUpGate` / `useStepUp()` from `@/features/step-up-engine`.
- Admin TTL/methods: `/platform/access` (`platform.access.read` / `.write`).

## Settings (letterhead)

`/v1/platform/settings` backs PDF/XLSX letterhead (`platform.settings.read` / `.write`). Logo upload goes through storage; the API returns `logo_url`.

## Queue and realtime

- Task names use the `app:` prefix (`app:notification:deliver`, `app:bulk:process`).
- Redis keys should use `app:<env>:…` if you add caching.
- Realtime channels: prefer `user:{user_uuid}`. Issue tokens only through `/v1/realtime/*`.

## What not to copy from old products

This starter was carved out of earlier apps. Do **not** bring back:

- Tenant / workspace / dealer hierarchy
- `/business` shell or JWT `tid` / `wid`
- CRM, inbox, commerce, AI pipeline, tickets, or reports modules
- Leftover locale files such as `tenants.json` / `business.json` as a pattern to extend
- Implementation prompts that referenced other repos

Brand the app via `frontend/src/config/brand.ts`, `NAME_PREFIX`, and root `.env` — not by renaming leftover product strings in comments.
