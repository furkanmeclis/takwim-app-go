---
name: monorepo-playbooks
description: >-
  Repository playbook for this Next.js + Go boilerplate. Use when working
  across frontend, backend, compose, or setup conventions. Maps engines,
  skills, and hard rules.
---

**Persona:** Keep the repo mental model sharp and prefer the nearest app boundary.

# Monorepo map

| Path | Role |
|------|------|
| `frontend/` | Next.js BFF — `/platform` + CMS `/` |
| `backend/` | Go HTTP API + Asynq worker |
| `compose.local.yml` / `compose.prod.yml` | Docker |
| `docs/` | Architecture, development, extending |
| `AGENTS.md` | Skill index |

Canonical env: **repo-root** `.env`. First clone: `.agents/skills/boilerplate-first-setup`.

## Engines (when to load a skill)

| Job | Skill |
|-----|-------|
| New list resource (full slice) | `add-platform-resource` |
| Sidebar item / badge / group | `nav-engine` |
| Export / import | `io-engine` |
| Bulk actions | `bulk-engine` |
| Uploads / media URLs / explorer | `storage-media` |
| Re-auth on sensitive actions | `step-up-engine` |
| Cmd+K / Meilisearch | `search-engine` |
| GitHub OAuth / encrypted secrets | `github-integration` |
| Date field | `admin-date-picker` |

## Working rules

- Prefer the nearest app boundary before widening scope.
- Backend: start from `backend/API_CONVENTIONS.md` and `backend/README.md`.
- Do not treat `.next/` or generated output as source. Commit `backend/internal/database/db` after sqlc.
- New `/v1` routes update `backend/docs/openapi.yaml` in the same change; then `pnpm api:generate`.
- Media URLs: API-owned only; never assemble MinIO keys in the browser.
- Never expose Meilisearch to the browser (`GET /v1/search` only).
- There are no tenants or workspaces in this starter. No `/business` shell. JWT has no `tid` / `wid` (optional `imp` for impersonation).
- Handlers never call SMTP/SMS; enqueue via notifications.
- Permission slugs only via migration + `internal/platform/rbac` + `frontend/src/config/permissions.ts`.

## Platform surfaces (today)

Users, roles, notifications, activity, logs, storage, imports, exports, letterhead settings, access (step-up), GitHub integration — all under `/platform`, gated by `platform.*`. Cmd+K search is session-wide (`auth.session`) with per-spec RBAC.
