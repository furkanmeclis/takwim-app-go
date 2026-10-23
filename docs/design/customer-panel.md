# Customer panel (TEC-909)

One area for the people who book, across every business that knows their phone: `takwim.link/panel` on the web, the same API for the future mobile app. A person signs in with their phone and a one-time code, sees their upcoming and past visits, cancels or moves a visit within the business's rules, and sees what they booked, paid and got back. Customers are not staff: they never get a `users` row, a staff token or the panel sidebar.

| Area         | Path                                                                                                    |
| ------------ | ------------------------------------------------------------------------------------------------------- |
| Use cases    | `backend/internal/modules/customerpanel/usecase` (`auth.go` sign-in and sessions, `panel.go` the panel) |
| HTTP, routes | `backend/internal/modules/customerpanel/handler`, `routes.go`                                           |
| Queries      | `backend/internal/database/queries/customer_panel.sql`                                                  |
| Migration    | `000055_customer_panel` (`customer_otp_challenges`, `customer_sessions`)                                |
| SMS          | `backend/internal/platform/sms` (`Sender` interface, `LogSender`)                                       |
| Token        | `backend/internal/platform/jwt` (`KindCustomer`, `IssueCustomer`, `ParseCustomer`)                      |
| Tests        | `backend/internal/modules/customerpanel/customerpanel_db_test.go`, `platform/jwt/token_test.go`         |
| BFF          | `frontend/src/lib/server/customer-session.ts` (httpOnly customer cookie, Bearer upstream)               |
| Web          | `frontend/src/app/panel/*`, `frontend/src/features/customer-panel/*`, locale `customer_panel.json`      |

## Routes

All under **`/v1/customer`**; the browser calls `/api/v1/customer/…` on the same origin.

| Route                                   | Notes                                                                                           |
| --------------------------------------- | ----------------------------------------------------------------------------------------------- |
| `POST auth/start`                       | `{phone}` → `202`, the same answer for every valid phone (`code_sent_if_known`, masked phone)   |
| `POST auth/verify`                      | `{phone, code}` → customer token, session expiry and profile; links the person's records        |
| `POST auth/logout`                      | `{everywhere}`: this session, or every session of the identity                                  |
| `GET me`                                | phone, sessions, the businesses that know the person                                            |
| `GET reservations`                      | `scope` (`upcoming`, `past`, `all`), `business`, `status`, `sort=starts_at`, `limit` / `offset` |
| `GET reservations/{uuid}`               | visit, lines, branch contact, business policies, `actions` (allowed / until / reason), history  |
| `POST reservations/{uuid}/cancel`       | `{expected_version}`                                                                            |
| `POST reservations/{uuid}/availability` | `{from, to}`: free starts for the same services on the online channel                           |
| `POST reservations/{uuid}/reschedule`   | `{starts_at, expected_version}`                                                                 |
| `GET payments`, `GET payments/summary`  | visits with money on them; totals per currency                                                  |

Another person's visit, an unknown uuid and a malformed uuid all answer `404`.

## Token and session

- The customer token is a JWT of kind **`customer`** (`knd`), signed like the staff tokens. `sub` is the identity uuid, `sid` a row of `customer_sessions`. Staff routes accept only kind `staff` (TEC-879), so a customer token never reaches `/v1/tenant/*` or `/v1/platform/*`; the customer routes accept only kind `customer`.
- Every request loads the session row: a revoked or expired session is `401` at once, not when the JWT runs out. Idle expiry is **7 days**, sliding (touched at most once a minute); the hard limit is **30 days** from sign-in.
- The web BFF keeps the token in an httpOnly cookie (`takwim-customer`, `__Secure-` in production, `SameSite=Lax`, path `/api/v1/customer`) and removes it from the verify answer (`authenticated: true` instead). The staff NextAuth cookie is never sent on customer paths and the customer cookie never on other paths. A `401` or a logout clears the cookie. Cookie-authenticated writes go through the BFF's same-origin (CSRF) check.
- The idempotency middleware also scopes `Idempotency-Key` by customer (`customer:{sub}`) on `/v1/customer/*`; the web sends one on cancel and reschedule.

