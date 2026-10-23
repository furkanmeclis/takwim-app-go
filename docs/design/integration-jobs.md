# Integration accounts and jobs

Status: **implemented** (TEC-880). The skeleton every external provider plugs into: SMS, WhatsApp and e-mail messages, payment providers and inbound webhooks. Real adapters arrive with their own issues; this change ships the contract, the storage, the runner, the panel and a development log provider.

Code:

| Area | Package |
|------|---------|
| Adapter / registry / credential vault | `backend/internal/platform/integrations/integrations.go` |
| Operation contract (`Submit` / `CheckStatus`, outcomes, `ErrUnsupported`) | `backend/internal/platform/integrations/operation.go` |
| Development log provider (one adapter per kind) | `backend/internal/platform/integrations/logprovider` |
| Wiring (`integrations.New` → vault, operations, accounts, runner) | `backend/internal/modules/integrations/kit.go` |
| Accounts API (`/v1/tenant/integrations`) | `backend/internal/modules/integrations/accounts` |
| Runner, inbound idempotency, jobs API (`/v1/tenant/integration-jobs`) | `backend/internal/modules/integrations/jobs` |
| Queues, dispatcher, sweep schedule | `backend/internal/queue/integrations.go` |
| Panel (Ayarlar → Entegrasyonlar) | `frontend/src/features/integrations/components` |

## 1. The rule

An outbound call whose result is not known (a timeout, a dropped connection, a 5xx after the request left, an unreadable 2xx) may have been carried out by the provider. Such a job goes to `needs_verification` and **only `CheckStatus` may move it on**. The same work is submitted again only when the provider confirmed it was not done (`not_found`, or a retryable `failed`). An operation an adapter cannot perform returns `integrations.ErrUnsupported`: a submit fails explicitly, a status check leaves the job for a person. Work that was not done never looks successful.

## 2. Providers and credentials

- `integrations.Adapter` (kind, provider, credential fields, `Check`) is registered in a `Registry`; modules talk to the kind, never to a provider, so providers are swappable per organization.
- Each organization connects its own account (`integration_accounts`, tenant scoped). Credentials are sealed by the `Vault` (field cipher, purpose `integration.credentials`, associated data binds the ciphertext to its organization and account). The API answers with masked hints only.
- In `development` and `test` the kit also registers the `log` provider for every kind: it writes a masked line to the server log and succeeds. It is never registered in production.

## 3. Tables (migration 000038)

- `integration_accounts`: kind, provider, mode (`sandbox | live`), status, sealed credentials, hints, last connection check.
- `integration_jobs` (mixed scope; platform jobs have no organization): direction, kind, provider, idempotency key (unique per provider), status, priority (`critical | normal`), attempts / `max_attempts`, `next_attempt_at`, `provider_ref`, payload (references and non-secret facts only), crash-safe claim `locked_until`.
- `integration_attempts`: one row per provider call (operation, trigger, outcome, short request / response summaries, error, timing). Never credentials, message bodies, OTP codes, full phone numbers or card data.

Statuses: `pending`, `processing`, `submitted`, `succeeded`, `failed`, `needs_verification`, `ignored`, `blocked`, `waiting_dependency`, `reported_manually`, `reconciled`.

## 4. Outbound: outbox and runner

- `Runner.EnqueueTx(ctx, tx, in)` inserts the job inside the caller's own transaction (a reservation, a payment) as `pending` with `next_attempt_at = now`, and sends nothing. After the commit the module calls `Runner.Kick(org, job)`; if the process stops in between, the minute sweep picks the job up. `Runner.Enqueue` is the same without a caller transaction.
- A repeated idempotency key returns the existing job and sends nothing.
- `Process` runs one step (`pending` → `Submit`; `submitted` / `needs_verification` → `CheckStatus`), records an attempt and applies the transition. Backoff 15 s, 30 s, 1 min … capped at 30 min; `max_attempts` bounds automatic calls.
- Asynq queues `integrations` (weight 2) and `integrations_critical` (weight 6, OTP / transactional); `app:integrations:sweep` runs every minute. Every change publishes the live list resource `tenant.integration_jobs`.

## 5. Inbound: webhook idempotency

`Runner.HandleInbound(ctx, input, handler)` upserts the inbound job by `(provider, idempotency key)`, claims it, runs the handler once and records the result. A redelivered notification that already succeeded (or is being handled) is answered without running the handler again; a handler panic is recovered and stored as a failure. Webhook handlers of later issues (payments, WhatsApp) use this instead of their own dedup tables.

## 6. Staff retry ("Yeniden dene")

`POST /v1/tenant/integration-jobs/{uuid}/retry`: a `pending` job is submitted now; otherwise **CheckStatus first** → `already_done`, `in_progress` or (confirmed not done) `resubmitted`. An unknown answer sends nothing and returns `409 INTEGRATION_JOB_UNVERIFIED`. Finished jobs and inbound notifications are `409 INTEGRATION_JOB_NOT_RETRYABLE`; a claimed job is `409 INTEGRATION_JOB_BUSY`.

## 7. API

```text
GET   /v1/tenant/integrations/meta                providers and their credential fields
GET   /v1/tenant/integrations                     accounts
POST  /v1/tenant/integrations                     connect
PATCH /v1/tenant/integrations/{uuid}              rename, mode, rotate secrets
POST  /v1/tenant/integrations/{uuid}/test         connection check
POST  /v1/tenant/integrations/{uuid}/enable|disable
GET   /v1/tenant/integrations/{uuid}/jobs         one account's jobs
GET   /v1/tenant/integration-jobs                 every job of the organization (failed / needs_verification work list)
GET   /v1/tenant/integration-jobs/{uuid}          job + attempt log
POST  /v1/tenant/integration-jobs/{uuid}/retry    §6
```

All behind `RequireOrganization` + `tenant.integrations.manage` (owner, admin, merkez_yoneticisi). RT-A01 covers every route.

## 8. Open items

- Real adapters: SMS and WhatsApp providers, payment providers (their issues).
- A step-up gate on connect / rotate / retry once the tenant shell has a step-up policy.
- Retention of `integration_attempts`.
