# Income and expenses (TEC-915)

Money in or out outside sales: rent, bills, supplies, a training fee received. Each record has a category, a branch (or none for business-wide records), a cash account, an amount, a date, an optional staff member, a description and an optional document. Sale lines are not entered here: they appear in the same list as income automatically.

Module `backend/internal/modules/finance`, migration `000066_finance_entries`, screen `/t/{slug}/finance`.

## Categories

`finance_categories` (`kind` income / expense, `name` unique per kind case-insensitively, `status` active / archived, `sort_order`). Every business gets 10 expense and 2 income defaults with a `code` and `is_system = true` (rent, utilities, supplies, product purchases, maintenance, marketing, staff costs excluding share payouts, taxes, bank fees, other / rental income, other income). The defaults are seeded by `finance_seed_default_categories(org)`: once for the existing businesses in the migration, then by an `AFTER INSERT` trigger on `organizations`, so every path that creates a business gets them.

A default category or one with records cannot be deleted (`409 in_use`); it is archived instead and no longer offered for new records. The kind never changes. Category writes need every branch (business-wide setting).

## Records

`finance_entries` is append-only. `POST /v1/tenant/finance/entries` posts the cash movement (`income` or `expense` kind, source `finance_entry`) through `cash.PostTx` and writes the record in one transaction, so the day-closing lock, the till floor and archived accounts apply as in the cash book. The account must be in the caller's scope and belong to the record's branch or be business-wide. Without `branch_uuid` the record takes the account's branch; a business-wide account then makes a business-wide record, which needs every branch. The date defaults to today and cannot be in the future. `idempotency_key` makes a double submit write once (the same key with another body is `409`).

Only the classification of a live record changes in place (`PATCH`: category of the same kind, description, staff member), with the change in the activity log. Money never changes in place:

- **Reverse** (`POST …/{uuid}/reverse`, reason): a `reversal` row, the exact negative of the record, dated today, plus the reversal of its cash movement. A record is reversed once; a reversal is not reversed.
- **Correct** (`POST …/{uuid}/correct`, reason and the changed values): the reversal and a replacement (`replaces_entry_id`) in one transaction. Missing values are copied, the date too, and the document moves to the replacement.

Triggers enforce this in the database: only the classification columns, the document columns and `updated_at` are mutable; a reversal must mirror its record; a replacement must name a reversed record of the same kind; rows are deleted only with their business.

## Documents

One document per record (`PUT …/{uuid}/document`, multipart `file`): PDF, JPEG, PNG or WebP sniffed from the content, at most 10 MiB, stored under `organizations/{org}/finance/{entry}/{document}.{ext}`. The record carries `document.url`, an API path; `GET …/{uuid}/document` streams it (`tenant.document.download`) after checking that the key is under the organization's prefix. Replacing or removing the document only changes the record; the object is kept for the audit trail.

## List, filters and export

`GET /v1/tenant/finance/entries` is the union of the business's records (`source=manual`) and the sale lines (`source=sale`, kind income, category `sale_service` / `sale_product`). Filters: `source`, `kind`, `category`, `branch` (with `head_office`), `staff`, `account`, `date_from`, `date_to`, `q`. `GET …/facets` gives categories, branches and staff in use. `POST …/export` is the io-engine export of the same list (CSV / XLSX / PDF, `tenant.report.export`); there is no import.

## Summary

`GET /v1/tenant/finance/summary?date_from&date_to&branch` per currency, kept apart (A20): `revenue` (net sale lines), `income` and `expenses` from these records (reversals netted) and `staff_payments` from the cash book. `net = revenue + income - expenses`. Staff payouts are not an expense (A17), and transfers and product costs are outside it. Plus the total per category. The sales summary (`/v1/tenant/sales/summary`) carries `income` and `expenses` too.

## Permissions

No new slug. Reading needs `tenant.finance.read`, writing `tenant.finance.write`, the export also `tenant.report.export`, the document stream also `tenant.document.download`. Rows outside the branch scope read as `404`.

## Not yet

No VAT and no e-invoice (outside the beta requirements). There is one document per record, there is no bulk action, and records are not in the Cmd+K search.

## Tests

`TestFinanceOverHTTP` covers these cases:

- Seeded categories and category CRUD.
- Cash posting and balances, idempotency, validation, branch scope.
- Classification with activity, the append-only SQL.
- Documents, reversal and correction.
- The list with a sale line and every filter, facets.
- The summary apart from staff payouts, the sales summary, another business.

RT-A01 covers the finance routes, filters and bodies naming another business's records.
