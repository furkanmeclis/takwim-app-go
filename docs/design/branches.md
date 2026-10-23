# Branches

Status: **implemented** (TEC-888, on top of the tenant core branches resource). A business has one or more branches (requirement K-001). Bookings, staff assignments, resources, stock and cash desks hang off a branch.

## Where it lives

| Area | Place |
|------|-------|
| Table | `branches` (migrations 000031, 000037 `timezone`, 000041 profile), `branch_images` (000041) |
| Module | `backend/internal/modules/branches` (usecase, repository, handler, io / bulk / search adapters) |
| API | `/v1/tenant/branches` (list, facets, meta, CRUD, activity, export / import / bulk), `/v1/tenant/branches/{uuid}/images` (gallery), `GET /v1/public/media/branches/{orgUuid}/images/{imageUuid}` (public photo) |
| Panel | `frontend/src/features/branches` (DataTable list, detail, create / edit form, quick edit drawer, gallery card) |

## Fields

| Field | Rule |
|-------|------|
| `code` | Unique in the business, upper case, 1-32 of `A-Z 0-9 _ -`. Import / export match on it. |
| `name` | Required, up to 255. |
| `slug` | URL name on the booking page, unique among live branches of the business, `^[a-z0-9]+(-[a-z0-9]+)*$`, up to 64. Empty on create or patch: generated from the name (Turkish letters folded, `-2`, `-3` … on a clash). An explicit slug that is taken answers 409 `CONFLICT` on `slug`. |
| `description` | Up to 2000; shown on the branch card of the customer screens. |
| `address`, `city`, `district`, `latitude` / `longitude` (both or neither), `phone`, `email` | Contact and location. |
| `timezone` | IANA zone or `null`. `null` follows the business zone; `effective_timezone` is the zone calendar decisions use. |
| `working_hours` | Weekly schedule, Monday first, `HH:MM` 24h, close after open; a closed day has no times. |
| `holiday_exceptions` | Dated overrides: closed days and special hours, one entry per date. |
| `prep_minutes` | Preparation / cleanup buffer between two reservations (0-10080). |
| `is_active` | Inactive branches cannot take new reservations. |
| `show_online` | Listed on the customer booking screens (requirements §6). The panel can still book a hidden branch. |
| `images`, `cover_url` | Gallery of up to 12 JPEG / PNG / WebP photos (sniffed, 5 MiB each). The first photo is the cover. |

## Media

Photos are stored under `organizations/{orgUUID}/branches/{branchUUID}/images/{imageUUID}.{ext}`. The API returns `public_url` / `cover_url`; clients never build object URLs. The public route serves only images of live branches of live businesses, checks the key prefix against the organization, and marks the response immutable (a new upload gets a new uuid).

## Decisions

- No separate branch logo: the business logo is the brand mark and the first gallery photo is the branch cover.
- Existing branches got their slug from their code in the migration; inserts that do not send a slug (seeds, raw inserts) get one from the code by a trigger.
- Service points (the reference app's pickup / delivery points) stay as they are until the resources module (rooms, tables, chairs) replaces them.
