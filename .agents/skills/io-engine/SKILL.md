---
name: io-engine
description: >-
  Add export/import to a list resource via ioengine adapters, Asynq jobs,
  letterhead, and the frontend ResourceIOToolbar. Use when adding CSV/XLSX/PDF/JSON
  export, import, mapping, preview, or rollback.
---

# I/O engine

Use when a list resource needs export and/or import.

Canonical example: `backend/internal/platform/ioengine/adapters/` (users, roles).
Frontend toolbar: `ResourceIOToolbar` from `@/features/io`.

## Checklist

1. **Adapter** — implement `ioengine.ResourceAdapter` in `backend/internal/platform/ioengine/adapters/`:
   - `Resource()` — stable slug, e.g. `platform.orders`
   - `ExportColumns()` + `Export(ctx, query, locale)` — honor list filters (`q`, `sort`, enums)
   - `ImportSchema()` + `ApplyRow` + `RevertRow` — skip import for read-only resources (return empty schema / error)
2. **Registry** — pass the adapter into `ioengine.NewRegistry(...)` in **both** `internal/httpserver/server.go` and `cmd/worker/main.go`.
3. **Migration** — seed `platform.<resource>.export` (and `.import` if needed); assign to `super_admin`.
4. **RBAC sync** — `internal/platform/rbac/rbac.go` + `frontend/src/config/permissions.ts` + `permissions.labels.*` in `en`/`tr`.
5. **resourcemeta** — `capabilities.export` / `import`; keep `GET /v1/.../meta`.
6. **Routes** — add `POST /v1/platform/<resource>/export` in `internal/modules/exports/routes.go`. For import: `POST .../import`, `GET .../import/sample`, plus shared job routes under `/v1/platform/imports/{uuid}`.
7. **Activity** — export request / import apply / rollback already go through the exports/imports use cases (`activity.Recorder.Record`). Do not SMTP from the adapter.
8. **Frontend**
   - Add the slug to `IoResource` and perm maps in `frontend/src/features/io/components/resource-io-toolbar.tsx`
   - Mount `<ResourceIOToolbar resource="platform.…" query={listQuery} capabilities={meta.capabilities} />` on the list page
   - i18n: `frontend/src/locales/{en,tr}/exports.json` and `imports.json` as needed

## Formats

| Direction | Formats |
|-----------|---------|
| Export | `pdf`, `xlsx`, `csv`, `json` |
| Import | `json`, `xlsx`, `csv`, `tsv` |

Letterhead for PDF/XLSX comes from `app_settings` (`/v1/platform/settings`). Logo bytes come from storage; never from a browser-assembled MinIO URL.

Import source files: MinIO key `imports/{job_uuid}/source.{ext}` (`internal/platform/storage/keys.go`). Rollback rows: `import_changes`.

## API (existing resources)

- `POST /v1/platform/users/export` · `/roles/export` · `/notifications/export` · `/activity/export`
- `POST /v1/platform/users/import` + `GET .../import/sample` (same for roles)
- `GET /v1/platform/exports` · `GET /v1/platform/exports/{uuid}` · `GET .../download`
- `GET /v1/platform/imports` · mapping / preview / confirm / rollback on `{uuid}`

Jobs return **202**. Worker processes Asynq queues `exports` / `imports`.
