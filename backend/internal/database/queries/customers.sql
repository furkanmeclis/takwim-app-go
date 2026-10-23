-- Tenant resource: customers (TEC-896). Every query on customers and
-- customer_identity_links filters by organization_id. customer_identities is
-- platform-level: only the link use case touches it, and no tenant response
-- carries its uuid.

-- name: CreateCustomer :one
INSERT INTO customers (
    organization_id, first_name, last_name, phone_e164, email, birth_date, notes, tags,
    sms_consent, email_consent, consent_updated_at, source
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(first_name), sqlc.arg(last_name), sqlc.narg(phone_e164), sqlc.arg(email),
    sqlc.narg(birth_date), sqlc.arg(notes), sqlc.arg(tags), sqlc.arg(sms_consent), sqlc.arg(email_consent),
    sqlc.narg(consent_updated_at), sqlc.arg(source)
)
RETURNING *;

-- name: UpdateCustomer :one
UPDATE customers
SET first_name = sqlc.arg(first_name), last_name = sqlc.arg(last_name), phone_e164 = sqlc.narg(phone_e164),
    email = sqlc.arg(email), birth_date = sqlc.narg(birth_date), notes = sqlc.arg(notes), tags = sqlc.arg(tags),
    sms_consent = sqlc.arg(sms_consent), email_consent = sqlc.arg(email_consent),
    consent_updated_at = sqlc.narg(consent_updated_at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: SetCustomerTags :exec
UPDATE customers SET tags = sqlc.arg(tags)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: SoftDeleteCustomer :execrows
UPDATE customers SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: RestoreCustomer :execrows
-- Bulk delete rollback.
UPDATE customers SET deleted_at = NULL
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND deleted_at IS NOT NULL;

-- name: GetCustomer :one
SELECT c.*, l.source AS link_source, l.verified_at AS link_verified_at
FROM customers c
LEFT JOIN customer_identity_links l ON l.organization_id = c.organization_id AND l.customer_id = c.id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid)
  AND c.deleted_at IS NULL;

