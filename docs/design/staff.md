# Staff (TEC-890)

`/t/{slug}/staff` lists the people who serve customers: which branches they work in, their weekly shifts in each branch, their time off and whether they can be booked online. The module is visible when the sector (and plan) includes `staff` (`RequireModule(sector.ModuleStaff)`). The same API serves the future mobile app.

## Data (migration 000044)

| Table            | Holds                                                                                                                                     |
| ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `staff`          | name, title, bio, phone, email, calendar `color` (`#RRGGBB`), `show_online`, `is_active`, optional `user_id` (panel account), soft delete |
| `staff_branches` | one row per branch the person works in, with `shifts` (JSONB weekly shifts of that branch)                                                |
| `staff_time_off` | `kind` (`leave`, `sick`, `training`, `other`), `starts_at` / `ends_at` instants, note, creator                                            |

All three carry `organization_id` with composite foreign keys, so a row can never point at another business's person or branch. One account links to at most one live staff record (`uq_staff_organization_user`).

## API

- `GET /v1/tenant/staff` (`tenant.staff.read`): list with `q` (name, title, phone, email, account), faceted `branch` (uuids), `is_active`, `show_online`, `has_account`; sort `name`, `title`, `is_active`, `created_at`, `updated_at`. `meta`, `facets`, export, import (match by `uuid`, otherwise create; branches as `branch_codes`), bulk `activate` / `deactivate` and search (`staff` spec) follow the branches pattern.
- `POST` / `PATCH` / `DELETE /v1/tenant/staff[/{uuid}]` (`tenant.staff.write`). `branch_uuids` replaces the assignments; `account_uuid` links a member (`null` unlinks). `GET /v1/tenant/staff/linkable-members` lists members without a staff record.
- `PUT /v1/tenant/staff/{uuid}/branches/{branchUuid}/shifts` (`tenant.schedules.write`): the weekly shifts of the person in one branch.
- `GET|POST /v1/tenant/staff/{uuid}/time-off`, `PATCH|DELETE …/time-off/{timeOffUuid}` (writes need `tenant.schedules.write`).
- `GET /v1/tenant/staff/{uuid}/activity`.

## Rules

- **Shifts** are wall-clock `HH:MM` times of the branch, Monday first; a day without shifts is a day off there. At most 6 shifts a day and 4 breaks a shift; `24:00` may end a shift; no overnight shifts. Shifts of a day must not overlap (`overlap`), breaks must sit inside their shift (`outside`) and not overlap, and the same hours in another branch of the person answer `branch_overlap` (compared by weekday wall-clock time). The API sorts shifts and breaks.
- **Time off** is person-level: it blocks every branch. At most 366 days per entry; entries may overlap.
- **Seats**: a person without an account uses a staff seat of the plan (`CheckStaffAdd`); a linked account counts as the member it is. Unlinking needs a free seat. The owner may be linked as staff on their own seat.
- **Branch scope**: a member limited to some branches sees staff with at least one assignment in its scope (others answer `404`), must give one of its branches on create, keeps assignments outside its scope untouched on patch (`editable: false` in the detail) and gets `403` deleting a person who also works elsewhere.
- **Activity**: `staff.created|updated|deleted` (changes of the profile, account and branch names), `staff.shifts_updated` (bound to the branch, before / after per day), `staff.time_off_created|updated|deleted`. Profile events carry the branch only when the person works in a single branch.

## Later

Service competence and per-person price / duration overrides live on the service (`PUT /v1/tenant/services/{uuid}/staff`, see [services.md](./services.md)). Profile photos are not stored yet.
