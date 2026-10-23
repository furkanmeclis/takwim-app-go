-- Availability engine inputs (TEC-897). Read-only; every query filters by
-- organization_id, joined tables by alias.organization_id too.

-- name: GetAvailabilityBranch :one
SELECT b.id, b.uuid, b.name, b.is_active, b.working_hours, b.holiday_exceptions,
       COALESCE(NULLIF(b.timezone, ''), o.timezone)::TEXT AS timezone
FROM branches b
JOIN organizations o ON o.id = b.organization_id
WHERE b.organization_id = sqlc.arg(organization_id) AND b.uuid = sqlc.arg(uuid) AND b.deleted_at IS NULL;

-- name: GetAvailabilityService :one
-- A live service with its offer in the branch (sb.* NULL = not offered).
SELECT s.id, s.uuid, s.name, s.duration_minutes, s.buffer_before_minutes, s.buffer_after_minutes,
       s.staff_mode, s.resource_kinds, s.show_online, s.is_active,
       sb.id AS offer_id, sb.duration_minutes AS offer_duration_minutes, sb.is_active AS offer_is_active
FROM services s
LEFT JOIN service_branches sb
       ON sb.organization_id = s.organization_id AND sb.service_id = s.id AND sb.branch_id = sqlc.arg(branch_id)
WHERE s.organization_id = sqlc.arg(organization_id) AND s.uuid = sqlc.arg(uuid) AND s.deleted_at IS NULL;

-- name: ListAvailabilityServiceStaff :many
-- Live, active staff who may perform the service and work in the branch,
-- with their personal duration and their weekly shifts there.
SELECT st.id, st.uuid, st.name, st.show_online, ss.duration_minutes, sbr.shifts
FROM service_staff ss
JOIN staff st ON st.organization_id = ss.organization_id AND st.id = ss.staff_id
JOIN staff_branches sbr ON sbr.organization_id = ss.organization_id AND sbr.staff_id = ss.staff_id
WHERE ss.organization_id = sqlc.arg(organization_id) AND ss.service_id = sqlc.arg(service_id)
  AND sbr.branch_id = sqlc.arg(branch_id)
  AND st.deleted_at IS NULL AND st.is_active
ORDER BY st.name, st.id;

-- name: GetAvailabilityStaffByUser :one
SELECT id, uuid FROM staff
WHERE organization_id = sqlc.arg(organization_id) AND user_id = sqlc.arg(user_id) AND deleted_at IS NULL;

-- name: ListAvailabilityTimeOff :many
SELECT staff_id, starts_at, ends_at FROM staff_time_off
WHERE organization_id = sqlc.arg(organization_id) AND staff_id = ANY(sqlc.arg(staff_ids)::BIGINT[])
  AND starts_at < sqlc.arg(until) AND ends_at > sqlc.arg(since);

-- name: ListAvailabilityResources :many
SELECT id, uuid, name, kind, zone, show_online, working_hours, min_capacity, capacity, combinable
FROM resources
WHERE organization_id = sqlc.arg(organization_id) AND branch_id = sqlc.arg(branch_id)
  AND kind = ANY(sqlc.arg(kinds)::TEXT[]) AND deleted_at IS NULL AND is_active
ORDER BY sort_order, name, id;

-- name: ListAvailabilityResourceClosures :many
SELECT resource_id, starts_at, ends_at FROM resource_closures
WHERE organization_id = sqlc.arg(organization_id) AND resource_id = ANY(sqlc.arg(resource_ids)::BIGINT[])
  AND starts_at < sqlc.arg(until) AND ends_at > sqlc.arg(since);
