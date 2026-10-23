# Availability engine (TEC-897)

One engine answers "when can this be booked, and who / what serves it?" for the panel, the booking page, the widget and the public API (TEC-906). Code: `backend/internal/modules/availability` (`engine` = pure planner without I/O, `usecase` = loads the inputs, `handler` + `routes.go` = HTTP).

## Inputs

| Input                     | Source                                                                                                                                                   |
| ------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Opening hours             | `branches.working_hours`, replaced on a date by `branches.holiday_exceptions` (closed or other hours). The branch time zone, else the business's.        |
| Working time of a person  | `staff_branches.shifts` of the requested branch, minus breaks                                                                                            |
| Busy time of a person     | `staff_time_off` (every branch) + commitments of every branch through `BusySource` (holds and bookings of [capacity-holds.md](./capacity-holds.md))      |
| Who may perform a service | `service_staff` ∩ people working in the branch (active, live)                                                                                            |
| Duration                  | personal `service_staff.duration_minutes` › branch `service_branches.duration_minutes` › `services.duration_minutes`                                     |
| Buffers                   | `services.buffer_before_minutes` / `buffer_after_minutes`                                                                                                |
| Resources                 | `services.resource_kinds` (one of each); active resources of the branch, their own `working_hours` (else the branch's), `resource_closures`, commitments |
| Booking window (online)   | business settings `reservation.min_lead_minutes` / `max_advance_days`                                                                                    |

## Rules

- **Lines run back to back.** A booking is one or more services (a bundle, TEC-895, is a list of lines); line _i+1_ starts when the customer's time of line _i_ ends. The customer's whole time sits inside one open interval of the branch.
- **Buffers hold, the customer does not wait.** A line holds its staff member and resources from `start − buffer_before` to `end + buffer_after`; that held time must be inside the person's working time (breaks excluded) / the resource's hours and clear of busy time. The first start of a day with a 10 minute preparation buffer is therefore 09:10 at the earliest when the shift starts at 09:00 (09:15 on a 15 minute step).
- **A booking never conflicts with itself.** When the same person or resource serves consecutive lines, the cleaning of one line may overlap the preparation of the next. A resource used by an earlier line is kept for the next one (same chair for the whole visit).
- **One calendar per person (A04).** Busy time of a person is loaded from every branch, so the same hours are never offered in two branches. Shifts of a person never overlap across branches (staff validation, TEC-890).
- **Staff mode.** `required`: one of the people who may perform the service; `optional`: a free person when there is one, otherwise the line runs without; `none`: no person.
- **"Fark etmez".** Without `staff_uuid` the planner tries the people with the least booked time that day first (ties: name order) and backtracks when a choice leaves no room for the next lines (a personal duration changes where the next line starts). Failed (line, start) pairs are memoised, so bundles stay fast with many people.
- **Steps.** Starts are offered every `step_minutes` (5–240, multiple of 5, default 15) counted from the opening of each interval. `Check` accepts any start (the reservation flow re-checks the exact start under its lock).
- **Booking window.** Panel: from now on, no upper limit (staff may book far ahead or squeeze in a walk-in). Online: from now + minimum notice to the end of today + maximum advance; until both settings are set the online channel answers `409` `online_not_ready` (the readiness of business settings lists them too).
- **Online visibility.** The online channel uses services, staff and resources with `show_online`. Inactive services, branches and resources, and services the branch does not offer, are never planned.

## Day statuses

`available` (at least one start), `closed` (no opening that day), `too_far` (after the latest bookable day), `no_staff` (a line needs a person and nobody who may perform it works that day or everyone is on leave), `too_soon` (every start is before the earliest start), `full` (open and staffed, every start taken).

## API

`POST /v1/tenant/availability/search` (`reservations` module; `tenant.reservations.read`, or `tenant.reservations.own`): body `branch_uuid`, `lines[]` (`service_uuid`, `staff_uuid` or null), `from` / `to` (local dates, default 7 days, at most 31), `step_minutes`, `channel` (`panel` | `online`). The answer has the branch, time zone, the window (`earliest_start`, `latest_date`), the planned lines (durations, staff mode, resource kinds, the people who may perform each) and every day with its slots; a slot names the staff member and resources of each line.

- A member with only `tenant.reservations.own` (Çalışan) searches its own calendar: every line that takes a person is planned with the member's staff record; naming someone else, or having no staff record, is `403`.
- A branch outside the caller's branch scope or of another business is `404`; services and staff are checked per line (`lines.N.service_uuid` `not_found` / `inactive` / `not_offered`, `lines.N.staff_uuid` `not_found`). RT-A01 names another business's branch, service and staff member in the body.

## Service API

`usecase.Service.Search(org, input, caller)` and `Check(org, input, caller, start)`; `SetBusySource` plugs in reservations and holds (`AllocationBusy`, see [capacity-holds.md](./capacity-holds.md)). The public booking API (TEC-906) calls the same service with the online channel; the reservation record (TEC-899) calls `Check` inside its transaction after taking the capacity lock (TEC-898).

## Tests

`engine/engine_test.go` (shifts and breaks, branch hours and special days, time off, A04, buffers, any staff and load order, pinned staff, personal durations, resources with own hours and closures, optional / no staff, bundles with backtracking, booking window and day statuses, `Check`, memoised performance) and `engine/schedule_test.go` (clock parsing, breaks, wall clock on daylight saving days). `availability_db_test.go` drives the HTTP route against Postgres (any staff, A04 through `BusySource`, time off, a week with a special day, bundle with a closed room, online window and visibility, the past, own calendar, branch scope, validation and isolation).
