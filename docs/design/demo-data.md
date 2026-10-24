# Demo data (TEC-928)

Status: **implemented**. One demo business per sector with two branches, staff, services, packages, products, resources, past and upcoming visits, and ready-to-use accounts for every panel role. For local work, staging and the pilot acceptance (M7); it refuses `APP_ENV=production` unless `-allow-production` is given.

| Area    | Place                                                                                                 |
| ------- | ----------------------------------------------------------------------------------------------------- |
| Command | `backend/cmd/demo-seed`, `make demo-seed` (root and `backend/`)                                       |
| Seeder  | `backend/internal/demo` (`catalog.go` the data, `business.go` the set-up, `visits.go` the visits)     |
| Test    | `backend/internal/demo/demo_db_test.go` (runs the whole seed on the test database, checks the limits) |

```sh
make demo-seed                                        # generated password, printed once at the end
DEMO_PASSWORD='Secret1x' make demo-seed               # your password (upper, lower, digit, 8+)
DEMO_PASSWORD='Secret1x' make demo-seed DEMO_RESET=1  # delete the demo businesses and write them again
make -C backend demo-seed                             # same, from backend/
go run ./cmd/demo-seed -only kuafor,klinik -history-days 7 -future-days 5
```

## How it writes

- **Only through the use cases**: organizations, branches, org settings, staff and shifts, resources, services with offers and staff, bundles, session packages, products and stock receipts, staff share rules, the class timetable, customers, reservations, payments, finance entries. No raw SQL writes, so the data passes the same validation, capacity checks and ledger triggers as panel input, and the seed breaks loudly (and its test fails) when a module changes a rule.
- **Visits like the panel books them**: for each day and branch the slot engine offers the free starts of the chosen services; one is picked and `reservations.Create` books it under the capacity lock (panel source, so active at once). Past days run with the use cases' clocks moved to that day: booked in the morning, started, completed and collected (cash into the branch till or card into the bank account) in the evening, a few cancelled or no-show. Completing writes the sale lines and staff shares through the same hooks as the API, so revenue, collections, cash, staff earnings and reports have about two weeks of history. Upcoming days (today on) are booked from the real time and left open.
- **No messages**: customer notifications and confirmation tasks are not wired into the seeder, so demo customers get no SMS, WhatsApp, call or e-mail.
- **Deterministic**: the choices come from a fixed random seed per business, so two runs on the same day write the same kind of data.
- **Search**: the seeder does not feed Meilisearch; run `make search-reindex` afterwards when search is on.

## Businesses

| Slug             | Sector          | Branches                  | What it shows                                                                                                                                                                                      |
| ---------------- | --------------- | ------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `demo-kuafor`    | Kuaför          | Kadıköy, Beşiktaş         | chairs, five services with per-service staff, a bundle (kesim + fön), products sold with visits and over the counter, staff shares (40 % services, 10 % products), Konfirmasyon add-on             |
| `demo-restoran`  | Restoran        | Alsancak, Karşıyaka       | template service "Masa rezervasyonu" in both branches, tables by area (Salon, Bahçe, Teras), bookings with party size and area; no staff time                                                      |
| `demo-spor`      | Spor salonu     | Çankaya, Bilkent          | group classes on a weekly timetable (fixed trainer and studio, seats), spinning, personal training, a 10-class package sold to two members and used on their bookings, protein bar and water stock |
| `demo-otoyikama` | Oto yıkama      | Nilüfer, Osmangazi        | wash bays, three wash services, vehicle type and plate on every booking, products with visits                                                                                                      |
| `demo-klinik`    | Klinik          | Ataşehir, Şişli           | rooms, appointment types with specialists, a 10-session treatment package, staff approval for online requests, Konfirmasyon add-on. No health record fields exist (requirements §5)                |
| `demo-guzellik`  | Güzellik salonu | Nişantaşı, Bağdat Caddesi | rooms and laser devices, a bundle (bakım günü), a 6-session laser package, products, staff shares                                                                                                  |

Each business also gets: plan `growth`, published online booking rules (auto approval except the clinic, 60 min lead, 60 days ahead, 2 h cancel / change cutoff, web channel on), a till per branch and a business-wide bank account, 14 customers (`0555 1BB NNNN`), a few income / expense records (rent, bills, supplies) and staff working the branch hours (a person of two branches works the days in turn).

## Accounts

Every account has the same password (`DEMO_PASSWORD` or the generated one); a rerun sets the new password on the existing accounts. Addresses use `example.com` (reserved, RFC 2606). Sign in at `/login`.

| Account                             | Role                                                                                                  |
| ----------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `demo.platform@example.com`         | Takwim yönetimi (`super_admin`): `/platform`, no business membership                                  |
| `demo.<key>.sahip@example.com`      | İşletme sahibi (owner), every branch                                                                  |
| `demo.<key>.yonetici@example.com`   | Şube yöneticisi, first branch only                                                                    |
| `demo.<key>.resepsiyon@example.com` | Resepsiyon, first branch only (it is the author of the demo visits)                                   |
| `demo.<key>.calisan@example.com`    | Çalışan, linked to a staff record (own calendar, own earnings)                                        |
| `demo.coklu@example.com`            | Resepsiyon in `demo-kuafor` (Beşiktaş only) and Çalışan in `demo-guzellik` (linked to a staff record) |

`<key>` is `kuafor`, `restoran`, `spor`, `otoyikama`, `klinik` or `guzellik`.

## Scenarios

- **Shared customer panel**: Ayşe Demir (`0555 000 00 01`) is a customer of the hairdresser and the clinic, with past and upcoming visits in both. Sign in at `/panel` with that phone; outside production the sign-in code is written to the API log (the SMS provider is not chosen yet). The panel lists both businesses' visits together; each business still sees only its own customer record.
- **Business boundary**: an owner of one demo business has no access to another (`403 NO_TENANT_MEMBERSHIP`); the platform account manages every business under `/platform` but has no tenant access without a membership.
- **One person, two businesses**: `demo.coklu` holds different roles and branch scopes in the two businesses; the permissions follow the business in the `X-Organization` header.
- **Branch boundary**: the branch manager and the reception see only the first branch (the second branch's visits answer 404); the owner sees every branch.
- **Staff own scope**: the staff account sees its own calendar and earnings, not the other staff members'.

The test checks the businesses, the accounts, the shared customer, revenue and collections, and the boundaries above.

## Reset

`DEMO_RESET=1` (`-reset`) deletes the existing demo businesses with the platform's delete (soft delete; the slug is free again) and writes them anew. Without it the command stops when a demo business exists. The accounts are kept and get the password of the run.

## Extending

Add or change a sector's data in `catalog.go`; a new module gets its set-up step in `business.go` through its use case. Keep strings Turkish (they are business data) and phones in the demo range.
