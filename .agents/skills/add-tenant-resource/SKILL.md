---
name: add-tenant-resource
description: >-
  Vertical slice for an organization-scoped (tenant) list resource under
  /t/{slug} and /v1/tenant/*: migration with organization_id, sqlc queries that
  filter by it, Go module behind RequireOrganization + RequireTenantPermission,
  resource meta, io/bulk/search engines with org scope, OpenAPI, and the full
  DataTable list standard in the frontend. Worked example: branches.
---

# Add a tenant resource

The tenant list standard (TEC-669). Worked example everywhere below: **branches**
(`tenant.branches`, TEC-688). Copy its files and rename; do not skip a step.

Design background: `docs/design/tenant-core.md` (§4 permissions, §5 request flow, §8 isolation).

| Layer | Branches example |
|-------|------------------|
| Migration | `backend/migrations/000031_branches.{up,down}.sql` |
| Queries | `backend/internal/database/queries/branches.sql` |
| Lint list | `backend/internal/database/queries/tenant_tables.txt` |
| Module | `backend/internal/modules/branches/` (`model`, `repository`, `usecase`, `handler`, `adapters`, `routes.go`) |
| Wiring | `internal/httpserver/server.go`, `cmd/worker/main.go`, `cmd/search-reindex/main.go` |
| Frontend | `frontend/src/features/branches/`, `frontend/src/app/(tenant)/t/[slug]/branches/` |

## 1. Database (organization isolation)

Follow `.agents/skills/db-migration`. Tenant-specific rules:

- `organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE` on every table.
- **Every index starts with `organization_id`**, uniques included: `UNIQUE (organization_id, uuid)`, codes unique per org among live rows: `CREATE UNIQUE INDEX … (organization_id, code) WHERE deleted_at IS NULL`.
- Child rows: composite FK `(organization_id, parent_id) REFERENCES parent (organization_id, id)` (needs `UNIQUE (organization_id, id)` on the parent) so a child can never point at another organization's parent.
- Soft delete (`deleted_at`) for the main resource; `set_updated_at()` triggers.
- Permission slugs `tenant.<plural>.read|write|export|import|bulk.<action>` seeded in the same migration and granted to the template roles: `owner` + `admin` everything, `member` read (`roles.scope = 'tenant' AND organization_id IS NULL`). The down migration removes the grants, the slugs, then the tables.
- Round-trip: `migrate up` → `down 1` → `up`.

## 2. Queries + repository

- Every query takes `sqlc.arg(organization_id)` and filters on it, **including lookups by uuid**. Add the table to `queries/tenant_tables.txt`; `internal/database/queries_test.go` fails CI when a query on a listed table does not mention `organization_id`.
- List query: `limit`/`offset`, `q` (ILIKE over the searchable columns), whitelisted `sort_field`/`sort_desc` (CASE ordering), nullable filters (`sqlc.narg`). Faceted filters that allow several values use `text[]` + `= ANY(...)` (see `cities`).
- Also add: count, facets (`GROUP BY` for dynamic filter options), uuid list for bulk "all matching", export list, search documents (join `organizations` for the slug).
- Repository methods take `orgID int64` explicitly (`GetBranch(ctx, orgID, uuid)`); they never read the context. Map `pgx.ErrNoRows` → `ErrNotFound` and the unique-index violation → a domain error.

## 3. Module (`internal/modules/<name>`)

- `routes.go`: `RegisterRoutes(mux, h, engines, tokens, loader, resolver)`; nil resolver (tenancy off) mounts nothing. Chain: `Authenticate` → `RequireOrganization(resolver)` → `RequireTenantPermission("tenant.<plural>.read|write")`. **No super_admin bypass** — tenant permissions come from the membership only.
- Static segments (`/meta`, `/facets`, `/export`, `/import`, `/import/sample`, `/bulk`) before `/{uuid}`.
- Handlers read the organization from `orgctx.Must(ctx).ID` and pass it down; the client never sends an organization id in a body or query.
- Another organization's uuid answers **404**, never 403 (do not leak existence).
- Validation: usecase returns field details → `response.ValidationError` (400). Use dot paths matching form fields (`working_hours.mon.close`, `holiday_exceptions.0.date`). Duplicate code → `409 CONFLICT` with a `code` detail (the frontend `AppForm` maps it onto the field).
- Activity: `activity.Recorder.Record(...)` after every mutation, resource = engine slug (`tenant.branches`), `resource_uuid` = the main record (child mutations use the parent uuid + child uuid in the payload). The recorder stamps `organization_id` from the context automatically. Expose `GET /{uuid}/activity` from `ListOrganizationResourceActivity`.
- `GET /v1/tenant/<plural>/meta` returns `resourcemeta.ResourceMeta` (columns, filters, sortable fields, capabilities, bulk actions from the adapter). Capabilities must match mounted routes.

### Organization hooks

Seed data for new organizations through `OrganizationHooks.OnCreated(ctx, tx, org, owner)` (runs inside the create transaction; use `db.New(tx)`). Branches seeds "Merkez" (`MRK`). Pass the hooks to `orgusecase.New(...)` in `server.go`.

### Branch scope (branch-bound resources)

A member's `branch_scope` is `all` or `selected` (TEC-689, migration 000032); `orgctx.Scope.BranchIDs` is nil for `all` and the allowed branch ids otherwise. A resource that belongs to a branch (reservations, vehicles, contracts, …; branches themselves) must apply it everywhere it reads:

- Table: `branch_id BIGINT NOT NULL` + composite FK `(organization_id, branch_id) REFERENCES branches (organization_id, id)` and an index starting with `(organization_id, branch_id)`.
- Queries: every list / count / facets / export / bulk-uuid query gets
  `AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR t.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))`
  (nil = every branch; an empty slice matches nothing).
- Usecase (not the handler, so engine adapters and worker jobs are covered too): set the filter from `orgctx.BranchScope(ctx)` (never from the request); after loading one record call `orgctx.CheckBranch(ctx, row.BranchID)` and answer **404** (`ErrNotFound`) when it fails; actions that need every branch use `orgctx.RequireAllBranches(ctx)` (**403**). Creating a record in a branch outside the scope is a validation error on `branch_uuid` (same answer as an unknown branch).
- Search: `Spec.BranchScoped = true` and `Document.BranchIDs = []int64{branchID}`; `/v1/tenant/search` filters by `branch_ids` for restricted members.
- Jobs: export / import / bulk workers run with `orgctx.WithJobScope` (the requester's current scope); keep adapters reading the organization and branch scope from the context.
- Tests: an RT-A02 style test (`branches/branch_scope_test.go`): member with `selected [A]` gets 404 for B and never sees B in list, facets, export (request and worker context), bulk and search filter.

Branches example: `branchusecase.Service.getBranch`, `ListFilter.BranchIDs`, `adapters.Search` (`BranchScoped`).

Permissions for sensitive actions are separate slugs, granted to roles one by one: `tenant.price.override`, `tenant.discount.apply`, `tenant.payment.collect`, `tenant.payment.refund`, `tenant.damage.approve`, `tenant.signature.waive`, `tenant.document.download`, `tenant.report.export`. When a new resource adds slugs, also grant them to the matching Rentopia template roles (`merkez_yoneticisi` everything; `sube_yoneticisi` read/write of branch-bound resources; `rezervasyon_satis`, `muhasebe`, `teslim_iade` as fits) in the same migration.

## 4. Engines with organization scope

Engine resource slugs start with `tenant.`; `orgctx.EngineScope` refuses a tenant resource without an organization and a platform resource inside one. Jobs are stamped with `organization_id`, the worker re-scopes the context from the job row (`orgctx.WithOrganizationID`), job lookups only match the caller's scope (`orgctx.SameEngineScope`), platform job lists exclude tenant jobs.

Adapters live in the module (`internal/modules/<name>/adapters`) and read the organization with `orgctx.OrganizationID(ctx)` — fail when it is missing. Register each adapter in `server.go` **and** `cmd/worker/main.go` (search also in `cmd/search-reindex/main.go`).

| Engine | Adapter | Routes (mounted by the module via `EngineHandlers`) |
|--------|---------|-------|
| io (`.agents/skills/io-engine`) | `ResourceAdapter` + `SampleProvider`; import creates or updates by natural key, `Previous` snapshot for rollback | `POST /v1/tenant/<plural>/export`, `POST …/import`, `GET …/import/sample` (handlers `exportH.TenantExport`, `importH.TenantUpload`, `importH.TenantSample`); job routes `/v1/tenant/exports/{uuid}[/download]`, `/v1/tenant/imports/{uuid}/…` are shared |
| bulk (`.agents/skills/bulk-engine`) | `BulkAdapter`, permission `tenant.<plural>.bulk.<action>`, `ResolveTargets` for `ids` and `query` inside the org | `POST /v1/tenant/<plural>/bulk` (`bulkH.TenantExecute` checks the action permission in the scope); `/v1/tenant/bulk/{uuid}[/rollback]` shared |
| search (`.agents/skills/search-engine`) | `Spec{Scope: searchengine.ScopeTenant, Permission: "tenant.<plural>.read"}`, document id `searchengine.TenantDocumentID(orgID, uuid)`, `OrganizationID` on the document, href `/t/{slug}/<plural>/{uuid}`; `ListAll` walks organizations | `/v1/tenant/search` + `/v1/tenant/search/specs` (shared, filtered by `organization_id`) |

Use cases call `Indexer.EnqueueUpsert/EnqueueDelete(spec, TenantDocumentID(...))` after mutations (fail-soft).

Backend i18n catalog (`internal/platform/i18n/catalog.go`, tr + en): every ioengine `LabelKey`, `export.title.tenant.<plural>`, `resources.tenant.<plural>`.

### 4a. Live list (realtime)

Open lists refresh themselves from `resource.changed` events (docs/EXTENDING.md "Live lists").

- Channel: `org:{org_uuid}:resource:tenant.<plural>` — `tenantmodule.ChannelResolver` scopes every `tenant.*` resource event by the organization on the context (request scope, or the job's organization id in the worker). Never publish a tenant resource on `resource:{name}`.
- Publish after each HTTP mutation with `resourceevents.Publisher` (`created` / `updated` / `deleted`, uuid = main record; child changes are `updated` of the parent). Branches does it in `handler.record`, so engine adapters that reuse the use case do not publish per row. Bulk and import jobs already publish one `bulk` event.
- Register the read permission: `<name>.RegisterLiveResources(tenantLive)` in `server.go` (`reg.Register("tenant.<plural>", rbac.PermTenant<Plural>Read)`). `tenantmodule.ChannelRule` grants the subscription token only to members of that organization whose tenant role has the permission.
- Test: publication channel + action per mutation and the subscription rule for member / non-member (`branches/live_integration_test.go`).

## 5. OpenAPI

Every new route in `backend/docs/openapi.yaml` (tag `Tenant`, `XOrganization` header parameter, `OrganizationRequired` / `TenantForbidden` responses). Create bodies are full objects (YAML anchor on the patch `properties`), not `allOf` + `required` — that generates an unusable TS type.

```sh
npx --yes @redocly/cli@1 lint backend/docs/openapi.yaml
cd frontend && pnpm api:generate
```

## 6. Frontend list standard (DataTable to the full extent)

`frontend/src/features/<plural>/` — `types.ts` (from `@/generated/api`), `constants.ts`, `services/` (**`tenantRequest(slug, …)` only**), `hooks/` (query keys include the slug), `schemas/`, `components/`, `search/`, `index.ts`. Pages under `app/(tenant)/t/[slug]/<plural>/` (`page`, `create`, `[uuid]`, `[uuid]/edit`). Slug from `useTenant().slug`.

List page checklist (`branches-page.tsx`):

- [ ] `EntityPage` gated by `permissions.tenant.<plural>.read`, forbidden fallback, breadcrumbs to `routes.tenant.home(slug)`
- [ ] `useServerListState` → `limit/offset/sort/q`; column filters mapped to API params (multi-value facets comma-joined; a boolean facet with both values = no filter)
- [ ] `createSelectColumnDef` + `createColumn` with `labelKey`, `enableSorting` only for meta `sortable_fields`, `filterVariant: "faceted"` with options (static enum or the `/facets` endpoint), `defaultHidden` for secondary columns
- [ ] Row actions (`EntityRowActions`, permission per action) and `onRowClick` → show page
- [ ] Quick edit `EntityDrawer` with a small `AppForm` (submit through `form={id}`)
- [ ] `ResourceIOToolbar resource="tenant.<plural>"` with the list query and `meta.capabilities`
- [ ] `useBulkSelection` + `SelectionBanner` + `BulkActionMenu` (`resolveBulkActionsWithIcons`; register icons in `bulk-action-icons.ts`)
- [ ] `EntityToolbar` refresh; empty state that differs with / without filters; loading and error (`EntityTable` handles them)
- [ ] `persistKey` for column visibility / density
- [ ] Live updates: `useServerListState({ realtime: tenantLiveList(organization, "tenant.<plural>") })` (`useTenant().organization`) and `realtime={listState.realtimeFor(refetch)}` on `EntityTable` (refetch list + facets). Backend side: step 4a

Shared engines pick the tenant API automatically: `useApiScope()` returns the organization inside `/t/{slug}`, and the io, bulk and search services take that `ApiScope`. Add the resource to `IoResource`, `EXPORT_PATHS`, `IMPORT_PATHS`, `IMPORT_SCHEMA` (`features/io/types.ts`), the permission maps in `resource-io-toolbar.tsx`, `RESOURCE_LABEL_KEYS` (`features/io/lib/display.ts`), and `BulkResource` / `BULK_PATHS`.

Forms: `AppForm` + zod; dates only through `AppDatePicker` / `DatePicker` (`.agents/skills/admin-date-picker`); nested values (weekly hours, holiday rows) use `useFieldArray` / `Controller` — form primitives resolve nested error paths. Show page: `EntityHeader` + `EntitySectionCard`s (details, domain sections, child sub-list with dialog CRUD, activity timeline).

## 7. Nav, routes, permissions, i18n

1. `routes.tenant.<plural>.{root,create,detail,edit}(slug, …)` in `config/routes.ts`.
2. Item in `tenantNav(slug)` (`config/nav.ts`) gated by `permissions.tenant.<plural>.read` and, when the resource is a sector module, `module: "<module>"` (`.agents/skills/nav-engine`, `docs/design/sectors.md`). Sector words (masa / oda / uzman) come from `useSector().term(...)`, never hard-coded.
3. Permission triad: migration seed + `internal/platform/rbac/rbac.go` + `frontend/src/config/permissions.ts` (`permissions.tenant.<plural>.*`) + `permissions.labels.tenant.<plural>.*` in tr/en.
4. Locales (Turkish primary, en mirror): `locales/{tr,en}/<plural>.json` registered in `lib/i18n/messages.ts`; `layout.nav_*`; `activity.resources.tenant.<plural>` + `activity.actions.*`; `exports/imports.resources.tenant.<plural>`; `bulk.actions.*` / `bulk.confirm.*`; `search.specs_<id>` / `search.prefix_<id>`. Prefer static key maps over template keys.
5. `node scripts/check-i18n.mjs` → 0 errors.

## 8. Tests

- Usecase tests with a fake repository keyed by organization: another org's uuid → not found on get/patch/delete/bulk; code unique per org (same code allowed in another org, reusable after soft delete); child belongs to parent (child through another parent → not found).
- Database tests (`TEST_DATABASE_URL`, skipped otherwise): repository isolation and composite FK, OrganizationHooks seed, adapter round trip, and an HTTP end-to-end test over the real resolver (`branches/integration_test.go`).
- Route test: member without write gets 403, missing header 400, non-member 403.

### Isolation checklist (what a new module must pass)

Two guard rails run in `go test ./...`; a new module is not done until both pass with its routes and queries in them.

1. **Query guard** (`internal/database/queries_test.go`, no database needed):
   - new table → `queries/tenant_tables.txt`. Every query on it references `organization_id`; an aliased reference needs `alias.organization_id` (joins of a second tenant table too). Composite FKs are not a substitute in the query.
   - a table that holds platform and organization rows (organization_id NULL = platform) → `queries/mixed_scope_tables.txt` with its scope markers. A query that legitimately skips the scope (worker by id, lookup followed by `orgctx.SameEngineScope`) goes into `queries/isolation_exempt.txt` **with the reason**.
   - hand-written SQL in Go (pgx `Exec`/`Query` literals) on a tenant table is scanned the same way; prefer sqlc.
2. **RT-A01** (`internal/httpserver/isolation_test.go`, `TEST_DATABASE_URL`): the real server with two organizations; every `/v1/tenant` route is enumerated from the route literals (cross-checked with the mux and `openapi.yaml`). It fails on an uncovered route, so for a new module:
   - seed the resource for both organizations in `seed()` (through the API) and map each new path parameter in `paramKey` (`"<segment>/<param>"` → seed key). Then, automatically: `X-Organization` of a foreign organization → 403; the other organization's uuids in the path (and own parent + foreign child) → **404** with none of its identifiers in the body; every GET without path parameters is scanned for foreign identifiers; B's records read the same afterwards.
   - a mutation without path parameters whose body names records (`*_uuid`, bulk `ids`, bulk `params`) → a case in `cases` with the other organization's uuids (expect 400/404/422, or a bulk run with 0 succeeded); one that names nothing (create with codes only, settings) → `isolationAllowlist` with the reason.
   - list filters taking uuids → a `probes` entry (must answer 200, or a `not_found` detail, and leak nothing); export routes are run and their files scanned automatically.
3. Module rules the tests rely on:
   - every uuid in a body, query or bulk param is resolved with the caller's `organization_id` (unknown → validation `not_found` on that field, never a cross-organization row); bulk params are validated in `ResolveTargets`.
   - object keys live under `organizations/{org_uuid}/` (`storage.*ObjectKey` helpers); streaming endpoints check `storage.OrganizationOwnsKey` before serving a key read from a row.
   - search documents carry `OrganizationID` and `searchengine.TenantDocumentID`.
   - jobs never take an organization id from the client; workers re-scope from the job row.
   - platform lists over mixed tables show platform rows only (`organization_id IS NULL`).
4. Database tests clean up their own rows and never rewrite shared tables: `TRUNCATE ... CASCADE` empties every referencing table under the other packages' tests. Use `dbtest.FreshDatabase(t, dsn)` for a private migrated database when a test needs empty tables.

## 9. Verify

`.agents/skills/verify-changes`, plus `TEST_DATABASE_URL=… go test ./internal/modules/<name>/...` and the migration round trip.

## Do not

- Read the organization from the request body or query, or trust a client-sent id
- Write a query on a tenant table without `organization_id`
- Answer 403 for another organization's record (use 404)
- Call `platformRequest` from a tenant page, or link tenant UI to `/platform/*` job pages
- Expose Meilisearch or assemble S3 URLs in the browser
