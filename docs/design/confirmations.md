# Customer confirmations (TEC-921)

The Konfirmasyon add-on: before a visit the business asks the customer to confirm it, on its channels in order, a set number of times, and cancels or flags the booking when no answer comes by the deadline. The customer answers in the customer panel or on the single-visit link of the message: "Geleceğim", "Değişiklik iste" or cancel. Everything is gated by the `confirmation` module (`sector.ModuleConfirmation`, an add-on, entitlements.md); without it no task is planned, no request is sent and no answer is offered.

| Area         | Path                                                                                                                        |
| ------------ | --------------------------------------------------------------------------------------------------------------------------- |
| Engine       | `backend/internal/modules/confirmations/usecase` (`engine.go` tasks, requests, deadline, answers; `plan.go`; `settings.go`) |
| Model        | `backend/internal/modules/confirmations/model` (rule, default template, limits)                                             |
| HTTP, routes | `backend/internal/modules/confirmations/handler`, `routes.go`                                                               |
| Answers      | `backend/internal/modules/customerpanel/usecase/confirm.go`, `handler/confirm.go`                                           |
| Queries      | `backend/internal/database/queries/confirmations.sql`                                                                       |
| Migration    | `000064_confirmations`                                                                                                      |
| Tests        | `confirmations/confirmations_db_test.go`, `confirmations/usecase/plan_test.go`, `customerpanel/confirm_db_test.go`          |
| Web          | `frontend/src/features/confirmations/*`, page `/t/{slug}/settings/confirmation`, locale `confirmations.json`; panel buttons |

## Data

| Table                       | Holds                                                                                                                                                                                                                                            |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `confirmation_settings`     | the business-wide rule (`branch_id` null) or one branch's own rule: on / off, ordered `channels`, `fallback`, first request, follow-up and deadline (minutes before the start), `max_attempts`, contact window, `on_no_answer` (`cancel`/`flag`) |
| `reservation_confirmations` | the task of one booking (one row per reservation): `round`, the tracked `reservation_version` and `starts_at`, the rule copied at planning, the planned `attempt_times`, `attempts_sent`, `next_attempt_at`, `deadline_at`, answer, outcome      |

No settings row = the template (`model.DefaultRule`: on, SMS then e-mail with fallback, first request 24 hours before, a follow-up 3 hours before, 2 requests, deadline 2 hours before, requests between 09:00 and 21:00, cancel). A branch without its own rule follows the business-wide one. Saving a rule does not re-plan tasks already planned; it applies to bookings planned afterwards.

## Planning

The engine is a reservations `Notifier` (`reservations.Notifiers(notify, confirm)`), so it runs inside the transaction of every reservation change.

- A booking that is `active` and `accepted` (created, accepted, paid or rescheduled) gets a task. An online booking waiting for payment or approval waits until it is accepted / paid.
- `PlanSchedule`: deadline = start − `deadline_minutes`; the last request goes out at least 30 minutes before the deadline. The first request is at start − `first_request_minutes` (now for a later booking), moved into the contact window; follow-ups start at start − `follow_up_minutes` and are spread evenly to the last request time (at least 15 minutes apart, inside the window). A booking too close to its start for any request stays `not_required`.
- The reservation's `confirmation_status` goes `not_required` → `scheduled` (event `confirmation_scheduled`, note = the deadline).
- A move re-plans (new `round`, the answer is cleared, `confirmed` goes back to `scheduled`). Any other change only tracks the new `version`. A cancel, reject, finish or start drops the task (`dropped`). A module or rule turned off drops it at the next step and sets `not_required`.

## Requests

The worker runs `Engine.Run` every 15 seconds: due tasks (`next_attempt_at` or `deadline_at` passed), one transaction each, locking the reservation first and then the task.

- A task whose tracked version or start no longer matches, or whose booking is not active, is dropped.
- A due attempt writes a `confirmation.request` notice through the customer notification outbox (`Engine.EmitReservation`) with the task's channels and fallback, key `r<round>:a<n>`, and the `deadline` variable in the branch time zone. Status `pending`, event `confirmation_requested` (note `n/total`).
- `confirmation.request` is a **managed** event of the notification catalog: it is not on the notification settings page and its rule comes from the confirmation settings. Its SMS and e-mail carry the single-visit link (`adapters.VisitLinks` over `customerpanel.IssueVisitLinkFor`), never a sign-in link.
- The notification engine asks the confirmation engine's **dispatch guard** before sending: a request of an answered or closed task, or of an older round, is cancelled.

## Deadline

At `deadline_at` without an answer (and without a change request):

| Situation                | Result                                                                                                                                                 |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| no request was ever sent | `expired`, outcome `unreachable`: the booking is never cancelled for a message the customer did not get; the business sees it                          |
| `on_no_answer = cancel`  | `reservations.Cancel` with the expected version, reason `confirmation_timeout`: capacity is freed by the ordinary transition; task outcome `cancelled` |
| `on_no_answer = flag`    | `expired`, outcome `flagged`: the booking stays; a late answer still counts until the start                                                            |

The cancel runs after the task's transaction, guarded on `confirmation_status` still `scheduled` / `pending`; a stale version (the booking changed meanwhile) does nothing.

## Answers

| Route                                                  | Who                          |
| ------------------------------------------------------ | ---------------------------- |
| `POST /v1/customer/reservations/{uuid}/confirm`        | signed-in panel              |
| `POST /v1/customer/reservations/{uuid}/change-request` | signed-in panel (`note`)     |
| `POST /v1/customer/visit/confirm`                      | visit link (`token`)         |
| `POST /v1/customer/visit/change-request`               | visit link (`token`, `note`) |

- Opening a visit link answers nothing (link previews); only the button does.
- **Confirm**: `confirmed`, event `confirmation_confirmed`, no more requests, no cancel.
- **Change request**: the booking stays, no more requests and no cancel; event `confirmation_change_requested` with the note (500 characters). Once per round; the customer may still confirm, and the business moves the visit (which re-plans).
- **Cancel**: the ordinary cancel under the business's cancel rules; while a task is open it counts as `declined` (event `confirmation_declined`).
- The panel's `actions.confirm` / `actions.request_change` say whether each is allowed and until when (`not_requested`, `answered`, `status`, `business_unavailable`); `confirmation` shows the status and deadline.

## Business settings API

`tenant.settings.read` reads, `tenant.settings.write` changes; no new permission. All routes need the add-on (`403 MODULE_NOT_ENABLED`).

| Route                                          | Does                                                                              |
| ---------------------------------------------- | --------------------------------------------------------------------------------- |
| `GET /v1/tenant/confirmations/settings`        | channels with readiness, the business-wide rule, template, branch rules, limits   |
| `PUT /v1/tenant/confirmations/settings`        | save the business-wide rule (no `branch_uuid`, every branch needed) or a branch's |
| `POST /v1/tenant/confirmations/settings/reset` | remove a saved rule                                                               |

Activity history: `confirmations.updated` / `.reset`, `reservations.confirmed` / `.change_requested` (source `customer_panel` / `visit_link`).

## For TEC-924 (list and follow-up)

The reservation detail carries `confirmation` (status, channels, planned and sent attempts, next attempt, deadline, answer and channel, change request and note, outcome, last error); the reservations list filters and shows `confirmation_status`; the history has the `confirmation_*` events. The confirmation list screen, staff follow-up alerts and a manual "confirmed by phone" belong to TEC-924.

## Not here yet

- WhatsApp (TEC-922) and voice / IVR (TEC-923) adapters on the same channel port; answers by reply on those channels.
- Editing the confirmation message text (the catalog's Turkish default is used).
