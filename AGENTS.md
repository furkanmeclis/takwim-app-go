# Agent entry

Read this first, then the matching skill. Human docs live under `docs/`. Do not invent tenants, workspaces, or a `/business` shell.

Canonical env is the **repo-root** `.env`. Compose files are `compose.local.yml` (infra) and `compose.prod.yml` (full stack).

## Skills (`.agents/skills/`)

| Skill | When |
|-------|------|
| [boilerplate-first-setup](./.agents/skills/boilerplate-first-setup/SKILL.md) | Clone, `.env`, Docker names, Dokploy, `make local-dev` |
| [monorepo-playbooks](./.agents/skills/monorepo-playbooks/SKILL.md) | Cross-app work; repo map |
| [add-platform-resource](./.agents/skills/add-platform-resource/SKILL.md) | New list resource (module → RBAC → OpenAPI → UI → nav) |
| [nav-engine](./.agents/skills/nav-engine/SKILL.md) | Sidebar catalog, badges, info |
| [io-engine](./.agents/skills/io-engine/SKILL.md) | Export / import adapters |
| [bulk-engine](./.agents/skills/bulk-engine/SKILL.md) | Bulk actions, jobs, rollback |
| [search-engine](./.agents/skills/search-engine/SKILL.md) | Cmd+K palette, Meilisearch adapters |
| [storage-media](./.agents/skills/storage-media/SKILL.md) | Object storage, media URLs |
| [github-integration](./.agents/skills/github-integration/SKILL.md) | GitHub App OAuth, encrypted credentials |
| [step-up-engine](./.agents/skills/step-up-engine/SKILL.md) | Re-auth gates (password/passkey) on sensitive actions |
| [admin-date-picker](./.agents/skills/admin-date-picker/SKILL.md) | Any date field in `frontend/` |
| [db-migration](./.agents/skills/db-migration/SKILL.md) | New table/column/seed/query: migration → sqlc → OpenAPI |
| [verify-changes](./.agents/skills/verify-changes/SKILL.md) | Checks to run before commit / PR (mirrors CI) |

## Human docs

| Doc | When |
|-----|------|
| [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) | Shells, request flow, stores, RBAC |
| [docs/DEVELOPMENT.md](./docs/DEVELOPMENT.md) | Local / prod commands |
| [docs/EXTENDING.md](./docs/EXTENDING.md) | How to add a feature (narrative) |
| [backend/API_CONVENTIONS.md](./backend/API_CONVENTIONS.md) | Envelope, lists, meta |
| [backend/DATABASE_RULES.md](./backend/DATABASE_RULES.md) | Migrations + sqlc |
| [backend/docs/auth.md](./backend/docs/auth.md) | JWT, roles, permissions |

## Hard rules

- Browser talks to same-origin `/api/v1/*`, never the Go host.
- Media URLs are API-owned. Never assemble MinIO/S3 keys in the browser.
- Never expose Meilisearch to the browser; search goes through `/v1/search`.
- New `/v1` routes update `backend/docs/openapi.yaml` in the same change.
- Permission slugs are added only via migration + `internal/platform/rbac` + `frontend/src/config/permissions.ts`.
- After OpenAPI changes: `cd frontend && pnpm api:generate`.
