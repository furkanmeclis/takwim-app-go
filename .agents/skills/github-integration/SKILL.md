# GitHub integration

Shipped as the first OAuth provider. Super admin configures a GitHub App; users **link** an existing account from profile, or **self-register** when global registration + GitHub `register_enabled` are on.

## Layout

| Piece | Path |
|-------|------|
| Settings API | `backend/internal/modules/integrations/github` |
| At-rest secrets | `backend/internal/platform/crypto` (`SecretBox`, AES-256-GCM) |
| Admin UI | `frontend/src/features/integrations/github` |
| Route | `/platform/settings/integrations?tab=github` |
| Identities | `GET /v1/auth/identities`, `DELETE /v1/auth/identities/{provider}` |
| Auth policy | `GET/PATCH /v1/platform/auth/settings` (`auth_settings`) |

Permissions: `platform.integrations.github.read` / `.write`. Routes also require **super admin**.

## Secrets

- Env: `APP_ENCRYPTION_KEY` — 32-byte key, raw or base64 (`openssl rand -base64 32`).
- Store `client_secret` and private key **encrypted** in Postgres. API responses expose only `*_configured` booleans — never plaintext.
- Do not log GitHub secrets or the encryption key.

## API

| Method | Path | Notes |
|--------|------|-------|
| GET | `/v1/platform/integrations/github` | Masked settings (`enabled` = login, `register_enabled`) |
| PATCH | `/v1/platform/integrations/github` | Partial update; omit secret fields to keep existing |
| GET | `/v1/auth/identities` | Linked providers for current user |
| DELETE | `/v1/auth/identities/{provider}` | Unlink (`github`, `google`, `facebook`, `apple`) |

Internal NextAuth adapter (not in OpenAPI): `/v1/internal/auth/oauth/{provider}`, `/v1/internal/auth/accounts`, `POST /v1/internal/auth/users` (OAuth self-register).

## Adding another OAuth provider

Google / Facebook / Apple share `backend/internal/modules/integrations/oauthprovider` + `oauth_provider_settings` (encrypt with `crypto.SecretBox`, mask on read, super-admin + `platform.integrations.<name>.*`).

Admin UI: `frontend/src/features/integrations/oauth` + tab on `/platform/settings/integrations` (`?tab={provider}`).

Nav: `layout.section_auth_methods` group in `frontend/src/config/nav.ts`.

Registration gates: global `registration_enabled` **and** provider `register_enabled` (plus credentials). Public flags on `GET /v1/app/config` → `auth_methods`.
