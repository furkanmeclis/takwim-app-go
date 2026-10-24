# Customer confirmations (TEC-921, TEC-922, TEC-923, TEC-924, TEC-979, TEC-981)

The Konfirmasyon add-on: before a visit the business asks the customer to confirm it, on its channels in order, a set number of times, and cancels or flags the booking when no answer comes by the deadline. The customer answers in the customer panel or on the single-visit link of the message: "Geleceğim", "Değişiklik iste" or cancel. Everything is gated by the `confirmation` module (`sector.ModuleConfirmation`, an add-on, entitlements.md); without it no task is planned, no request is sent and no answer is offered.

| Area         | Path                                                                                                                        |
| ------------ | --------------------------------------------------------------------------------------------------------------------------- |
| Engine       | `backend/internal/modules/confirmations/usecase` (`engine.go` tasks, requests, deadline, answers; `plan.go`; `settings.go`) |
| Model        | `backend/internal/modules/confirmations/model` (rule, default template, limits)                                             |
| HTTP, routes | `backend/internal/modules/confirmations/handler`, `routes.go`                                                               |
| Answers      | `backend/internal/modules/customerpanel/usecase/confirm.go`, `handler/confirm.go`                                           |
| Queries      | `backend/internal/database/queries/confirmations.sql`                                                                       |
| Migration    | `000064_confirmations`                                                                                                      |
| Tests        | `confirmations/confirmations_db_test.go`, `confirmations/usecase/plan_test.go`, `customerpanel/confirm_db_test.go`          |
| Web          | `frontend/src/features/confirmations/*`, page `/t/{slug}/settings/confirmation`, locale `confirmations.json`; panel buttons |

## Data

| Table                       | Holds                                                                                                                                                                                                                                            |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `confirmation_settings`     | the business-wide rule (`branch_id` null) or one branch's own rule: on / off, ordered `channels`, `fallback`, first request, follow-up and deadline (minutes before the start), `max_attempts`, contact window, `on_no_answer` (`cancel`/`flag`) |
| `reservation_confirmations` | the task of one booking (one row per reservation): `round`, the tracked `reservation_version` and `starts_at`, the rule copied at planning, the planned `attempt_times`, `attempts_sent`, `next_attempt_at`, `deadline_at`, answer, outcome      |

No settings row = the template (`model.DefaultRule`: on, SMS then e-mail with fallback, first request 24 hours before, a follow-up 3 hours before, 2 requests, deadline 2 hours before, requests between 09:00 and 21:00, cancel). A branch without its own rule follows the business-wide one. Saving a rule does not re-plan tasks already planned; it applies to bookings planned afterwards.

## Planning

The engine is a reservations `Notifier` (`reservations.Notifiers(notify, confirm)`), so it runs inside the transaction of every reservation change.

- A booking that is `active` and `accepted` (created, accepted, paid or rescheduled) gets a task. An online booking waiting for payment or approval waits until it is accepted / paid.
- `PlanSchedule`: deadline = start − `deadline_minutes`; the last request goes out at least 30 minutes before the deadline. The first request is at start − `first_request_minutes` (now for a later booking), moved into the contact window; follow-ups start at start − `follow_up_minutes` and are spread evenly to the last request time (at least 15 minutes apart, inside the window). A booking too close to its start for any request stays `not_required`.
- The reservation's `confirmation_status` goes `not_required` → `scheduled` (event `confirmation_scheduled`, note = the deadline).
- A move re-plans (new `round`, the answer is cleared, `confirmed` goes back to `scheduled`). Any other change only tracks the new `version`. A cancel, reject, finish or start drops the task (`dropped`). A module or rule turned off drops it at the next step and sets `not_required`.

## Requests

The worker runs `Engine.Run` every 15 seconds: due tasks (`next_attempt_at` or `deadline_at` passed), one transaction each, locking the reservation first and then the task.

- A task whose tracked version or start no longer matches, or whose booking is not active, is dropped.
- A due attempt writes a `confirmation.request` notice through the customer notification outbox (`Engine.EmitReservation`) with the task's channels and fallback, key `r<round>:a<n>`, and the `deadline` variable in the branch time zone. Status `pending`, event `confirmation_requested` (note `n/total`).
- `confirmation.request` is a **managed** event of the notification catalog: it is not on the notification settings page and its rule comes from the confirmation settings. Its SMS and e-mail carry the single-visit link (`adapters.VisitLinks` over `customerpanel.IssueVisitLinkFor`), never a sign-in link.
- The notification engine asks the confirmation engine's **dispatch guard** before sending: a request of an answered or closed task, or of an older round, is cancelled.

