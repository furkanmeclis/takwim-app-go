---
name: step-up-engine
description: >-
  Add step-up (re-authentication) gates to sensitive API routes and UI
  sections: password/passkey verification, admin TTL policy, StepUpGate,
  and STEP_UP_REQUIRED handling.
---

# Step-up engine

Use when a flow must confirm the **current user's** password or passkey even though they are already signed in.

## Checklist

1. **Backend route** — add `middleware.RequireStepUp(stepUpSvc)` after `Authenticate` on the handler chain.
2. **OpenAPI** — document the route; clients rely on `STEP_UP_REQUIRED` (403).
3. **Frontend UI (optional)** — wrap sensitive sections in `StepUpGate` or call `ensure()` from `useStepUp()`.
4. **i18n** — add `stepup.gate.purpose.<your_slug>` if using a custom Gate purpose label.
5. **Activity** — record sensitive mutations; step-up verification emits `stepup.verified` automatically.

## API

| Method | Path | Notes |
|--------|------|-------|
| GET | `/v1/auth/step-up` | `{ valid, expires_at, methods[] }` |
| POST | `/v1/auth/step-up/password` | `{ password }` → grant |
| POST | `/v1/auth/step-up/passkey/options` | WebAuthn challenge |
| POST | `/v1/auth/step-up/passkey/verify` | Assertion body → grant |
| GET/PATCH | `/v1/platform/access/settings` | Admin TTL + enabled methods |

Grant is **global** per user until TTL expires (Redis key `app:{env}:stepup:grant:{user_uuid}`).

## Frontend

```tsx
import { StepUpGate, useStepUp } from "@/features/step-up-engine";

// UI gate
<StepUpGate purpose="my.feature">
  <SensitivePanel />
</StepUpGate>

// Imperative
const { ensure } = useStepUp();
await ensure();
await doSensitiveThing();
```

`StepUpProvider` is mounted in `app-providers.tsx`. Do **not** use NextAuth `signIn("passkey")` for step-up — it replaces the session.

## Admin

Route: `/platform/settings/access` — permissions `platform.access.read` / `platform.access.write`.

## Shipped consumers

- `POST /v1/platform/*/export` — export jobs
- `POST /v1/platform/users/{uuid}/password` — admin set password
- User detail roles card — `StepUpGate`
