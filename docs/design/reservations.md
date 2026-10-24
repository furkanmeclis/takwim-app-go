# Reservation record (TEC-899)

A reservation is one visit of a customer: one or more services served back to back in a branch. Code: `backend/internal/modules/reservations` (`model`, `usecase`), migration `000049_reservations`. The capacity of the visit is a commitment of [capacity-holds.md](./capacity-holds.md); the record and its capacity are written in the same transaction (`Holds.Run`), so they always commit or fail together. The HTTP API and the panel screens come with TEC-900.

## Separate state fields (requirements §8)

| Field                 | Values                                                                                            | Moved by                                            |
| --------------------- | ------------------------------------------------------------------------------------------------- | --------------------------------------------------- |
| `status`              | `requested`, `active`, `in_service`, `completed`, `cancelled`, `rejected`, `no_show`              | the lifecycle below                                 |
| `approval_status`     | `pending`, `accepted`, `rejected` (`approval_mode`: `auto`, `staff`, `panel`)                     | the business                                        |
| `confirmation_status` | `not_required`, `scheduled`, `pending`, `confirmed`, `declined`, `expired`                        | the confirmation module (TEC-921, confirmations.md) |
| `payment_status`      | `on_site`, `online_pending`, `paid`, `refund_pending`, `partially_refunded`, `refunded`, `failed` | payments                                            |

A request the business accepted but the customer has not confirmed shows both values; a payment never confirms on its own. With the confirmation add-on the detail carries `confirmation` (the task: attempts, deadline, answer, outcome), the history has `confirmation_*` events, and the list filters by `confirmation_status`. A booking cancelled for no answer has the reason `confirmation_timeout`; one the customer cancelled, `customer_cancelled`.

## Lines and snapshots

`reservation_lines` keeps each service as it was booked: service name, staff member (name kept when the record is deleted), resources (`[{uuid, name, kind}]`), times, duration, buffers and the unit price. Price order: the person's price (`service_staff.price`) › the branch offer (`service_branches.price`) › the list price. `total_price` is the sum, in the business currency. Renaming a service or changing a price later never rewrites a booking. `vat_rate` / `staff_share_rate` stay empty until the finance modules fill them.

Also stored: `custom_fields` (answers to booking form questions, a JSON object), `customer_note`, `internal_note`, `source` (`panel`, `online`, `embed`), a per-business `number` and `version`.

## Creating

| Source / settings             | Hold                        | Result                                               |
| ----------------------------- | --------------------------- | ---------------------------------------------------- |
| Panel                         | checkout, confirmed at once | `active`, `accepted` (`panel`), `on_site`            |
| Online, automatic, on site    | checkout, confirmed at once | `active`, `accepted`, `on_site`                      |
| Online, automatic, online pay | payment (payment hold time) | `requested`, `accepted`, `online_pending`            |
| Online, manual approval       | approval (review window)    | `requested`, `pending`, `on_site` / `online_pending` |

The start is re-planned under the business's lock with the slot engine; a taken start is `ErrSlotTaken` with nothing written. Online bookings need the approval mode and a payment method in the business settings (`ErrOnlineNotReady`). An empty `payment` picks on site when the business takes it, else online; the method must be one every service of the visit takes (`payment` / `not_allowed`). The panel collects at the counter (`payment` other than `on_site` is refused).

A car wash booking may carry its `vehicle` (TEC-904): `type` (`car`, `suv`, `van`, `motorcycle`, `other`) and `plate` (letters, digits and spaces, at most 16, stored upper case with single spaces; `vehicle.type` / `vehicle.plate` `invalid`). The list, detail and list search (`q` matches the plate without spaces) show it. A group class booking takes `party_size` seats of its session (see [availability.md](./availability.md)).

## Transitions

