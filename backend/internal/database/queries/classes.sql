-- TEC-970: the class timetable (class_schedules) and single-session
-- changes (class_session_changes). Every query takes the organization id;
-- joined tables are matched by alias.organization_id too.

-- name: InsertClassSchedule :one
INSERT INTO class_schedules (
    organization_id, branch_id, service_id, weekday, start_time, staff_id, resource_id, capacity,
    valid_from, valid_until, is_active, note, created_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(branch_id), sqlc.arg(service_id), sqlc.arg(weekday), sqlc.arg(start_time),
    sqlc.narg(staff_id), sqlc.narg(resource_id), sqlc.narg(capacity), sqlc.arg(valid_from), sqlc.narg(valid_until),
    sqlc.arg(is_active), sqlc.arg(note), sqlc.narg(created_by)
)
RETURNING id, uuid;

-- name: UpdateClassSchedule :execrows
UPDATE class_schedules SET
    branch_id = sqlc.arg(branch_id), service_id = sqlc.arg(service_id), weekday = sqlc.arg(weekday),
    start_time = sqlc.arg(start_time), staff_id = sqlc.narg(staff_id), resource_id = sqlc.narg(resource_id),
    capacity = sqlc.narg(capacity), valid_from = sqlc.arg(valid_from), valid_until = sqlc.narg(valid_until),
    is_active = sqlc.arg(is_active), note = sqlc.arg(note)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: SoftDeleteClassSchedule :execrows
UPDATE class_schedules SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: GetClassSchedule :one
SELECT cs.id, cs.uuid, cs.branch_id, cs.service_id, cs.weekday, cs.start_time, cs.staff_id, cs.resource_id,
    cs.capacity, cs.valid_from, cs.valid_until, cs.is_active, cs.note, cs.created_at, cs.updated_at,
    b.uuid AS branch_uuid, b.name AS branch_name,
    COALESCE(NULLIF(b.timezone, ''), o.timezone)::TEXT AS timezone,
    s.uuid AS service_uuid, s.name AS service_name, s.group_capacity AS service_capacity,
    COALESCE(sb.duration_minutes, s.duration_minutes)::INT AS duration_minutes,
    s.buffer_before_minutes, s.buffer_after_minutes,
    st.uuid AS staff_uuid, st.name AS staff_name,
    r.uuid AS resource_uuid, r.name AS resource_name, r.kind AS resource_kind
FROM class_schedules cs
JOIN organizations o ON o.id = cs.organization_id
JOIN branches b ON b.organization_id = cs.organization_id AND b.id = cs.branch_id
JOIN services s ON s.organization_id = cs.organization_id AND s.id = cs.service_id
LEFT JOIN service_branches sb ON sb.organization_id = cs.organization_id AND sb.service_id = cs.service_id AND sb.branch_id = cs.branch_id
LEFT JOIN staff st ON st.organization_id = cs.organization_id AND st.id = cs.staff_id
LEFT JOIN resources r ON r.organization_id = cs.organization_id AND r.id = cs.resource_id
WHERE cs.organization_id = sqlc.arg(organization_id) AND cs.uuid = sqlc.arg(uuid) AND cs.deleted_at IS NULL;

-- name: ListClassSchedules :many
-- The timetable page: filters, search over the class, trainer, area and
-- note, weekday-aware ordering. branch_ids is the caller's branch scope
-- (NULL = every branch).
SELECT cs.id, cs.uuid, cs.branch_id, cs.service_id, cs.weekday, cs.start_time, cs.staff_id, cs.resource_id,
    cs.capacity, cs.valid_from, cs.valid_until, cs.is_active, cs.note, cs.created_at, cs.updated_at,
    b.uuid AS branch_uuid, b.name AS branch_name,
    COALESCE(NULLIF(b.timezone, ''), o.timezone)::TEXT AS timezone,
    s.uuid AS service_uuid, s.name AS service_name, s.group_capacity AS service_capacity,
    COALESCE(sb.duration_minutes, s.duration_minutes)::INT AS duration_minutes,
    s.buffer_before_minutes, s.buffer_after_minutes,
    st.uuid AS staff_uuid, st.name AS staff_name,
    r.uuid AS resource_uuid, r.name AS resource_name, r.kind AS resource_kind
