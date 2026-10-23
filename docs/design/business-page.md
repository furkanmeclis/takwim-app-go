# Business page (TEC-907)

The customer-facing page of a business: **takwim.link/{slug}**. It shows the profile and branches and runs the booking wizard. It is built only on the [public booking API](./public-booking-api.md); the browser calls the same-origin `/api/v1/public/businesses/{slug}/…` through the BFF and never the Go host.

| Area           | Path                                                                    |
| -------------- | ----------------------------------------------------------------------- |
| Routes         | `frontend/src/app/(booking)/b/[slug]/…`                                 |
| Feature        | `frontend/src/features/business-page`                                   |
| Server lookup  | `features/business-page/server/get-business.ts` (metadata, first paint) |
| API client     | `features/business-page/services/public-booking.service.ts`             |
| Wizard rules   | `features/business-page/lib/wizard.ts`                                  |
| Texts          | `frontend/src/locales/{tr,en}/business_page.json`                       |
| Reserved slugs | `backend/internal/modules/organizations/usecase/service.go`             |

## Routes and the takwim.link host

The app serves the page under its own route group, so it cannot collide with the panel (`/t/{slug}`, `/platform`, `/login`, …):

| App path                        | Page                                               |
| ------------------------------- | -------------------------------------------------- |
| `/b/{slug}`                     | profile, branches, booking wizard                  |
| `/b/{slug}/reservations/{uuid}` | confirmation and status of one booking (`noindex`) |

On production the **takwim.link** host maps to these paths at the edge (Traefik / Dokploy domain rule, or any reverse proxy in front of the frontend container):

| Public URL                               | Rewritten to                        |
| ---------------------------------------- | ----------------------------------- |
| `takwim.link/{slug}`                     | `/b/{slug}`                         |
| `takwim.link/{slug}/reservations/{uuid}` | `/b/{slug}/reservations/{uuid}`     |
| `takwim.link/panel…`                     | unchanged (customer panel, TEC-909) |
| `takwim.link/api/…`, `/_next/…`, assets  | unchanged                           |

It is a rewrite, not a redirect, so the address bar keeps `takwim.link/{slug}`. The page's own links use `/b/{slug}`; on the takwim.link host the proxy may also rewrite them back or simply serve both. Business slugs that would shadow those paths are reserved: `panel`, `b`, `embed`, `share`, `invite`, `health` join the existing `admin`, `api`, `platform`, `login`, `t`, `www` (`reservedSlug`). A business that already holds one of them keeps it; new and renamed businesses cannot take it.

## Server render and SEO

`getPublicBusiness` (React `cache`, one call per request) fetches the profile on the server with the visitor's IP in `X-Forwarded-For`, so the API's per-IP read limit counts visitors, not the app.

