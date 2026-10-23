---
name: add-platform-resource
description: >-
  Vertical slice for a new platform list resource: Go module, sqlc, RBAC,
  OpenAPI, resource meta, Next.js feature, nav, permissions, i18n. Use when
  adding a CRUD/list entity under /platform.
---

# Add a platform resource

Do this in order. Skip engines you do not need; do not skip OpenAPI, RBAC, or `/meta`.

Human narrative: `docs/EXTENDING.md`.

## 1. Database

```sh
make -C backend migrate-create NAME=create_<plural>
# write up + down SQL
make -C backend migrate-up
make -C backend sqlc
```

Commit `backend/internal/database/db`. Follow `backend/DATABASE_RULES.md`.

## 2. Backend module

Create `backend/internal/modules/<name>/` with `handler`, `usecase`, `repository`, `model`, `routes.go`.

Register from `internal/httpserver/server.go`.

Lists: `pkg/apiquery` (`limit`, `offset`, `q`, `sort`). Envelope: `pkg/response`.

`GET /v1/platform/<plural>/meta` via `internal/platform/resourcemeta`. Register `/meta` **before** `/{uuid}`.

## 3. RBAC

Permission slugs (typical):

- `platform.<plural>.read`
- `platform.<plural>.write`

Optional later: `.export`, `.import`, `.bulk.<action>`. After mutations, enqueue search: `Indexer.EnqueueUpsert` / `EnqueueDelete` (see `.agents/skills/search-engine`).

Same change: migration seed (grant `super_admin`) + `internal/platform/rbac/rbac.go` + `frontend/src/config/permissions.ts` + `permissions.labels.*` in `en`/`tr`.

## 4. OpenAPI

Update `backend/docs/openapi.yaml` in the **same** change. Then:

```sh
make -C backend openapi-lint
cd frontend && pnpm api:generate
```

## 5. Frontend feature

`frontend/src/features/<name>/` — `components`, `hooks`, `services`, `schemas`.

App Router page under `frontend/src/app/(platform)/platform/<plural>/`.

Call `/api/v1/...` (BFF), never the Go host.

Dates: `.agents/skills/admin-date-picker`.

## 6. Routes, nav, i18n

1. `frontend/src/config/routes.ts`
2. Sidebar via `.agents/skills/nav-engine` (`frontend/src/config/nav.ts`)
3. Locale keys in `frontend/src/locales/{en,tr}/` (`layout.json` for nav labels)
4. `make check-i18n` — activity actions/resources, permission labels, and `t()` keys must resolve in both locales

## 7. Optional engines

| Need | Skill |
|------|-------|
| Export / import | `.agents/skills/io-engine` |
| Bulk actions | `.agents/skills/bulk-engine` |
| Files | `.agents/skills/storage-media` |
| Re-auth gate | `.agents/skills/step-up-engine` |
| Cmd+K / Meilisearch | `.agents/skills/search-engine` |
| GitHub / OAuth secrets | `.agents/skills/github-integration` |
| Audit row | `activity.Recorder.Record` in the use case (fail-soft) |

## 8. Do not

- Tenants, workspaces, `/business`, JWT `tid` / `wid`
- SMTP/SMS from a handler
- Assemble MinIO/S3 URLs in the browser
- Advertise `capabilities.export|import|bulk` until routes exist
