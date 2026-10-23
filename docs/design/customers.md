# Customers (TEC-896)

`/t/{slug}/customers` is the business's own customer list: contact details, notes, tags and message consent. The module is visible when the sector (and plan) includes `customers` (`RequireModule(sector.ModuleCustomers)`). Customers are not staff: they never get a row in `users`. The same API serves the panel, the booking page and the future mobile app.

## Two layers

A person may be a customer of several businesses on the platform. Takwim keeps two things apart:

| Table                     | Level    | Holds                                                                                                                                                                                                                                   |
| ------------------------- | -------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `customer_identities`     | platform | one row per **verified** E.164 phone, with an internal permanent uuid and the last verification time. No business reads it.                                                                                                             |
| `customers`               | business | first / last name, `phone_e164` (nullable), email, birth date, notes, `tags[]`, `sms_consent` / `email_consent` with `consent_updated_at`, `source` (`panel`, `import`, `online`), soft delete. One live record per phone per business. |
| `customer_identity_links` | business | the link between a local record and an identity: `source` (`otp`, `panel_login`), `verified_phone_e164`, `verified_at`. One identity has at most one record per business, and one record at most one identity.                          |

`customers` and `customer_identity_links` carry `organization_id` with composite foreign keys (query guard: `queries/tenant_tables.txt`). `customer_identities` has no organization; only the link use case touches it.

## Rules

- **Phones are E.164.** `pkg/phone` (libphonenumber) normalizes every input; a number without a country code is read as Turkish. Lists show `phone_display` (national format for Turkish numbers). `q` matches a phone fragment with or without the leading `0`.
- **No reach across businesses (A23).** Every query filters by the caller's organization; another business's uuid answers `404`, its records never appear in a list, a search, an export or a bulk target. The shared identity is never in a tenant response; the API only says whether the record is `verification`-linked and how.
- **Local data stays local (A26).** Notes, tags, consent and names belong to the business that wrote them. Two businesses linked to the same person still see only their own record.
- **Unverified matches open nothing (A30).** A phone typed in the panel or imported from a file never creates a link, even when it equals a verified identity's phone. `LinkVerified` (used by the OTP and customer panel flows, TEC-909 / TEC-910) is the only way in: it needs the very phone on file and stores how and when it was proven. `LinkedCustomer(org, phone)` returns only that business's linked record. A record already linked to another identity stays with it (TEC-910), and a record older than the phone's last change of hands opens only later visits ([customer-panel.md](./customer-panel.md#access-security-tec-910)).
- **A changed phone drops the link.** Editing the phone of a verified record removes its link; the person has to verify the new number.
- **Consent**: changing `sms_consent` or `email_consent` stamps `consent_updated_at` (İYS trail). Booking reminders do not depend on it.
- **Tags**: trimmed, deduplicated case-insensitively (first spelling kept), at most 20 of up to 32 characters. The tag filter is exact.
- **Delete** is soft; the phone becomes free for a new record and the old link is not reused.

## API

- `GET /v1/tenant/customers` (`tenant.customers.read`): `q` (full name, email, phone fragment), faceted `tag`, `source`, `verified`, `sms_consent`, `email_consent`; sort `name`, `phone`, `email`, `source`, `created_at`, `updated_at` (default first name, last name). `meta`, `facets`, `{uuid}`, `{uuid}/activity`.
- `POST` / `PATCH` / `DELETE /v1/tenant/customers[/{uuid}]` (`tenant.customers.write`). `phone` and `birth_date` take `null` to clear. A duplicate phone is `409` with detail `phone` / `taken`.
- Export (`tenant.customers.export`) and import (`tenant.customers.import`): a row whose phone matches a live record updates it, otherwise a record with source `import` is created; imported phones stay unverified.
- Bulk (`/bulk`): `add_tag` / `remove_tag` (`tenant.customers.bulk.tag`, the tag in `target.params.tag`) and `delete` (`tenant.customers.bulk.delete`, restored on rollback).
- Search spec `customers` (`tenant.customers.read`): name, email and phone.

## Roles

Owner and admin hold every slug. `sube_yoneticisi` reads, writes, exports and tags; `resepsiyon` reads and writes (reception books customers). Customers are business-level: a branch-limited member sees every customer of the business, since one person visits several branches.

## Activity

`customers.created|updated|deleted` at business level, with field diffs (`verified` shows a dropped link).

## Later

In-business history (bookings, payments, no-shows) joins the detail page with reservations. The customer panel sign-in ([customer-panel.md](./customer-panel.md)) calls `LinkVerified` with source `panel_login`; KVKK export / erasure requests of a person come with their own issue.
