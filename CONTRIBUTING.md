# Contributing

This is a starter, not a product. Keep the surface small and the conventions strict so the next app can grow cleanly.

## Working rules

- Prefer the nearest app boundary (`backend/` or `frontend/`) before widening scope.
- Canonical env is the repo-root `.env`.
- Do not treat `.next/` or generated output as source. Commit `backend/internal/database/db` after `make -C backend sqlc`.
- New or changed `/v1` routes update `backend/docs/openapi.yaml` in the same change.
- Media URLs are API-owned. Never assemble MinIO/S3 keys in the browser.
- If you change a convention (envelope, RBAC, shells, env), update the matching file under `docs/` or `backend/docs/` in the same change.
- Agent checklists live under `.agents/skills/`. Index: [AGENTS.md](./AGENTS.md).

## Backend

Start from [backend/API_CONVENTIONS.md](./backend/API_CONVENTIONS.md) and [backend/DATABASE_RULES.md](./backend/DATABASE_RULES.md).

```sh
make -C backend test
make -C backend lint
make -C backend openapi-lint
```

## Frontend

```sh
cd frontend
pnpm typecheck
pnpm lint
```

From repo root: `make check-i18n` (missing translation keys). Date fields use the shared pickers (`.agents/skills/admin-date-picker`). Do not add native `<input type="date" />`.

## Docs

Do not add product-specific ADRs here unless you are building that product on a fork.
