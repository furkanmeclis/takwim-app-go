# Customer panel (TEC-909, TEC-910)

One area for the people who book, across every business that knows their phone: `takwim.link/panel` on the web, the same API for the future mobile app. A person signs in with their phone and a one-time code, sees their upcoming and past visits, cancels or moves a visit within the business's rules, and sees what they booked, paid and got back. Customers are not staff: they never get a `users` row, a staff token or the panel sidebar.

| Area         | Path                                                                                                                                                                                |
| ------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Use cases    | `backend/internal/modules/customerpanel/usecase` (`auth.go` sign-in and sessions, `panel.go` the panel, `security.go` links, devices, number change, `visit.go` visit links)        |
| HTTP, routes | `backend/internal/modules/customerpanel/handler`, `routes.go`                                                                                                                       |
| Queries      | `backend/internal/database/queries/customer_panel.sql`                                                                                                                              |
| Migration    | `000055_customer_panel` (`customer_otp_challenges`, `customer_sessions`), `000058_customer_panel_security`, `000075_customer_phone_reassignment`, `000077_customer_history_restore` |
| SMS          | `backend/internal/platform/sms` (`Sender` interface, `LogSender`)                                                                                                                   |
| Token        | `backend/internal/platform/jwt` (`KindCustomer`, `IssueCustomer`, `ParseCustomer`)                                                                                                  |
| Tests        | `backend/internal/modules/customerpanel/customerpanel_db_test.go`, `security_db_test.go`, `platform/jwt/token_test.go`                                                              |
| BFF          | `frontend/src/lib/server/customer-session.ts` (httpOnly customer cookie, Bearer upstream)                                                                                           |
| Web          | `frontend/src/app/panel/*`, `frontend/src/features/customer-panel/*`, locale `customer_panel.json`                                                                                  |

## Routes

All under **`/v1/customer`**; the browser calls `/api/v1/customer/…` on the same origin.

| Route                                    | Notes                                                                                           |
| ---------------------------------------- | ----------------------------------------------------------------------------------------------- |
| `POST auth/start`                        | `{phone}` → `202`, the same answer for every valid phone (`code_sent_if_known`, masked phone)   |
| `POST auth/verify`                       | `{phone, code}` → customer token, session expiry and profile; links the person's records        |
| `POST auth/logout`                       | `{everywhere}`: this session, or every session of the identity                                  |
| `GET me`                                 | phone, sessions, the businesses that know the person                                            |
| `GET reservations`                       | `scope` (`upcoming`, `past`, `all`), `business`, `status`, `sort=starts_at`, `limit` / `offset` |
| `GET reservations/{uuid}`                | visit, lines, branch contact, business policies, `actions` (allowed / until / reason), history  |
| `POST reservations/{uuid}/cancel`        | `{expected_version}`                                                                            |
| `POST reservations/{uuid}/availability`  | `{from, to}`: free starts for the same services on the online channel                           |
| `POST reservations/{uuid}/reschedule`    | `{starts_at, expected_version}`                                                                 |
| `GET payments`, `GET payments/summary`   | visits with money on them; totals per currency, from the payment ledger (TEC-967)               |
| `POST auth/link`                         | `{token}` of a panel link → the same answer as `auth/verify` (TEC-910)                          |
| `GET sessions`, `DELETE sessions/{uuid}` | the device list; sign one device out                                                            |
| `POST phone/start`, `POST phone/verify`  | number change: a code to the new number, then the profile moves                                 |
| `POST phone/reassigned`                  | `{keep_businesses}`: "this number was recently given to me"                                     |
| `POST visit`, `POST visit/cancel`        | `{token}` of a visit link: one reservation, no session                                          |
| `GET packages`, `GET packages/{uuid}`    | "Paketlerim" (TEC-984): `business`, `state`, `limit` / `offset`; the detail adds usage history  |
| `POST businesses/{slug}/reservations`    | online booking as the signed-in person, with `package_sessions` (TEC-984)                       |
| `POST businesses/{slug}/history-request` | "reopen my history at this business" after a release closed it (TEC-991)                        |

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

The amounts come from the business's payment ledger (TEC-967, [sales-ledger.md](./sales-ledger.md), [payments.md](./payments.md)), in the visit's currency, with reversed entries netted out. The list, the detail, the visit link and the summary use the same rule (`moneyOf` in `panel.go`):

