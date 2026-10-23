# Plans and entitlements

Status: **implemented** (TEC-883). A business has a plan (a set of default limits), optional add-on modules and per-business limit overrides. The panel, the booking page and the future mobile app all read the effective result from the API.

## Where it lives

| Area | Place |
|------|-------|
| Catalog (plans, add-ons, resolution) | `backend/internal/platform/entitlement/entitlement.go` |
| Stored choice | `organizations.plan`, `addons`, `branch_limit`, `staff_limit` (migration 000040) |
| Usage counters | `organization_usage(organization_id, metric, period, quantity)`; metrics `sms`, `whatsapp`, `email`, `voice_call`; `period` is the first day of the month in the business time zone |
| Request scope | `orgctx.Scope.Plan`, `Addons`, `BranchLimit`, `StaffLimit`; `entitlement.ForScope(scope)` gives the effective set |
| Service | `backend/internal/modules/entitlements` (summary, limit checks, `AddUsage`) |
| API | `GET /v1/tenant/entitlements` (`tenant.settings.read`), `GET`/`PUT /v1/platform/organizations/{uuid}/entitlements` (platform organizations read / write), `entitlements` in `GET /v1/tenant/context` |
| Module gate | `middleware.RequireModule(sector.Module)` answers 403 `MODULE_NOT_ENABLED` |
| Panel | `frontend/src/features/entitlements`: Settings → Plan ve kullanım (tenant), the "Paket / özellikler" tab of a business (platform). `TenantProvider` exposes `entitlements`; `NavModulesProvider` and `useSector().hasModule` use `entitlements.modules`. |

## Plans

| Plan | Branches | Staff seats |
|------|----------|-------------|
| starter (Başlangıç, default for new businesses) | 1 | 5 |
| growth (Büyüme) | 3 | 20 |
| enterprise (Kurumsal) | unlimited | unlimited |

Plans carry limits only, no price; billing is outside the beta. The only add-on is `confirmation` (Konfirmasyon): without it the `confirmation` module is removed from the sector's modules. A plan never adds a module the sector hides.

## Rules

- Effective limit = override when set, else the plan default; `null` means unlimited. A `null` override means "use the plan default".
- Staff seats are members plus open invitations until the staff module exists.
- Adding a branch, a member or an invitation over the limit answers 409 `PLAN_LIMIT_REACHED` with `data: {kind, limit, used}`. The member and invitation checks run inside the transaction that locks the organization row, so they are race-safe; the branch check reads the count before the insert.
- Lowering a limit or the plan never removes data; it only blocks new additions.
- Businesses that existed before migration 000040 were moved to `enterprise` so nothing they already use is blocked.
- `PUT` replaces plan, add-ons and both overrides at once, invalidates the organization scope cache and records `organizations.entitlements_updated`.
- Communication modules (SMS, WhatsApp, e-mail, voice calls) count what they send with `AddUsage`; the summary shows the current month.

## Adding a gated feature

1. If the feature is a sell-separately module, add an add-on code to the catalog and the `addons` CHECK in a migration, then map it to its sector module.
2. Mount its tenant routes with `middleware.RequireModule(<module>)` after `RequireOrganization`.
3. Give its nav items `module: "<module>"`; the panel hides them when the plan lacks the module.
4. A countable resource with a seat limit checks it with the entitlements service before insert, inside the organization-row lock when it can race.