FROM class_schedules cs
JOIN organizations o ON o.id = cs.organization_id
JOIN branches b ON b.organization_id = cs.organization_id AND b.id = cs.branch_id
JOIN services s ON s.organization_id = cs.organization_id AND s.id = cs.service_id
LEFT JOIN service_branches sb ON sb.organization_id = cs.organization_id AND sb.service_id = cs.service_id AND sb.branch_id = cs.branch_id
LEFT JOIN staff st ON st.organization_id = cs.organization_id AND st.id = cs.staff_id
LEFT JOIN resources r ON r.organization_id = cs.organization_id AND r.id = cs.resource_id
WHERE cs.organization_id = sqlc.arg(organization_id) AND cs.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR cs.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(branch_uuids)::uuid[] IS NULL OR b.uuid = ANY(sqlc.narg(branch_uuids)::uuid[]))
  AND (sqlc.narg(service_uuids)::uuid[] IS NULL OR s.uuid = ANY(sqlc.narg(service_uuids)::uuid[]))
  AND (sqlc.narg(staff_uuids)::uuid[] IS NULL OR st.uuid = ANY(sqlc.narg(staff_uuids)::uuid[]))
  AND (sqlc.narg(weekdays)::text[] IS NULL OR cs.weekday = ANY(sqlc.narg(weekdays)::text[]))
  AND (sqlc.narg(is_active)::boolean IS NULL OR cs.is_active = sqlc.narg(is_active)::boolean)
  AND (sqlc.narg(q)::text IS NULL OR s.name ILIKE '%' || sqlc.narg(q) || '%' OR st.name ILIKE '%' || sqlc.narg(q) || '%'
       OR r.name ILIKE '%' || sqlc.narg(q) || '%' OR cs.note ILIKE '%' || sqlc.narg(q) || '%')
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'service' AND NOT sqlc.arg(sort_desc)::boolean THEN lower(s.name) END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'service' AND sqlc.arg(sort_desc)::boolean THEN lower(s.name) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN cs.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN cs.created_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'weekday' AND sqlc.arg(sort_desc)::boolean
       THEN array_position(ARRAY['mon','tue','wed','thu','fri','sat','sun']::text[], cs.weekday::text) END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'weekday' AND sqlc.arg(sort_desc)::boolean THEN cs.start_time END DESC,
  array_position(ARRAY['mon','tue','wed','thu','fri','sat','sun']::text[], cs.weekday::text) ASC,
  cs.start_time ASC, lower(s.name) ASC, cs.id ASC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountClassSchedules :one
SELECT COUNT(*)::bigint
FROM class_schedules cs
JOIN branches b ON b.organization_id = cs.organization_id AND b.id = cs.branch_id
JOIN services s ON s.organization_id = cs.organization_id AND s.id = cs.service_id
LEFT JOIN staff st ON st.organization_id = cs.organization_id AND st.id = cs.staff_id
LEFT JOIN resources r ON r.organization_id = cs.organization_id AND r.id = cs.resource_id
WHERE cs.organization_id = sqlc.arg(organization_id) AND cs.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR cs.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(branch_uuids)::uuid[] IS NULL OR b.uuid = ANY(sqlc.narg(branch_uuids)::uuid[]))
  AND (sqlc.narg(service_uuids)::uuid[] IS NULL OR s.uuid = ANY(sqlc.narg(service_uuids)::uuid[]))
  AND (sqlc.narg(staff_uuids)::uuid[] IS NULL OR st.uuid = ANY(sqlc.narg(staff_uuids)::uuid[]))
  AND (sqlc.narg(weekdays)::text[] IS NULL OR cs.weekday = ANY(sqlc.narg(weekdays)::text[]))
  AND (sqlc.narg(is_active)::boolean IS NULL OR cs.is_active = sqlc.narg(is_active)::boolean)
  AND (sqlc.narg(q)::text IS NULL OR s.name ILIKE '%' || sqlc.narg(q) || '%' OR st.name ILIKE '%' || sqlc.narg(q) || '%'
       OR r.name ILIKE '%' || sqlc.narg(q) || '%' OR cs.note ILIKE '%' || sqlc.narg(q) || '%');