## Deadline

At `deadline_at` without an answer (and without a change request):

| Situation                | Result                                                                                                                                                 |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| no request was ever sent | `expired`, outcome `unreachable`: the booking is never cancelled for a message the customer did not get; the business sees it                          |
| `on_no_answer = cancel`  | `reservations.Cancel` with the expected version, reason `confirmation_timeout`: capacity is freed by the ordinary transition; task outcome `cancelled` |
| `on_no_answer = flag`    | `expired`, outcome `flagged`: the booking stays; a late answer still counts until the start                                                            |

The cancel runs after the task's transaction, guarded on `confirmation_status` still `scheduled` / `pending`; a stale version (the booking changed meanwhile) does nothing.

## Answers

| Route                                                  | Who                          |
| ------------------------------------------------------ | ---------------------------- |
| `POST /v1/customer/reservations/{uuid}/confirm`        | signed-in panel              |
| `POST /v1/customer/reservations/{uuid}/change-request` | signed-in panel (`note`)     |
| `POST /v1/customer/visit/confirm`                      | visit link (`token`)         |
| `POST /v1/customer/visit/change-request`               | visit link (`token`, `note`) |
| `POST /v1/public/whatsapp/meta/{account}/webhook`      | a WhatsApp button (TEC-922)  |

- Opening a visit link answers nothing (link previews); only the button does.
- **Confirm**: `confirmed`, event `confirmation_confirmed`, no more requests, no cancel.
- **Change request**: the booking stays, no more requests and no cancel; event `confirmation_change_requested` with the note (500 characters). Once per round; the customer may still confirm, and the business moves the visit (which re-plans).
- **Cancel**: the ordinary cancel under the business's cancel rules; while a task is open it counts as `declined` (event `confirmation_declined`).
- The panel's `actions.confirm` / `actions.request_change` say whether each is allowed and until when (`not_requested`, `answered`, `status`, `business_unavailable`); `confirmation` shows the status and deadline.

## Business settings API

`tenant.settings.read` reads, `tenant.settings.write` changes; no new permission. All routes need the add-on (`403 MODULE_NOT_ENABLED`).

| Route                                          | Does                                                                              |
| ---------------------------------------------- | --------------------------------------------------------------------------------- |
| `GET /v1/tenant/confirmations/settings`        | channels with readiness, the business-wide rule, template, branch rules, limits   |
| `PUT /v1/tenant/confirmations/settings`        | save the business-wide rule (no `branch_uuid`, every branch needed) or a branch's |
| `POST /v1/tenant/confirmations/settings/reset` | remove a saved rule                                                               |
| `GET /v1/tenant/confirmations/texts`           | the request's SMS and e-mail texts with defaults, variables, the call text        |
| `PUT /v1/tenant/confirmations/texts`           | save the SMS / e-mail texts (TEC-979)                                             |
| `POST /v1/tenant/confirmations/texts/reset`    | every request text back to the default                                            |

Activity history: `confirmations.updated` / `.reset`, `confirmations.texts_updated` / `.texts_reset`, `reservations.confirmed` / `.change_requested` (source `customer_panel` / `visit_link`).

## Request texts (TEC-979)

What the request says is the business's on SMS and e-mail, like any customer notice; the rule (channels, order, fallback, timing) stays the confirmation rule's.

| Channel      | Text                                                                                                                               |
| ------------ | ---------------------------------------------------------------------------------------------------------------------------------- |
| `sms`        | the business's text, else the catalog default                                                                                      |
| `email`      | the business's subject and text, else the catalog default                                                                          |
| `whatsapp`   | the template approved in the business's Meta account (below); a business-initiated message cannot carry free text, so not editable |
| `voice_call` | the catalog's call text (`catalog.Event.Voice`), read out with the three keys; not editable (TEC-923)                              |