-- name: ListCustomers :many
SELECT c.*, l.source AS link_source, l.verified_at AS link_verified_at
FROM customers c
LEFT JOIN customer_identity_links l ON l.organization_id = c.organization_id AND l.customer_id = c.id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.deleted_at IS NULL
  AND (sqlc.narg(tags)::text[] IS NULL OR c.tags && sqlc.narg(tags)::text[])
  AND (sqlc.narg(sources)::text[] IS NULL OR c.source = ANY(sqlc.narg(sources)::text[]))
  AND (sqlc.narg(sms_consent)::boolean IS NULL OR c.sms_consent = sqlc.narg(sms_consent))
  AND (sqlc.narg(email_consent)::boolean IS NULL OR c.email_consent = sqlc.narg(email_consent))
  AND (sqlc.narg(verified)::boolean IS NULL OR (l.id IS NOT NULL) = sqlc.narg(verified))
  AND (
    sqlc.narg(q)::text IS NULL
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
    OR c.email ILIKE '%' || sqlc.narg(q) || '%'
    OR (sqlc.narg(q_digits)::text IS NOT NULL AND c.phone_e164 LIKE '%' || sqlc.narg(q_digits) || '%')
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN lower(c.first_name || ' ' || c.last_name) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN lower(c.first_name || ' ' || c.last_name) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'phone' AND NOT sqlc.arg(sort_desc)::boolean THEN c.phone_e164 END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'phone' AND sqlc.arg(sort_desc)::boolean THEN c.phone_e164 END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'email' AND NOT sqlc.arg(sort_desc)::boolean THEN lower(c.email) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'email' AND sqlc.arg(sort_desc)::boolean THEN lower(c.email) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'source' AND NOT sqlc.arg(sort_desc)::boolean THEN c.source END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'source' AND sqlc.arg(sort_desc)::boolean THEN c.source END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN c.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN c.created_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN c.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN c.updated_at END DESC,
  c.first_name ASC, c.last_name ASC, c.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCustomers :one
SELECT COUNT(*)::bigint
FROM customers c
LEFT JOIN customer_identity_links l ON l.organization_id = c.organization_id AND l.customer_id = c.id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.deleted_at IS NULL
  AND (sqlc.narg(tags)::text[] IS NULL OR c.tags && sqlc.narg(tags)::text[])
  AND (sqlc.narg(sources)::text[] IS NULL OR c.source = ANY(sqlc.narg(sources)::text[]))
  AND (sqlc.narg(sms_consent)::boolean IS NULL OR c.sms_consent = sqlc.narg(sms_consent))
  AND (sqlc.narg(email_consent)::boolean IS NULL OR c.email_consent = sqlc.narg(email_consent))
  AND (sqlc.narg(verified)::boolean IS NULL OR (l.id IS NOT NULL) = sqlc.narg(verified))
  AND (
    sqlc.narg(q)::text IS NULL
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
    OR c.email ILIKE '%' || sqlc.narg(q) || '%'
    OR (sqlc.narg(q_digits)::text IS NOT NULL AND c.phone_e164 LIKE '%' || sqlc.narg(q_digits) || '%')
  );

-- name: ListCustomerUUIDs :many
SELECT c.uuid
FROM customers c
LEFT JOIN customer_identity_links l ON l.organization_id = c.organization_id AND l.customer_id = c.id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.deleted_at IS NULL
  AND (sqlc.narg(tags)::text[] IS NULL OR c.tags && sqlc.narg(tags)::text[])
  AND (sqlc.narg(sources)::text[] IS NULL OR c.source = ANY(sqlc.narg(sources)::text[]))
  AND (sqlc.narg(sms_consent)::boolean IS NULL OR c.sms_consent = sqlc.narg(sms_consent))
  AND (sqlc.narg(email_consent)::boolean IS NULL OR c.email_consent = sqlc.narg(email_consent))
  AND (sqlc.narg(verified)::boolean IS NULL OR (l.id IS NOT NULL) = sqlc.narg(verified))
  AND (
    sqlc.narg(q)::text IS NULL
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
    OR c.email ILIKE '%' || sqlc.narg(q) || '%'
    OR (sqlc.narg(q_digits)::text IS NOT NULL AND c.phone_e164 LIKE '%' || sqlc.narg(q_digits) || '%')
  )
ORDER BY c.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListCustomersForExport :many
SELECT c.*, l.source AS link_source, l.verified_at AS link_verified_at
FROM customers c
LEFT JOIN customer_identity_links l ON l.organization_id = c.organization_id AND l.customer_id = c.id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.deleted_at IS NULL
  AND (sqlc.narg(tags)::text[] IS NULL OR c.tags && sqlc.narg(tags)::text[])
  AND (sqlc.narg(sources)::text[] IS NULL OR c.source = ANY(sqlc.narg(sources)::text[]))
  AND (sqlc.narg(sms_consent)::boolean IS NULL OR c.sms_consent = sqlc.narg(sms_consent))
  AND (sqlc.narg(email_consent)::boolean IS NULL OR c.email_consent = sqlc.narg(email_consent))
  AND (sqlc.narg(verified)::boolean IS NULL OR (l.id IS NOT NULL) = sqlc.narg(verified))
  AND (
    sqlc.narg(q)::text IS NULL
    OR (c.first_name || ' ' || c.last_name) ILIKE '%' || sqlc.narg(q) || '%'
    OR c.email ILIKE '%' || sqlc.narg(q) || '%'
    OR (sqlc.narg(q_digits)::text IS NOT NULL AND c.phone_e164 LIKE '%' || sqlc.narg(q_digits) || '%')
  )
ORDER BY c.first_name ASC, c.last_name ASC, c.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListCustomerTagFacets :many
SELECT t.tag::text AS value, COUNT(*)::bigint AS count
FROM customers c, unnest(c.tags) AS t(tag)
WHERE c.organization_id = sqlc.arg(organization_id) AND c.deleted_at IS NULL
GROUP BY t.tag
ORDER BY t.tag ASC;

-- name: ListCustomerSourceFacets :many
SELECT c.source AS value, COUNT(*)::bigint AS count
FROM customers c
WHERE c.organization_id = sqlc.arg(organization_id) AND c.deleted_at IS NULL
GROUP BY c.source
ORDER BY c.source ASC;

-- name: CustomerPhoneTaken :one
SELECT EXISTS (
    SELECT 1 FROM customers
    WHERE organization_id = sqlc.arg(organization_id)
      AND phone_e164 = sqlc.arg(phone_e164)
      AND deleted_at IS NULL
      AND (sqlc.narg(exclude_id)::bigint IS NULL OR id <> sqlc.narg(exclude_id))
)::boolean;

-- name: GetCustomerByPhone :one
-- Import key: the live record of the business with the phone.
SELECT id, uuid FROM customers
WHERE organization_id = sqlc.arg(organization_id)
  AND phone_e164 = sqlc.arg(phone_e164)
  AND deleted_at IS NULL;

-- name: UpsertCustomerIdentity :one
-- Platform-level: called only after the phone was proven.
INSERT INTO customer_identities (phone_e164, phone_verified_at)
VALUES (sqlc.arg(phone_e164), sqlc.arg(verified_at))
ON CONFLICT (phone_e164) DO UPDATE SET phone_verified_at = GREATEST(customer_identities.phone_verified_at, EXCLUDED.phone_verified_at)
RETURNING *;

-- name: GetCustomerIdentityByPhone :one
SELECT * FROM customer_identities WHERE phone_e164 = sqlc.arg(phone_e164);

-- name: CreateCustomerIdentityLink :one
INSERT INTO customer_identity_links (organization_id, customer_id, identity_id, source, verified_phone_e164, verified_at)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(customer_id), sqlc.arg(identity_id), sqlc.arg(source),
    sqlc.arg(verified_phone_e164), sqlc.arg(verified_at)
)
RETURNING *;

-- name: GetCustomerIdentityLink :one
SELECT * FROM customer_identity_links
WHERE organization_id = sqlc.arg(organization_id) AND customer_id = sqlc.arg(customer_id);

-- name: DeleteCustomerIdentityLink :execrows
DELETE FROM customer_identity_links
WHERE organization_id = sqlc.arg(organization_id) AND customer_id = sqlc.arg(customer_id);

-- name: GetLinkedCustomer :one
-- The business's own record of an identity (customer panel, online booking).
SELECT c.id, c.uuid
FROM customer_identity_links l
JOIN customers c ON c.organization_id = l.organization_id AND c.id = l.customer_id AND c.deleted_at IS NULL
WHERE l.organization_id = sqlc.arg(organization_id) AND l.identity_id = sqlc.arg(identity_id);

-- name: ListCustomerSearchDocuments :many
SELECT c.id, c.uuid, c.organization_id, c.first_name, c.last_name, c.phone_e164, c.email,
    o.slug AS organization_slug
FROM customers c
JOIN organizations o ON o.id = c.organization_id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.deleted_at IS NULL;

-- name: GetCustomerSearchDocument :one
SELECT c.id, c.uuid, c.organization_id, c.first_name, c.last_name, c.phone_e164, c.email,
    o.slug AS organization_slug
FROM customers c
JOIN organizations o ON o.id = c.organization_id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND c.uuid = sqlc.arg(uuid)
  AND c.deleted_at IS NULL;
