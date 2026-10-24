# Customer notifications (TEC-911)

The messages a business's customers get about their bookings: request received, accepted, rejected, changed, cancelled, and a reminder before the visit. They work in every business, without the confirmation add-on (A28), and every text carries the business's name and the customer panel link. Each business chooses which events notify, on which channels in which order, and with which text.

| Area         | Path                                                                                                                            |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------- |
| Engine       | `backend/internal/modules/customernotify/usecase` (`engine.go` outbox and sending, `settings.go` rules, texts, history)         |
| Catalog      | `backend/internal/modules/customernotify/catalog` (events, default rules, Turkish default texts, variables, limits)             |
| Channels     | `backend/internal/modules/customernotify/channel` (the `Channel` port), `adapters` (SMS, e-mail)                                |
| HTTP, routes | `backend/internal/modules/customernotify/handler`, `routes.go`                                                                  |
| Port         | `backend/internal/modules/reservations/usecase/notify.go` (`Notifier`, `Change`)                                                |
| Queries      | `backend/internal/database/queries/customer_notifications.sql`                                                                  |
| Migration    | `000059_customer_notifications`                                                                                                 |
| Tests        | `backend/internal/modules/customernotify/customernotify_db_test.go`                                                             |
| Web          | `frontend/src/features/customer-notifications/*`, page `/t/{slug}/settings/notifications`, locale `customer_notifications.json` |

## Data

| Table                              | Holds                                                                                                                                                                                       |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `customer_notification_rules`      | a business's rule of one event: on / off, ordered `channels`, `fallback`, `reminder_offsets` (minutes before the visit). No row = the catalog default.                                      |
| `customer_notification_templates`  | a business's text of one event on one channel (`subject` for e-mail, `body`). No row = the catalog default.                                                                                 |
| `customer_notification_dispatches` | the outbox: one notice of one event for one customer (`idempotency_key` unique per business), the channels and fallback copied from the rule, the variables, `due_at` for reminders, status |
| `customer_notification_deliveries` | one attempt per channel of a dispatch (unique dispatch + channel): status, provider, provider reference, masked recipient, the rendered subject and text, error, sent / finished times      |

Dispatches and deliveries belong to the customer and the reservation (composite foreign keys with `organization_id`, `ON DELETE CASCADE`). Rendered texts hold names, the booking number, times, the branch, the services and the public panel address, never prices, notes or a personal sign-in link.

## Writing (outbox)

`reservations` calls the `Notifier` port inside the transaction of every reservation event it records (create, accept, reject, cancel, expire, reschedule, payment). The engine maps the event and writes the dispatch with the same `*db.Queries`, so a notice commits with the change and never exists for a rolled back one.

| Reservation event                               | Notice                    | Key (once per)       |
| ----------------------------------------------- | ------------------------- | -------------------- |
| created as `requested` (online, needs approval) | `reservation.requested`   | visit                |
| created as `active`, accepted, paid             | `reservation.accepted`    | visit                |
| rejected                                        | `reservation.rejected`    | visit                |
| cancelled, request expired                      | `reservation.cancelled`   | visit                |
| rescheduled                                     | `reservation.rescheduled` | reservation version  |
| reminder (worker, below)                        | `reservation.reminder`    | offset + visit start |

The key is `event|reservation:<uuid>|<discriminator>`; inserting a dispatch with a key that exists does nothing, so a replayed request or a payment after the acceptance never sends a second "accepted". A visit whose start has passed gets no notice (a walk-in recorded afterwards). Panel bookings notify as well as online ones. An event that is off writes nothing; a rule without channels writes a `skipped` dispatch.

The direct "panel link" SMS of the public booking flow is gone: the requested / accepted notice carries the link (customer-panel.md).

## Sending

The worker runs `Engine.Run`: due dispatches every 5 seconds, reminder planning every minute, per business.

1. **Claim** (one transaction): lock the dispatch (`FOR UPDATE SKIP LOCKED`), check it is still worth sending (business active; for a reminder the visit is still active at the same start and has not begun, else `cancelled` with the reason), render the business's text or the default, and insert the attempt row of the next channel. The insert is `ON CONFLICT DO NOTHING` on (dispatch, channel): **each channel is tried at most once per dispatch**, so a retry or a second worker never sends the same notice twice on one channel.
2. **Send** through the channel adapter. On a ready SMS channel the `LinkIssuer` (`adapters.PanelLinks` over `customerpanel.PanelLink`, TEC-910) puts the phone's single-use panel link into the text at send time only; the stored text keeps the public address. Other channels, a channel that is not ready, and a phone past its daily link cap send the public address, which asks for a code.
3. **Record**: `sent` / `delivered` ends the dispatch (`delivered`); `failed` or `skipped` moves on to the next channel when `fallback` is on, else ends it (`skipped` when every attempt was skipped, otherwise `failed`). An `accepted` result (provider queued it) keeps the dispatch `sending` until the provider's report (`ReportDelivery`), which runs the next channel on failure.

