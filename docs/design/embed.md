# Embedding: inline frame, button window, pre-selection (TEC-908)

A business shows its booking screen on its own site in two ways (requirements §7):

- **Inline.** The booking area sits in the page. The frame's height follows its content.
- **Button.** A "Randevu al" button opens the screen in a window.

Both use the business page's wizard and status page ([business-page.md](./business-page.md)) on the public booking API's `widget` channel ([public-booking-api.md](./public-booking-api.md)). The full view and the compact view share this one booking infrastructure.

| Area           | Path                                                                                       |
| -------------- | ------------------------------------------------------------------------------------------ |
| Loader script  | `frontend/public/embed.js` (served as `/embed.js`)                                         |
| Embedded pages | `frontend/src/app/(booking)/embed/[slug]/…`                                                |
| Frame side     | `features/business-page/components/booking-surface.tsx`, `embed-pages.tsx`, `lib/embed.ts` |
| Framing policy | `frontend/src/proxy.ts`, `src/lib/server/embed-frame.ts`, `next.config.ts`                 |
| Allowed sites  | settings document `embed.allowed_origins` (`orgsettings`), public profile `embed`          |
| Code generator | `/t/{slug}/settings/embed` (`features/tenant-settings/components/embed-code-page.tsx`)     |

## Embed code

```html
<!-- inline -->
<div data-takwim-slug="kuafor-ali"></div>
<!-- or a button that opens the window -->
<button type="button" data-takwim-open="kuafor-ali">Randevu al</button>

<script async src="https://takwim.link/embed.js"></script>
```

Optional attributes on either element:

| Attribute             | Meaning                                                         |
| --------------------- | --------------------------------------------------------------- |
| `data-takwim-view`    | `full`: header, profile and tabs; default compact (wizard only) |
| `data-takwim-branch`  | branch uuid                                                     |
| `data-takwim-service` | service uuid                                                    |
| `data-takwim-staff`   | person uuid                                                     |
| `data-takwim-date`    | `YYYY-MM-DD`, first day of the date strip                       |

The script also offers `Takwim.open({ slug, … })`, `Takwim.close()` and `Takwim.scan()` (for elements added later). The panel's **Ayarlar › Gömme kodu** page builds the snippet, a pre-selected direct link (`/b/{slug}?branch=…`) and a live preview. It warns when the `widget` channel is off or no site is allowed.

## Embedded pages

| App path                            | Page                                     |
| ----------------------------------- | ---------------------------------------- |
| `/embed/{slug}`                     | wizard (compact) or business page (full) |
| `/embed/{slug}/reservations/{uuid}` | status of a booking made in the frame    |

The loader adds the following query parameters, and the pages keep them across their own navigation:

- `frame`: the frame id echoed in messages;
- `origin`: the parent page's origin;
- `view`;
- `mode=modal` for the button window.

The pages are `noindex`. Bookings run on channel `widget` and get source `embed`. When the channel is off, the screen shows the closed notice like the business page.

- **Compact view.** No sticky header and no viewport height, so the frame is exactly as tall as its content. The footer holds "powered by" and the language switch.
- **Full view.** Adds the business's name, Randevularım, language and colour mode.
- **Leaving the frame.** Randevularım and every link out of the booking open in a new tab, because the panel is never framed.

## Allowed sites and framing

The settings document has `embed.allowed_origins` (channels tab, "Sitenize gömme"). At most 20 entries, one origin each:

- `example.com` is saved as `https://example.com`;
- `https://*.example.com` allows every subdomain (as in CSP; the bare domain needs its own entry);
- `http` is accepted only for `localhost` / `127.0.0.1`;
- paths, queries, credentials, IP addresses and bare names are refused (`embed.allowed_origins.{i}` / `invalid`, more than 20 `too_many`);
- duplicates are dropped.

The public profile answers the list as `embed.allowed_origins`.

