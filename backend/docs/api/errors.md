# API errors

Failed `/v1/*` responses use the standard envelope:

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Request validation failed",
    "details": [
      { "field": "sort", "message": "unknown sort field \"foo\"", "code": "unknown" }
    ]
  },
  "meta": { "request_id": "..." }
}
```

Branch on `error.code`, not on message text. `details` is optional (field-level validation).

| Code | HTTP | When |
|------|------|------|
| `VALIDATION_ERROR` | 400 | Decode/invalid JSON, field validation, password policy, unknown sort |
| `INVALID_RESET_CODE` | 400 | Password-reset OTP wrong/expired/consumed |
| `INVALID_VERIFICATION_CODE` | 400 | Email-verification OTP wrong/expired/consumed |
| `UNAUTHENTICATED` | 401 | Missing/invalid access token, invalid refresh |
| `INVALID_CREDENTIALS` | 401 | Login or change-password: wrong password |
| `FORBIDDEN` | 403 | Missing permission or disabled user |
| `NOT_FOUND` | 404 | Resource does not exist or is not visible |
| `CONFLICT` | 409 | Unique constraint / duplicate (e.g. email already registered) |
| `INVALID_IDEMPOTENCY_KEY` | 400 | `Idempotency-Key` is empty, longer than 255 or not printable ASCII |
| `IDEMPOTENCY_IN_PROGRESS` | 409 | A request with the same `Idempotency-Key` is still running; retry later |
| `IDEMPOTENCY_KEY_REUSED` | 422 | The `Idempotency-Key` was already used with a different request body |
| `INTEGRATION_JOB_NOT_RETRYABLE` | 409 | Integration job is done, inbound, or a refund (no staff retry) |
| `INTEGRATION_JOB_UNVERIFIED` | 409 | Retry could not confirm the job's state with the provider; nothing was sent (`data` = job) |
| `INTEGRATION_JOB_BUSY` | 409 | A worker holds the integration job right now |
| `PLAN_LIMIT_REACHED` | 409 | Adding a branch or a staff seat (member / invitation) exceeds the plan limit; `data` = `{kind, limit, used}` |
| `MODULE_NOT_ENABLED` | 403 | The organization may not use the module (sector does not show it, or the add-on is not granted) |
| `INTEGRATION_CHECK_FAILED` | 422 | Provider rejected the connection test of an integration account |
| `RATE_LIMITED` | 429 | Auth endpoints: login, register, forgot-password, reset-password |
| `PASSWORD_RESET_FAILED` | 500 | Forgot/reset persist or enqueue failure |
| `INTERNAL_ERROR` | 500 | Unexpected server failure |

Probes (`/healthz`, `/readyz`) are **not** enveloped.

See also [`docs/auth.md`](../auth.md) and [`API_CONVENTIONS.md`](../../API_CONVENTIONS.md).