-- name: ListClassTimetable :many
-- Active classes of live, active group services in live, active branches
-- whose validity touches [from, to]: the sessions of a period. Filters are
-- optional (NULL = all); branch_ids is the caller's branch scope.
SELECT cs.id, cs.uuid, cs.branch_id, cs.service_id, cs.weekday, cs.start_time, cs.staff_id, cs.resource_id,
    cs.capacity, cs.valid_from, cs.valid_until,
    b.uuid AS branch_uuid, b.name AS branch_name,
    COALESCE(NULLIF(b.timezone, ''), o.timezone)::TEXT AS timezone,
    s.uuid AS service_uuid, s.name AS service_name, s.group_capacity AS service_capacity,
    COALESCE(sb.duration_minutes, s.duration_minutes)::INT AS duration_minutes,
    s.buffer_before_minutes, s.buffer_after_minutes,
    st.uuid AS staff_uuid, st.name AS staff_name,
    r.uuid AS resource_uuid, r.name AS resource_name, r.kind AS resource_kind
FROM class_schedules cs
JOIN organizations o ON o.id = cs.organization_id
JOIN branches b ON b.organization_id = cs.organization_id AND b.id = cs.branch_id
JOIN services s ON s.organization_id = cs.organization_id AND s.id = cs.service_id
LEFT JOIN service_branches sb ON sb.organization_id = cs.organization_id AND sb.service_id = cs.service_id AND sb.branch_id = cs.branch_id
LEFT JOIN staff st ON st.organization_id = cs.organization_id AND st.id = cs.staff_id
LEFT JOIN resources r ON r.organization_id = cs.organization_id AND r.id = cs.resource_id
WHERE cs.organization_id = sqlc.arg(organization_id) AND cs.deleted_at IS NULL AND cs.is_active
  AND s.deleted_at IS NULL AND s.is_active AND s.group_capacity IS NOT NULL
  AND b.deleted_at IS NULL AND b.is_active
  AND cs.valid_from <= sqlc.arg(to_date)::date
  AND (cs.valid_until IS NULL OR cs.valid_until >= sqlc.arg(from_date)::date)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR cs.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
  AND (sqlc.narg(service_ids)::bigint[] IS NULL OR cs.service_id = ANY(sqlc.narg(service_ids)::bigint[]))
  AND (sqlc.narg(staff_ids)::bigint[] IS NULL OR cs.staff_id = ANY(sqlc.narg(staff_ids)::bigint[]))
ORDER BY cs.start_time, cs.id;

-- name: ListClassTimetableBusy :many
-- The classes that commit a trainer or an area (any branch: a person has
-- one calendar) and whose validity touches [from, to]: their sessions are
-- busy time for every other booking.
SELECT cs.id, cs.branch_id, cs.service_id, cs.weekday, cs.start_time, cs.staff_id, cs.resource_id,
    cs.valid_from, cs.valid_until,
    COALESCE(NULLIF(b.timezone, ''), o.timezone)::TEXT AS timezone,
    COALESCE(sb.duration_minutes, s.duration_minutes)::INT AS duration_minutes,
    s.buffer_before_minutes, s.buffer_after_minutes
