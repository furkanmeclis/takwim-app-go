# Capacity holds and double booking (TEC-898)

The slot engine ([availability.md](./availability.md)) says which starts are free. This layer makes taking one safe: a start is consumed by writing a commitment, and two commitments of the same person or resource can never overlap (A03). Code: `backend/internal/modules/availability/usecase/holds.go`, migration `000048_capacity_holds`.

## Data

| Table                  | Holds                                                                                                                                                                                                                                                                                                            |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `capacity_holds`       | one visit: `kind` `hold` (timed, `expires_at`) or `booking` (confirmed), `reason` (`approval`, `payment`, `checkout`), `status` (`active`, `released`, `expired`), `channel`, the customer's time (`starts_at` – `ends_at`), the record it belongs to (`ref_type`, `ref_uuid`), creator, release time and reason |
| `capacity_allocations` | what each line holds: a staff member **or** a resource for a `period` (held time, buffers included)                                                                                                                                                                                                              |

`capacity_allocations` has two exclusion constraints (`btree_gist`): `(organization_id, staff_id, period &&)` and `(organization_id, resource_id, period &&)`. They do not look at the branch, so a person has one calendar across branches (A04). Only active commitments have allocation rows; releasing or expiring a hold deletes its rows in the transaction that changes the status. The rows of one holder in one visit are merged where they touch (a cleaning buffer overlapping the next line's preparation), so a visit never conflicts with itself.

## Why double booking is impossible

1. **Exclusion constraints (database).** Two overlapping allocations of one person or resource cannot exist, whatever the application does. A concurrent writer that loses gets `23P01` (it waits for the winner's commit, then fails) and the API answers `409` `starts_at` / `taken` with nothing written.
2. **Organization lock (application).** Writers of a business serialise on `pg_advisory_xact_lock`. Inside it they expire lapsed holds, re-plan the start with the slot engine on fresh data (busy time = allocations of active, unexpired commitments) and write. The lock spares losers the work and keeps answers clean; it is not the correctness argument: `TestA03LastSlotConcurrency` runs 6 and 12 parallel writers with the lock off and still gets exactly one winner.

## Holds

| Reason     | Waits for                          | Default time                                            |
| ---------- | ---------------------------------- | ------------------------------------------------------- |
| `checkout` | the customer finishing the booking | 10 minutes                                              |
| `approval` | the business accepting a request   | settings `staff_review_window_minutes`, else 60 minutes |
| `payment`  | online payment                     | settings `payment_hold_minutes`, else 15 minutes        |

`minutes` (1–1440) overrides it. A hold **stops counting at `expires_at`**: searches and writers ignore it from that instant, so an abandoned payment page never leaves a lasting lock. The worker's minute sweep (`app:capacity:expire-holds`) then marks it `expired` and deletes its rows; every writer expires lapsed holds of the business first as well.

| Operation | Effect                                                                                                                                            |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| Create    | lock → expire lapsed → plan the start (`Service.Check`) → write hold + allocations; `ErrSlotTaken` otherwise                                      |
| Confirm   | hold → booking on the same rows (capacity consumed once), optional `ref_type` / `ref_uuid`; a lapsed hold is expired and answers `ErrHoldExpired` |
| Extend    | `expires_at = now + minutes` for an active hold                                                                                                   |
| Release   | frees a hold or booking exactly once (`ErrNotActive` afterwards)                                                                                  |
| ExpireDue | the sweep, per business under its lock                                                                                                            |

`AllocationBusy` is the slot engine's `BusySource` (`ExcludeHoldID` leaves one commitment out, for moving it). The reservation record (TEC-899) creates a hold or confirms one with `ref_type = reservation`.

## API

- `POST /v1/tenant/availability/holds` (`tenant.reservations.write`, or `.own` on the caller's own calendar): body as the search (`branch_uuid`, `lines`, `channel`) plus `starts_at`, `reason`, `minutes`.
- `GET|DELETE /v1/tenant/availability/holds/{uuid}`, `POST …/{uuid}/extend` (`minutes`). Holds of another business or outside the branch scope are `404`; a `.own` caller sees only the holds it created.
- `409` details: `starts_at` / `taken`, `uuid` / `expired`, `uuid` / `not_active`, `uuid` / `not_hold`.

Confirming is not an HTTP action: the reservation flow confirms inside its own transaction.

## Not yet

Group class quotas (several customers in one session) come with the classes module: a session will hold a counted capacity instead of an exclusive one. Party sizes on restaurant tables come with restaurant bookings.

## Tests

`TestA03LastSlotConcurrency` (2, 6 and 12 parallel writers, with and without the lock; `-race -count=20`), `TestHoldLifecycle` (a quote holds nothing, a hold consumes in every branch, lapses before the sweep, confirm / extend / release once, the sweep, hold times from settings), `TestBundleHoldMergesItsOwnBuffers`, `TestExclusionConstraintRefusesDoubleBooking` (direct SQL; touching ranges allowed), `TestHoldsOverHTTP` (routes, own calendar, `409` details) and RT-A01 (another business's branch, staff and hold uuid).
