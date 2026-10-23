---
name: bulk-engine
description: >-
  Add permission-gated bulk actions to list resources: backend bulkengine
  adapters, async jobs, rollback, and frontend selection scope UX.
---

# Bulk engine

Use when adding batch actions (disable, delete, …) to a DataTable list resource.

Canonical adapters: `backend/internal/platform/bulkengine/adapters/` (users, roles).

## Checklist

1. **Adapter** — `backend/internal/platform/bulkengine/adapters/`:
   - `Resource()` stable slug (`platform.users`)
   - `BulkActions()` with unique `id` per action
   - Permission slug: `platform.<resource>.bulk.<action_id>`
   - `ResolveTargets` for `ids` and `query` scopes
   - `ApplyItem` returns `Previous` for reversible ops
   - `RevertItem` restores from `Previous`
2. **Migration** — seed permission(s); assign to `super_admin`.
3. **RBAC sync** — `backend/internal/platform/rbac/rbac.go` + `frontend/src/config/permissions.ts` + `permissions.labels.*` in `en`/`tr`.
4. **Registry** — register adapter in `internal/httpserver/server.go` **and** `cmd/worker/main.go`.
5. **resourcemeta** — `capabilities.bulk = true`, `bulk_actions` from adapter.
6. **Routes** — add handler in `internal/modules/bulk/routes.go` if new resource path (`POST /v1/platform/<resource>/bulk`).
7. **i18n** — `frontend/src/locales/{en,tr}/bulk.json` action + confirm keys.
8. **Icons** — register `LucideIcon` in `frontend/src/features/bulk-engine/lib/bulk-action-icons.ts` (required per action id).
9. **Activity** — exports/imports/bulk use cases already call `activity.Recorder.Record`. Add `activity.actions.bulk.*` keys in `frontend/src/locales/{en,tr}/activity.json` if you introduce a new action label.
10. **Frontend list page** — `rowSelection: true`, select column, `useBulkSelection`, `SelectionBanner`, `BulkActionMenu` via `resolveBulkActionsWithIcons`.

## Env

- `BULK_SYNC_MAX` — inline threshold (default 50)
- `BULK_ROLLBACK_HOURS` — undo window (default 24)

## API

- `POST /v1/platform/{resource}/bulk` — body: `{ action, target: { scope, ids\|query }, locale }`
- `GET /v1/platform/bulk` — job list (`platform.bulk.read`)
- `GET /v1/platform/bulk/{uuid}` — job status
- `POST /v1/platform/bulk/{uuid}/rollback` — undo within window

Jobs above `BULK_SYNC_MAX` return **202** and run on Asynq queue `bulk`.
