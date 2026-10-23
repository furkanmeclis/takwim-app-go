# Public booking API (TEC-906)

One unauthenticated API for the business page (`takwim.link/{slug}`, TEC-907), the embedded widget (TEC-908) and the future mobile app. It sits on the panel's own use cases: the slot engine ([availability.md](./availability.md)) on the online channel, the capacity holds ([capacity-holds.md](./capacity-holds.md)) and the reservation record ([reservations.md](./reservations.md)). A slot therefore means the same thing on every screen.

| Area         | Path                                                                                            |
| ------------ | ----------------------------------------------------------------------------------------------- |
| Use cases    | `backend/internal/modules/publicbooking/usecase`                                                |
| HTTP, routes | `backend/internal/modules/publicbooking/handler`, `routes.go`                                   |
| Queries      | `backend/internal/database/queries/public_booking.sql`                                          |
| Migration    | `000051_public_checkouts`                                                                       |
| Tests        | `backend/internal/modules/publicbooking/publicbooking_db_test.go`                               |
| BFF          | `frontend/src/lib/server/bff-proxy.ts` forwards `X-Reservation-Token` and returns `Retry-After` |

## Business and versioning

Routes live under **`/v1/public/businesses/{slug}`**. The browser reaches them through the same-origin `/api/v1/public/businesses/{slug}/…`; the mobile app calls the API directly. The version is the `/v1` prefix, like every other route; a breaking change gets `/v2`.

The business comes from the slug only, never from a header or the body. An unknown slug, a suspended business and one outside its access window all answer `404`, so the route does not reveal which businesses exist.

## Endpoints

| Route                                         | Notes                                                                                                                                    |
| --------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `GET {slug}`                                  | profile: name, logo URL, sector flow and terms, contact, policies, `booking` (channels, readiness, rules, payment methods), branch count |
| `GET {slug}/logo`                             | logo stream (versioned URL in the profile)                                                                                               |
| `GET {slug}/branches`                         | active, `show_online` branches: hours, today's and later special days, photos                                                            |
| `GET {slug}/services?branch_uuid=`            | active, `show_online` services a public branch offers; branch prices / durations; people shown online with their own price / duration    |
| `GET {slug}/staff?branch_uuid=&service_uuid=` | active, `show_online` people in public branches; no contact details                                                                      |
| `POST {slug}/availability`                    | `availability.Search` with channel `online`; the answer drops buffers and resources                                                      |
| `POST {slug}/checkouts`                       | holds the picked slot (`checkout` hold, 10 minutes); answers the checkout token and the planned lines                                    |
| `GET`, `DELETE {slug}/checkouts/{token}`      | checkout status (`active`, `expired`, `released`, `booked`); release when the customer goes back                                         |
| `POST {slug}/reservations`                    | books a checkout for a customer identified by phone; answers the reservation and its `access_token`                                      |
| `GET {slug}/reservations/{uuid}`              | status view behind `X-Reservation-Token`                                                                                                 |

Hidden, inactive, deleted and other businesses' branches, services and staff are never listed. When a body or filter names them, the answer is `400` with `not_found` (or the slot engine's `inactive` / `not_offered`).

## Booking gate

Availability, checkouts and bookings need the business to take public bookings on the caller's **channel** (`web` default, `widget`, `api`; the body field `channel`, stored on the checkout):

1. the plan and sector include the `reservations` and `booking_page` modules (`module_disabled`);
2. the business settings are ready for online sales (`online_not_ready`): approval mode, minimum notice, maximum advance and the related windows;
3. the channel is switched on in `sales_channels` (`channel_disabled`).

Otherwise the answer is `409 BOOKING_UNAVAILABLE` with detail `channel` / reason. The profile and catalog stay readable, so the page can say why booking is closed (`booking.ready`, `booking.unavailable`, `booking.channels`).

## Checkout and booking

1. **Checkout.** `POST …/checkouts` runs `Holds.CreateIn` (channel online, reason `checkout`) and stores a `public_checkouts` row in the same transaction: the hold, branch, channel, start and the planned lines with the staff member of each line. The token is the row's random uuid; the capacity hold's uuid never leaves the API.
2. **Booking.** `POST …/reservations` takes `checkout_token` and the contact form. It calls `reservations.Service.CreateWith`, so the approval and payment plan of the business settings apply exactly as on the panel's online path. Two hooks run inside the booking's capacity transaction, under the business lock:
   - **Before:**
     - lock the checkout;
     - if it is already booked, stop and answer that reservation;
     - free the checkout hold (an expired or released hold is skipped);
     - find or create the customer.
   - **After:** link the checkout to the reservation.

   The slot is then planned again with the stored lines and their pinned staff. So an expired checkout still books while its time is free, and a lost one answers `409 starts_at / taken` with nothing written.