- An unknown, suspended or expired business answers `404`, and the page is a real `notFound()`.
- With the profile, `generateMetadata` sets the title (the business's name), a Turkish description (`business_page.meta.description` with the sector name), Open Graph fields and a canonical `/b/{slug}`.
- When the API is down or rate limited, the page renders without the profile and the browser asks again.

The status page is `noindex` with `referrer: no-referrer`.

## Page

- **Header.** The business's logo (API-owned `logo_url`, initials without one) and name, **Randevularım** (→ `/panel`, the shared customer panel of TEC-909), language and colour mode. There is no panel sidebar.
- **Hero.** The cover is the first photo of the first branch that has one (the branch's first photo is its cover). Then the logo, the name, the sector name and the branches' cities.
- **Tabs.** "Randevu al" / "Rezervasyon yap" (the sector's booking term) holds the wizard. "Bilgiler" holds the branches with their address (map link), phone, e-mail, weekly hours and coming special days, the business's contact details, and its cancellation / change / no-show texts.
- **Closed bookings.** When `booking.ready` is false or the `web` channel is off, the wizard is replaced by a notice with the business's phone. Profile and information stay visible.
- **Theme.** The API exposes no colours, fonts, cover or texts of the business yet, so the page uses the app's neutral theme (light and dark) with the business's logo and branch photo. The panel settings for the theme are a follow-up (TEC-966).
- **Mobile first.** Everything is laid out for 375 px. The step buttons sit in a sticky bottom bar, the day strip scrolls sideways, and the time buttons are a four-column grid.

## Wizard

The steps follow the sector flow of the profile (`flow`); see [sectors.md](./sectors.md).

1. **Branch.** Shown only with more than one public branch; one branch is picked by itself. Each branch shows its address and today's hours in its own time zone (special days first).
2. **Service.** One or more services (a combined visit, at most 10 lines), grouped by category, with the branch's price and duration. The title is the sector's word for the step (`sectors.steps.service`, `appointment_type`, `session_type`, `class`).
   - A flow without a service step (restaurant) still books a service, because a booking line is a service in the API. When the branch offers exactly one service, it is booked without asking; with several (menus, events), the step is shown.
3. **Person.** Shown when the flow has a `staff` or `class` step and a picked service has people shown online in the branch. For each service the customer picks a person or "no preference" (`staff_uuid: null`, the slot engine assigns someone free). A person's own price and duration are shown when they differ from the branch's.
4. **Day and time.** `POST …/availability` on channel `web`, 14 days per request (the API allows 31), 15-minute steps.
   - Arrows move the window, never before today and never past `latest_date`.
   - A day without free starts is greyed out; its status says why (closed, full, too soon, too far, nobody available).
   - The first day with free starts is picked by itself.
   - Picking a time shows the planned lines with the assigned people.
5. **Hold.** "Devam" on the time calls `POST …/checkouts` with a fresh `Idempotency-Key`. The countdown of `expires_at` (`booking.checkout_minutes`) shows on the next steps.
   - Going back from the contact step releases the hold (`DELETE …/checkouts/{token}`).
   - A taken time (`409 starts_at / taken`) clears the pick and reloads the days.
   - When the countdown ends, the page says that the booking still goes through while the time is free, because the API books an expired checkout again in one transaction.
6. **Contact.** First and last name, phone (the API normalizes it to E.164), and optional e-mail and note. SMS and e-mail consent are off by default. The `website` honeypot sits off screen with `aria-hidden` and `tabIndex=-1`. There is no OTP; phone verification comes with TEC-909 / TEC-910.
7. **Summary.** The time, the branch, each line with person, time and catalog price, the total, the contact, the payment choice and the cancellation rule. With staff approval the button says "Talep gönder" and a note says the business confirms. The final amount is the API's.
8. **Booking.** `POST …/reservations` with one `Idempotency-Key` per hold, so a retry returns the same reservation.
   - The answer's `access_token` is kept in `localStorage` (`takwim.booking.{uuid}`), and the page opens `/b/{slug}/reservations/{uuid}#t={token}`.
   - A taken time sends the customer back to the time step.

### Payment

The choices are the business's `payment_methods` that every picked service allows (`payment_mode`).

- A free booking (total 0, such as a table) offers only payment on site, because the API refuses online payment for it (`400 payment / not_allowed`).
- Until the payment layer (TEC-912) can send the customer to a payment page, **on site is offered whenever it is allowed**, and online only when it is the only way. Such a booking answers `online_pending` with `expires_at`, and the status page says the payment is awaited and until when.

## Status page

`GET …/reservations/{uuid}` with `X-Reservation-Token`. The token comes from the link's fragment, which never reaches a server or a Referer, or from this device's storage. Without it the page says the booking opens only from its link and points to Randevularım.

The headline follows the states:

| State                           | Headline                                     |
| ------------------------------- | -------------------------------------------- |
| `requested`, approval `pending` | waiting for the business, until `expires_at` |
| payment `online_pending`        | payment awaited, until `expires_at`          |
| `active`, `in_service`          | confirmed                                    |
| `cancelled`, `rejected`         | cancelled / declined                         |
| `completed`, `no_show`          | visit done / marked absent                   |

Below the headline come the date and time in the business's time zone, the branch, the booking number, the state chips (the panel's labels), the lines and the total. A request waiting for the business is reloaded every 30 seconds.

## Errors and limits

- Calls go out with `credentials: "omit"`, so the page stays anonymous even when a panel user is signed in on the same browser.
- `429` answers show "try again in N minutes" from `Retry-After`.
- `409 BOOKING_UNAVAILABLE` says bookings are closed (`errors.codes.BOOKING_UNAVAILABLE`).
- Anything else goes through the global API error toast.

## Not in this issue

- **Theme settings.** Colours, fonts, cover and texts set in the panel need a settings model and an API field; that is TEC-966. The page reads only the logo and branch photos today.
- **Restaurant party size and area.** These come with TEC-903: `party_size` and `zone` on availability and checkouts, and `zones` on branches. The wizard then adds the `party_size` and `area` steps of the flow.
- **Products, packages, vehicle steps.** They come with their catalog modules.
- **Online payment redirect** (TEC-912), **phone OTP and the customer panel** (TEC-909 / TEC-910), **embedding and pre-selection** (TEC-908).
