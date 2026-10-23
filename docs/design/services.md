# Services (TEC-894)

`/t/{slug}/services` is the service catalog of a business: what it sells, how long it takes, what it costs, which branches offer it and who can perform it. The module is visible when the sector (and plan) includes `services` (`RequireModule(sector.ModuleServices)`). The same API serves the booking page and the future mobile app; reservations (later issues) book these rows.

## Data (migration 000046)

| Table                | Holds                                                                                                                                                                                                                                                                                                              |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `service_categories` | name (unique per business, case-insensitive), description, `sort_order`, soft delete                                                                                                                                                                                                                               |
| `services`           | optional category, name (unique per business, case-insensitive), description, `duration_minutes` (5–1440), `buffer_before_minutes` / `buffer_after_minutes` (0–480), `price` (NUMERIC(14,2)), `payment_mode`, `staff_mode`, `resource_kinds[]`, `show_online`, `is_active`, `sort_order`, cover image, soft delete |
| `service_branches`   | the branches offering a service: optional branch `price` / `duration_minutes` (null = catalog value), `is_active`                                                                                                                                                                                                  |
| `service_staff`      | the staff who may perform a service: optional personal `price` / `duration_minutes`                                                                                                                                                                                                                                |

Every table carries `organization_id` with composite foreign keys, so a row never points at another business's category, branch or staff member.

## API

- `GET /v1/tenant/services` (`tenant.services.read`): list with `q` (name, description, category name), faceted `category` (uuids, `none` = uncategorized), `branch` (services offered there), `payment_mode`, `is_active`, `show_online`; sort `name`, `duration`, `price`, `sort_order`, `is_active`, `created_at`, `updated_at` (default: category order, `sort_order`, name). `meta`, `facets`, export, import, bulk `activate` / `deactivate` and search (`services` spec) follow the resources pattern.
- `POST` / `PATCH` / `DELETE /v1/tenant/services[/{uuid}]` (`tenant.services.write`). `branch_uuids` on create adds offers at the catalog price. The detail adds `staff`.
- `PUT /v1/tenant/services/{uuid}/offers` (`tenant.services.offer`): replaces the offers of the branches inside the caller's scope.
- `PUT /v1/tenant/services/{uuid}/staff` (`tenant.services.write`): replaces the staff list.
- `PUT|DELETE /v1/tenant/services/{uuid}/image` (`tenant.services.write`): JPEG / PNG / WebP up to 5 MiB, sniffed; served at `GET /v1/public/media/services/{orgUuid}/images/{imageUuid}` (public, immutable). Clients use `image_url` as returned.
- `GET|POST /v1/tenant/service-categories`, `PATCH|DELETE …/{uuid}` (writes need `tenant.services.write`).
- `GET /v1/tenant/services/{uuid}/activity`.

## Rules

- **Business level catalog**: every reader sees every service. Branches differ only through their offers.
- **Offers**: a branch manager with `tenant.services.offer` sets price, duration and availability for the branches of their scope; offers in other branches are left as they are (`offers[].editable` tells the panel which rows it may change). A branch outside the scope is `offers.N.branch_uuid` `not_found`.
- **Effective values**: a null override means the catalog value; `effective_price` / `effective_duration_minutes` are what the branch charges and books. Staff overrides work the same way on top of the service.
- **Staff mode**: `required` (a staff member is picked), `optional`, `none`. The default follows the sector (`none` for restaurants, otherwise `required`).
- **Resource kinds**: the kinds a booking needs, one of each (a haircut needs a `chair`).
- **Payment mode**: `on_site`, `online` or `both`.
- **Categories**: deleting a category keeps its services; they become uncategorized.
- **Import**: a row matches a service by name (case-insensitive) and updates it, otherwise creates one; `category` names a category and creates it when missing; `branch_codes` add offers to new services.
- **Roles**: owner and admin hold every slug; `sube_yoneticisi` reads, sets offers and exports; `resepsiyon` reads.
- **Activity**: `services.created|updated|deleted`, `services.offers_updated`, `services.staff_updated`, `services.image_updated|image_removed` and `services.category_created|updated|deleted`, recorded at business level (no branch).

## Later

Availability (which staff and resource are free for a service at a time) is the slot engine, see [availability.md](./availability.md). Packages, extras and online payment collection come with their own issues.