FROM class_schedules cs
JOIN organizations o ON o.id = cs.organization_id
JOIN branches b ON b.organization_id = cs.organization_id AND b.id = cs.branch_id
JOIN services s ON s.organization_id = cs.organization_id AND s.id = cs.service_id
LEFT JOIN service_branches sb ON sb.organization_id = cs.organization_id AND sb.service_id = cs.service_id AND sb.branch_id = cs.branch_id
WHERE cs.organization_id = sqlc.arg(organization_id) AND cs.deleted_at IS NULL AND cs.is_active
  AND s.deleted_at IS NULL AND s.is_active AND s.group_capacity IS NOT NULL
  AND b.deleted_at IS NULL AND b.is_active
  AND cs.valid_from <= sqlc.arg(to_date)::date
  AND (cs.valid_until IS NULL OR cs.valid_until >= sqlc.arg(from_date)::date)
  AND (cs.staff_id = ANY(sqlc.arg(staff_ids)::bigint[]) OR cs.resource_id = ANY(sqlc.arg(resource_ids)::bigint[]));

-- name: ListClassScheduleNeighbours :many
-- Live, active classes on one weekday that share the branch and service,
-- the trainer or the area of a class being saved (overlap is checked by
-- the caller). exclude_id leaves the class itself out (0 = none).
SELECT cs.id, cs.uuid, cs.branch_id, cs.service_id, cs.weekday, cs.start_time, cs.staff_id, cs.resource_id,
    cs.valid_from, cs.valid_until, s.name AS service_name,
    COALESCE(sb.duration_minutes, s.duration_minutes)::INT AS duration_minutes,
    s.buffer_before_minutes, s.buffer_after_minutes
FROM class_schedules cs
JOIN services s ON s.organization_id = cs.organization_id AND s.id = cs.service_id
LEFT JOIN service_branches sb ON sb.organization_id = cs.organization_id AND sb.service_id = cs.service_id AND sb.branch_id = cs.branch_id
WHERE cs.organization_id = sqlc.arg(organization_id) AND cs.deleted_at IS NULL AND cs.is_active
  AND s.deleted_at IS NULL AND cs.weekday = sqlc.arg(weekday) AND cs.id <> sqlc.arg(exclude_id)
  AND ((cs.branch_id = sqlc.arg(branch_id) AND cs.service_id = sqlc.arg(service_id))
       OR cs.staff_id = sqlc.narg(staff_id) OR cs.resource_id = sqlc.narg(resource_id));

-- name: ListClassSessionChanges :many
SELECT c.schedule_id, c.session_date, c.cancelled, c.capacity, c.reason, c.updated_at
FROM class_session_changes c
WHERE c.organization_id = sqlc.arg(organization_id) AND c.schedule_id = ANY(sqlc.arg(schedule_ids)::bigint[])
  AND c.session_date BETWEEN sqlc.arg(from_date)::date AND sqlc.arg(to_date)::date;

-- name: UpsertClassSessionChange :exec
INSERT INTO class_session_changes (organization_id, schedule_id, session_date, cancelled, capacity, reason, updated_by)
VALUES (sqlc.arg(organization_id), sqlc.arg(schedule_id), sqlc.arg(session_date), sqlc.arg(cancelled),
        sqlc.narg(capacity), sqlc.arg(reason), sqlc.narg(updated_by))
ON CONFLICT (organization_id, schedule_id, session_date) DO UPDATE SET
    cancelled = EXCLUDED.cancelled, capacity = EXCLUDED.capacity, reason = EXCLUDED.reason,
    updated_by = EXCLUDED.updated_by;

-- name: DeleteClassSessionChange :exec
DELETE FROM class_session_changes
WHERE organization_id = sqlc.arg(organization_id) AND schedule_id = sqlc.arg(schedule_id)
  AND session_date = sqlc.arg(session_date);

-- name: GetClassSessionBulkJob :one
-- The caller's latest bulk run of one action over the bookings of one
-- session (cancel, TEC-989; move, TEC-994) that can still be rolled back.
SELECT j.uuid, j.result_json, j.target_json, j.rollback_until, j.created_at
FROM bulk_jobs j
WHERE j.organization_id = sqlc.arg(organization_id) AND j.resource = sqlc.arg(resource)
  AND j.action = sqlc.arg(action)
  AND j.actor_id = sqlc.arg(actor_id) AND j.status = 'completed'
  AND j.rollback_until > NOW()
  AND j.target_json -> 'params' ->> 'class' = sqlc.arg(class_uuid)::text
  AND j.target_json -> 'params' ->> 'date' = sqlc.arg(session_date)::text
