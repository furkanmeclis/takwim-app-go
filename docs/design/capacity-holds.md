# Capacity holds and double booking (TEC-898)

The slot engine ([availability.md](./availability.md)) says which starts are free. This layer makes taking one safe: a start is consumed by writing a commitment, and two commitments of the same person or resource can never overlap (A03). Code: `backend/internal/modules/availability/usecase/holds.go`, migration `000048_capacity_holds`.

## Data

| Table                  | Holds                                                                                                                                                                                                                                                                                                            |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `capacity_holds`       | one visit: `kind` `hold` (timed, `expires_at`) or `booking` (confirmed), `reason` (`approval`, `payment`, `checkout`), `status` (`active`, `released`, `expired`), `channel`, the customer's time (`starts_at` – `ends_at`), the record it belongs to (`ref_type`, `ref_uuid`), creator, release time and reason |
| `capacity_allocations` | what each line holds: a staff member **or** a resource for a `period` (held time, buffers included)                                                                                                                                                                                                              |

`capacity_allocations` has two exclusion constraints (`btree_gist`): `(organization_id, staff_id, period &&)` and `(organization_id, resource_id, period &&)`. They do not look at the branch, so a person has one calendar across branches (A04). Only active commitments have allocation rows; releasing or expiring a hold deletes its rows in the transaction that changes the status. The rows of one holder in one visit are merged where they touch (a cleaning buffer overlapping the next line's preparation), so a visit never conflicts with itself.

## Why double booking is impossible

1. **Exclusion constraints (database).** Two overlapping allocations of one person or resource cannot exist, whatever the application does. A concurrent writer that loses gets `23P01` (it waits for the winner's commit, then fails), or a deadlock when two writers wait on each other's exclusion check (they compete for the same capacity, so the aborted one lost it); the API answers `409` `starts_at` / `taken` with nothing written.
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

`AllocationBusy` is the slot engine's `BusySource` (`ExcludeHoldID` leaves one commitment out, for moving it).

## Composing with records

`Run(org, fn)` opens a capacity transaction: lock, expire lapsed holds (running the expiry hook), then `fn` in a savepoint, commit. When `fn` fails its writes are undone but the expiry still commits. Inside it `CreateIn`, `ConfirmIn`, `RenewIn` (new reason and expiry, e.g. an accepted request now waiting for payment) and `ReleaseIn` work on the transaction, so the reservation record ([reservations.md](./reservations.md)) and its capacity commit together. `SetExpiryHook` registers the reaction to lapsed holds (the reservation record cancels its requests); the API server and the worker both register it.

## API

- `POST /v1/tenant/availability/holds` (`tenant.reservations.write`, or `.own` on the caller's own calendar): body as the search (`branch_uuid`, `lines`, `channel`) plus `starts_at`, `reason`, `minutes`.
- `GET|DELETE /v1/tenant/availability/holds/{uuid}`, `POST …/{uuid}/extend` (`minutes`). Holds of another business or outside the branch scope are `404`; a `.own` caller sees only the holds it created.
- `409` details: `starts_at` / `taken`, `uuid` / `expired`, `uuid` / `not_active`, `uuid` / `not_hold`.

Confirming is not an HTTP action: the reservation flow confirms inside its own transaction.

## Group class seats (TEC-904)

A booking of a group class names its session on the hold: `session_service_id` (with `branch_id` and `starts_at`), the `seats` it takes and the `session_seats` of the class when it was booked (migration `000056_group_sessions_vehicle`). Two database rules keep the last seat from going to two writers (A03), with or without the organization lock:

1. **Seat count.** A trigger on `capacity_holds` takes a transaction advisory lock of the session and sums the seats of its active commitments; more than `session_seats` raises an exclusion violation (`ErrSlotTaken`).
2. **Shared holders.** A trigger fills `capacity_allocations.session_key` from the hold. The exclusion constraints compare `COALESCE(session_key, 'hold:' || hold_id)` with `<>`: allocations of one session may share the trainer and the area; any other pair of one holder still never overlaps.

Releasing or expiring a booking frees its seats with its rows. `TestGymTemplate` runs eight parallel writers for three seats, with and without the lock, and gets exactly three winners.

## Tests

`TestA03LastSlotConcurrency` (2, 6 and 12 parallel writers, with and without the lock; `-race -count=20`), `TestHoldLifecycle` (a quote holds nothing, a hold consumes in every branch, lapses before the sweep, confirm / extend / release once, the sweep, hold times from settings), `TestBundleHoldMergesItsOwnBuffers`, `TestExclusionConstraintRefusesDoubleBooking` (direct SQL; touching ranges allowed), `TestHoldsOverHTTP` (routes, own calendar, `409` details) and RT-A01 (another business's branch, staff and hold uuid).