- The texts live in `customer_notification_templates` (event `confirmation.request`), business-wide: one text for every branch. The notification engine renders them as for any event, so a saved text applies to requests sent from then on, retries and repeated calls of a request already planned included.
- The catalog marks the event's editable channels (`Editable`), its variables (the reservation variables plus `deadline`) and the ones every text keeps (`Required`: `link`, the single-visit link, since the customer answers an SMS or an e-mail only through it). Saving refuses a text without `{{link}}` (`missing_variable`), a placeholder that is not a variable (`unknown_variable`) and any other channel (`invalid`). An empty text, or the default one, goes back to the default; a channel left out keeps its text. Limits are the notification texts' (subject 200, text 1000).
- `customernotify` `Engine.ManagedTexts` / `SaveManagedTexts` / `ResetManagedTexts` do the work; the confirmation handler serves them behind the add-on (`TextStore`). `PUT` / reset of the notification settings still refuse the event. The preview (`POST /v1/tenant/customer-notifications/preview`) knows `deadline` for it.
- Web: the "Teyit mesajı" card of `/t/{slug}/settings/confirmation` shows each channel's text (default or custom; WhatsApp and the call read-only) and opens the notification editor (`TemplatesField`: tabs per channel, variables, preview, "Varsayılan metin") for SMS and e-mail.
- Tests: `confirmations/texts_db_test.go`.

## Confirmation list and follow-up (TEC-924)

The desk's work list: every booking with a confirmation task, the business's approval and the customer's confirmation side by side (A08), the requests on every channel, the answers, and what needs a person before the visit. No new table and no new permission: `tenant.reservations.read` reads, `tenant.reservations.write` acts; members of their own calendar only (`tenant.reservations.own`) do not get the business-wide list. Every route needs the add-on (`403 MODULE_NOT_ENABLED`) and applies the branch scope.

| Area    | Path                                                                                                                                                      |
| ------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Engine  | `confirmations/usecase/list.go` (list, facets, summary, attempts, phone confirmation)                                                                     |
| HTTP    | `confirmations/handler/list.go`, `routes.go`                                                                                                              |
| Queries | `backend/internal/database/queries/confirmation_list.sql`                                                                                                 |
| Tests   | `confirmations/list_db_test.go`; the routes are in `httpserver/isolation_test.go`                                                                         |
| Web     | `frontend/src/features/confirmations/*` (list page, drawer, attempts table, phone dialog, alerts, nav badge), page `/t/{slug}/reservations/confirmations` |

### List states

`state` is the task status with two refinements: an open task the customer asked to move is `change_requested`, and an expired task shows its outcome (`cancelled`, `flagged`, `unreachable`). So: `change_requested`, `unreachable`, `flagged`, `pending`, `scheduled`, `confirmed`, `declined`, `cancelled`, `dropped`. Without a `state` filter `dropped` tasks (the visit was cancelled, moved past the deadline, or the add-on was turned off) are left out.

`send_failed`: an open task with a request of the current planning (`scheduled_at >= planned_at`) that ended `failed` or `skipped` on every channel. `attention`: the visit is `active` and still ahead, and the state is `change_requested`, `unreachable` or `flagged`, or `send_failed`. After the start nothing needs a person any more.

### Routes

| Route                                                      | Does                                                                                                                           |
| ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `GET /v1/tenant/confirmations`                             | the list: `q` (name, phone, number), `branch`, `state`, `attention`, `starts_from` / `starts_to`; sort `starts_at` (default) … |
| `GET /v1/tenant/confirmations/facets`                      | counts per state and `attention` with the other filters                                                                        |
| `GET /v1/tenant/confirmations/summary`                     | upcoming visits that need a person, by reason (`change_requested`, `unreachable`, `flagged`, `send_failed`) and `waiting`      |
| `GET /v1/tenant/confirmations/meta`                        | the table contract                                                                                                             |
| `GET /v1/tenant/reservations/{uuid}/confirmation/attempts` | the requests of a booking: one row per channel tried (provider result, masked recipient, error, times, round / request number) |
| `POST /v1/tenant/reservations/{uuid}/confirmation/confirm` | "Telefonla teyit edildi": `note`, `expected_version`                                                                           |

A phone confirmation is `Engine.Answer` with source `phone` and the member as the history actor (event `confirmation_confirmed`, note = the member's note): no more requests, no automatic cancel. It takes the same tasks as the customer's answer (open, or expired without a cancel, visit ahead), else `409 CONFIRMATION_NOT_OPEN`; a stale `expected_version` is `409` `expected_version` / `stale`. Activity: `reservations.confirmed` with source `phone`. Moving and cancelling use the reservation transitions (a move plans the task again).

### Live updates and alerts

The list follows the reservations live channel (`tenant.reservations`): reservation handlers publish there, the customer panel publishes the answers, the phone confirmation handler publishes, and the worker's engine publishes each request it writes and each deadline it applies (`Engine.SetResourceEvents`). The notification engine publishes the outcome of every request (TEC-981, `customernotify` `Engine.SetResourceEvents(pub, confirmationsusecase.Event)`): the send result once per processing (sent, accepted, failed, skipped, cancelled, no channel left), and every delivery report it records (WhatsApp webhook in the API, receipt timeout in the worker; a failure and the next channel's result are one event, a late delivery / read receipt is one too). A future channel (the voice call of TEC-923) goes through the same send / report path and needs no wiring of its own.

