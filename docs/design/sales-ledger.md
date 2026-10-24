# Sale lines and the customer ledger (TEC-916)

Revenue (ciro) is what the business delivered; collection (tahsilat) is the money it received. They are recorded in separate tables and never added together, so an online prepayment counts as a collection on the day it was paid and as revenue on the day of the service, once each (A20). Code: `backend/internal/modules/sales`, migration `000057_sales_ledger`.

## Data

| Table            | Holds                                                                                                                                                                                                                                                                                                                    |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `sale_lines`     | one service delivered (a line of a completed reservation) or product handed over (a stock sale): quantity, unit price, gross / discount / net (`NUMERIC`, per currency), staff, branch, customer, `sale_date`. `kind` `sale` or `reversal` (the exact negative of one sale line). Revenue of a day = sum of `net_amount` |
| `ledger_entries` | what the customer owes and paid: `sale` (debit, mirrors a sale line), `collection` (credit, with its cash movement), `refund` (debit, pays back a collection, with its cash movement), `reversal` (opposite direction of one entry). Balance = debits - credits                                                          |

Both tables are **append-only**. Triggers refuse `UPDATE` and `DELETE` (except clearing references to a deleted customer, staff member or user, and the organization cascade); insert triggers check the rules below, so a bug in the application cannot break them.

- A sale line is identified by its **accrual key** (`reservation:{uuid}:{line}` or `stock_movement:{uuid}`). A key has at most one live (unreversed) sale line, so one service or product is never revenue twice. `accrual_generation` counts the corrections.
- A reversal mirrors its original exactly and exists at most once (`uq_sale_lines_reverses`, `uq_ledger_entries_reverses`).
- A correction is a reversal plus a replacement sale line with the same key (`replaces_line_id`).
- A sale entry is reversed only together with its sale line; a refund points at a collection of the same customer / reservation and never exceeds what is left of it.
- Reversals and corrections are dated the day they are posted, so a closed day keeps its figures.

## Where lines come from

| Event                                         | Line                                                                                                                                                                                                                                       |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| reservation `complete` (reservations hook)    | one service line per reservation line, dated the day the visit ended (at most today) in the branch time zone; one product line per product handed over with the visit (TEC-919), keyed by its stock movement and carrying `reservation_id` |
| stock movement of kind `sale` (products hook) | one product line, dated `occurred_at` in the branch time zone; walk-in sales have no customer                                                                                                                                              |
| `POST /v1/tenant/sales/lines/{uuid}/correct`  | reversal + replacement (new quantity, unit price or discount)                                                                                                                                                                              |
| `POST /v1/tenant/sales/lines/{uuid}/reverse`  | reversal only                                                                                                                                                                                                                              |

Both hooks run inside the transaction of the event, so a completed visit without its revenue (or the reverse) cannot exist. Zero-amount lines are kept but post no ledger entry. Reservations completed before this change are not backfilled.

## Service API for later modules

All `…Tx` calls take the caller's `*db.Queries`, so they commit with the caller's own record.

| Call                                               | For                                                                                                                          |
| -------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `RecordCollectionTx(ctx, q, org, CollectionInput)` | payments (TEC-912): a collection credit + a cash movement into the account; idempotency key replays                          |
| `RecordRefundTx(ctx, q, org, RefundInput)`         | refunds (TEC-913): a refund debit against a collection + a cash movement out of the account                                  |
| `ReverseEntryTx(ctx, q, org, uuid, reason, user)`  | undo a collection or refund (and its cash movement); refused for sale entries and refunded collections                       |
| `ReverseLineTx(ctx, q, org, uuid, reason, user)`   | undo a sale line and its ledger debit (returns, TEC-913)                                                                     |
| `LiveLineOfStockSale(ctx, q, org, movement)`       | the live line of a stock sale                                                                                                |
| `SetLineHook(hook)`                                | staff shares (TEC-917): called in the transaction of every new sale line and reversal ([staff-shares.md](./staff-shares.md)) |

Errors are sentinel values in `usecase` (`ErrAlreadyReversed`, `ErrHasRefunds`, `ErrRefundTooLarge`, `ErrCurrencyMismatch`, `ErrIdempotencyReused`, …) and `ValidationError`.

## API

All routes need the finance module and `tenant.finance.read`; writes also need `tenant.finance.write`. Changing a price needs `tenant.price.override`, changing a discount `tenant.discount.apply`. Rows outside the caller's branch scope are `404`.

- `GET /v1/tenant/sales/lines` (+ `/meta`, `/{uuid}`), `POST …/{uuid}/reverse`, `POST …/{uuid}/correct`
- `GET /v1/tenant/sales/entries` (+ `/meta`, `/{uuid}`), `POST …/{uuid}/reverse`
- `GET /v1/tenant/sales/summary?from&to&branch`: per day and currency: revenue (gross, discount, reversed, net, service / product), collections (collected, refunded, reversed, net), account transfers and staff payouts from the cash book, and the income and expenses recorded under finance ([finance-entries.md](./finance-entries.md), TEC-915), kept apart (A20). At most 366 days, default today.
- `GET /v1/tenant/sales/reservations/{uuid}/balance`: expected (the posted sale lines, or the booked total of an open visit), collected, refunded, paid, remaining, credit, with its lines and entries.
- `GET /v1/tenant/sales/customers/{uuid}/balance`: charged, paid and balance per currency.

## Not yet

No ledger screen and no export (reports TEC-925); the finance list ([finance-entries.md](./finance-entries.md)) shows sale lines as income and exports them with the business's own records. Collections come from the payments module ([payments.md](./payments.md), TEC-912): the verified online notification and the panel's on-site payment both post through `RecordCollectionTx`, and the reservation detail shows the visit's money entries. A hand-recorded stock return does not reverse a sale line. The manager's return flow (`POST /v1/tenant/stock-movements/{uuid}/return`, TEC-913) writes the return movement and calls `ReverseLineTx` on the sale's line in the same transaction. Refunds post through `RecordRefundTx` from the refunds flow ([payments.md](./payments.md#refunds-tec-913)).

## Tests

`TestSalesLedger` (prepayment then completion, re-posting, append-only SQL, correction, refunds, reversal rules, customer balance, A20 summary with transfers and staff payouts, product line, other business and out-of-scope branch, HTTP permissions) and RT-A01 (sale line, ledger entry and summary filters of another business).
