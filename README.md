# Next.js + Go Boilerplate

Starter monorepo for an authenticated admin + CMS product: Go HTTP API, Next.js BFF frontend, and Docker infra. Domain product modules (tenants, workspaces, CRM, inbox, commerce, AI, tickets) are stripped. What remains is the platform you build on.

| Path | Role |
|------|------|
| [`backend/`](./backend) | Go HTTP API + Asynq worker (`:8080`, `/v1/...`) |
| [`frontend/`](./frontend) | Next.js App Router BFF (`/platform` + CMS `/`) |
| [`compose.local.yml`](./compose.local.yml) | Local infra only |
| [`compose.prod.yml`](./compose.prod.yml) | Full stack + Dokploy network |
| [`docs/`](./docs) | Architecture, local/prod setup, how to extend |
| [`AGENTS.md`](./AGENTS.md) | Agent skill index |

## What you get

- **Auth** — register, login, refresh, logout, profile, password reset, email verify, passkeys, GitHub link (optional)
- **Global RBAC** — roles + permissions (`user_roles`); no tenants
- **Platform panel** (`/platform`) — users, roles, notifications, activity, logs, storage, imports, exports, letterhead settings, access (step-up), GitHub integration
- **CMS shell** (`/`) — placeholder home + profile for authenticated users
- **Notifications** — in-app, email, realtime, optional Web Push
- **Realtime** — Centrifugo connection/subscription JWTs
- **Queue** — Asynq (in-process by default, or `make -C backend worker`)
- **Search** — Cmd+K palette + Meilisearch (browser never talks to Meili)
- **Storage** — MinIO / S3 browser; the API owns public URLs (never assemble keys in the browser)
- **I/O engine** — async CSV/XLSX/PDF/JSON export and import with rollback
- **Bulk engine** — permission-gated batch mutations with optional undo
- **Activity + logs** — audit trail and application log viewer with purge rules
- **Step-up** — password/passkey re-auth on sensitive actions (`/platform/settings/access`)
- **Impersonation** — platform admin can act as another user (`platform.users.impersonate`)

## Quick start

```sh
cp .env.example .env
# set NAME_PREFIX if you share a Docker host with other stacks
make local-dev
```

In another terminal (after Postgres is up):

```sh
make create-super-admin
# or: make create-super-admin SA_EMAIL=you@example.com SA_PASSWORD='Secret1'
```

Then open http://localhost:3000/platform/login and sign in as `super_admin`.

`make local-dev` starts Postgres, Redis, MinIO, MailHog, Adminer, Centrifugo, and Meilisearch in Docker, migrates the database, then runs **air** (API) and **pnpm dev** (frontend) on the host. It does **not** build app images.

| Who | Lands on |
|-----|----------|
| `super_admin` or any `platform.*` permission | `/platform` |
| Other authenticated users | `/` (CMS) |

## Docs

| Doc | When to read |
|-----|----------------|
| [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) | Repo map, shells, RBAC, request flow |
| [docs/DEVELOPMENT.md](./docs/DEVELOPMENT.md) | Env, local commands, production / Dokploy |
| [docs/EXTENDING.md](./docs/EXTENDING.md) | Adding a module, API + UI conventions |
| [CONTRIBUTING.md](./CONTRIBUTING.md) | How to work in this repo |
| [backend/README.md](./backend/README.md) | API commands and curl examples |
| [frontend/README.md](./frontend/README.md) | BFF, panels, scripts |
| [AGENTS.md](./AGENTS.md) | Which agent skill to load |

Agent playbooks live under [`.agents/skills/`](./.agents/skills).
