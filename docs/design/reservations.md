# Reservation record (TEC-899)

A reservation is one visit of a customer: one or more services served back to back in a branch. Code: `backend/internal/modules/reservations` (`model`, `usecase`), migration `000049_reservations`. The capacity of the visit is a commitment of [capacity-holds.md](./capacity-holds.md); the record and its capacity are written in the same transaction (`Holds.Run`), so they always commit or fail together. The HTTP API and the panel screens come with TEC-900.

## Separate state fields (requirements §8)

| Field                 | Values                                                                                            | Moved by                                     |
| --------------------- | ------------------------------------------------------------------------------------------------- | -------------------------------------------- |
| `status`              | `requested`, `active`, `in_service`, `completed`, `cancelled`, `rejected`, `no_show`              | the lifecycle below                          |
| `approval_status`     | `pending`, `accepted`, `rejected` (`approval_mode`: `auto`, `staff`, `panel`)                     | the business                                 |
| `confirmation_status` | `not_required`, `scheduled`, `pending`, `confirmed`, `declined`, `expired`                        | the confirmation module (`not_required` now) |
| `payment_status`      | `on_site`, `online_pending`, `paid`, `refund_pending`, `partially_refunded`, `refunded`, `failed` | payments                                     |

A request the business accepted but the customer has not confirmed shows both values; a payment never confirms on its own.

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

## Transitions

| Action   | From                              | Effect                                                                                |
| -------- | --------------------------------- | ------------------------------------------------------------------------------------- |
| Accept   | `requested` + `pending`           | on site: booking, `active`; online payment: the hold now waits for the payment        |
| Reject   | `requested` + `pending`           | `rejected`, capacity freed                                                            |
| MarkPaid | `requested` + `accepted` + unpaid | `paid`, `active`, booking (called by the payments module)                             |
| Cancel   | `requested`, `active`             | `cancelled` with a reason, capacity freed; paid → `refund_pending`, unpaid → `failed` |
| Start    | `active`                          | `in_service`                                                                          |
| Complete | `active`, `in_service`            | `completed`; the time stays booked                                                    |
| NoShow   | `active`                          | `no_show`; the time stays booked (it was kept free)                                   |
| Expiry   | `requested` whose hold lapsed     | `cancelled` with `approval_timeout` or `payment_timeout` (the hold expiry hook)       |

Anything else is `ErrInvalidTransition`. A visit in service or finished is never cancelled (an old reminder cannot cancel it). Every change raises `version` and writes a `reservation_events` row (type, from / to status, actor, note); `expected_version` makes a stale caller fail with `ErrStaleVersion`.

Lapsed holds are expired by the minute sweep and by every writer before it works, and the expiry hook (`ExpireRequests`) cancels their requests in the same transaction. A writer whose own action then fails (e.g. accepting a request that just timed out) still commits the expiry: the action runs in a savepoint.

Releasing products and session rights on cancel comes with those modules; capacity is released here.

## Visibility

Every read and write checks the business and the caller's branch scope; a member with only `tenant.reservations.own` sees the reservations it created or serves. Anything else is `ErrNotFound`.

## Tests

`reservations_db_test.go`: panel booking (prices, snapshots, double booking writes nothing, lifecycle, versions, history, cancel frees the time, no-show, validation), online flows (manual approval on site / online payment, reject, approval timeout through a writer, automatic approval with a payment timeout through the sweep, payment rules, no hold left after a refusal), visibility and isolation, and parallel bookings of the last start (one record, one commitment).
