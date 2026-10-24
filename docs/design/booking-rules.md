# Booking flow and rules customization

Status: **implemented** (TEC-905). A business fine-tunes its sector's ready-made booking flow (requirements §6, K-010, A06): it hides optional steps, asks its own questions, and sets online limits. Everything lives in the versioned business settings document ([business-settings.md](./business-settings.md)), so every change is a new immutable version, and the API enforces it for every online caller (booking page, widget, mobile app, partners). Nothing is checked only in a browser.

## Settings

| Path                                | Meaning                                                                                                                                                         | Default                                             |
| ----------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------- |
| `booking.hidden_steps`              | Optional steps of the sector flow the customer screens skip: `staff`, `products`, `area`                                                                        | `[]` (all shown)                                    |
| `booking.fields[]`                  | Booking form questions: `key`, `label`, `type` (`text`, `number`, `select`, `checkbox`), `options` (select), `required`, `online`, `panel`, `help` (at most 20) | `[]`                                                |
| `reservation.slot_step_minutes`     | Distance between the starts offered online (multiple of 5, 5–240)                                                                                               | `null` (the client's step, 15 on the business page) |
| `reservation.max_open_per_customer` | Open bookings (waiting, booked, in service and not over) a customer may hold when booking online                                                                | `null` (no limit)                                   |
| `reservation.max_online_per_day`    | Online bookings (page, widget, API) a branch takes per local day; cancelled and rejected ones do not count                                                      | `null` (no limit)                                   |

The existing rules stay where they were: earliest / latest booking (`min_lead_minutes`, `max_advance_days`), cancel / change cutoffs, approval mode, payment options and channels. Preparation and cleaning times are the services' buffers. Visibility of branches, services, staff, resources and products is the record's own `show_online` flag. With the defaults the behaviour is the same as before TEC-905.

## What cannot change

- The steps that make a booking (branch, service, date and time, party size, contact, summary) cannot be hidden, and the step order is the sector template's: a free step editor is out of the beta scope (requirements §3).
- Capacity and availability are always checked. Hiding the staff step does not drop the staff requirement: the engine assigns a free person. Hiding the area step lets the engine pick a table in any area.

## Enforcement (API)

| Rule              | Where                                                                                                                                   | Error                                                         |
| ----------------- | --------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------- |
| Hidden `staff`    | public availability and checkout: a line with `staff_uuid`                                                                              | 400 `lines.N.staff_uuid` / `not_offered`                      |
| Hidden `area`     | public availability and checkout: a `zone`                                                                                              | 400 `zone` / `not_offered`                                    |
| Hidden `products` | public booking with `products`; `GET …/products` answers `[]`                                                                           | 400 `products` / `not_offered`                                |
| Slot step         | online availability uses it whatever the request asks; an online start off the grid of its open interval is not free                    | 409 `starts_at` / `taken`                                     |
| Questions         | every booking: online bookings answer the `online` questions (other keys are dropped), the panel the `panel` ones (other keys are kept) | 400 `custom_fields.<key>` / `required`, `invalid`, `too_long` |
| Open bookings     | online bookings, under the business's capacity lock                                                                                     | 400 `customer` / `open_limit`                                 |
| Bookings per day  | public checkout and online booking (and a customer panel move to another day), under the lock                                           | 400 `starts_at` / `daily_limit`                               |

Panel bookings are the business's own decision: the limits do not apply to them. The business page profile (`GET /v1/public/businesses/{slug}`) returns `flow` without the hidden steps, `booking.hidden_steps`, `booking.fields` (online questions only), `booking.slot_step_minutes` and `booking.max_open_per_customer`.

## History

A reservation stores `settings_version` (the settings version it was booked under) and `custom_field_defs` (the questions it answered, as they were). The panel detail and list (`custom_answers`) and the customer's status page (`answers`) show each answer with that label, so renaming or removing a question never rewrites a booking (migration 000065). Editing the answers in the panel keeps the stored definition of a question; `PATCH` without `custom_fields` leaves the answers as they are.

## Screens

- **Settings › Akış & form**: the sector flow with switches for its optional steps and the question editor (label, type, choices, required, online / panel, order). The limits are under **Kurallar & ödeme › Online rezervasyon sınırları**.
- **Panel booking form and detail**: the `panel` questions (`GET /v1/tenant/reservations/form`, readable with the reservations permission); the list has an optional "Ek alanlar" column.
- **Business page**: the steps follow `flow`, the contact step asks the online questions, the summary and the status page show the answers.

## Not in this issue

- Per-service minimum notice: the requirements ask for business-wide earliest / latest booking only.
- Showing a full day in the availability answer: a day over `max_online_per_day` still lists its starts; the checkout refuses them with `daily_limit`.
- Packages (TEC-920) get their visibility when the module arrives.