3. **One booking per checkout.** A retry answers `200` with the same reservation and `Idempotent-Replayed: true`. This holds even without an `Idempotency-Key`.
4. **Source.** `widget` bookings are `embed`, `web` and `api` bookings are `online`. The reservation has no creator user.

## Customer by phone

The phone is normalized to E.164 (`pkg/phone`; a number without a country code is Turkish).

- **Phone on file.** The business's live record with that phone becomes the customer, unchanged. Names, e-mail and consent typed by a public caller never overwrite what the business holds, and nothing about the record is returned.
- **New phone.** A new record is created with source `online`, unverified, with the given names, e-mail and consent. It is indexed for Cmd+K after the commit.
- **No verification yet.** There is no phone verification in this issue: no OTP or customer session exists yet, and the customers module links an identity only through `LinkVerified`. A public booking therefore never calls `LinkVerified` and never opens shared history (A25 / A30). OTP sign-in and the shared customer panel (TEC-909 / TEC-910) can later require a verified phone before booking, or link the record after it.

## Reservation access

The booking answer carries `access_token`. This is the field cipher's keyed hash (`BlindIndex`, purpose `public.reservation_access`) of the business uuid and the reservation uuid.

- **No storage.** It needs no table, cannot be derived without the server key, and is the same on every replay.
- **Status route.** `GET …/reservations/{uuid}` compares the token in constant time. A missing or wrong token, an unknown reservation and another business's slug all answer `404`.
- **What the view leaves out.** It holds the states, branch and time zone, times, lines (service, staff name, price) and total. It leaves out the internal note, the history, resources and customer data.

## Abuse protection

- **Rate limits.** Redis fixed windows (`ratelimit.Limiter`), failing open without Redis like the auth routes. Over a limit the answer is `429 RATE_LIMITED` with `Retry-After`.

  | Limit                                     | Window            |
  | ----------------------------------------- | ----------------- |
  | profile, catalog and status reads, per IP | 300 per minute    |
  | availability, per IP                      | 60 per minute     |
  | checkouts and releases, per IP            | 20 per 10 minutes |
  | bookings, per IP                          | 10 per 10 minutes |
  | bookings, per business and phone          | 5 per hour        |

  The client IP is the first `X-Forwarded-For` address (set by the BFF), as on the other public routes.

- **Honeypot.** The booking body has a `website` field that people never see. A filled one answers `400 website / rejected`.
- **Idempotency.** `middleware.Idempotency` also replays anonymous requests under `/v1/public/businesses/`, scoped by method, path and key; the fingerprint covers the body. Clients send a uuid `Idempotency-Key` on checkouts and bookings. `429` and `5xx` outcomes free the key, so a retry runs again.
- **No internal data.** No internal ids, buffers, resources, staff contact details, provider keys, tax numbers or settings internals appear in any answer.

## Data

`public_checkouts` holds:

- `organization_id`, `uuid` (the token), `branch_id`, `hold_id` (unique), `channel`;
- `lines` (JSON: service, start, end, staff), `starts_at`;
- `reservation_id`, set once the checkout is booked.

It has composite foreign keys to branches, capacity holds and reservations, and is listed in `tenant_tables.txt`. The down migration drops the table; open checkout tokens then stop working.

## Not in this issue

- **Pages.** The business page ([business-page.md](./business-page.md), TEC-907) and the widget (TEC-908).
- **Phone verification and the customer panel** (TEC-909 / TEC-910): see above.
- **Packages and products in the public catalog.** They come with their catalog modules (bundles TEC-895, products TEC-918 / TEC-919).
- **Payment.** An online payment link for `online_pending` bookings comes with the payment layer (TEC-912). Until then the booking answers `requested` / `online_pending` with `expires_at`.
- **Customer actions.** Cancellation and change requests come with the customer panel (TEC-909); booking form fields with flow customization (TEC-905).

## Tests

`publicbooking_db_test.go` runs against Postgres:

| Test                     | What it checks                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `TestPublicCatalog`      | profile and readiness; unknown, suspended and expired slugs; hidden branch, service and person left out; branch prices and personal prices; filters naming hidden or foreign records; no leaked fields; business B's slug shows only B                                                                                                                                                                                                                                          |
| `TestPublicBookingFlow`  | online availability and per-line validation (hidden branch, service and person; foreign records; channel); checkout hold and a taken slot; contact validation and honeypot (no customer written); booking, record source and normalized phone; one active commitment; replay; status token rules and another slug; reuse of an existing phone record unchanged; release; an expired checkout that still books with online payment; channel switched off while `api` still books |
| `TestPublicBookingRules` | staff approval request; settings not ready; `Idempotency-Key` replay of a checkout; IP and phone rate limits with `Retry-After` (miniredis)                                                                                                                                                                                                                                                                                                                                     |