The summary (sidebar badge, panel alert) subscribes to the same channel and refreshes on any event (debounced); the request table of a visit refreshes on events of that visit. The summary keeps a 5 minute refresh for what no event marks (a visit whose time came leaves the counts) and for events missed while disconnected; with live updates off it refreshes every minute.

- Sidebar: "Teyit listesi" under the operations group (`module: confirmation`, `tenant.reservations.read`) with the `attention` count and the reasons in its info popover.
- Panel alert (`ConfirmationAlerts`) on the organization home and on top of the list: the count by reason and a link to the "Takip gerekenler" view.
- Reservation detail: the confirmation card offers "Telefonla teyit edildi" and lists the requests.

The list's views are `Takip gerekenler` (`attention=true`, visits from now on), `Bugün` (default), `Yarın`, `Yaklaşan`, `Tümü`; days are counted in the business's time zone. `?view=attention` and `?state=…&branch=…` open the list with them.

## Channels (TEC-922)

The request goes out on the rule's channels in order (fallback on). What the provider accepted, what it delivered, when the customer read it and what the customer answered are recorded apart, and only an explicit answer counts: a delivered or read message, an opened e-mail or an opened link is never a confirmation (§9), and a reply that cannot be applied is recorded as such, never turned into a confirmation.

| Area              | Path                                                                                                                        |
| ----------------- | --------------------------------------------------------------------------------------------------------------------------- |
| Cloud API adapter | `backend/internal/platform/whatsapp/meta` (integration adapter, template / text send, signature, webhook parsing)           |
| Channel, webhook  | `backend/internal/modules/whatsapp` (`usecase/channel.go`, `usecase/service.go`, `handler`, `routes.go`)                    |
| Answers           | `customerpanel/usecase/confirm.go` (`ChannelAnswer`), `confirmations/usecase/engine.go` (`RequestCurrent`)                  |
| Receipts, timeout | `customernotify/usecase/engine.go` (`RecordDelivery`, `ExpireReceipts`), queries `confirmation_channels.sql`                |
| Migration         | `000067_confirmation_channels` (`delivered_at`, `read_at`, `customer_notification_responses`)                               |
| Tests             | `customerpanel/whatsapp_db_test.go`, `platform/whatsapp/meta/meta_test.go` (`TestLiveTemplate`: the real-account test, A09) |
| Web               | attempts table (delivered / read, replies), Ayarlar → Entegrasyonlar (WhatsApp Cloud API account, webhook address)          |

### Receipt times

| Column (`customer_notification_deliveries`) | Means                                                                      |
| ------------------------------------------- | -------------------------------------------------------------------------- |
| `sent_at`                                   | the provider accepted the message (SMTP took it, the Cloud API gave an id) |
| `delivered_at`                              | the provider reported it delivered (WhatsApp `delivered`)                  |
| `read_at`                                   | the customer opened it (WhatsApp `read`)                                   |
| `finished_at` / `status`                    | the attempt ended: `sent`, `delivered`, `failed`, `skipped`                |