| Field            | Meaning                                                                                                                                                      |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `total`          | the booked total of the visit                                                                                                                                |
| `expected`       | what the visit costs, as the business's reservation balance counts it: the posted sale charges once completed, the booked total while it stands, 0 otherwise |
| `paid`           | every collection for the visit: online, at the business, in parts                                                                                            |
| `refunded`       | the refunds posted to the ledger                                                                                                                             |
| `balance`        | still owed: `expected` less `paid` net of `refunded`, never below 0                                                                                          |
| `refund_pending` | money held beyond `expected` on a cancelled or rejected visit or one in `refund_pending` / `partially_refunded`, or the open (`requested`) refunds if more   |

A no-show prepayment is not shown as a pending refund: it stays with the business's no-show policy. Neither is money the business closed without a refund (`kept`, TEC-978): it stays there as the customer's credit. A payment made at the business shows once the business records it (the payments page says so). The payments list leaves out a cancelled or rejected visit with no collection in the ledger. The summary adds the visits' figures per currency (`booked` = the sum of `expected`, `balance` = still owed; `reservation_count` = visits not cancelled or rejected).

The visit detail answers `payments`: the live collections (`payment`) and refunds (`refund`) of the ledger, oldest first, with method, amount and business day; a reversed entry and its reversal are left out, and notes, accounts and staff are never shown. It also answers `refunds` (TEC-913): each refund's amount, its status (`requested` in progress, `succeeded` paid back, or `failed`, shown as "being followed up" by the business) and its dates.

The visit detail also answers `products` (TEC-977): the products taken with the visit (name, variant, quantity, unit price, amount) and their state: `held` (set aside for the visit), `delivered` (handed over at the visit) or `released` (the visit was cancelled, rejected, expired or missed). A line the business removed is left out; stock codes, costs and the seller are never shown. The visit total already includes held and delivered products.

## Packages ("Paketlerim", TEC-984)

The session packages the person bought from the businesses that know them, inside each link's visibility window: remaining sessions, last day, state and the visits that used a session (released sessions are left out). Another person's package is `404`. A signed-in person can pay booking lines with these packages on the business page; that booking goes through `POST /v1/customer/businesses/{slug}/reservations` so the session's cookie reaches it, and the customer is the business's linked record (`customer` / `not_linked` otherwise). Rules and money: [session-packages.md](./session-packages.md).

## Panel link by SMS

The customer notification engine sends it ([customer-notifications.md](./customer-notifications.md), TEC-911). Every reservation notice (received, accepted, rejected, changed, cancelled, reminder) carries `{{link}}`, so a new public booking gets "your request was received" or "your booking was accepted" with the link to see and manage all bookings (A28). On SMS, `PanelLink` puts a single-use panel link (below) into the text at send time; past 5 links a day for the phone, on other channels, or while the channel is not ready, it is the plain `takwim.link/panel` address, which asks for a code. Stored texts never hold a token.

## SMS provider

`sms.Sender` is the only way out. Until the provider is chosen (TEC-884) the server uses `LogSender`: it logs the masked phone and, outside production, the text (so the code can be read in development). No new environment key.

## Web

`/panel/login` (phone, then the code with `autocomplete="one-time-code"`), `/panel` (Upcoming / Past tabs), `/panel/reservations/{uuid}` (cancel confirmation, reschedule dialog with a date picker and free starts), `/panel/payments` and `/panel/profile` (businesses, sign out, sign out everywhere). A top bar with a bottom tab bar on phones; no staff sidebar. Lists use the DataTable stack (grid view on phones); every string is in `customer_panel.json` (Turkish first).

## Access security (TEC-910)

References: OWASP Authentication and Authorization cheat sheets. Acceptance: A25, A27, A29, A30.

### Panel links

- `takwim.link/panel/l#<token>`: 192 random bits, base64url. The token sits **after `#`**: browsers never send the fragment, so no server or proxy log holds it. The page (`/panel/l`) reads it, removes it from the address bar and sends it in the body of `POST auth/link`; a mobile app does the same with the universal link.
- Only a keyed hash is stored (`customer_access_links`, its own hash purpose). A link works **once** (atomic consume) and for **24 hours**; an unknown, expired or used link answers `400 LINK_INVALID` and the page leads to the code sign-in (no sign-up).
- Opening the page does not spend the link: SMS previews and link scanners open it too. The person presses "Panele gir".
- The session it opens has `method = link`; records it links have source `panel_link`.