- **Panel.** Every path except `/embed/…` keeps `frame-ancestors 'self'` and `X-Frame-Options: SAMEORIGIN` (`next.config.ts`).
- **Embedded pages.** For `/embed/{slug}/…` the frontend proxy (`src/proxy.ts`) sets `Content-Security-Policy: frame-ancestors 'self' <allowed sites>; base-uri 'self'` and no `X-Frame-Options`.
- **Cache.** The list is read from the profile and cached per business for a minute, so a settings change applies within a minute.
- **Fail closed.** When the profile cannot be read, the page is `'self'` only for ten seconds. An entry that does not look like an origin is dropped.

`'self'` stays allowed so the panel's preview works.

## Messages

The frame and the loader talk with `postMessage` only. Every message is `{ source: "takwim-embed", v: 1, frame, type, height? }`:

| Type         | When                                                  | Loader                                                 |
| ------------ | ----------------------------------------------------- | ------------------------------------------------------ |
| `ready`      | the frame has loaded                                  | ignored                                                |
| `resize`     | the content height changed (inline frames only)       | sets the frame's height (240–20000 px)                 |
| `scroll_top` | the wizard moved to another step (inline frames only) | scrolls the frame into view when its top is off screen |

- **Frame side.** The frame sends only to the parent's checked origin. It takes `location.ancestorOrigins[0]`, else the referrer's origin, else the loader's `origin`. The origin must be an allowed site or this app itself; otherwise nothing is sent.
- **Loader side.** The loader accepts a message only when all of these hold:
  - it comes from the Takwim origin (the script's own origin);
  - its source window is the frame it names;
  - it has the expected shape and version.
- **No customer data.** No message carries customer data, booking details or tokens.

## Pre-selection

`branch`, `service`, `staff` and `date` work on `/embed/{slug}` and on the business page `/b/{slug}`.

1. **Server page.** Keeps only well-formed values: uuids and a real calendar date.
2. **Wizard.** Applies a value only when the API lists it for this business:
   - a public branch;
   - a service the branch offers online;
   - a person shown online who performs the picked service.

   Anything else is ignored.

3. **Booking.** Availability, checkout and booking check every value again on the server.

The wizard opens on the first step the pre-selection does not answer: a link with a branch and a service (or a service of a one-branch business) starts at the person or the time. Back still reaches the pre-filled steps.

- **Branch change.** Changing the branch restores the pre-selected service.
- **Person.** A pre-selected person serves every picked service they perform, until the customer picks someone else.
- **Date.** A past date is ignored. A date beyond `latest_date` shows as too far.

## Without third-party cookies (A21)

The screen needs no cookie:

- API calls go with `credentials: "omit"`.
- The reservation's access token travels in the status link's fragment (`/embed/{slug}/reservations/{uuid}#t=…`). It never reaches a server or a Referer.
- `localStorage` is a convenience only. Every access is wrapped, also the language preference, so a frame with site data blocked still books and shows its status.

## Online payment

The provider's page never opens inside the frame.

1. **Start.** "Şimdi öde" opens a new tab in the click itself, so popup blockers allow it. The tab gets the provider's address once `POST …/reservations/{uuid}/payment` answers; only http(s) addresses are opened. When the tab could not be opened, an "Ödeme sayfasını aç" link does the same.
2. **Watch.** The frame then watches the booking: every 3 seconds for up to 15 minutes while the payment is `online_pending`. It shows the server's `payment_status`, recorded only from the provider's verified notification ([payments.md](./payments.md)).
3. **Return.** The provider returns the new tab to `/b/{slug}/reservations/{uuid}?payment=…`. That tab has no access token, because the frame's storage is partitioned. It therefore says the result shows in the booking window and offers to close the tab.

Payment inside the frame (a provider's own iframe) is left to the provider decision (TEC-884).

## Not in this issue

- **Theme.** Business colours and fonts in the frame follow the theme settings (TEC-966). The frame uses the app's neutral theme and the visitor's colour mode today.
- **Packages.** Pre-selecting a package waits for packages in the public catalog (TEC-895).
- **Analytics events.** Messages such as "booked" for the host page are left out on purpose: they would tell the host page about the customer's booking.
