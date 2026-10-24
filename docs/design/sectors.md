# Sectors

Status: **implemented** (TEC-882). Every business belongs to one of six sectors: hairdresser, restaurant, gym, car wash, clinic, beauty salon (requirements K-008, §5). The sector decides the default customer booking flow, what a reservation holds, the words the panel uses and which panel modules are visible or put first.

## Where it lives

| Area                                                 | Place                                                                                                                                               |
| ---------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| Catalog (flow, capacity, terms, modules, highlights) | `backend/internal/platform/sector/sector.go`                                                                                                        |
| Stored choice                                        | `organizations.sector` (migration 000039, CHECK on the six codes, default `hairdresser`)                                                            |
| Request scope                                        | `orgctx.Scope.Sector` (with `Timezone`, `Currency`)                                                                                                 |
| API                                                  | `GET /v1/sectors` (catalog), `GET /v1/tenant/context` (`organization.sector` + the full `sector` definition)                                        |
| Assignment                                           | Platform: create / `PATCH /v1/platform/organizations/{uuid}` take `sector`. The tenant API does not change it (Takwim yönetimi assigns the sector). |
| Panel                                                | `frontend/src/features/sectors` (`useSector()`: definition, `hasModule`, `term`), `NavModulesProvider` in the tenant shell                          |

The catalog is code, not data: a sector is a product decision and every client (panel, booking page, the future mobile app) reads the same definition from the API. A business's own settings fine-tune the flow (requirements §6, [booking-rules.md](./booking-rules.md)): they hide optional steps (staff, products, area) and add questions; they never change the sector's module set, the step order or the capacity checks.

## Catalog

| Sector          | Flow                                                                              | Holds                                      | Staff / resource words | Highlights                                 | Hidden modules                                      |
| --------------- | --------------------------------------------------------------------------------- | ------------------------------------------ | ---------------------- | ------------------------------------------ | --------------------------------------------------- |
| Kuaför          | branch → service → staff → datetime → products → contact → summary → payment      | staff time; a chair when needed            | çalışan / koltuk       | calendar, staff shares, services, products | classes, vehicles                                   |
| Restoran        | branch → party size → datetime → area → contact → summary                         | a table (required), party size             | çalışan / masa         | resources, calendar, reservations          | packages, products, staff shares, classes, vehicles |
| Spor salonu     | branch → session type → class/trainer → datetime → contact → summary → payment    | class seats or trainer + area              | eğitmen / alan         | classes, packages, calendar                | vehicles                                            |
| Oto yıkama      | branch → vehicle → service → datetime → products → contact → summary → payment    | a wash bay (required), staff when needed   | çalışan / yıkama alanı | resources, vehicles, services              | classes                                             |
| Klinik          | branch → appointment type → specialist → datetime → contact → summary → payment   | specialist time; a room when needed        | uzman / oda            | calendar, staff, resources                 | products, classes, vehicles                         |
| Güzellik salonu | branch → service → specialist → datetime → products → contact → summary → payment | specialist time; room / device when needed | uzman / oda-cihaz      | resources, packages, staff shares          | classes, vehicles                                   |

Modules: reservations, calendar, customers, services, packages, products, staff, staff_shares, resources, classes, vehicles, cash, finance, reports, confirmation, booking_page. Core areas (home, branches, members, roles, settings, integrations) are not modules; every business has them. Plan entitlements (TEC-883, `docs/design/entitlements.md`) narrow the visible set further and never add a module the sector hides.

## Panel rules

- A nav item of a module sets `module` (nav-engine). Hidden when the sector does not show it; until the tenant scope loads, module items stay hidden (no flash). The sector's highlights move to the top of their group. The command palette applies the same filter.
- Sector words come from `useSector().term("staff" | "resource" | "booking")` (`sectors.terms.<code>`), never hard-coded.
- The tenant home shows the sector card: highlights, booking flow, capacity and the other modules.

## Starter templates (TEC-903, TEC-904)

`internal/modules/sectortemplates` seeds a starter catalog into the default branch when a business is created (`branches` `OnCreated` hook, same transaction), so a new business can take bookings before it sets anything up. A template only seeds what is generic to the sector; every record is ordinary data the owner edits or deletes.

- **Restaurant**: service "Masa rezervasyonu" (90 minutes, no staff, needs a `table`, paid at the business, price 0) and seven tables: Salon Masa 1–2 (1–2 guests), Masa 3–4 (2–4, combinable), Masa 5 (4–6); Bahçe Masa 6–7 (2–4, combinable). The panel booking form then asks for the number of guests and an area, picks the only service and lets the engine choose the table ([availability.md](./availability.md)).
- **Gym**: an area "Stüdyo" (capacity 12) and the group class "Grup dersi" (60 minutes, 12 seats per session, trainer optional, needs an `area`, paid at the business, price 0). Bookings of one start share the session's trainer and studio until the seats run out ([availability.md](./availability.md), [capacity-holds.md](./capacity-holds.md)); an individual session is an ordinary service with a required trainer.
- **Car wash**: two wash bays ("Yıkama alanı 1–2"). No services: the wash menu and prices are the business's own; a new service asks for a `bay` and takes a person when one is free by default. The panel booking form asks for the vehicle (type and plate).
- **Clinic**: two examination rooms ("Muayene odası 1–2"). No appointment types: they and the specialists are the clinic's own; an appointment type takes a room when it lists `room`. No health record fields exist anywhere (requirements §5).
- **Hairdresser and beauty salon**: nothing is seeded. Their booking flow already comes from the sector definition, and each salon's services, prices, chairs, rooms and devices are its own; a generic list would only be deleted.
- Changing the sector later seeds nothing; the owner edits services and resources as usual.

## Open items

- Module screens arrive with their issues and tag their nav items with `module`.
- Changing the sector of a business with data (resources, classes) needs a migration plan; for now the platform can change it freely.
- Package rights (paket hakkı) of the gym, clinic and beauty flows come with multi-session packages (TEC-920).
- A class timetable (recurring sessions with a fixed trainer and area, the classes module) comes with TEC-970; the booking page steps for the vehicle and group class seats with TEC-971.
