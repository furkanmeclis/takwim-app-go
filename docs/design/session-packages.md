# Multi-session packages (TEC-920)

A business sells N sessions of one service for one price ("5 seans lazer 2.500 TL"). The customer pays the package at the sale; each session is then used on a visit (requirements §13, A19). Code: `backend/internal/modules/sessionpackages` (definitions, sold packages, refunds, report) and `backend/internal/modules/reservations/usecase/sessions.go` (sessions on visits), migration `000068_session_packages`. The module is named _session packages_ so it does not clash with service bundles (TEC-895).

## Data

| Table                      | Holds                                                                                                                                                                                                                                                                                                                              |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `session_packages`         | the definition: name, description, service, `total_sessions` (1–1000), `price`, `validity_days` (null = no end), `all_branches`, `is_active`; soft delete; name unique among live rows                                                                                                                                             |
| `session_package_branches` | the branches a definition is usable in (when not `all_branches`)                                                                                                                                                                                                                                                                   |
| `customer_packages`        | a package sold to a customer: snapshot of name, service, sessions, price, currency, `branch_ids` (null = every branch), the selling branch, `sold_on`, `expires_on` = `sold_on + validity_days`, staff, note, the ledger entry ids (collection, package charge), `idempotency_key`, `status` `active` / `refunded` with the refund |
| `package_sessions`         | one session of a package given to a visit line: `session_no`, `amount` (the session value), `status` `held` → `consumed` (with `consumed_on`) or `released` (with `release_reason`)                                                                                                                                                |

Every reference is a composite foreign key with `organization_id`; the four tables are in `tenant_tables.txt`. Triggers keep the sold terms immutable, allow a new session only on an active package inside its count and refuse any change to a consumed or released session. The down migration drops the tables and removes the `package` ledger entries (with their reversals) before it restores the ledger checks: package history is lost.

### One session, one visit

A session is held when it is applied to a line, consumed when the visit is completed and released otherwise. Two unique partial indexes (where `status <> 'released'`) give a package's session number and a visit line at most one live session, and the apply locks the package row (`FOR UPDATE`) first, so the last session is never held by two visits. The next free `session_no` is used.

### Session value

`session value = floor(price / total)` in minor units, the last session takes the remainder: 2.500 TL / 5 = 500 TL each; 1.000 TL / 3 = 333,33 + 333,33 + 333,34. It is the share base of the staff member who does the service.

## Money

