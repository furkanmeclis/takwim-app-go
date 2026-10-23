---
name: search-engine
description: >-
  Cmd+K command palette and Meilisearch-backed record search: backend
  searchengine adapters, compose service, frontend catalog, and RBAC-filtered
  specs. Use when adding searchable resources or extending the command palette.
---

# Search engine

Use when adding **record search** to the Cmd+K palette or wiring a new resource into Meilisearch.

## Backend adapter

1. Implement `searchengine.Adapter` in `backend/internal/platform/searchengine/adapters/`.
   - `Spec()` — `id`, `label_key` (`search.specs_<id>`), `permission`, `icon`, `searchable_fields`
   - `ListAll` — full reindex source
   - `Document` — single entity by UUID
   - Never index secrets (passwords, tokens, SMTP, VAPID).
2. Register in `searchengine.NewRegistry(...)` from `internal/httpserver/server.go` **and** `cmd/worker/main.go`.
3. After create/update/delete in the owning use case, call `Indexer.EnqueueUpsert` / `EnqueueDelete` (fail-soft).
4. OpenAPI: generic `/v1/search` + `/v1/search/specs` already cover new specs; no new routes per resource unless you add custom behavior.

## Frontend spec (optional)

Remote specs are listed by `GET /v1/search/specs`. For local metadata/icons, export from the feature:

```ts
// frontend/src/features/users/search/index.tsx
export const usersSearchSpec = defineSearchSpec({
  id: "users",
  labelKey: "search.specs_users",
  icon: Users,
});
```

Pages/navigation items are built from `config/nav.ts` automatically (permission-filtered).

## Compose + env

- Local: `compose.local.yml` → Meilisearch on `${MEILI_PORT:-7700}`
- Prod: private `app` network only (not `dokploy-network`)
- Root `.env`: `SEARCH_ENABLED`, `MEILI_HOST`, `MEILI_MASTER_KEY`, `MEILI_INDEX_PREFIX`

## Ops

```sh
make search-reindex          # full rebuild (users + roles + future adapters)
```

Boot: worker/API call `Indexer.Bootstrap` when an index is empty.

## RBAC

- Palette open: any session (`auth.session`)
- Spec chips + hits: filtered by each adapter's `permission` (e.g. `platform.users.read`)
- Super admin sees all registered specs

## Do not

- Expose Meilisearch to the browser
- Use `search` query param (use `q`)
- Index logs/activity in v1 (noisy + sensitive)