A channel with delivery reports keeps its attempt open (`sent`, the dispatch `sending`) until the report. A `failed` report sends the request on the next channel; no report within 30 minutes (`DefaultReceiptTimeout`, the worker's minute sweep) fails the attempt as `no delivery report` and the next channel runs too. A late report still records its time and changes nothing else.

### WhatsApp

- **Account.** The business connects its own WhatsApp Business number in Ayarlar → Entegrasyonlar (`kind = whatsapp`, provider `meta`, registered when `WHATSAPP_META_ENABLED=true`): phone number id, a permanent access token, the Meta app secret, a verify token of its choice, and optionally the template name (default `randevu_teyit`) and language (`tr`). The connection check reads the number with the token. A `log` account (development) or no account: the channel is not ready for the business and every send is `skipped`.
- **Template.** Only the confirmation request goes out on WhatsApp (a business-initiated message needs a Meta-approved template); every other notice is `skipped` and its next channel runs. The template the business registers: category _Utility_, body with five parameters and three quick reply buttons in this order:

  > Merhaba {{1}}, {{2}} işletmesindeki {{3}} randevunuza gelecek misiniz? Lütfen {{4}} saatine kadar yanıtlayın. Randevu no: {{5}}
  >
  > [Geleceğim] [İptal] [Değişiklik]

  Parameters: `customer_name`, `business_name`, `starts_at`, `deadline`, `reference`. The buttons carry the payloads `tkw1:confirm`, `tkw1:cancel`, `tkw1:change`. No link: the buttons answer.

- **Webhook.** `GET|POST /v1/public/whatsapp/meta/{account}/webhook` (the address is shown when the account is edited). `GET` is the subscription handshake (`hub.verify_token` = the account's verify token). `POST` must carry `X-Hub-Signature-256` with the account's app secret (`401` otherwise); events of another number of the same app are left alone. `500` (an internal error) makes Meta send it again.
- **Statuses.** `delivered` / `read` → receipt times (and the attempt `delivered`); `failed` → the next channel; `sent` repeats the acceptance. The webhook only records; the worker sends the next channel (links, usage).
- **Replies.** A button press is an answer only when it replies to our message (context id), comes from the customer's own number and belongs to the booking's current request (`RequestCurrent`: the task's round and the same visit start; a press on the request of a visit that moved since never acts, A12). Then `customerpanel.ChannelAnswer` applies it with the visit link's rules: `confirm` and `change` through `Engine.Answer` (source `whatsapp`), `cancel` through the ordinary cancel under the business's cancel cutoff. Each inbound message is handled once (`integration_jobs`, kind `whatsapp.reply`, key = the inbound message id) and recorded in `customer_notification_responses` with `answer` (`confirm`, `cancel`, `change`, `unknown`), `result` and `reason`:

| Result     | When                                                                                                                              |
| ---------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `applied`  | the answer was taken (activity `reservations.confirmed` / `.change_requested` / `.cancelled`, source `whatsapp`; live list event) |
| `rejected` | an answer the booking does not take now: `answered`, `status`, `cutoff`, `request_outdated`, `not_open`, `reservation_not_found`  |
| `ignored`  | not the customer's answer: `other_number`, `not_an_answer` (a typed text), `not_confirmation_request`                             |

The customer gets a short Turkish answer in the chat (allowed inside the service window): confirmed, cancelled, change sent, or why nothing changed. Messages that do not reply to ours are not touched.

### SMS and e-mail

Both carry the single-visit link (`/panel/v#<token>`, issued at send time). The token is in the URL fragment, so a link preview (a messenger or mail scanner fetching the URL) never even sends it to the server; opening the page only shows the visit (`POST /v1/customer/visit`), and every answer is a separate explicit request from a button (A21, `customerpanel/confirm_db_test.go`). E-mails have no open tracking. The SMS provider is chosen in TEC-884 (the log sender counts as not ready, so nothing is recorded as sent through it); its delivery reports will use the same receipt columns.

### Real integration test (A09)

`TestLiveTemplate` in `platform/whatsapp/meta` sends the confirmation template from a real number when `WHATSAPP_IT_PHONE_NUMBER_ID`, `WHATSAPP_IT_ACCESS_TOKEN` and `WHATSAPP_IT_TO` (and `WHATSAPP_IT_TEMPLATE` / `WHATSAPP_IT_LANGUAGE` if not the defaults) are set; the end-to-end check is then a booking of a test business with the account connected: the request arrives, the status reaches `delivered` / `read` in the attempts table, and each button gives its answer. SMS and e-mail follow with their providers (TEC-884).

## Voice call (TEC-923)

The automatic call with keys (K-006, A10): `voice_call` is a channel of the confirmation rule like the others (order, fallback). The call reads out the visit and takes one key: **1** Geleceğim, **2** cancel, **3** another time. Picking up is never a confirmation, and neither is no key or another key.

| Area             | Path                                                                                                               |
| ---------------- | ------------------------------------------------------------------------------------------------------------------ |
| Netgsm adapter   | `backend/internal/platform/voice/netgsm` (integration adapter, call order XML, result parsing)                     |
| Channel, webhook | `backend/internal/modules/voicecall` (`usecase/channel.go`, `usecase/service.go`, `handler`)                       |
| Calls again      | `confirmations/usecase/engine.go` (`CallMissed`, `call`, call hours in `attempt`)                                  |
| Migration        | `000070_confirmation_voice_calls` (call rules on `confirmation_settings` and `reservation_confirmations`)          |
| Tests            | `customerpanel/voicecall_db_test.go`, `platform/voice/netgsm/netgsm_test.go`, `confirmations/usecase/plan_test.go` |
| Web              | rule dialog "Otomatik arama" section, attempts table (calls made again), Ayarlar → Entegrasyonlar                  |

- **Account.** The business connects its own Netgsm account in Ayarlar → Entegrasyonlar (`kind = voice_call`, provider `netgsm`, registered when `VOICE_NETGSM_ENABLED=true`): user code and API password; voice messages with key presses must be on for the account. The connection check asks for the last hour's report (no call). Without an account the channel is not ready and every call is `skipped`.
- **Call.** Only the confirmation request is called (any other notice is `skipped`). The catalog's voice text (`catalog.Event.Voice`) is read out: the customer, the business, the date and time of the visit and the three keys. Each key answers with a short text that promises nothing the business's rules could refuse ("İptal isteğinizi aldık…"). The order rings for 30 seconds, starting now; Netgsm needs a window of at least an hour. Only Turkish numbers are called.
- **Result.** Netgsm posts each call's result to `POST /v1/public/voice/netgsm/{account}/webhook?d=<attempt>&s=<token>`, the address given with the call (`VOICE_WEBHOOK_BASE_URL`, default `PUBLIC_FRONTEND_URL` + `/api`). Netgsm signs nothing, so `s` is a keyed hash of the account and the attempt (`FieldCipher.BlindIndex`, purpose `voice.webhook`) that only this server can make (`401` otherwise); the order id must be the attempt's. Each result is handled once (`integration_jobs`, kind `voice_call.result`).

| Result                          | Attempt                                                            | Then                                          |
| ------------------------------- | ------------------------------------------------------------------ | --------------------------------------------- |
| answered, key 1 / 2 / 3         | `delivered` (`delivered_at` = answered)                            | the answer (below)                            |
| answered, no key / another key  | `delivered` with the error, no answer (another key: `invalid_key`) | the next channel runs; the call is made again |
| not answered, busy, unreachable | `failed`                                                           | the next channel runs; the call is made again |
| no result within 30 minutes     | `failed` (`no delivery report`, the receipt timeout)               | the next channel runs                         |

- **Answer.** Like a WhatsApp button: the key acts only for the customer's number and the booking's current request (`RequestCurrent`), through `customerpanel.ChannelAnswer` (source `voice_call`): 1 and 3 through `Engine.Answer`, 2 through the ordinary cancel under the business's cancel cutoff (a paid booking goes to `refund_pending` there and the refund is followed like any cancel). Each key is a `customer_notification_responses` row (channel `voice_call`, `provider_ref` = the order id).
- A picked up call without an answer counts as reached for the deadline: the K-007 cancel applies when no answer comes. Calls that never got through do not count, so a booking only called and never reached is `unreachable`, never cancelled.

### Call hours and calls made again

| Rule field                                            | Means                                                                                                                                                 |
| ----------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| `call_window_start_minute` / `call_window_end_minute` | local hours a call may ring; null = the request window. With the call listed and both windows set, they must overlap by an hour. Template 10:00–20:00 |
| `call_retries` (0–3)                                  | extra calls per planning when a call is missed (not answered, busy, unreachable, no valid key). Template 1                                            |
| `call_retry_minutes` (15–240)                         | the wait before the next one. Template 30                                                                                                             |

- The task keeps the call rules from planning (the call hours as they apply: the rule's own, else the request hours), `calls_made` and `next_call_at`.
- A request due outside the call hours goes out without the call (the other channels). A request whose only channel is the call waits for the call hours when an answer still fits before the deadline.
- A missed call of the latest request plans the next one (`Engine.CallMissed`): after the wait, moved into the call hours, at least 30 minutes before the deadline and not within 15 minutes of the next planned request (which calls anyway). The worker makes it as its own dispatch of the same request on the call alone (key `r<round>:a<n>:c<k>`), so the dispatch guard and the answer check treat it as that request. An answer, a change request, a new request or a new planning stops it.
- The attempts table shows such a call as "n. istek, k. tekrar arama" (`call`).

## Not here yet

- Live list events for delivery failures and provider reports (TEC-981).
- Twilio as a second voice provider (TEC-986); polling Netgsm's report when a result never arrives (the receipt timeout runs the next channel).
