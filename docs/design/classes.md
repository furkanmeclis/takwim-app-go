# Class timetable (TEC-970)

A gym places its group classes on a weekly timetable: "Pilates, Monday 18:00, Ayşe, Stüdyo 1, 12 seats". Each such weekday between the class's start and end dates is a session, and a group class (a service with `group_capacity`, TEC-904) can be booked only at its sessions. Code: `backend/internal/modules/classes` (timetable, sessions, attendees), `backend/internal/modules/availability/usecase/classes.go` and the engine's scheduled lines, migration `000073_class_schedules`. The module is the sector module `classes` (gym template); every route sits behind it.

## Data

| Table                   | Holds                                                                                                                                                                                                                                                                                                                                |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `class_schedules`       | a class: branch, group service, `weekday` (`mon` … `sun`), `start_time` (branch wall clock, `HH:MM`), trainer (`staff_id`, null = picked by the first booking), area (`resource_id`, null = picked), `capacity` (null = the service's `group_capacity`), `valid_from`, `valid_until` (null = no end), `is_active`, note; soft delete |
| `class_session_changes` | how one session differs from its class, keyed by class and local `session_date`: `cancelled` and / or its own `capacity`, with a reason. No row = the session as the class says                                                                                                                                                      |

References are composite foreign keys with `organization_id`; both tables are in `tenant_tables.txt`. Permissions `tenant.classes.read` (owner, admin, branch manager, reception) and `tenant.classes.write` (owner, admin, branch manager). The down migration drops both tables: the timetable is lost, bookings stay.

A session is identified by its class and its **local date** in the branch time zone (`/v1/tenant/classes/{uuid}/sessions/2030-01-14`). The duration is the service's (branch override included).

## Availability

- A group line is a _scheduled_ line: the engine offers only the starts of the sessions of its service in the branch on that day (`Line.Planned`, keyed by start). A day with no session is `no_class`; every other start is refused as an unavailable start, also for staff bookings. With no timetable a group class is not bookable at all.
- Seats of a session: the session change's capacity, else the class's capacity, else the service's `group_capacity`. The capacity hold trigger (TEC-905, `session_seats`) keeps A03: the seats of a session are never overbooked, also under concurrent bookings.
- The session's trainer needs no shift; it must only be free (no other visit, leave or session). Its area is fixed the same way. An empty trainer or area is picked by the first booking as in TEC-904, and later bookings join it.
- A scheduled session blocks its trainer and area from the start, before the first booking: individual visits and other classes at that time are not offered. A cancelled session frees them.
- Online bookings use the same sessions: the public API and the booking page get the timetable starts with no change of contract (the gym's class step on the booking page is TEC-971).

## Writes

All timetable writes run under the capacity advisory lock (`availabilityusecase.Holds.Run`), so a booking and a change of the same session are ordered.

- **Create / change a class.** The service must be a group service offered in the branch; the trainer must teach it and work in the branch (`not_allowed` for a service without staff); the area must be a resource of the branch of one of the service's kinds. `409` when the trainer (`staff_busy`) or the area (`resource_busy`) has an overlapping class on that weekday in an overlapping period, or the service already runs then in the branch (`duplicate`). Overlap compares wall-clock times.
- **Bookings are never orphaned.** A change or delete that would leave a booked future session out of the timetable (another day, time, branch or service, a shorter period, inactive, removed) is `409` `has_bookings`: cancel or move those bookings first. A new trainer or area applies to sessions without bookings.
- **One session.** `PATCH …/sessions/{date}` cancels it or sets its seats. A booked session is not cancelled (`has_bookings`), seats never go below the seats booked (`below_booked`), a session that has started is not changed (`past`). Both off deletes the change row. Restoring a cancelled session checks nothing; if its trainer or area got busy meanwhile it is simply not offered.

## Panel

`/t/{slug}/classes`: the sessions of the chosen days (at most 31) with their seats and state (`open`, `full`, `cancelled`, `past`), and the weekly timetable (add, edit, copy, remove). A session page lists its bookings: _Geldi_ completes the visit and _Gelmedi_ marks it no-show, the visit's own steps, so payments, packages and staff shares follow as for any visit.
