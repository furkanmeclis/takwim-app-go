# API conventions

HTTP contract for `github.com/furkanmeclis/nextjs-go-boilerplate/backend`. Paths use `/v1/...` (no `/api` prefix).

## Response envelope

All `/v1/*` handlers return:

```json
{
  "success": true,
  "data": {},
  "meta": { "request_id": "uuid" }
}
```

Errors:

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "...",
    "details": [{ "field": "email", "message": "...", "code": "invalid" }]
  },
  "meta": { "request_id": "uuid" }
}
```

- Success: `response.JSON(w, r, status, data)`
- Errors: `Unauthorized` / `Forbidden` / `NotFound` / `Conflict` / `ValidationError` / `Internal`
- Branch on `error.code`, not message text
- Correlation: middleware sets `X-Request-ID` (generate if missing) and mirrors into `meta.request_id`
- **Outside envelope:** `GET /healthz`, `GET /readyz`

## Lists

```json
{
  "success": true,
  "data": { "items": [], "total": 0, "limit": 20, "offset": 0 },
  "meta": { "request_id": "..." }
}
```

Parse with `pkg/apiquery`:

| Param | Notes |
|-------|--------|
| `limit` | default 20, max 100 |
| `offset` | default 0 |
| `q` | search text |
| `sort` | e.g. `-created_at,name` |
| `include` / `fields` | sparse / expand (when supported) |

**Forbidden query names:** `page`, `page_size`, `per_page`, `search` (use `q` only). Unknown `sort` → `400 VALIDATION_ERROR` with details.

## Resource meta

Primary list resources expose `GET /v1/{resource}/meta` via `internal/platform/resourcemeta`:

- `GET /v1/platform/users/meta`
- `GET /v1/platform/roles/meta`
- `GET /v1/notifications/meta`
- `GET /v1/platform/notifications/meta`
- `GET /v1/platform/activity/meta`
- `GET /v1/platform/storage/meta`
- `GET /v1/platform/logs/meta`
- `GET /v1/platform/log-rules/meta`

Register `/meta` **before** `/{uuid}` on `ServeMux`. Capabilities must match live routes (do not advertise unfinished bulk/export).

## Auth

- Bearer JWT on protected routes
- Global users; no tenant (`tid`) or workspace (`wid`) claims
- User fields: `name` / `surname`
- Platform routes require the matching `platform.*` permission (`super_admin` bypasses `HasPermission`)

## Notifications

Domain / auth use cases call `notifications.Service.Enqueue(...)` only. HTTP handlers never call SMTP/SMS inline.

- Channels: `inapp` | `email` | `realtime` | `sms` | `push` (sms/push = noop unless VAPID is configured)
- Delivery: Asynq task `app:notification:deliver` on queue `notifications`
- **Security emails** (`auth.password_reset`, `auth.email_verification`) ignore `email_enabled` preference
- Realtime channel hint: `user:{user_uuid}`

Cross-module signals use the platform event bus (`internal/platform/events`). Use cases `Publish`; bus handlers may enqueue notifications — never SMTP.

## Money and time

- **Amounts** are `pkg/money.Money`: int64 minor units (kuruş) plus a currency. Columns are `NUMERIC(14,2)`; JSON is a decimal string with two digits (`"1300.00"`), never a float. Rounding is half away from zero, applied once. Splitting a total (installments, lines) uses `Money.Allocate` / `Split`: the remainder goes to the last part so the parts add up exactly.
- **Currency**: every amount field has the currency next to it or inherits `organization.currency` (ISO 4217, default `TRY`).
- **Instants** are stored as `timestamptz` and returned as RFC 3339 UTC. Calendar decisions (which day, opening hours, "today") use `pkg/localtime` with the zone of the branch, else the business (`organization.timezone`, default `Europe/Istanbul`): `localtime.ZoneOr(branch, org)`.
- **Ranges** are half-open `[start, end)`: back-to-back appointments touch but do not overlap. Dates are `"2006-01-02"` (`localtime.Date`); `Date.Bounds(zone)` gives the instant range of a local day (23 or 25 hours on DST days).
- Frontend: `amount`, `toMinor` / `fromMinor` / `allocateMinor`, `datetimeInZone` and `dateKeyInZone` in `frontend/src/lib/utils/format.ts`; defaults in `frontend/src/config/regional.ts`.

## Mobile client rules

The web panel and the future staff / customer mobile apps share one API. Keep it that way:

- **One contract.** `/v1` is versioned; breaking changes go to a new version, additive changes (new optional fields, new endpoints) do not. `docs/openapi.yaml` is the single source; the web types (`pnpm api:generate`) and any mobile client are generated from it. Clients ignore unknown fields.
- **Rules live in the API.** The Next.js BFF only holds the session (httpOnly cookies) and proxies `/api/v1/*`; it never validates, computes prices or availability, or decides permissions. Anything the web can do, a mobile app can do with the same calls.
- **Auth.** Mobile apps send `Authorization: Bearer <access>` and rotate with `POST /v1/auth/refresh` (opaque refresh token, one row per device, listed under `/v1/auth/sessions`). Native Apple / Google sign-in: `POST /v1/auth/oauth/{provider}/native`. Access tokens carry a kind (`knd`): only `staff` is accepted on these routes; the customer panel gets its own kind and middleware.
- **Business scope.** Tenant calls send `X-Organization: <slug|uuid>`; the list comes from `GET /v1/auth/me` organizations.
- **Retries.** Every unsafe request a client may retry (creating a reservation, taking a payment, cancelling) sends `Idempotency-Key: <uuid>` — a new key per user action, the same key for its retries. Same key + body → the first response is replayed (`Idempotent-Replayed: true`); `409 IDEMPOTENCY_IN_PROGRESS` → retry later; `422 IDEMPOTENCY_KEY_REUSED` → client bug. Kept 24 h; 5xx outcomes are not kept.
- **Lists, errors, formats.** Same envelope, `limit`/`offset` lists, error codes (`docs/api/errors.md`) and money / time formats as above. Never parse `message`; branch on `error.code`.
- **Push.** Devices register with `POST /v1/push-devices` (Expo token, platform, locale, app version) and unregister with `DELETE /v1/push-devices/{token}` on sign-out. Payload `data.url` is the deep link.
- **Media** URLs come from the API (signed or streamed); clients never build storage keys.

## OpenAPI

Contract: `docs/openapi.yaml` (OpenAPI 3.1) + Scalar at `/docs/`. Run `make openapi-lint` after path/schema changes.

Live routes are registered in `internal/modules/**/routes.go`. If the YAML still describes a stripped product path, the Go router wins until the spec is trimmed.
