# Sectors

Status: **implemented** (TEC-882). Every business belongs to one of six sectors: hairdresser, restaurant, gym, car wash, clinic, beauty salon (requirements K-008, §5). The sector decides the default customer booking flow, what a reservation holds, the words the panel uses and which panel modules are visible or put first.

## Where it lives

| Area | Place |
|------|-------|
| Catalog (flow, capacity, terms, modules, highlights) | `backend/internal/platform/sector/sector.go` |
| Stored choice | `organizations.sector` (migration 000039, CHECK on the six codes, default `hairdresser`) |
| Request scope | `orgctx.Scope.Sector` (with `Timezone`, `Currency`) |
| API | `GET /v1/sectors` (catalog), `GET /v1/tenant/context` (`organization.sector` + the full `sector` definition) |
| Assignment | Platform: create / `PATCH /v1/platform/organizations/{uuid}` take `sector`. The tenant API does not change it (Takwim yönetimi assigns the sector). |
| Panel | `frontend/src/features/sectors` (`useSector()`: definition, `hasModule`, `term`), `NavModulesProvider` in the tenant shell |

The catalog is code, not data: a sector is a product decision and every client (panel, booking page, the future mobile app) reads the same definition from the API. A business's own settings fine-tune the flow later (requirements §6); they never change the sector's module set.

## Catalog

| Sector | Flow | Holds | Staff / resource words | Highlights | Hidden modules |
|--------|------|-------|------------------------|------------|----------------|
| Kuaför | branch → service → staff → datetime → products → contact → summary → payment | staff time; a chair when needed | çalışan / koltuk | calendar, staff shares, services, products | classes, vehicles |
| Restoran | branch → party size → datetime → area → contact → summary | a table (required), party size | çalışan / masa | resources, calendar, reservations | packages, products, staff shares, classes, vehicles |
| Spor salonu | branch → session type → class/trainer → datetime → contact → summary → payment | class seats or trainer + area | eğitmen / alan | classes, packages, calendar | vehicles |
| Oto yıkama | branch → vehicle → service → datetime → products → contact → summary → payment | a wash bay (required), staff when needed | çalışan / yıkama alanı | resources, vehicles, services | classes |
| Klinik | branch → appointment type → specialist → datetime → contact → summary → payment | specialist time; a room when needed | uzman / oda | calendar, staff, resources | products, classes, vehicles |
| Güzellik salonu | branch → service → specialist → datetime → products → contact → summary → payment | specialist time; room / device when needed | uzman / oda-cihaz | resources, packages, staff shares | classes, vehicles |

Modules: reservations, calendar, customers, services, packages, products, staff, staff_shares, resources, classes, vehicles, cash, finance, reports, confirmation, booking_page. Core areas (home, branches, members, roles, settings, integrations) are not modules; every business has them. Plan entitlements (TEC-883) narrow the visible set further and never add a module the sector hides.

## Panel rules

- A nav item of a module sets `module` (nav-engine). Hidden when the sector does not show it; until the tenant scope loads, module items stay hidden (no flash). The sector's highlights move to the top of their group. The command palette applies the same filter.
- Sector words come from `useSector().term("staff" | "resource" | "booking")` (`sectors.terms.<code>`), never hard-coded.
- The tenant home shows the sector card: highlights, booking flow, capacity and the other modules.

## Open items

- Module screens arrive with their issues and tag their nav items with `module`.
- Changing the sector of a business with data (resources, classes) needs a migration plan; for now the platform can change it freely.
