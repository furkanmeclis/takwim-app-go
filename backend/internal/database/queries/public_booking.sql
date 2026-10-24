-- Public booking API (TEC-906): the business page, the widget and the
-- mobile app read the catalog of one business, resolved from its slug.
-- Every query below filters by the organization id of that business.

-- name: GetPublicOrganizationBySlug :one
SELECT id, uuid, slug, name, logo_object_key, cover_object_key, status, access_starts_at, access_ends_at,
       sector, timezone, currency, plan, addons, branch_limit, staff_limit
FROM organizations
WHERE slug = sqlc.arg(slug) AND deleted_at IS NULL;

-- name: ListPublicBranches :many
-- Active branches shown on the customer screens.
SELECT b.id, b.uuid, b.slug, b.name, b.description, b.address, b.city, b.district,
       b.latitude, b.longitude, b.phone, b.email, b.working_hours, b.holiday_exceptions,
       COALESCE(NULLIF(b.timezone, ''), o.timezone)::TEXT AS timezone
FROM branches b
JOIN organizations o ON o.id = b.organization_id
WHERE b.organization_id = sqlc.arg(organization_id)
  AND b.deleted_at IS NULL AND b.is_active AND b.show_online
ORDER BY b.name, b.id;

-- name: ListPublicBranchImages :many
SELECT i.uuid, i.branch_id, i.sort_order
FROM branch_images i
WHERE i.organization_id = sqlc.arg(organization_id) AND i.branch_id = ANY(sqlc.arg(branch_ids)::BIGINT[])
ORDER BY i.branch_id, i.sort_order, i.id;

-- name: ListPublicServices :many
-- Active services shown online, with their category.
SELECT s.id, s.uuid, s.name, s.description, s.duration_minutes, s.price, s.payment_mode,
       s.staff_mode, s.sort_order, s.image_uuid, s.group_capacity,
       c.uuid AS category_uuid, c.name AS category_name, c.sort_order AS category_sort_order
FROM services s
LEFT JOIN service_categories c
       ON c.organization_id = s.organization_id AND c.id = s.category_id AND c.deleted_at IS NULL
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.deleted_at IS NULL AND s.is_active AND s.show_online
ORDER BY c.sort_order NULLS LAST, c.name NULLS LAST, s.sort_order, s.name, s.id;

-- name: ListPublicServiceOffers :many
-- Active branch offers of services (branch price / duration overrides).
SELECT sb.service_id, sb.branch_id, sb.price, sb.duration_minutes
FROM service_branches sb
WHERE sb.organization_id = sqlc.arg(organization_id) AND sb.is_active;

-- name: ListPublicStaff :many
-- Active staff shown online.
SELECT st.id, st.uuid, st.name, st.title, st.bio
FROM staff st
WHERE st.organization_id = sqlc.arg(organization_id)
  AND st.deleted_at IS NULL AND st.is_active AND st.show_online
ORDER BY st.name, st.id;

-- name: ListPublicStaffBranches :many
SELECT sb.staff_id, sb.branch_id
FROM staff_branches sb
WHERE sb.organization_id = sqlc.arg(organization_id);

-- name: ListPublicServiceStaff :many
-- Who may perform which service, with personal price / duration.
SELECT ss.service_id, ss.staff_id, ss.price, ss.duration_minutes
FROM service_staff ss
WHERE ss.organization_id = sqlc.arg(organization_id);

-- name: CreatePublicCheckout :one
INSERT INTO public_checkouts (organization_id, branch_id, hold_id, channel, lines, starts_at, party_size, zone)
VALUES (sqlc.arg(organization_id), sqlc.arg(branch_id), sqlc.arg(hold_id), sqlc.arg(channel), sqlc.arg(lines), sqlc.arg(starts_at),
        sqlc.narg(party_size), sqlc.arg(zone))
RETURNING *;

-- name: GetPublicCheckout :one
SELECT pc.*, b.uuid AS branch_uuid, b.name AS branch_name, h.uuid AS hold_uuid,
       h.status AS hold_status, h.ends_at AS hold_ends_at, h.expires_at AS hold_expires_at,
       r.uuid AS reservation_uuid
FROM public_checkouts pc
JOIN branches b ON b.organization_id = pc.organization_id AND b.id = pc.branch_id
JOIN capacity_holds h ON h.organization_id = pc.organization_id AND h.id = pc.hold_id
LEFT JOIN reservations r ON r.organization_id = pc.organization_id AND r.id = pc.reservation_id
WHERE pc.organization_id = sqlc.arg(organization_id) AND pc.uuid = sqlc.arg(uuid);

-- name: GetPublicCheckoutForUpdate :one
SELECT * FROM public_checkouts
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid)
FOR UPDATE;

-- name: SetPublicCheckoutReservation :exec
UPDATE public_checkouts
SET reservation_id = sqlc.arg(reservation_id)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: GetPublicReservationUUID :one
SELECT uuid FROM reservations
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id);

-- name: GetPublicReservationMeta :one
-- What the public status view adds to the reservation: the source and
-- the branch's time zone.
SELECT r.source, COALESCE(NULLIF(b.timezone, ''), o.timezone)::TEXT AS timezone
FROM reservations r
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
JOIN organizations o ON o.id = r.organization_id
WHERE r.organization_id = sqlc.arg(organization_id) AND r.uuid = sqlc.arg(uuid);

-- name: ListPublicBranchZones :many
-- Areas the customer may choose (restaurant): zones of the active, online
-- tables and areas of each branch.
SELECT DISTINCT branch_id, zone::TEXT AS zone FROM resources
WHERE organization_id = sqlc.arg(organization_id) AND branch_id = ANY(sqlc.arg(branch_ids)::BIGINT[])
  AND deleted_at IS NULL AND is_active AND show_online AND zone <> '' AND kind IN ('table', 'area')
ORDER BY branch_id, zone;

-- name: ListPublicClassSessionStaff :many
-- The person serving the bookings of each group session (service, start)
-- of a branch in [since, until), from its first live booking: a session
-- that has bookings keeps their trainer when the timetable's trainer
-- changes (TEC-990).
SELECT DISTINCT ON (l.service_id, l.starts_at)
    l.service_id::BIGINT AS service_id, l.starts_at, st.uuid AS staff_uuid,
    COALESCE(st.name, l.staff_name, '')::TEXT AS staff_name
FROM reservation_lines l
JOIN reservations r ON r.organization_id = l.organization_id AND r.id = l.reservation_id
LEFT JOIN staff st ON st.organization_id = l.organization_id AND st.id = l.staff_id
WHERE l.organization_id = sqlc.arg(organization_id) AND r.branch_id = sqlc.arg(branch_id)
  AND l.service_id = ANY(sqlc.arg(service_ids)::BIGINT[]) AND l.staff_id IS NOT NULL
  AND r.status NOT IN ('cancelled', 'rejected')
  AND l.starts_at >= sqlc.arg(since) AND l.starts_at < sqlc.arg(until)
ORDER BY l.service_id, l.starts_at, r.created_at, r.id;