| Action        | From                                                      | Effect                                                                                                                                                                                                                                                                                                                 |
| ------------- | --------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Accept        | `requested` + `pending`                                   | on site: booking, `active`; online payment: the hold now waits for the payment                                                                                                                                                                                                                                         |
| Reject        | `requested` + `pending`                                   | `rejected`, capacity freed                                                                                                                                                                                                                                                                                             |
| MarkPaid      | `requested` + `accepted` + unpaid                         | `paid`, `active`, booking                                                                                                                                                                                                                                                                                              |
| RecordPayment | any visit (payments module, [payments.md](./payments.md)) | under the business lock, with the collection posted in the same transaction: online and waiting → `paid`, `active`, booking (`paid`); cancelled / rejected by then → `refund_pending` (`late_payment`); otherwise `paid` once the total is covered (`payment_recorded`). On site refuses a cancelled or rejected visit |
| Cancel        | `requested`, `active`                                     | `cancelled` with a reason, capacity freed; paid → `refund_pending`, unpaid → `failed`                                                                                                                                                                                                                                  |
| Start         | `active`                                                  | `in_service`                                                                                                                                                                                                                                                                                                           |
| Complete      | `active`, `in_service`                                    | `completed`; the time stays booked                                                                                                                                                                                                                                                                                     |
| NoShow        | `active`                                                  | `no_show`; the time stays booked (it was kept free)                                                                                                                                                                                                                                                                    |
| Expiry        | `requested` whose hold lapsed                             | `cancelled` with `approval_timeout` or `payment_timeout` (the hold expiry hook)                                                                                                                                                                                                                                        |

Anything else is `ErrInvalidTransition`. A visit in service or finished is never cancelled (an old reminder cannot cancel it). Every change raises `version` and writes a `reservation_events` row (type, from / to status, actor, note); `expected_version` makes a stale caller fail with `ErrStaleVersion`.

Lapsed holds are expired by the minute sweep and by every writer before it works, and the expiry hook (`ExpireRequests`) cancels their requests in the same transaction. A writer whose own action then fails (e.g. accepting a request that just timed out) still commits the expiry: the action runs in a savepoint.

Products of the visit ([reservation-products.md](./reservation-products.md), TEC-919) are released by reject, cancel, no-show and expiry and handed over by complete, in the same transaction. Releasing session rights on cancel comes with that module; capacity is released here.

## Rescheduling and notes (TEC-900)

`Reschedule` moves a request or an active booking in one transaction: it lets the old hold go, plans the new start (panel channel) and takes it again; for a booking it confirms at once, for a request it keeps the hold reason and the time left. Lines with the same service and person keep their old price; changed lines take today's price. Lines are rewritten, `version` goes up and a `rescheduled` event is written, so jobs planned against the old version (confirmation messages) no longer apply. If the new start is taken, nothing changes. `UpdateNotes` changes the customer and internal notes (`notes_updated` event).

## Panel API (TEC-900)

| Route                                                                               | What                                                                                                                                   |
| ----------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /v1/tenant/reservations`                                                       | List with search (number, customer, phone), filters (branch, staff, status, approval, payment, source, customer, start range) and sort |
| `GET /v1/tenant/reservations/meta`, `/facets`                                       | List metadata; branch and staff facets                                                                                                 |
| `POST /v1/tenant/reservations`                                                      | Panel booking: always source `panel`, paid at the business, confirmed at once                                                          |
| `GET /v1/tenant/reservations/{uuid}`                                                | Record with lines and history                                                                                                          |
| `PATCH /v1/tenant/reservations/{uuid}`                                              | Notes                                                                                                                                  |
| `POST /v1/tenant/reservations/{uuid}/{accept,reject,cancel,start,complete,no-show}` | Transitions; optional `expected_version`, `note`, `reason` (cancel)                                                                    |
| `POST /v1/tenant/reservations/{uuid}/reschedule`                                    | New start, optionally other people per line                                                                                            |

Errors: a taken start is `409 starts_at/taken`, a stale version `409 expected_version/stale`, a transition the state does not allow `409 status/invalid_transition`. Reads need `tenant.reservations.read` or `.own`, writes `.write` or `.own`; the module must be on. The list is live (`tenant.reservations` channel) for members with read. Export, bulk actions and search: see below (TEC-963).

## Export, bulk actions and Cmd+K (TEC-963)

All three work inside the caller's reach, exactly like the list: the business, the branch scope and, for a member with only `tenant.reservations.own`, the bookings it created or serves. Worker jobs re-resolve the requester's membership (`orgctx.WithJobScope`) and act as that user (`orgctx.JobActor`).

- **Export** (`POST /v1/tenant/reservations/export`, `tenant.reservations.export` plus read or own): the list filters, `q` and `sort`; dates and times in the business timezone; state columns carry their `tr` / `en` labels. There is no import: a booking is only written through the capacity engine.
- **Bulk** (`POST /v1/tenant/reservations/bulk`, write or own plus `tenant.reservations.bulk.{accept,reject,cancel}`): each record runs its own transition, so capacity is confirmed or released with it. Cancel takes `params.reason`; `params.versions` (`uuid:version,…`) carries the versions the panel listed. Records the action does not apply to (`invalid_transition`), changed since (`stale`) or whose hold lapsed (`expired`) are skipped and listed with their number in the summary (`skipped`, `issues`). Not reversible: a freed time may be booked again at once.
- **Cmd+K** (spec `reservations`): `#number · customer`, the visit time and branch; number, name and phone (with or without the country code) match. Documents carry `branch_ids` and `owner_user_ids` (booker and the accounts of the staff serving it); a member with only the own slug matches only documents listing it (`searchengine.TenantOwnFilter`). Documents are written on booking and on a move; a state change does not touch them. A customer rename shows up after the next reindex.

