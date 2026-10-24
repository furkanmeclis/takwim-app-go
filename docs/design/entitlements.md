# Plans and entitlements

Status: **implemented** (TEC-883). A business has a plan (a set of default limits), optional add-on modules and per-business limit overrides. The panel, the booking page and the future mobile app all read the effective result from the API.

## Where it lives

| Area                                 | Place                                                                                                                                                                                                                                                    |
| ------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Catalog (plans, add-ons, resolution) | `backend/internal/platform/entitlement/entitlement.go`                                                                                                                                                                                                   |
| Stored choice                        | `organizations.plan`, `addons`, `branch_limit`, `staff_limit` (migration 000040)                                                                                                                                                                         |
| Usage counters                       | `organization_usage(organization_id, metric, period, quantity)`; metrics `sms`, `whatsapp`, `email`, `voice_call`; `period` is the first day of the month in the business time zone                                                                      |
| Request scope                        | `orgctx.Scope.Plan`, `Addons`, `BranchLimit`, `StaffLimit`; `entitlement.ForScope(scope)` gives the effective set                                                                                                                                        |
| Service                              | `backend/internal/modules/entitlements` (summary, limit checks, `AddUsage`)                                                                                                                                                                              |
| API                                  | `GET /v1/tenant/entitlements` (`tenant.settings.read`), `GET`/`PUT /v1/platform/organizations/{uuid}/entitlements` (platform organizations read / write), `entitlements` in `GET /v1/tenant/context`                                                     |
| Module gate                          | `middleware.RequireModule(sector.Module)` answers 403 `MODULE_NOT_ENABLED`                                                                                                                                                                               |
| Panel                                | `frontend/src/features/entitlements`: Settings → Plan ve kullanım (tenant), the "Paket / özellikler" tab of a business (platform). `TenantProvider` exposes `entitlements`; `NavModulesProvider` and `useSector().hasModule` use `entitlements.modules`. |

## Plans

| Plan                                            | Branches  | Staff seats |
| ----------------------------------------------- | --------- | ----------- |
| starter (Başlangıç, default for new businesses) | 1         | 5           |
| growth (Büyüme)                                 | 3         | 20          |
| enterprise (Kurumsal)                           | unlimited | unlimited   |

Plans carry limits only, no price; billing is outside the beta. The only add-on is `confirmation` (Konfirmasyon): without it the `confirmation` module is removed from the sector's modules. A plan never adds a module the sector hides.

## Rules

- Effective limit = override when set, else the plan default; `null` means unlimited. A `null` override means "use the plan default".
- Staff seats are members plus open invitations until the staff module exists.
- Adding a branch, a member or an invitation over the limit answers 409 `PLAN_LIMIT_REACHED` with `data: {kind, limit, used}`. The member and invitation checks run inside the transaction that locks the organization row, so they are race-safe; the branch check reads the count before the insert.
- Lowering a limit or the plan never removes data; it only blocks new additions.
- Businesses that existed before migration 000040 were moved to `enterprise` so nothing they already use is blocked.
- `PUT` replaces plan, add-ons and both overrides at once, invalidates the organization scope cache and records `organizations.entitlements_updated`.
- Communication modules (SMS, WhatsApp, e-mail, voice calls) count what they send with `AddUsage`; the summary shows the current month.

## Takwim yönetimi 360 (TEC-927)

`GET /v1/platform/organizations/{uuid}/overview` (`platform.organizations.read`, `backend/internal/modules/orgoverview`) is the platform view of one business; the panel shows it on the "Genel bakış" and "Kanallar ve ödeme" tabs of the business page.

- **Opening a business**: `POST /v1/platform/organizations` takes `sector`, `plan`, `addons` and `access_ends_at` next to the owner (account or `owner_email` invitation); they are written in the creating transaction. An empty plan with add-ons means the default plan.
- **Use**: the entitlement summary (branches, staff seats, this month's counters), last month's counters, customers, services, reservation volume (bookings by `created_at`; this / last month, cancelled + rejected + no-show this month, upcoming requested / active visits, six-month series in the business time zone).
- **Konfirmasyon right vs channel use**: `confirmation` says whether the sector offers the module, whether the add-on is granted and whether it is active, plus this month's confirmation tasks by outcome. The messages those tasks send are counted only under the channels.
- **Channels** (`sms`, `whatsapp`, `email`, `voice_call`): `ready` when the notification engine has a provider that really delivers, `not_ready` when the channel exists without one (its sends are recorded as skipped, never as sent), `unavailable` when the channel has no adapter yet (voice calls). Each row carries the business's own connected account of the kind (no secrets), this month's usage counter and the customer notification outcomes (sent / failed / skipped).
- **Online payment**: `ready` (`payments.OnlineReady`), `not_connected` (no active payment account), `no_cash_account` (no online cash account for the provider) or `provider_unavailable` (no working adapter for the connected provider).

## Adding a gated feature

1. If the feature is a sell-separately module, add an add-on code to the catalog and the `addons` CHECK in a migration, then map it to its sector module.
2. Mount its tenant routes with `middleware.RequireModule(<module>)` after `RequireOrganization`.
3. Give its nav items `module: "<module>"`; the panel hides them when the plan lacks the module.
4. A countable resource with a seat limit checks it with the entitlements service before insert, inside the organization-row lock when it can race.