ORDER BY j.created_at DESC, j.id DESC
LIMIT 1;

-- name: ListClassSessionSeats :many
-- Seats booked per group session (service, branch, start) in [since,
-- until): active holds that have not lapsed and confirmed bookings.
SELECT h.session_service_id::BIGINT AS service_id, h.branch_id, h.starts_at, SUM(h.seats)::INT AS seats
FROM capacity_holds h
WHERE h.organization_id = sqlc.arg(organization_id)
  AND h.session_service_id = ANY(sqlc.arg(service_ids)::BIGINT[])
  AND h.status = 'active' AND (h.expires_at IS NULL OR h.expires_at > sqlc.arg(now))
  AND h.starts_at >= sqlc.arg(since) AND h.starts_at < sqlc.arg(until)
GROUP BY h.session_service_id, h.branch_id, h.starts_at;

-- name: ListClassSessionAttendees :many
-- The bookings of one session: visits in the branch with a line of the
-- class at the start, except cancelled and rejected ones.
SELECT r.uuid, r.number, r.status, r.approval_status, r.payment_status, r.confirmation_status, r.source,
    r.party_size, r.created_at,
    c.uuid AS customer_uuid, c.first_name, c.last_name, c.phone_e164
FROM reservations r
JOIN customers c ON c.organization_id = r.organization_id AND c.id = r.customer_id
WHERE r.organization_id = sqlc.arg(organization_id) AND r.branch_id = sqlc.arg(branch_id)
  AND r.status NOT IN ('cancelled', 'rejected')
  AND EXISTS (
    SELECT 1 FROM reservation_lines l
    WHERE l.organization_id = r.organization_id AND l.reservation_id = r.id
      AND l.service_id = sqlc.arg(service_id) AND l.starts_at = sqlc.arg(starts_at))
ORDER BY r.created_at, r.id;

-- name: ListClassFacets :many
-- The branches, classes and trainers that appear in the timetable, for
-- the list filters (kind = branch | service | staff).
SELECT 'branch'::TEXT AS kind, b.uuid, b.name, COUNT(*)::bigint AS count
FROM class_schedules cs
JOIN branches b ON b.organization_id = cs.organization_id AND b.id = cs.branch_id
WHERE cs.organization_id = sqlc.arg(organization_id) AND cs.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR cs.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY b.uuid, b.name
UNION ALL
SELECT 'service'::TEXT AS kind, s.uuid, s.name, COUNT(*)::bigint AS count
FROM class_schedules cs
JOIN services s ON s.organization_id = cs.organization_id AND s.id = cs.service_id
WHERE cs.organization_id = sqlc.arg(organization_id) AND cs.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR cs.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY s.uuid, s.name
UNION ALL
SELECT 'staff'::TEXT AS kind, st.uuid, st.name, COUNT(*)::bigint AS count
FROM class_schedules cs
JOIN staff st ON st.organization_id = cs.organization_id AND st.id = cs.staff_id
WHERE cs.organization_id = sqlc.arg(organization_id) AND cs.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR cs.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY st.uuid, st.name
ORDER BY 1, 3;

-- ------------------------------------------------------------ form lookups

-- name: GetClassBranch :one
SELECT b.id, b.uuid, b.name, b.is_active, COALESCE(NULLIF(b.timezone, ''), o.timezone)::TEXT AS timezone
FROM branches b
JOIN organizations o ON o.id = b.organization_id
WHERE b.organization_id = sqlc.arg(organization_id) AND b.uuid = sqlc.arg(uuid) AND b.deleted_at IS NULL;

