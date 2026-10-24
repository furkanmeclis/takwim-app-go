# Reservation products and stock reservation (TEC-919)

A visit can carry products the customer takes with it (requirements §12, A18). Each product line keeps its price, reserves its units in the branch's stock while the visit is open and becomes a stock sale when the visit is completed. Code: `backend/internal/modules/reservations/usecase/products.go`, migration `000062_reservation_products`.

## Data

`reservation_products` holds one line per product (or variant) of a visit:

- `reservation_id`, `branch_id`, `product_id`, `variant_id`;
- snapshot of `product_name`, `variant_name`, `sku`, `unit_price`, `unit_cost` taken when the line is added;
- `quantity` (1–99 in the API, 1–1000 in the table), `staff_id` (who sells it; default the person of the first service line);
- `status`: `held` → `delivered` (with `stock_movement_uuid`, `delivered_at`) or `released` (with `release_reason`, `released_at`);
- `source` (`panel`, `online`, `embed`), `created_by`.

Composite foreign keys keep every reference inside the organization; the table is in `tenant_tables.txt`. A trigger refuses any change to a delivered or released line. The down migration drops the table and sets `stock_levels.reserved` back to 0 (the reservations it held are lost).

## Stock

Available = `on_hand − reserved` per branch and item (`stock_levels`). Every writer locks the stock rows it touches (`FOR UPDATE`, sorted by product and variant), so two bookings cannot take the last unit twice.

| Event                                          | Stock                                                                                                                                                    |
| ---------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| booking with `products`, or `POST …/products`  | `reserved += quantity` in the same transaction; not enough available units = `400 products[i].quantity / insufficient_stock` and nothing is written      |
| `DELETE …/products/{uuid}`                     | the held line is released (`removed`), `reserved -= quantity`                                                                                            |
| reject, cancel, no-show, expiry of the request | every held line is released (`rejected`, `cancelled`, `no_show`, `expired`)                                                                              |
| complete                                       | every held line is delivered: `reserved -= quantity`, `on_hand -= quantity`, a `sale` stock movement (source `reservation`, the visit's uuid, `#number`) |

Completing fails with `products / insufficient_stock` when a count brought `on_hand` below the reserved units; the count has to be fixed first. The timed hold of products is the visit's own hold (request review or the online payment window); the 10-minute checkout of the public page holds only the time, products are reserved when the booking is made.

## Money

- `total_price` = service lines + held and delivered products. Adding or removing a product recomputes it; a cancelled visit keeps its total for history.
- A paid visit whose total grows goes back to `on_site` (the rest is collected at the visit). A visit waiting for its online payment refuses product changes (`409 payment_status / online_pending`), so the amount sent to the provider cannot change under it.
- On completion the sales hook posts one product sale line per delivered product with the visit's customer, staff and date and `reservation_id` set. The accrual key is the stock movement (`stock_movement:{uuid}`), so the products hook and the reservation hook cannot post the same product twice. The reservation balance counts these lines.

## API

| Route                                                          | Permission                                  |
| -------------------------------------------------------------- | ------------------------------------------- |
| `GET /v1/tenant/reservations/{uuid}/product-options?q`         | reservations read + products module + sell  |
| `GET /v1/tenant/reservations/product-options?branch_uuid&q`    | reservations read + products module + sell  |
| `POST /v1/tenant/reservations/{uuid}/products`                 | reservations write + `tenant.products.sell` |
| `DELETE /v1/tenant/reservations/{uuid}/products/{productUuid}` | reservations write + `tenant.products.sell` |
| `POST /v1/tenant/reservations` with `products`                 | as above                                    |
| `GET /v1/public/businesses/{slug}/products?branch_uuid`        | public                                      |

Changes take `expected_version`, raise `version` and write a `product_added` / `product_removed` event and an activity row. The products module (entitlement) must be on. Public products are active, shown online, sold in the branch and have units left; `max_quantity` is `min(available, 10)` so the exact stock is never shown.

## Panel and public page

- The reservation detail has a **Products** card (EntityTable: product, quantity, unit price, amount, seller, status) with add (combobox of the branch's stock with available units) and remove for held lines while the visit is open.
- The business page shows an optional product step after the time, when the sector flow has `products` and the branch sells products online ([business-page.md](./business-page.md)). The step is minimal: a list with quantities, no images or categories.

- The panel's new reservation form has an optional **Products** section (TEC-977) when the products module is on and the member has `tenant.products.sell`: a combobox of the branch's stock (`GET /v1/tenant/reservations/product-options?branch_uuid`, with the sellable units) and an EntityTable of the picked items with quantity steppers (at most the sellable units, 99). The items go in the booking's `products` and are reserved in its transaction; a refused line (`products[i].…`) is marked in the table. Changing the branch clears them.
- The customer panel shows the visit's products (TEC-977, [customer-panel.md](./customer-panel.md)): held, delivered and released lines; lines the business removed are hidden.

## Tests

`reservations/products_db_test.go` (add, remove, cancel, complete with the sale line and stock movement, version and payment rules, expiry, concurrent last unit, options), `publicbooking/products_db_test.go` (product step, booking with products, insufficient stock, hidden products) and RT-A01 in `isolation_test.go` (another business's product, variant or staff is refused; product routes of another business answer 404).