## Sign-in codes

- `start` normalizes the phone (Turkish numbers to E.164) and always answers the same way. A code is **sent only when a business knows the phone** (a live customer record in a live business, or an existing identity); for any other phone the row is stored with `sent=false` and never verifies. This keeps the endpoint from revealing who is a customer and from sending SMS to arbitrary numbers.
- 6 digits, **5 minutes**, **5 attempts**, single use. Only a keyed hash is stored (`FieldCipher.BlindIndex`, bound to the challenge uuid). A new code closes the open ones.
- Limits: 60 s between codes and 5 codes an hour per phone (counted in the database), plus Redis windows per IP and per phone on `start` and `verify` (`429`, fails open when Redis is down). Rows older than a day are removed on each `start`.
- `verify` creates or reuses the platform-level identity and links every live, unlinked record with that phone in every business through `customers.LinkVerified` (source `panel_login`). A record linked to another identity is skipped. The list and profile reads link again, so a booking made after sign-in shows up without signing in again.

## Cancel and reschedule

The panel never decides on capacity or states itself; it calls the reservations use case.

- **Cancel** uses `reservations.Cancel` (reason `customer_cancelled`) with the visit's version.
- **Reschedule** uses `reservations.Reschedule` with the same services and people on the **online** channel: the slot engine, capacity holds and the old-time release work exactly as on the panel and the booking page.
- Both pass a `Guard` that runs on the locked row: it checks again that the visit belongs to the person and that the deadline has not passed. A change between reading and writing cannot slip through.
- Allowed while the visit is `requested` or `active`, the business is live and before the business's cutoff: `cancel_cutoff_minutes` / `change_cutoff_minutes` before the start (`null` means until the start). Reschedule also needs the reservations and booking page modules, online readiness and every line's service still offered. The detail answers `actions.{cancel,reschedule}` with `allowed`, `until` and `reason` (`status`, `cutoff`, `business_unavailable`, `online_unavailable`, `services_changed`); a refused write is `409 ACTION_NOT_ALLOWED` with the same reason. A taken start is `409` `taken`, a changed visit `409` `stale`.
- Each change writes the business's activity log (`reservations.cancelled` / `reservations.rescheduled`, payload `source: customer_panel`) and a live update on `tenant.reservations`.

## Payments

There is no payment ledger yet (TEC-912). The amounts come from the visit total and `payment_status`: online payments count as the full total, a refund as the full total once refunded (`refunded` is `null` while it is unknown), `balance` is `null` for a finished visit paid at the business. A cancelled or rejected visit with nothing paid is left out of the payments list. The summary groups by currency.

## Panel link by SMS

After a new public booking (not a replay), `publicbooking` calls `PanelLinkNotice`, which sends "your booking was received; see and manage all your bookings at takwim.link/panel" (A28).

## SMS provider

`sms.Sender` is the only way out. Until the provider is chosen (TEC-884) the server uses `LogSender`: it logs the masked phone and, outside production, the text (so the code can be read in development). No new environment key.

## Web

`/panel/login` (phone, then the code with `autocomplete="one-time-code"`), `/panel` (Upcoming / Past tabs), `/panel/reservations/{uuid}` (cancel confirmation, reschedule dialog with a date picker and free starts), `/panel/payments` and `/panel/profile` (businesses, sign out, sign out everywhere). A top bar with a bottom tab bar on phones; no staff sidebar. Lists use the DataTable stack (grid view on phones); every string is in `customer_panel.json` (Turkish first).

## Not in this issue

- **TEC-910**: access security beyond the above (phone number change and number reuse, device list, magic links, single-visit links, abuse monitoring).
- **Packages** ("Paketlerim"): there is no packages module yet (TEC-895).
- **Payment amounts** from a ledger: TEC-912.
- **Confirmation** (teyit): the confirmation module drives `confirmation_status`; the panel only shows it.
