---
name: verify-changes
description: >-
  Run the same checks CI runs (backend vet/test/lint, OpenAPI lint, frontend
  typecheck/lint/build) before committing or opening a PR. Use before saying a
  change is done.
---

# Verify changes

CI (`.github/workflows/ci.yml`) runs on every branch push. Run the matching subset locally first.

## Backend (any `backend/**` change)

```bash
cd backend
gofmt -l .            # must print nothing
go build ./...
go vet ./...
go test ./...
make openapi-lint     # when backend/docs/openapi.yaml changed
```

`make lint` runs golangci-lint (needs `make tools`).

## Frontend (any `frontend/**` change)

```bash
cd frontend
pnpm api:generate     # after OpenAPI changes; commit src/generated/api.d.ts
pnpm typecheck
pnpm lint
pnpm format:check
pnpm build            # slow; run before a PR
```

Stale `.next/types` after deleting a route: `rm -rf .next/types` and typecheck again.

## Database

New migration → round-trip it (see the `db-migration` skill).

## Prod compose

```bash
docker compose --env-file <filled env> -f compose.prod.yml config >/dev/null
```

`NAME_PREFIX` must be set; internal hosts stay `<service>.<app network>`.

## Shipping

Push a feature branch: when backend + frontend pass, the `auto-pr` job opens a PR into `main` and merges the tested commit. Put `[no-automerge]` in the head commit message to keep the PR open for review.
