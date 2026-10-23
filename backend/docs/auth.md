# Auth & RBAC

Single-scope role model for this boilerplate: users are global; permissions come from assigned roles (union). No tenants or workspaces.

## Roles

| Role | Type | Notes |
|------|------|-------|
| `super_admin` | system (`is_system=true`) | All permissions; cannot delete slug or demote last member |
| `cms_user` | editable seed | `auth.session`, `notifications.read` |
| Custom roles | platform-managed | Created via `/v1/platform/roles`; permissions from DB catalog only |

`users.is_super_admin` column is removed. JWT and `/me` still expose `is_super_admin` when the user has the `super_admin` role.

## Permissions (seed)

| Permission | Purpose |
|------------|---------|
| `platform.users.read` / `.write` / `.export` / `.import` | Platform user CRUD + I/O |
| `platform.users.impersonate` | Impersonate another user from platform admin (step-up) |
| `platform.users.bulk.disable` / `.bulk.enable` | User bulk actions |
| `platform.roles.read` / `.write` / `.export` / `.import` | Role CRUD + I/O |
| `platform.roles.bulk.delete` | Role bulk delete |
| `platform.bulk.read` | Bulk job list / rollback |
| `platform.notifications.read` / `.read_all` / `.export` | Platform notification log (`.read_all` unlocks other users' rows) |
| `platform.settings.read` / `.write` | Letterhead / export branding |
| `platform.activity.read` | Audit log |
| `platform.imports.read` / `platform.exports.read` | I/O job lists |
| `platform.storage.read` / `.write` | S3/MinIO file browser |
| `platform.logs.read` / `.write` | Application logs + purge |
| `platform.access.read` / `.write` | Step-up policy |
| `platform.auth.settings.read` / `.write` | Registration policy (also requires super admin) |
| `platform.integrations.github.read` / `.write` | GitHub App settings (also requires super admin) |
| `platform.integrations.google.read` / `.write` | Google OAuth settings (also requires super admin) |
| `platform.integrations.facebook.read` / `.write` | Facebook OAuth settings (also requires super admin) |
| `platform.integrations.apple.read` / `.write` | Apple OAuth settings (also requires super admin) |
| `auth.session` | Sign-in / CMS access |
| `notifications.read` / `notifications.manage` | User notifications |

Super admin bypasses permission checks in `HasPermission`. Most platform HTTP routes use `RequirePermission` only. Auth settings and OAuth integration routes also use `RequireSuperAdmin`.

## Login & registration

- Password / passkey / OAuth availability is controlled by `auth_settings` + per-provider flags (`GET /v1/app/config` → `auth_methods`).
- Global `registration_enabled` must be on, plus the method’s `register` flag, for self-registration.
- Password register assigns the configured **default role** when set; otherwise no roles.
- **OAuth**: linked accounts can sign in when `login` is enabled. Unlinked accounts may self-register only when register is allowed for that provider.
- Secrets for OAuth apps are encrypted with `APP_ENCRYPTION_KEY`.

## Email code sign-in (web + mobile)

| Method | Path | Auth |
|--------|------|------|
| POST | `/v1/auth/email-code/request` `{email}` | — |
| POST | `/v1/auth/email-code/verify` `{email, code, totp_code?}` | — |

- 6-digit code, 10 min TTL, 5 attempts, single use, stored as SHA-256 (`otp_codes.type = login_code`).
- `request` always answers `200 {status: accepted}` (no enumeration). Disabled / deactivated users still get a code so `verify` can report `FORBIDDEN` / `ACCOUNT_DEACTIVATED` after the mailbox is proven.
- **Sign-up by email code**: for an email without an account, when `registration_enabled` is on, `request` sends a code too (`otp_codes.user_id` NULL) and a valid `verify` creates the user (email verified, no password, `name` = email local part, empty `surname`, default role, welcome notification) and returns tokens like a login. With registration off, unknown emails get no code and `verify` answers `400 INVALID_EMAIL_CODE` as before.
- Users with authenticator 2FA: `verify` without `totp_code` → `403 MFA_REQUIRED`, the email code stays valid; resend with `totp_code`. A wrong TOTP burns one of the code's attempts. The admin "password login requires 2FA" policy applies to password login only.
- Email: `auth.login_code` template (tr/en, user locale), security email (ignores preferences).
- **App review accounts**: `AUTH_REVIEW_ACCOUNTS=review@example.com:246810,other@x.com:135790` (comma-separated `email:code`, code ≥ 6 chars). For those emails no email is sent and the fixed code signs in (also confirms account deletion). The code works only for its own email; the account must exist. Unset/empty = disabled. Rotate or remove after review.

## Native OAuth (mobile)

NextAuth keeps handling web OAuth; mobile apps send the SDK id_token to the API.

| Method | Path | Body |
|--------|------|------|
| POST | `/v1/auth/oauth/{apple\|google}/native` | `{id_token, nonce?, given_name?, family_name?, authorization_code?, totp_code?}` |
| POST | `/v1/auth/oauth/link/request` | `{link_ticket, email}` |
| POST | `/v1/auth/oauth/link/verify` | `{link_ticket, email, code, totp_code?}` |
| POST | `/v1/auth/oauth/link/create` | `{link_ticket, totp_code?}` |

- Verification: RS256 signature against the provider JWKS (cached per `Cache-Control`, refetch on unknown `kid` at most once a minute), `iss`, `exp`, `aud`, optional `nonce` (raw or SHA-256 hex).
- Audiences: Apple `AUTH_APPLE_NATIVE_CLIENT_IDS`; Google `AUTH_GOOGLE_NATIVE_CLIENT_IDS` (iOS + Android client ids) plus the web client id stored in Google OAuth settings (Android Credential Manager tokens use it as `aud`). Native sign-in is enabled when a provider has at least one audience; otherwise `403 OAUTH_PROVIDER_DISABLED`.
- Linked identity → login (2FA round-trip like password login). Unlinked + existing email → `409 OAUTH_ACCOUNT_NOT_LINKED` (same as NextAuth's `OAuthAccountNotLinked`: sign in with the existing method, then link from the profile). Unlinked + new email → OAuth sign-up (global `registration_enabled` must be on) + link.
- **Apple private relay** (`@privaterelay.appleid.com`, no account): `409 OAUTH_LINK_CHOICE_REQUIRED` with `details` `link_ticket` (AES-GCM sealed with `APP_ENCRYPTION_KEY`, 15 min), `provider`, `expires_in`, `register_allowed`. "I have an account" → `link/request` + `link/verify` (email code `auth.oauth_link_code` to the existing account). "No" → `link/create`.
- Apple tokens: when `authorization_code` is sent and `AUTH_APPLE_TEAM_ID` / `AUTH_APPLE_KEY_ID` / `AUTH_APPLE_PRIVATE_KEY` (the Sign in with Apple `.p8`, `\n` escapes allowed) are set, the refresh token is stored encrypted (`oauth_accounts.refresh_token_enc`, `client_id`) and revoked on account deletion.
- Apple signing key resolution (`internal/platform/appleauth.Resolver`): the `.p8` uploaded in Platform → Integrations → Apple (`oauth_provider_settings.apple_team_id` / `apple_key_id` / `apple_private_key_enc`, encrypted with `APP_ENCRYPTION_KEY`) wins; the `AUTH_APPLE_*` env vars are the fallback. The parsed key is cached for 1 minute and dropped immediately when an admin saves the Apple settings. With a key, the web (NextAuth) Apple `client_secret` returned by `GET /v1/internal/auth/oauth/apple` is generated per request (ES256, `iss` = Team ID, `sub` = Services ID, `kid` = Key ID, valid 24h, `client_secret_expires_at` set); a pasted client secret JWT is only used when no key is configured.

## Account deletion (deactivation)

| Method | Path | Auth |
|--------|------|------|
| POST | `/v1/auth/account/deactivate/request` | Bearer — emails `auth.account_deactivation_code` |
| POST | `/v1/auth/account/deactivate` `{code?}` | Bearer + recent step-up **or** `code` |

- No data is deleted. `users.status = disabled`, `users.deactivated_at = now()`; all refresh tokens revoked; step-up grant revoked; stored Apple refresh tokens revoked via `https://appleid.apple.com/auth/revoke` (skipped when none stored or no key/secret configured); `auth.account_deactivated` email.
- Every login path (password, refresh, email code, native OAuth, NextAuth OAuth / passkey session issue, org context) returns `403 ACCOUNT_DEACTIVATED`; existing access tokens stop working on the next request (identity loader rejects disabled users).
- The last super admin cannot deactivate (`409`). Not allowed while impersonating.
- Reactivation: a platform admin sets the user's status to `active` (clears `deactivated_at`).

## QR sign-in (web QR approved in the mobile app)

Module: `internal/modules/qrlogin`. State is Redis only (`app:<env>:qrlogin:<sha256(id)>`, TTL 120 s; 60 s exchange window after approval). No migration.

1. Web login page → `POST /v1/auth/qr/sessions` (public, 60 / 15 min per IP). Returns `session_id` (QR: `{PUBLIC_FRONTEND_URL}/login/qr/{id}`), a `browser_secret` that stays in the tab, and an **anonymous** Centrifugo grant: connection JWT with `sub: ""` + subscription JWT for one channel `qrlogin:{random}` (not derived from the session id), both expiring 30 s after the session. The tab subscribes; it never polls. After each (re)subscribe it reads `POST /v1/auth/qr/sessions/{id}/state {browser_secret}` once to catch events missed while offline.
2. Phone (Bearer) → `GET /v1/auth/qr/sessions/{id}`: browser / OS from the web request's User-Agent, IP, approximate location, created / expires. The first viewer claims the session (`409 QR_SESSION_CLAIMED` for other accounts); the tab gets `{type:"scanned"}`.
3. `POST …/{id}/approve` (caller's active org, re-validated at exchange) or `…/reject`. One Lua script per transition, so exactly one approve / reject wins (`409 QR_SESSION_RESOLVED`). Deactivated accounts get `403 ACCOUNT_DEACTIVATED`; impersonation sessions cannot approve. Events: `{type:"approved", exchange_token}` / `{type:"rejected"}`.
4. NextAuth `qr-login` credentials provider → `POST /v1/auth/qr/exchange {session_id, browser_secret, exchange_token}` → normal token pair (+ `user`). Single use (key deleted); 5 wrong secrets delete the session. The BFF refuses `/api/v1/auth/qr/exchange` from browsers. No TOTP prompt: the approving phone already holds a signed-in session.

Location: `AUTH_QR_TRUST_GEO_HEADERS=true` trusts `CF-IPCountry` / `CF-IPCity` / `CF-Region` (only behind Cloudflare with visitor location headers, otherwise they are spoofable); else `AUTH_QR_GEOIP_DB` (GeoLite2-City `.mmdb`); else IP only. The BFF forwards User-Agent and these headers for the create call only.

Centrifugo: namespace `qrlogin` (all client permissions off) in `deploy/centrifugo/config*.json`. Anonymous JWT connections need no extra flag; `allow_anonymous_connect_without_token` stays off. Restart Centrifugo after deploying the config.

Phishing note: QR sign-in can be abused by showing a victim an attacker's QR. The approval screen shows browser, IP and location and the app warns to approve only codes on a screen in front of you.

## Linked identities

| Method | Path | Auth |
|--------|------|------|
| GET | `/v1/auth/identities` | Bearer → `{items, total, has_password}` |
| POST | `/v1/auth/identities/{apple\|google}/native` | Bearer, `{id_token, nonce?, authorization_code?}` → `LinkedIdentity` |
| DELETE | `/v1/auth/identities/{provider}` | Bearer (`github`, `google`, `facebook`, `apple`) |

- `has_password` comes from `users.password_set`: OAuth / native sign-up stores a random hash and sets it `false`; any password update (reset, change, admin set) sets it `true`.
- Native link verifies the token like native sign-in (same audiences, nonce, rate limit). Idempotent for the same user; `409 CONFLICT` when the identity belongs to another user or the user already has a different identity for that provider. Apple `authorization_code` → stored refresh token.
- Unlink → `409 LAST_SIGN_IN_METHOD` when the user has no password, no passkey and this is the last linked identity.

GitHub: `GET/PATCH /v1/platform/integrations/github`. Google / Facebook / Apple: `/v1/platform/integrations/{provider}`. Auth policy: `GET/PATCH /v1/platform/auth/settings`.

Internal NextAuth adapter routes (not in OpenAPI): `/v1/internal/auth/oauth/{provider}`, `/v1/internal/auth/accounts`, `POST /v1/internal/auth/users`.

## JWT

HS256 claims: `sub`, `roles`, `is_super_admin`, `exp`, optional `imp` (impersonator user UUID), optional `sid` (refresh-session UUID). No `tid` / `wid`.

## Sessions

Refresh tokens are listed as devices. `sid` on the access token marks the current row.

| Method | Path | Auth |
|--------|------|------|
| GET | `/v1/auth/sessions` | Bearer |
| DELETE | `/v1/auth/sessions/{uuid}` | Bearer (own session) |
| POST | `/v1/auth/sessions/revoke-others` | Bearer (keeps current `sid`) |

## Rate limits

Redis INCR, 15-minute window, fail-open if Redis is down. `429 RATE_LIMITED` + `Retry-After`.

| Endpoint | Key | Max |
|----------|-----|-----|
| Login | IP + email | 10 |
| Register | IP | 5 |
| Forgot password | IP + email | 5 |
| Reset password | IP | 10 |
| Verify email (`/auth/email/verify`) | IP | 10 |
| Verification email request | user | 5 |
| Email code request / OAuth link request | IP 10, email 5 | |
| Email code verify / OAuth link verify | IP 30, email 10 | |
| Native OAuth | IP | 30 |
| OAuth link create | IP (register bucket) | 5 |
| Account deletion code request / deactivate | user | 5 / 10 |

The BFF forwards the browser IP as `X-Forwarded-For`; the Go API must not be reachable directly from clients or the header could be spoofed.

## `/v1/auth/me`

Returns `user`, `roles[]`, `permissions[]`, self-service links, and realtime hints. No tenant list.

## Platform users

| Method | Path | Permission |
|--------|------|------------|
| GET | `/v1/platform/users/meta` | `platform.users.read` |
| GET | `/v1/platform/users` | `platform.users.read` — query: `limit`,`offset`,`q`,`sort`,`status`,`role` |
| POST | `/v1/platform/users` | `platform.users.write` — body includes `role_uuids[]` |
| GET | `/v1/platform/users/{uuid}` | `platform.users.read` — includes `roles[]` |
| PATCH | `/v1/platform/users/{uuid}` | `platform.users.write` — optional `role_uuids[]` |
| POST | `/v1/platform/users/{uuid}/password` | `platform.users.write` |
| POST | `/v1/platform/users/{uuid}/impersonate` | `platform.users.impersonate` (step-up) |
| POST | `/v1/auth/impersonation/stop` | Bearer (active impersonation session) |

Last super admin cannot be demoted via role removal or disable.

## Platform roles

| Method | Path | Permission |
|--------|------|------------|
| GET | `/v1/platform/roles/meta` | `platform.roles.read` |
| GET | `/v1/platform/roles` | `platform.roles.read` |
| POST | `/v1/platform/roles` | `platform.roles.write` |
| GET | `/v1/platform/roles/{uuid}` | `platform.roles.read` |
| PATCH | `/v1/platform/roles/{uuid}` | `platform.roles.write` — system role slug immutable |
| DELETE | `/v1/platform/roles/{uuid}` | `platform.roles.write` — system roles → `409` |
| GET | `/v1/platform/permissions` | `platform.roles.read` — read-only catalog |

New permission slugs are added via migrations/seeds only (no permission CRUD API).

## Tenancy (phase 1)

Behind `TENANCY_ENABLED` (default `false`). Design: [`docs/design/tenant-core.md`](../../docs/design/tenant-core.md).

**Organization per request.** Nothing about the organization lives in the access token. Tenant routes (`/v1/tenant/*`) read the `X-Organization` header (slug, or uuid) on every request; the BFF forwards that header on `/api/v1/tenant/*` only. Two tabs on two organizations are independent and switching never rotates tokens.

**Chain.** `Authenticate` → `RequireOrganization(resolver)` → `RequireTenantPermission("tenant.x.y")` → handler. `RequireOrganization` resolves the organization and the caller's membership (`orgctx.Resolver`) and stores an `orgctx.Scope` (`orgctx.From` / `orgctx.Must`); handlers pass `scope.ID` to repositories.

| Case | Response |
|------|----------|
| Header missing or empty | `400 ORGANIZATION_REQUIRED` |
| Unknown organization, or not a member | `403 NO_TENANT_MEMBERSHIP` (same answer for both: no organization enumeration) |
| Organization suspended | `403 ORGANIZATION_SUSPENDED` |
| Outside the access window (`access_starts_at` / `access_ends_at`) | `403 ORGANIZATION_ACCESS_EXPIRED` |
| Tenant permission missing in this organization | `403 FORBIDDEN` |

**Per-organization permissions.** Platform permissions (`platform.*`, global roles) are unchanged. Tenant permissions (`tenant.*`) come only from the membership role in the **current** organization: an owner of A who is a member of B has member rights in B. `super_admin` is not exempt: it needs a membership (or impersonation) like everyone else, and `RequireTenantPermission` has no super-admin bypass. Tenant slugs follow the same triad rule (migration + `internal/platform/rbac` + `frontend/src/config/permissions.ts`).

**Caching.** `orgcache.CachedResolver` wraps the resolver with Redis (TTL `TENANCY_CACHE_TTL`, default `60s`; nil Redis = no cache; Redis errors fall back to the database). Entries are keyed by (organization id, version, user, ref) and only successful resolutions are cached. Every change that affects resolution (status, access window, membership, role, role permissions, slug) must call `orgctx.Invalidate(ctx, inv, orgID)` after commit: it bumps the organization's version counter, so old entries are never read again and revocation is effectively immediate. A cached entry whose `access_ends_at` has passed is rejected without waiting for the TTL.

Probe: `GET /v1/tenant/context` returns `{organization, role, permissions}` of the resolved scope.

## CLI

`make create-super-admin` upserts the user and assigns the `super_admin` role.