### Visit links

- `takwim.link/panel/v#<token>` opens **one reservation** (`customer_visit_links`, organization-owned, own hash purpose). `POST visit` answers the panel detail of that visit with `actions.reschedule` refused (`sign_in_required`); `POST visit/cancel` cancels it under the same rules and guard as the panel. It opens no session and no other visit; a visit token is never a panel link or a Bearer token, and the other way round.
- It works until a day after the visit ends. `IssueVisitLink(org, reservation)` creates one; the confirmation module puts it in its requests (`IssueVisitLinkFor`).
- `POST visit/confirm` ("Geleceğim") and `POST visit/change-request` answer the business's confirmation request (TEC-921, confirmations.md); the signed-in panel has the same on `reservations/{uuid}/confirm` and `/change-request`. `actions.confirm` / `actions.request_change` say whether they are offered (`not_requested`, `answered`, `status`, `business_unavailable`) and until when; `confirmation` shows the status and deadline.

### Number change

- Signed in: `phone/start` sends a code to the new number. The code has purpose `phone_change` and is bound to the identity: it never signs anybody in, and another person cannot use it. The new number's cooldown and caps apply, plus 5 number change codes a day per profile.
- `phone/verify` moves the identity to the new number in one transaction: the identity (uuid, links, everything it sees) stays; the old number is **released** (`customer_phone_releases`, `number_changed`); the links proven with the old number get `visible_until` (a later booking made with the old number is the next holder's); every other session ends; this one continues on the new number. Sessions whose phone is no longer the identity's are refused.
- A new number that has its own profile answers `409 PHONE_IN_USE`: two profiles are never merged on a number.

### Reassigned number

- A person who received a recycled number and sees somebody else's visits reports it (`phone/reassigned`) and ticks the businesses whose visits are theirs. Every other business's link gets `visible_from = now` (earlier visits and the name the business keeps are hidden), the number gets a `reassigned` release, and every other session of the profile (the previous holder's devices) ends. Nothing is deleted at the businesses; the previous holder gets their history back when a business puts their new number on the record (the record's link drops, the new number links it).

### History never moves to the next holder (A29, A30)

- `customers.LinkVerified` never takes a record linked to another identity (`ErrIdentityLinked`); before TEC-910 it re-linked.
- A record created **before the phone's last release** is linked with `visible_from` = the release: its new holder sees only the visits booked after it.
- Every panel query (list, count, totals, detail, businesses) applies the window `visible_from ≤ reservation.created_at < visible_until`. Release times and windows use the database clock, like `created_at`.
- A name on a record is never evidence: a matching or different name links or merges nothing. Only a proven phone links, and a phone opens no history from before it changed hands. The profile is the person who manages the bookings; the name each business keeps is shown per business.

### Devices

`GET sessions` lists the live sessions (device from the user agent, `code` or `link`, masked address, sign-in and last use, expiry; `current` marks this one). `DELETE sessions/{uuid}` ends one at once; another person's session is `404`. The BFF forwards the user agent on `auth/verify` and `auth/link`.

### Abuse limits and monitoring

- Database limits (hold without Redis): codes of a phone 60 s apart, 5 an hour, **10 a day**; an address that asked codes for **20 different phones** in an hour gets no more codes for that hour (same answer). Redis windows per IP: `auth/link` 10, `visit` 30 per 10 minutes; number change, reassignment and device sign-out 10 per session per 10 minutes.
- `customer_auth_events` records sign-ins, failed codes, failed panel and visit links, throttled addresses, number changes (and refusals), reassignments and device sign-outs (90 days). 20 failures of a kind from one address in an hour, a throttled address and a reassignment write a warning with `source=customer_auth`, which the platform logs page lists (`/platform/settings/logs`). The logs page has a ready filter for them ("Müşteri erişim uyarıları": `source=customer_auth`, level `warn`) and takes `?source=…&level=…` in its URL.

## Reassignments from Takwim management (TEC-972)

Acceptance: A29, A30. A number can change hands without anybody telling the panel: the operator recycles it, and the next holder signs in with a code.

### Notices processed by an operator

- `/platform/settings/customer-phones` (permission `platform.customer_phones.read`) lists every release: number changes and reports made by the person (`source=customer`), notices processed here (`platform`, with the operator and their note) and dormant numbers (`system`). Numbers are masked; `q` finds a number in any spelling (exact match) or a part of the note.
- `POST /v1/platform/customer-phone-releases` (`platform.customer_phones.write`, after a confirmation in the page) processes a notice at once, in one transaction: every business link of the number's profile gets `visible_from = now` (nothing is kept: the operator does not know which visits are whose), **every** session of the profile ends, and the number gets a `reassigned` release, so records of the number found later open only what is booked from now on. A number without a profile gets the release too. Nothing is deleted at the businesses.
- When nobody proved the number since its last release (a number change, a report or an earlier notice already closed it) the notice answers `409 PHONE_ALREADY_RELEASED`: it would close nothing, and a second notice after the next holder signed in would hide the next holder's own visits.
- A `platform_reassigned` auth event, a `source=customer_auth` warning and an activity entry (`customer_phones.reassigned`) record it.

### Numbers not proven for a long time

- A number whose profile has not proven it for **180 days** (`DormantAfter`, `customer_identities.phone_verified_at`) may have been recycled unreported. At its next sign-in (code or panel link), before the session opens, the profile's earlier history is closed the same way: `visible_from = now` on every link, a `dormant` release (`source=system`), every older session ends. A `dormant_closed` event and a warning record it. A number released after its last proof is left alone; a failure fails the sign-in, so history never opens unchecked.
- We close the history instead of asking "are these visits yours?": the answer to such a question is no proof (anybody holding the number can say yes), and the question itself would show the businesses the previous holder visited. The person sees everything booked from now on; the businesses keep every record.
- The profile (`GET me`) carries `history_notice` (`reassigned` or `dormant`, with the time) after either closing, and the panel's profile page explains why earlier visits are not shown.

## Reopening closed history (TEC-991)

A closing (a reassignment notice, the dormant check) is deliberately blind: it hides every business's earlier visits from the profile because nobody can tell from the number alone whose they are. When the number stayed with the same person (a customer back after seven months), the proof that the history is theirs comes from the **business that knows them**, one business at a time. Nothing at the businesses changed meanwhile, so nothing is restored from a backup: the link's `visible_from` is lifted.

- **The business reopens, after a check at the reception.** `POST /v1/tenant/customers/{uuid}/history/restore` (`tenant.customers.write`, so the reception can do it) sets `visible_from = NULL` on this business's link only. The other businesses are untouched: each decides for its own records. The record page shows the hidden window (`verification.history_hidden_from`), any request from the person, and a confirmation that says in so many words that the staff recognise the person. A name match is never evidence (A29, A30): the record's name may be the previous holder's, and the check is the business's.
- **The trail.** The link keeps `history_restored_at`, `history_restored_by` (the user) and `history_restored_from` (the window it lifted); the business's activity gets `customers.history_restored` (actor, the hidden window, whether the person had asked); the platform trail gets a `history_restored` auth event. A record that is not linked or whose history is open answers `409` (`history` / `open`).
- **The person asks from the panel.** The profile notice explains the way back, and every closed business in the list offers "Geçmişimi geri iste": `POST /v1/customer/businesses/{slug}/history-request` writes `history_requested_at` on that link (the first request keeps its time), a `history_requested` auth event and `customers.history_requested` in the business's activity (source `customer_panel`, no actor), so the reception sees the request on the record. The panel never reopens anything itself: "are these visits yours?" is no proof. A business whose history is open answers `409 ACTION_NOT_ALLOWED` (`history_request` / `history_open`); one that does not know the person `404`. Limit: the security window (10 per session per 10 minutes).
- **The next closing wins.** A later reassignment or dormant closing hides the business again and clears the request and the restore trail on the link (the activity entries remain); the business checks again if the person asks again.
- Not done: a code sent to a second contact channel of the record (e-mail). Businesses rarely hold a verified e-mail of the person, and a code to an address the previous holder typed would prove nothing; the reception check is the proof we have.

## Not in this issue

- **Confirmation** (teyit): the confirmation module drives `confirmation_status` (confirmations.md); the panel shows it and offers the answers.