-- name: GetClassService :one
-- A live service with its offer in the branch (sb.* NULL = not offered).
SELECT s.id, s.uuid, s.name, s.group_capacity, s.staff_mode, s.resource_kinds, s.is_active,
    COALESCE(sb.duration_minutes, s.duration_minutes)::INT AS duration_minutes,
    s.buffer_before_minutes, s.buffer_after_minutes,
    sb.id AS offer_id, sb.is_active AS offer_is_active
FROM services s
LEFT JOIN service_branches sb ON sb.organization_id = s.organization_id AND sb.service_id = s.id AND sb.branch_id = sqlc.arg(branch_id)
WHERE s.organization_id = sqlc.arg(organization_id) AND s.uuid = sqlc.arg(uuid) AND s.deleted_at IS NULL;

-- name: GetClassTrainer :one
-- A live staff member who may perform the service and works in the branch.
SELECT st.id, st.uuid, st.name, st.is_active,
    EXISTS (SELECT 1 FROM service_staff ss
            WHERE ss.organization_id = st.organization_id AND ss.staff_id = st.id AND ss.service_id = sqlc.arg(service_id)) AS performs,
    EXISTS (SELECT 1 FROM staff_branches sbr
            WHERE sbr.organization_id = st.organization_id AND sbr.staff_id = st.id AND sbr.branch_id = sqlc.arg(branch_id)) AS in_branch
FROM staff st
WHERE st.organization_id = sqlc.arg(organization_id) AND st.uuid = sqlc.arg(uuid) AND st.deleted_at IS NULL;

-- name: GetClassResource :one
SELECT r.id, r.uuid, r.name, r.kind, r.branch_id, r.is_active
FROM resources r
WHERE r.organization_id = sqlc.arg(organization_id) AND r.uuid = sqlc.arg(uuid) AND r.deleted_at IS NULL;

-- name: ListClassFormServices :many
-- Live group services with the branches that offer them.
SELECT s.id, s.uuid, s.name, s.group_capacity, s.staff_mode, s.resource_kinds, s.duration_minutes, s.is_active,
    COALESCE((SELECT array_agg(b.uuid ORDER BY b.name) FROM service_branches sb
              JOIN branches b ON b.organization_id = sb.organization_id AND b.id = sb.branch_id
              WHERE sb.organization_id = s.organization_id AND sb.service_id = s.id AND sb.is_active
                AND b.deleted_at IS NULL), '{}')::uuid[] AS branch_uuids
FROM services s
WHERE s.organization_id = sqlc.arg(organization_id) AND s.deleted_at IS NULL AND s.group_capacity IS NOT NULL
ORDER BY lower(s.name), s.id;

-- name: ListClassFormTrainers :many
-- Live staff who may perform a group service, with the branches they work in.
SELECT ss.service_id, st.uuid, st.name, st.is_active,
    COALESCE((SELECT array_agg(b.uuid ORDER BY b.name) FROM staff_branches sbr
              JOIN branches b ON b.organization_id = sbr.organization_id AND b.id = sbr.branch_id
              WHERE sbr.organization_id = st.organization_id AND sbr.staff_id = st.id AND b.deleted_at IS NULL), '{}')::uuid[] AS branch_uuids
FROM service_staff ss
JOIN staff st ON st.organization_id = ss.organization_id AND st.id = ss.staff_id
JOIN services s ON s.organization_id = ss.organization_id AND s.id = ss.service_id
WHERE ss.organization_id = sqlc.arg(organization_id) AND st.deleted_at IS NULL
  AND s.deleted_at IS NULL AND s.group_capacity IS NOT NULL
ORDER BY lower(st.name), st.id;

-- name: ListClassFormResources :many
SELECT r.uuid, r.name, r.kind, r.is_active, b.uuid AS branch_uuid
FROM resources r
JOIN branches b ON b.organization_id = r.organization_id AND b.id = r.branch_id
WHERE r.organization_id = sqlc.arg(organization_id) AND r.deleted_at IS NULL AND b.deleted_at IS NULL
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR r.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
ORDER BY r.sort_order, lower(r.name), r.id;
