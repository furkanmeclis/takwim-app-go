# Staff shares (TEC-917)

`/t/{slug}/staff-shares` shows what each staff member earned from their sales in a period, what the business kept, and what can be paid out now. `/t/{slug}/my-shares` ("Paylarım") is the same picture for the signed-in staff member only. Rates are kept on the staff detail page. The module is on when the sector (and plan) includes `staff_shares` (`RequireModule(sector.ModuleStaffShares)`).

## Tables

| Table                 | Holds                                                                                                                                                                                                                                                                 |
| --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `staff_share_rules`   | an agreed rate of a staff member: `item_type` (`service` / `product`), optional `service_id` (a per-service rate), `basis` `percent` (`rate` 0–100) or `fixed` (`fixed_amount` per unit), `valid_from`, `note`. One rule per staff, item type, service and start day. |
| `staff_share_lines`   | the share of one sale line (`share`) or the exact negative of one (`reversal`): the rule, basis, rate / amount and base applied at the time, `share_amount`, `share_date`, branch, staff. Append-only; one share per sale line and one reversal per share.            |
| `staff_share_payouts` | money paid to a staff member (`payout`) or the reversal of one (`reversal`), with its cash movement. Append-only; idempotency key per organization.                                                                                                                   |

All three carry `organization_id` with composite foreign keys (`queries/tenant_tables.txt`). Triggers refuse updates and deletes on lines and payouts (a deleted rule only clears `rule_id`), and check that a share matches its sale line and a reversal is the exact negative of the share it reverses.

## How a share is calculated

The sales module calls a line hook (`sales.Service.SetLineHook`) inside the transaction that writes a sale line, so a sale line without its share cannot exist:

- **Rule.** A service line first looks for the staff member's rate of that service, then for their default service rate; a product line for their default product rate. Within a scope the rule with the latest `valid_from` on or before the sale day wins. No rule means a zero share (the row is still written so the line counts in the totals).
- **Percent** shares are `net_amount × rate`, rounded half up to the kuruş. **Fixed** shares are `fixed_amount × quantity`, never more than the line's net amount and never below zero.
- **Owners earn no share.** A staff record whose account holds the `owner` role of the business is skipped.
- **Corrections keep the rate.** A correction (`POST /v1/tenant/sales/lines/{uuid}/correct`) reverses the old line and writes a new one; the new line's share reuses the rule and rate of the line it replaces, so a later rate change does not reach it.
- **Reversals reverse the share.** Reversing a sale line (refund, return, correction) writes a `reversal` row with the exact negative share on the reversal's day. Shares already written never change; changing or deleting a rule only affects later sales (A17).
- Lines sold before this change, or before a rule existed, are not recalculated.

Example (A15): a 1.000 TL service at 40 % and a 400 TL product at 10 % give 400 + 40 = 440 TL to the staff member and 960 TL to the business. A16: a 1.000 TL package over list prices 800 + 400 splits into 666,67 + 333,33; at 40 % and 30 % (two staff members) the shares are 266,67 + 100,00 = 366,67 and the business keeps 633,33.

## Pending, payable, paid

A share is **payable** when its visit is fully paid: the reservation's ledger collections cover what it was charged. Until then it is **pending**. Counter sales (no reservation) are payable at once. Reversal rows count against the payable amount immediately.

Per staff member, over every branch and day:

- `earned` = all shares, `payable` = shares of collected sales, `paid` = payouts minus reversed payouts,
- `remaining` = earned − paid, `payable_now` = payable − paid.

The period figures (sales, share, business share, pending, paid in the period) follow the branch picked in the header; balances do not, since a staff member is paid once for every branch.

## Payout

`POST /v1/tenant/staff-shares/payouts` posts a `staff_payment` movement on the chosen cash or bank account (`cash.PostTx`) and the payout in one transaction. It locks the staff row, takes the account's currency, refuses more than `payable_now` in that currency (`409`) and refuses a future date. The payout is the expense: no other expense is entered for it (A17). The idempotency key (`staff_payout:{key}`, or the payout uuid when none is sent) makes a double submit post once; the same key with a different body is `409`.

`POST …/payouts/{uuid}/reverse` reverses the payout and its cash movement (`cash.ReverseModuleTx`) with a written reason; the amount returns to `payable_now`.

## API

All routes need the module and `tenant.earnings.read` or `tenant.earnings.own`. With only `.own` every list and summary is limited to the caller's own staff record (`scope.Reach`); a member without a staff record gets `404` on `/me`.

- `GET /v1/tenant/staff-shares/summary?date_from&date_to`: per staff member and currency, per branch, and totals (revenue, share, business share, pending, paid in the period) with each staff member's balance. Default the first of the month to today, at most 366 days.
- `GET /v1/tenant/staff-shares/me?date_from&date_to`: the caller's today and period rows.
- `GET /v1/tenant/staff-shares/rules` (`?staff&item_type`), `GET …/rules/{uuid}`. `POST`, `PATCH`, `DELETE …/rules[/{uuid}]` need `tenant.earnings.read` + `tenant.earnings.manage` and access to every branch (rates are business-wide). A second rule for the same slot and day is `409`.
- `GET /v1/tenant/staff-shares/lines` (+ `/meta`): `staff`, `branch`, `kind`, `item_type`, `status`, `date_from`, `date_to`, `q`; sort `share_date`, `share_amount`, `base_amount`, `created_at`.
- `GET /v1/tenant/staff-shares/payouts` (+ `/meta`, `/{uuid}`): `staff`, `branch`, `kind`, `date_from`, `date_to`, `q`. `POST …/payouts` and `POST …/payouts/{uuid}/reverse` need `tenant.earnings.read` + `tenant.earnings.pay`.

## Roles

| Slug                     | Who                                   |
| ------------------------ | ------------------------------------- |
| `tenant.earnings.read`   | owner, admin, `sube_yoneticisi`       |
| `tenant.earnings.own`    | `calisan`                             |
| `tenant.earnings.manage` | owner, admin (new in 000061)          |
| `tenant.earnings.pay`    | owner, admin, `sube_yoneticisi` (new) |

## Activity

`staff_share_rules.created` / `.updated` (before and after) / `.deleted` and `staff_share_payouts.created` / `.reversed` on the resources `tenant.staff_share_rules` and `tenant.staff_share_payouts`. Rule and payout lists are live resources (`tenant.earnings.read`).

## Panel

- **Çalışan payları**: period picker, totals, tabs for the per-staff summary (with "Pay öde" on rows with a payable balance) and per-branch summary, the calculated shares and the payouts (with reversal).
- **Staff detail → Pay oranları**: the staff member's rules; add, edit and delete with `tenant.earnings.manage`.
- **Paylarım**: today, the period, payable now and paid, then the member's own shares and payouts.

Multi-session packages (TEC-920, [session-packages.md](./session-packages.md)): a service paid by a package session is a sale line at the session value (5 sessions for 2.500 TL → 500 TL), so its share base is the session value. The line has no ledger debit of its own (the package charge covers it), so the visit is fully paid and the share is payable.

## Not yet

- Service bundles (TEC-895): the bundles module will split a bundle price over its services with `usecase.AllocateBase` (half-up rounding, remainder on the last part) before shares are calculated.
- Per-product rates (only a default product rate today) and an export of shares and payouts.

## Tests

`backend/internal/modules/staffshares/staffshares_db_test.go` covers A15, A16, A17 (rate change, correction, return reversal), the owner exemption, pending / payable, the payout cap, idempotency, payout reversal, append-only triggers, the own view and organization isolation. `httpserver/isolation_test.go` probes every route with another organization's records.
