# Payments: online full payment and payment at the business (TEC-912)

A booking is paid in one of two ways. Online, the customer pays the full amount on the hosted page of the business's own payment provider. On site, staff record cash, card or a transfer on the reservation. Both write a collection to the customer ledger through `sales.RecordCollectionTx` and update the reservation's `payment_status`. Refunds are TEC-913; the customer panel reads its amounts from the same ledger (TEC-967, [customer-panel.md](./customer-panel.md#payments)).

Code:

- `backend/internal/platform/payments`: the provider contract and the providers (`testpay`, `paytr`).
- `backend/internal/modules/payments`: attempts, notifications and on-site collection.
- Migration `000060_payments`.

## Providers

`payments.Provider` is an integration adapter (`integrations.Adapter`, kind `payment`) with four more calls:

| Call                                            | Does                                                                               |
| ----------------------------------------------- | ---------------------------------------------------------------------------------- |
| `CreateCheckout(ctx, account, CheckoutRequest)` | opens the hosted page for one order (our `order_ref`, amount, buyer, return links) |
| `ParseNotification(header, body)`               | reads the provider's server notification without trusting it                       |
| `VerifyNotification(account, n, header, body)`  | checks its signature with the account's sealed credentials                         |
| `Acknowledge(w)`                                | writes the answer the provider expects (PayTR: plain `OK`)                         |

Credentials live in the integration vault like every adapter's; a business connects its own account on the integrations screen. No credential is in the code or the env.

| Provider  | Registered when                                                     | Notes                                                                                                                                                                                                                         |
| --------- | ------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `testpay` | `APP_ENV` is `development` or `test`                                | Development provider. Its page is the frontend's `/pay/test/{ref}` with Pay / Decline. The server signs the notification with the account's `secret_key` and handles it on the same verified path.                            |
| `paytr`   | `PAYMENTS_PAYTR_ENABLED=true` (`PAYMENTS_PAYTR_BASE_URL` for tests) | PayTR iFrame API: token from `merchant_id`, `merchant_key`, `merchant_salt`; sandbox mode sends `test_mode=1`; one payment, no installments. The check validates the format only (PayTR has no call that only authenticates). |

PayTR is the candidate provider of the requirements (R3). The provider decision (TEC-884) is still open, so PayTR stays off by default until a real merchant has run an end-to-end test.

The business sets the notification URL in the PayTR panel to `https://<domain>/api/v1/public/payments/paytr/notify`. The BFF forwards server-to-server posts, because a request without `Origin` / `Sec-Fetch-Site` is not a cross-site browser request.

## Data

`payments`: one row per online attempt.

- Links: reservation, branch, the integration account, and the receiving cash account (kind `online`).
- Order: `order_ref` (32 alphanumeric characters, unique across businesses, because the provider's notification carries only it), amount and currency.
- Lifecycle: `status` `pending` → `succeeded` | `failed`, with `page_url` / `provider_token`, `provider_payment_id`, `paid_amount`, `failure_reason`, `expires_at` and `completed_at`.
- Result: `outcome` (`confirmed`, `recorded`, `late`) and `ledger_entry_id`.

A check keeps a succeeded row tied to its ledger entry.

## Online payment

1. **Ready.** A business can take online payments when it has an active `payment` integration account and an active online cash account in the booking's currency. The branch's own account is preferred over a business-wide one, and the account's `provider` must be empty or the same provider. `GET /v1/public/businesses/{slug}` answers `booking.online_payment_ready`; the wizard offers online next to on site only then.
2. **Start.** `POST /v1/public/businesses/{slug}/reservations/{uuid}/payment` takes the access token (`X-Reservation-Token`). It works for a booking that is `requested` + `accepted` + `online_pending` and whose hold has not lapsed. It opens an attempt for the full remaining amount and answers the hosted page.
   - Asking again while the attempt is open returns the same page.
   - A failed attempt lets the customer try again with a new one.
   - The page expires with the hold.
   - The status page shows "Şimdi öde" while `can_pay_online` is true.
3. **Notification.** `POST /v1/public/payments/{provider}/notify` runs these steps:
   1. Parses the notification.
   2. Finds the attempt by `order_ref`.
   3. Verifies the signature with that business's account. A bad signature is `401`; nothing is written.
   4. Runs through `Runner.HandleInbound` with key `org:<id>:<event>`.
4. **Result.** Only the verified notification records a result. The return link (`?payment=done|failed`) proves nothing: the status page polls and shows the server's `payment_status`.
   - **Success.** `reservations.RecordPayment` locks the visit, and a collection (method `online`, idempotency key `payment:<uuid>`) goes into the online cash account. Then:
     - a visit still waiting for the payment becomes `paid` and `active`, its hold becomes the booking, and the history records `paid` (outcome `confirmed`);
     - a visit the hold expiry already cancelled moves to `refund_pending` and is not reopened (outcome `late`, history `late_payment`); the refund is TEC-913;
     - any other visit just records the collection (outcome `recorded`).
   - **Decline.** The attempt becomes `failed`; the visit keeps waiting until its hold ends.
   - **Amount mismatch.** A paid amount other than the attempt's fails the attempt (`amount_mismatch`) and confirms nothing.

A payment never confirms attendance (`confirmation_status` is untouched).

### Idempotency

A redelivered or concurrent notification records one collection. Three layers make sure of it:

- the inbound job of `(provider, key)`, which counts the deliveries;
- the conditional `SucceedPayment` update (applied only while the attempt is not yet succeeded);
- the ledger idempotency key.

## Payment at the business

`POST /v1/tenant/reservations/{uuid}/collections` takes `amount`, `method` (`cash`, `card`, `transfer`), `cash_account_uuid` and an optional `note`. It needs:

- the reservation reach;
- `tenant.payment.collect`;
- the cash module.

Rules:

- The amount is at most the remaining amount; a visit can be paid in parts.
- The account is an active till or bank account of the visit's branch, or a business-wide one, in the visit's currency.
- A cancelled or rejected visit is refused (`409 PAYMENT_NOT_COLLECTABLE`).
- A closed cash day or an archived account is `409 cash_account_uuid`.
- When the payments cover the total, `payment_status` becomes `paid` (history `payment_recorded`).

`GET /v1/tenant/reservations/{uuid}/payments` answers expected, paid, remaining and overpaid amounts, the money entries of the ledger (collections, refunds and their reversals), the online attempts, and the accounts that can take money (`can_collect`). The reservation detail page shows this as the Payments card with a "Tahsilat al" dialog.

## Refunds (TEC-913)

A refund pays money back on one collection of a visit. It is a `refunds` row (migration `000063_refunds`) and, once it succeeds, a `refund` entry in the sales ledger (`RecordRefundTx`, idempotency `refund:<uuid>`, source `refund`) plus a cash movement out of the chosen account.

`POST /v1/tenant/reservations/{uuid}/refunds` needs `tenant.payment.refund` and the cash module. The body names the collection, the amount, the channel and an `idempotency_key`:

- `provider`: the provider's refund call, for an online payment whose provider can refund. `testpay` decides by the cents of the amount: `.13` is declined and `.97`, `.98` and `.99` give an unknown result (its status query then answers still pending, not done and refunded). `paytr` posts to `/odeme/iade`.
- `manual`: a refund paid by hand (`cash`, `card`, `transfer`, or `online` = paid back outside Takwim) from a till or account of the branch. A till cannot go below zero (`409 cash_account_uuid` / `insufficient_funds`).

Rules:

- The amount is at most what is left of the collection: the collection less its succeeded refunds and the refunds still open. The check runs again under a lock on the collection, so partial refunds can never exceed the collection.
- The same key with the same body answers the first result (`200`, `replayed: true`). The same key with another body is `409 REFUND_IDEMPOTENCY_REUSED`. The UI makes a new key each time the dialog opens.
- The provider is called synchronously, outside the database transaction:
  - A definite refusal ends the refund as `failed`, with its reason.
  - A timeout, a 5xx or an unreadable answer leaves it `requested` with `provider_status` `uncertain`. It is never sent again by itself.
  - `POST /v1/tenant/refunds/{uuid}/resolve` (`succeeded` or `failed`) settles it after someone checks the provider's panel. A refund the provider confirmed can only be posted.
  - When the provider has a refund status query, the worker asks it instead (see "Refund status queries").
- After a posted refund the visit's `payment_status` becomes `refunded` when nothing paid is left, otherwise `partially_refunded` (history `refund_recorded`).
- A cancel, a reject or the request expiry of a visit that still holds money now sets `refund_pending`, also for money taken on site. The dialog defaults to the full amount left, so cancelling a confirmed booking ends in a full refund.

The Payments card shows the refundable amount, a refunds table (status, channel, reason) with "Sonucu işle" for open refunds, and the "İade yap" dialog.

### Closing without a refund (TEC-978)

`POST /v1/tenant/reservations/{uuid}/close-without-refund` closes a refund to make when the money stays with the business: the customer agreed to leave it, or it counts for the next visit. It needs the same permissions as a refund and a `reason`.

- Allowed when money is held, no refund is open (`409 REFUND_IN_PROGRESS`) and either `payment_status` is `refund_pending` or the last refund failed. Otherwise `409 REFUND_NOTHING_TO_CLOSE`. The payment view says so beforehand with `can_close`.
- `payment_status` becomes `kept` (migration `000076_refund_follow_through`). The reason goes into the history as event `payment_kept`, and the view shows it as `kept` (when, why, who).
- No money moves. The amount stays on the customer's ledger as a credit and can still be refunded later. A refund then makes the visit `partially_refunded` or `refunded`.
- The visit leaves the follow-up list. A refund that fails after the closing puts it back as `refund_failed`, and it can be closed again.

The Payments card offers "İadesiz kapat" next to "İade yap".

### Refund notices (TEC-978)

The customer notification engine tells the customer about a refund, using the business's rule for the event (notification settings):

- `refund.completed` ("iadeniz yapıldı"): a refund was posted (manual, provider, resolve or status query).
- `refund.processing` ("iadeniz işleniyor"): the provider refused the refund or its result is unknown.

Each goes once per refund (key `refund:<uuid>`). Like every customer message, they carry no amount.

### Refund status queries (TEC-978)

A provider adapter may implement `payments.RefundChecker` (`CheckRefund`). It only asks about a refund (by our refund reference and the payment's references) and never refunds. The worker (`payments.Service.RunRefundChecks`, every minute) takes the provider refunds with `provider_status = uncertain` whose `next_check_at` is due. Each claim counts in `provider_checks` and leases the row for two minutes, so workers do not ask twice. Then:

- found: the answer becomes `succeeded` and the refund is posted (done notice). If the posting fails, the refund waits for resolve.
- not done: the refund ends as `failed` ("not refunded at the provider (automatic check)"). It is not sent again.
- still pending, or the query failed: the next query is due after 5 min, 15 min, 1 h, 6 h, then daily.
- a provider without a status query, or an account that is gone: looked at again daily. A person settles it with resolve.

The refund view shows `auto_check`, `provider_checks`, `provider_checked_at` and `next_check_at`. `testpay` implements the query. PayTR's status query is added when the live provider is chosen (TEC-884).

### Follow-up list

`GET /v1/tenant/payments/follow-ups` (plus `/meta` and `/facets`) lists visits whose money needs a person, in the caller's branch scope. It needs the reservations module and one of `tenant.payment.refund`, `tenant.payment.collect` or `tenant.finance.read`. Kinds, first match wins:

| Kind                | When                                                                          |
| ------------------- | ----------------------------------------------------------------------------- |
| `refund_unverified` | a provider refund is still open                                               |
| `refund_failed`     | the last refund failed and money is still held                                |
| `refund_pending`    | `payment_status = refund_pending` (`late` = the money arrived after a cancel) |
| `unpaid`            | a completed visit whose charges are not fully paid                            |

`amount` is what is left to refund, or what is still owed for `unpaid`. The panel page is `/t/{slug}/reservations/follow-ups` ("Ödeme takibi" in the sidebar). A row opens the visit.

### Product returns

`POST /v1/tenant/stock-movements/{uuid}/return` needs `tenant.products.stock` and `tenant.payment.refund`. It takes back every unit of a `sale` movement in one transaction:

- a `return` movement (source `sale_return`), unique per sale;
- `ReverseLineTx` on the sale's revenue line.

It does not pay money back: a refund owed for it is recorded separately. Stock sales do not record a collection yet, so there is no money to refund against; paying back a walk-in sale needs the sale-time collection first (TEC-992). Damaged stock is fixed with a count afterwards. The stock movements table offers "Geri al" on sales not yet returned and shows a "Geri alındı" badge.

The customer panel's visit detail lists the visit's refunds (in progress, refunded, or being followed up) and the refunded total.

## Errors

| Code                         | Status | When                                                               |
| ---------------------------- | ------ | ------------------------------------------------------------------ |
| `PAYMENT_NOT_AVAILABLE`      | 409    | nothing to pay online (not accepted, paid, cancelled, window over) |
| `PAYMENT_NOT_READY`          | 409    | no working provider account or online cash account                 |
| `PAYMENT_PROVIDER_ERROR`     | 502    | the provider did not open the page                                 |
| `PAYMENT_NOT_COLLECTABLE`    | 409    | on-site payment of a cancelled or rejected visit                   |
| `PAYMENT_SIGNATURE_INVALID`  | 401    | a notification whose signature does not verify                     |
| `REFUND_IDEMPOTENCY_REUSED`  | 409    | a refund key sent again with another body                          |
| `REFUND_NOT_OPEN`            | 409    | resolving a refund that is already settled                         |
| `REFUND_COLLECTION_REVERSED` | 409    | refunding a collection that was reversed                           |
| `REFUND_NOTHING_TO_CLOSE`    | 409    | closing a visit without a refund when it has none to make          |
| `REFUND_IN_PROGRESS`         | 409    | closing a visit without a refund while a refund is still open      |
| `VALIDATION_ERROR` `amount`  | 422    | `exceeds_refundable`: more than is left of the collection          |
| `STOCK_ALREADY_RETURNED`     | 409    | a sale taken back twice                                            |
| `STOCK_NOT_RETURNABLE`       | 409    | a return of a movement that is not a sale                          |

## Configuration

| Key                       | Default                 | Meaning                                      |
| ------------------------- | ----------------------- | -------------------------------------------- |
| `PAYMENTS_PAYTR_ENABLED`  | `false`                 | registers the PayTR adapter (API and worker) |
| `PAYMENTS_PAYTR_BASE_URL` | `https://www.paytr.com` | PayTR API host (tests point it at a fake)    |

## Tests

- `payments_db_test.go`:
  - readiness;
  - reopening an attempt;
  - forged, unknown and malformed notifications;
  - concurrent and repeated deliveries (one collection, three deliveries, one `paid` event);
  - decline then retry;
  - a late payment (`refund_pending`);
  - on-site collection in parts with validation;
  - a cancelled visit;
  - the test page outside development.
- `paytr_test.go`: the token and notification hashes, the checkout call against a fake host, and verification.
- `testpay_test.go`: signing and the refund answers.
- `refund_test.go` (paytr): the refund token, and success, refusal and unknown answers against a fake host.
- `refunds_db_test.go` (`TestRefundsFlow`):
  - provider partial refunds of a cancelled online booking, a replayed key and a reused key;
  - an over-refund;
  - declined and unknown results with resolve;
  - manual refunds, including an empty till;
  - a cash partial refund after an on-site payment;
  - the follow-up kinds, facets, search and branch scope.
- `followthrough_db_test.go` (`TestRefundFollowThrough`, TEC-978):
  - closing without a refund (reason, history, off the list, back on it after a later failed refund, closing again);
  - the refund notices;
  - the status queries (found, not done, still pending with the backoff, nothing sent again).
- `TestSalesLedger` step 9: the product return and its reversal, once only.
- RT-A01 covers these tenant routes for another business's records:
  - the two original tenant routes;
  - the refund and resolve routes;
  - the follow-up routes;
  - the product return route.