Roles (migration 000050): owner, admin and branch manager get export and every bulk action; reception gets bulk accept and reject.

## Calendar (TEC-901)

`GET /v1/tenant/reservations/calendar?from=&to=&by=&branch=` answers at most a week of local dates. `by=staff` and `by=resource` need a branch: the columns are the active people working there (their shifts inside the opening hours, their leave) or the active resources (their own hours or the branch hours, their closures). `by=branch` (all branches of the caller's scope) has one column per branch with its opening hours. Items are visit lines placed in their columns; a person or resource no longer listed keeps a column while it has visits, and lines without one go to `unassigned`. Cancelled and rejected visits only with `include_cancelled=true`. An own calendar member gets its own column and the lines it created or serves.

The panel calendar (`/t/{slug}/calendar`) shows a day (one column per person / resource / branch) or a week (one column per day, all visits or one column's). Clicking free space opens the booking form with branch, day, person and time filled in (the time is picked once the slot engine offers it). Dragging a visit asks for confirmation and calls reschedule with the visit shifted as a whole; dropped on another person's column, the dragged line goes to that person. The API decides whether the new time is free. The page refetches on the live channel of `tenant.reservations`.

## Staff panel (TEC-902)

`GET /v1/tenant/reservations/agenda?from=&to=` is the signed-in member's own programme (at most a week, default today): its staff record, its working periods in every branch of its scope (shifts inside the opening hours), its leave and the lines it serves with the customer's phone and note. Any member who may see reservations (`read` or `own`) gets its own record; `staff` is null when the account is not linked to a staff record.

A member with only `tenant.reservations.own` acts on its own visits. A visit shared with a colleague (another person serves one of its lines) changes status only when the member serves every staffed line; otherwise the API answers `409` `lines` / `other_staff`. It can move a shared visit as a whole: the colleague's lines must keep their service and person and are checked again at the new time (the own pin is relaxed for them only). Notes stay editable. Earnings are left to TEC-917.

The panel page `/t/{slug}/my-agenda` ("Programım") shows a day: the shifts and leave as chips, the visits in a table (cards on a phone) with status, phone (tap to call), note and the same action and reschedule dialogs as the list. Known conflicts (`other_staff`, `taken`, `stale`, `invalid_transition`) are toasted in the panel's language.

## Visibility

Every read and write checks the business and the caller's branch scope; a member with only `tenant.reservations.own` sees the reservations it created or serves. Anything else is `ErrNotFound`.

## Tests

`reservations_db_test.go`: panel booking (prices, snapshots, double booking writes nothing, lifecycle, versions, history, cancel frees the time, no-show, validation), online flows (manual approval on site / online payment, reject, approval timeout through a writer, automatic approval with a payment timeout through the sweep, payment rules, no hold left after a refusal), visibility and isolation, and parallel bookings of the last start (one record, one commitment). `engines_db_test.go` (TEC-963): export, bulk and search documents stay inside the caller's reach (another branch, another member's own calendar, a missing slug), a worker job acts with its requester's membership, and bulk cancel frees the time and reports stale and inapplicable records.