An attempt that stops between claim and record stays `sending` and is not resent: delivery is at most once per channel, never twice.

**A channel that is not ready is never a success.** `Channel.Ready()` says whether a real provider can deliver. The engine turns any success of a channel that is not ready into `skipped` ("provider not configured"), and only real sends count usage (`entitlements.AddUsage`, metric = channel name).

## Channels

The `channel.Channel` port (`Name`, `Ready`, `Send`) is the provider layer; adapters are registered on the engine at start, a later one replacing an earlier one of the same name. A channel that reports later implements `ReportingChannel` and gets the engine as its `Reporter`.

| Channel    | Adapter                                                                                                                                                                                         |
| ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `sms`      | `adapters.SMS` over `sms.Sender`. `LogSender` (until the provider is chosen, TEC-884) is not ready: the message is logged in development, the attempt is `skipped`. No phone number: `skipped`. |
| `email`    | `adapters.Email` over the SMTP mailer; ready when `MAIL_DRIVER=smtp` and `SMTP_HOST` is set (another driver: `skipped`); an SMTP error is `failed`. No address: `skipped`.                      |
| `whatsapp` | a known channel without an adapter yet: always `skipped`. WhatsApp (TEC-922) and voice (TEC-923) adapters register on the same port.                                                            |

The default rule of every event is SMS, then e-mail, with fallback; the reminder defaults to 24 hours before.

## Rules and texts

Texts are Turkish and use `{{variables}}`: `customer_name`, `business_name`, `branch_name`, `reference`, `starts_at`, `date`, `time`, `services`, `link`. Times are in the branch's time zone (else the business's). Limits: subject 200, text 1000 characters; reminders 15 minutes to 7 days before, at most 3. Several reminder times do not stack: when one is due, the smallest passed offset planned after the booking was made is sent, so a customer never gets two at once and a booking made 2 hours before the visit gets no 24 hour reminder.

Saving validates the event, channels, offsets and texts (`unknown_variable` for a placeholder that is not a variable). An empty text, or one equal to the default, deletes the business's text and goes back to the default. Reset deletes the rule and texts of an event. Both are recorded in activity history (`customer_notifications.updated` / `.reset`).

## API

`tenant.settings.read` reads, `tenant.settings.write` changes; no new permission.

| Route                                                   | Does                                                                   |
| ------------------------------------------------------- | ---------------------------------------------------------------------- |
| `GET /v1/tenant/customer-notifications/settings`        | channels with readiness, every event with its rule, texts and defaults |
| `PUT /v1/tenant/customer-notifications/settings`        | save one event (`event` in the body)                                   |
| `POST /v1/tenant/customer-notifications/settings/reset` | back to the defaults                                                   |
| `POST /v1/tenant/customer-notifications/preview`        | render a text with sample values and the business's name               |
| `GET /v1/tenant/customer-notifications/deliveries`      | history, one row per channel attempt, inside the caller's branch scope |
| `GET /v1/tenant/customer-notifications/deliveries/meta` | table contract of the history                                          |

## Web

Settings → Müşteri bildirimleri. "Olaylar ve metinler" lists the events (on / off, channels in order, timing) with a warning for channels that are not ready; a row opens the editor: switch, ordered channels with readiness, fallback, reminder times, one text per channel with variables, preview and "Varsayılan metin". "Gönderim geçmişi" is the read-only history with event, channel and status filters and a drawer with the text and error. Without `tenant.settings.write` the editor is read-only.

## Confirmation requests (TEC-921)

`confirmation.request` is a **managed** event: the confirmation module (confirmations.md) writes it through `Engine.EmitReservation` with its own channels, fallback, key and variables (`deadline`), so it is not listed on the settings page and `PUT` / reset refuse it. Two hooks serve it:

- `SetDispatchGuard`: before a dispatch is claimed, the confirmation engine may cancel it (the task was answered, closed or planned again).
- `SetLinkIssuerFor(event, issuer)`: confirmation requests carry the single-visit link (`adapters.VisitLinks` over `customerpanel.IssueVisitLinkFor`) on every channel instead of the panel link.

## Not here yet

- A real SMS provider (TEC-884) and delivery reports; WhatsApp (TEC-922) and voice (TEC-923).
- Notices on the reservation and customer detail pages (the history API already filters by `reservation_uuid` / `customer_uuid`).