| Event                      | Cash book                                        | Customer ledger                                                                                     | Revenue (`sale_lines`)                                           |
| -------------------------- | ------------------------------------------------ | --------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| sale                       | money in (collection)                            | `collection` credit + `package` debit (the price): the customer owes nothing                        | nothing                                                          |
| session applied to a visit | –                                                | –                                                                                                   | the line's price is the session value; it leaves the visit total |
| visit completed            | –                                                | no `sale` debit for the line (the package debit already covers it)                                  | a service line at the session value                              |
| refund of `amount`         | money out (refund against the sale's collection) | `refund` debit; the `package` debit is reversed and the kept part (price − amount) is charged again | nothing                                                          |

So the package payment is a cash entry, operational revenue comes only from the sessions used, and the package sales total is reported apart (`summary.sold`), never added to revenue (A19, A20). A free package (price 0) posts no money.

The visit's `total_price` counts only the lines without a live session (plus products), so the visit expected amount and its payment status ignore lines paid by a package. `ListSalesReservationLines` returns the consumed session's amount, `RecordReservationTx` uses it as the line price and `postSaleEntry` skips the debit of such a line.

## Visit transitions

| Transition                         | Sessions                                                                                                                            |
| ---------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| reject, cancel, no-show, expiry    | every held session is released (`rejected`, `cancelled`, `no_show`, `expired`)                                                      |
| complete                           | every held session is consumed (`consumed_on` = the visit day in the branch time zone), before the sales hook                       |
| reschedule                         | a session stays when its line keeps the service and the new day is not after `expires_on`; otherwise it is released (`rescheduled`) |
| `DELETE …/package-sessions/{uuid}` | the held session is released (`removed`); the line is charged at its price again                                                    |

A no-show releases the session: the customer keeps the right; a no-show fee is a separate decision of the business.

## API

| Route                                                                            | Permission                                                                    |
| -------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| `GET /v1/tenant/session-packages` (+ `/meta`, `/{uuid}`)                         | `tenant.session_packages.read`                                                |
| `POST`, `PATCH`, `DELETE /v1/tenant/session-packages[/{uuid}]`                   | `.read` + `.write`, every branch                                              |
| `GET /v1/tenant/customer-packages` (+ `/meta`, `/facets`, `/summary`, `/{uuid}`) | `tenant.session_packages.read`, branch scope of the selling branch            |
| `POST /v1/tenant/customer-packages`                                              | cash module + `.read` + `.sell` + `tenant.payment.collect`; `idempotency_key` |
| `POST /v1/tenant/customer-packages/{uuid}/refund`                                | cash module + `.read` + `tenant.payment.refund`                               |
| `GET /v1/tenant/reservations/{uuid}/package-options`                             | reservation read or own                                                       |
| `POST /v1/tenant/reservations/{uuid}/package-sessions`                           | reservation write or own                                                      |
| `DELETE /v1/tenant/reservations/{uuid}/package-sessions/{sessionUuid}`           | reservation write or own                                                      |

Every route needs the `packages` module. Using a session is front-desk work, so the reservation slugs are enough there. A package is usable on a visit when it is not refunded, the visit day is not after `expires_on`, the visit's branch is allowed and the line's service is the package's service; the validation codes are `customer_package_uuid` / `not_found`, `refunded`, `expired`, `branch_not_allowed`, `no_sessions_left` and `line_index` / `not_found`, `service_mismatch`, `already_on_package`.

The state of a sold package is computed: `refunded`, else `used_up` (every session consumed), else `expired` (past `expires_on`), else `active`. A refund is one closing step: held sessions must be released first (`409 sessions / held`), `amount` is 0 to the price, the reason is at least 3 characters.

`GET …/customer-packages/summary?date_from&date_to&branch` (default this month): per currency `sold` (packages and money taken), `used` (sessions consumed and their value), `refunded`, and today's `open` (unused rights of valid packages), `held`, `expired` (rights left on expired packages) and `used_up`.

## Reports (TEC-983)

`GET /v1/tenant/reports/summary` carries a package sales section in every money entry (totals, days, branches): `packages.sold` (packages, sessions, price by `sold_on`), `packages.used` (sessions consumed and their value by `consumed_on`), `packages.refunded` (packages, sessions the refund closed and money paid back by `refunded_on`) and `packages.expired` (sessions and value left on packages whose `expires_on` is in the range and already past, by `expires_on`). The totals and branch rows also have `package_balance`: today's valid packages, their free sessions and value, and the sessions held by visits (not bound to the range, like the stock).

- The package price is never added to revenue or the operating result; the value of the sessions used is already service revenue through the sale lines, and the price stays in collections.
- Sales, refunds and expiries count on the selling branch; a session counts on the visit's branch. There is no head office row for packages.
- A staff filter leaves the section out (`omitted_for_staff` lists `packages`): a package is sold to a customer, not by the staff member who serves its sessions.
- The CSV has the `package_*` columns in both sections; the branch section also has today's open rights.
- The panel's **Raporlar** page shows the package cards and a "Paket satışları" tab (by day and by branch) when the business has the packages module.

Queries: `ReportPackageSales`, `ReportPackageUsage`, `ReportPackageRefunds`, `ReportPackageExpiries`, `ReportPackageBalance` in `queries/reports.sql`; no migration.

## Permissions (migration 000068)

| Slug                            | Granted to                                    |
| ------------------------------- | --------------------------------------------- |
| `tenant.session_packages.read`  | owner, admin, `sube_yoneticisi`, `resepsiyon` |
| `tenant.session_packages.write` | owner, admin                                  |
| `tenant.session_packages.sell`  | owner, admin, `sube_yoneticisi`, `resepsiyon` |

## Activity and live lists

`session_packages.created` / `.updated` / `.deleted`, `customer_packages.sold` / `.refunded`, `reservations.package_applied` / `.package_released`. The live resources `tenant.session_packages` and `tenant.customer_packages` refresh the lists.

## Panel

- **Paketler**: period picker, report cards (package sales as a cash figure, sessions used, unused rights, refunds and expired rights), the sold packages list and the definitions (create, edit, delete); "Paket sat" sells a package.
- **Paket detayı**: balance (used, held, left, session value), terms, sessions with their visits, refund.
- **Müşteri detayı → Paketler**: the customer's packages and "Paket sat".
- **Randevu detayı → Paket seansları**: the visit's sessions, "Paketten kullan" (package and line) and "Paketten çıkar" for held sessions.

## Not yet

Follow-up TEC-984: using a session from the booking form and the public booking page, the customer panel's "Paketlerim", and the export / bulk / search engines for packages.

## Tests

`backend/internal/modules/sessionpackages/sessionpackages_db_test.go`: session value rounding, definitions, sale and idempotency, customer balance, apply, cancel, no-show, reschedule, remove, completion (sale line 500, no visit debit, staff share on 500), concurrent last session, expiry, report figures, refund, immutability triggers and another organization. RT-A01 in `isolation_test.go` covers every new route.
