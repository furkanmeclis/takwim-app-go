# Business settings and the central panel (TEC-892)

## Business profile

`/t/{slug}/settings` holds two parts:

- **Profile** (`GET/PATCH /v1/tenant/organization`, logo and cover upload): name, logo, time zone, currency, and the business page cover image (`PUT/DELETE /v1/tenant/organization/cover`, PNG / JPEG / WebP up to 5 MiB, stored as `organizations.cover_object_key` and streamed publicly at `/v1/public/businesses/{slug}/cover`; TEC-966). The sector is set by Takwim management. Times are stored in UTC and shown in the business zone; every amount is in the business currency.
- **Settings document** (`/v1/tenant/settings`): a versioned JSON document. Every save is a new immutable version, `base_version` gives optimistic concurrency (`409` on a stale edit), and old versions can be compared.

| Section          | Fields                                                                                                                                                                                                                                                                                                                   |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `company`        | legal name, VKN/TCKN (checksum), tax office, address, phone, email, website                                                                                                                                                                                                                                              |
| `reservation`    | `approval_mode` (`auto` / `staff`), `staff_review_window_minutes`, `payment_hold_minutes`, `min_lead_minutes` (earliest bookable start), `max_advance_days` (latest bookable day), `cancel_cutoff_minutes`, `change_cutoff_minutes`, online `slot_step_minutes`, `max_open_per_customer`, `max_online_per_day` (TEC-905) |
| `payment`        | `pay_at_venue`, `pay_online` (at least one; online is the full amount)                                                                                                                                                                                                                                                   |
| `policies`       | cancellation, no-show and change texts shown to customers                                                                                                                                                                                                                                                                |
| `sales_channels` | `web` (takwim.link page), `widget` (embed), `api`; panel entries are always allowed                                                                                                                                                                                                                                      |
| `embed`          | `allowed_origins`: the sites that may frame the booking screen (at most 20, saved as origins; [embed.md](./embed.md))                                                                                                                                                                                                    |
| `tax`            | default VAT rate (no built-in legal rate), accounting codes for `service`, `product`, `package`                                                                                                                                                                                                                          |
| `booking`        | hidden optional flow steps and the booking form questions (TEC-905, [booking-rules.md](./booking-rules.md))                                                                                                                                                                                                              |
| `theme`          | business page look (TEC-966, [business-page.md](./business-page.md)): `primary_color`, `accent_color` (`#rrggbb` or empty), `font` (`default`, `inter`, `manrope`, `nunito`, `lora`, `playfair_display`), `headline` (80), `description` (300); the cover image is on the profile                                        |

These are business-wide defaults; the booking flow customization is described in [booking-rules.md](./booking-rules.md). Online booking is ready when `approval_mode`, `min_lead_minutes` and `max_advance_days` are set, plus the review window for staff approval and the payment hold when online payment is on (`ready_for_online_sales`, `missing[]`).

The rental-era fields (prepayment rules, quote validity, late return, fuel, mileage, marketplace channels) are gone. Stored versions that still carry them decode without them.

## Branch picker (`X-Branch`)

The tenant header has a branch picker: "all branches" or one branch. The pick is per organization in `localStorage` (`takwim.branch.{slug}`) and every `tenantRequest` sends it as `X-Branch: {branch uuid}`. A mobile client sends the same header.

- `RequireOrganization` checks the branch against the member's branch scope and sets `Scope.SelectedBranchID`. Unknown, deleted or out-of-scope branches answer `403 BRANCH_NOT_ALLOWED`; the panel then drops the pick and retries for all branches.
- `orgctx.BranchScope(ctx)` returns only the picked branch, so branch-bound lists, counts, facets, search and the overview narrow to it with no module change. The members list shows members who can work in the picked branch.
- Access checks on single records (`CheckBranch`, `RequireAllBranches`) keep the member's full scope: a record of another branch still opens, and an owner can still create branches.
- Queued exports run in the worker with the member's full scope, not the pick.
- Switching invalidates the organization's queries, so every open list refetches.

## Central panel (`GET /v1/tenant/overview`)

The tenant home page shows the branches the member sees, in total (stat cards) and one by one (table):

- totals: branches, active, online (active and shown on booking screens), open now, members (each member once);
- per branch: status now, today's hours in the branch zone (dated exception first; hours that close at or before they open run past midnight), member count, online visibility.

It needs `tenant.branches.read` and follows the branch picker. Reservation, revenue and confirmation figures join this payload when those modules land (TEC-926 daily summary).
