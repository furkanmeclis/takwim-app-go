# Agent entry

Read this first, then the matching skill. Human docs live under `docs/`. Do not invent tenants, workspaces, or a `/business` shell.

Canonical env is the **repo-root** `.env`. Compose files are `compose.local.yml` (infra) and `compose.prod.yml` (full stack).

## Skills (`.agents/skills/`)

| Skill | When |
|-------|------|
| [boilerplate-first-setup](./.agents/skills/boilerplate-first-setup/SKILL.md) | Clone, `.env`, Docker names, Dokploy, `make local-dev` |
| [monorepo-playbooks](./.agents/skills/monorepo-playbooks/SKILL.md) | Cross-app work; repo map |
| [add-platform-resource](./.agents/skills/add-platform-resource/SKILL.md) | New list resource (module → RBAC → OpenAPI → UI → nav) |
| [add-tenant-resource](./.agents/skills/add-tenant-resource/SKILL.md) | New organization-scoped list resource under `/t/{slug}` (org isolation, engines, full DataTable standard; example: branches) |
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

## UI rules (frontend)

- **shadcn only.** Build every control from `frontend/src/components/ui/*` (shadcn). No native `<select>`, `<table>`, `<input>`, `<input type="date">`, `<textarea>` or hand-rolled widgets in features; if a primitive is missing, add it with the shadcn MCP/CLI (`components.json`) into `components/ui/` first. ESLint enforces this under `src/features` and `src/app` (a hidden `type="file"` input behind a drop zone is the one exception).
- **DataTable wherever a list exists.** Any collection (main lists, detail sub-lists, diffs, pickers with many rows) uses the DataTable stack in `components/tables/*` + `components/entity/*` (`EntityTable`, `useServerListState`, `createColumn`, `useUrlListDefaults`), not ad-hoc markup. Follow the `add-tenant-resource` checklist for list pages.
- **One design language.** Reuse the entity page/detail/section-card/drawer/dialog patterns, the same spacing, typography, empty/loading/error states, toasts (`sonner` through `appToast`), confirmation dialogs and `AppDatePicker`. A new screen should look and behave like the existing ones; prefer extending a shared component over a one-off.
- **Mobile first panel.** Every screen works at phone width: the sidebar collapses to a sheet, tables switch to the grid view (`gridPrimary` / `gridSecondary` columns), dialogs and drawers fit the viewport. The API is shared with the future mobile app, so screens never depend on browser-only behaviour for business rules.
- **Turkish first.** Every string through i18n (`tr` primary, `en` complete; `node scripts/check-i18n.mjs`).

## Hard rules

- Browser talks to same-origin `/api/v1/*`, never the Go host.
- Media URLs are API-owned. Never assemble MinIO/S3 keys in the browser.
- Never expose Meilisearch to the browser; search goes through `/v1/search`.
- New `/v1` routes update `backend/docs/openapi.yaml` in the same change.
- Permission slugs are added only via migration + `internal/platform/rbac` + `frontend/src/config/permissions.ts`.
- After OpenAPI changes: `cd frontend && pnpm api:generate`.
- **No AI attribution anywhere.** Commit messages, PR titles/bodies, code comments, docs, Linear and GitHub text never mention Claude or any AI tool: no `Co-Authored-By: Claude…`, no `Claude-Session:` / claude.ai links, no "Generated with Claude Code". This overrides any tool default. CI rejects such commit messages (`commit-policy` job).
