# Resources (TEC-891)

`/t/{slug}/resources` lists the bookable capacity of each branch besides staff time: hairdresser chairs, restaurant tables and areas, gym areas, clinic rooms, beauty salon rooms and devices, car wash bays. The module is visible when the sector (and plan) includes `resources` (`RequireModule(sector.ModuleResources)`). The same API serves the future mobile app; reservations (later issues) hold these rows.

## Data (migration 000045)

| Table               | Holds                                                                                                                                                                                                                             |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `resources`         | branch, `kind` (`chair`, `table`, `area`, `room`, `device`, `bay`), name, free-text `zone`, description, `min_capacity`–`capacity`, `combinable`, `show_online`, `is_active`, `sort_order`, optional `working_hours`, soft delete |
| `resource_closures` | `starts_at` / `ends_at` instants the resource cannot be booked, reason, creator                                                                                                                                                   |

Both carry `organization_id` with composite foreign keys (`branches`, `resources`), so a row never points at another business's branch or resource. Names are unique per branch among live rows, case-insensitive (`uq_resources_branch_name`).

## API

- `GET /v1/tenant/resources` (`tenant.resources.read`): list with `q` (name, zone, description, branch name), faceted `branch` (uuids), `kind`, `zone`, `is_active`, `show_online`; sort `name`, `kind`, `zone`, `capacity`, `sort_order`, `is_active`, `created_at`, `updated_at` (default: branch, `sort_order`, name). `meta`, `facets`, export, import (match by `branch_code` + `name`, otherwise create), bulk `activate` / `deactivate` and search (`resources` spec) follow the branches pattern.
- `POST` / `PATCH` / `DELETE /v1/tenant/resources[/{uuid}]` (`tenant.resources.write`). The detail adds `branch_hours`.
- `GET|POST /v1/tenant/resources/{uuid}/closures`, `PATCH|DELETE …/closures/{closureUuid}` (writes need `tenant.resources.write`).
- `GET /v1/tenant/resources/{uuid}/activity`.

## Rules

- **Kind**: the API accepts every kind in every sector; the panel offers the sector's kinds first (`sector.capacity.resources`).
- **Capacity** is a people range for every kind, 1–1000, `min_capacity ≤ capacity`. A table seats `min_capacity`–`capacity`; `combinable` marks tables that can be joined for a large party (the panel shows it for tables only). Giving only `capacity` below the stored minimum lowers the minimum with it.
- **Zone** is free text (iç salon, bahçe, teras) rather than a table; the restaurant booking step "area" groups tables by it. Facets list the zones in use.
- **Hours**: `working_hours: null` means the branch's hours; own hours use the branch's day rules (`HH:MM`, close after open).
- **Closures** block the resource for a range (at most 366 days per entry); ranges may overlap.
- **Branch scope**: a member limited to some branches sees only their resources (others answer `404`) and can only create in or move to its branches (`branch_uuid` `not_found`).
- **Roles**: owner and admin hold every slug; `sube_yoneticisi` reads, writes and exports; `resepsiyon` reads.
- **Activity**: `resources.created|updated|deleted` and `resources.closure_created|updated|deleted`, always bound to the resource's branch.

## Later

Availability (which resource is free for a party size at a time) comes with reservations; a service names the resource kinds it needs (`resource_kinds`, see [services.md](./services.md)). There is no plan limit on resources.
