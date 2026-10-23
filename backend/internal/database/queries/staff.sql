-- Tenant resource: staff (TEC-890). Every query filters by organization_id.

-- name: CreateStaff :one
INSERT INTO staff (organization_id, user_id, name, title, bio, phone, email, color, show_online, is_active)
VALUES (
    sqlc.arg(organization_id), sqlc.narg(user_id), sqlc.arg(name), sqlc.arg(title), sqlc.arg(bio),
    sqlc.arg(phone), sqlc.arg(email), sqlc.arg(color), sqlc.arg(show_online), sqlc.arg(is_active)
)
RETURNING *;

-- name: UpdateStaff :one
UPDATE staff
SET user_id = sqlc.narg(user_id), name = sqlc.arg(name), title = sqlc.arg(title), bio = sqlc.arg(bio),
    phone = sqlc.arg(phone), email = sqlc.arg(email), color = sqlc.arg(color),
    show_online = sqlc.arg(show_online), is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid) AND deleted_at IS NULL
RETURNING *;

-- name: SetStaffActive :exec
UPDATE staff SET is_active = sqlc.arg(is_active)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: SoftDeleteStaff :execrows
UPDATE staff SET deleted_at = NOW()
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: LockStaff :one
-- Serializes the shift edits of one person (A04 overlap check).
SELECT id FROM staff
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND deleted_at IS NULL
FOR UPDATE;

-- name: GetStaff :one
SELECT
    s.*,
    u.uuid AS account_uuid, u.name AS account_name, u.surname AS account_surname, u.email AS account_email,
    COALESCE((
        SELECT jsonb_agg(jsonb_build_object('uuid', b.uuid, 'name', b.name) ORDER BY b.name)
        FROM staff_branches sb
        JOIN branches b ON b.id = sb.branch_id AND b.organization_id = sb.organization_id AND b.deleted_at IS NULL
        WHERE sb.organization_id = s.organization_id AND sb.staff_id = s.id
    ), '[]'::jsonb)::jsonb AS branches
FROM staff s
-- The linked account counts only while the user is a member.
LEFT JOIN organization_members om ON om.organization_id = s.organization_id AND om.user_id = s.user_id
LEFT JOIN users u ON u.id = om.user_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.uuid = sqlc.arg(uuid)
  AND s.deleted_at IS NULL;

-- name: StaffAccountTaken :one
SELECT EXISTS (
    SELECT 1 FROM staff
    WHERE organization_id = sqlc.arg(organization_id)
      AND user_id = sqlc.arg(user_id)
      AND deleted_at IS NULL
      AND (sqlc.narg(exclude_uuid)::uuid IS NULL OR uuid <> sqlc.narg(exclude_uuid))
)::boolean;

-- name: GetMemberUserByUUID :one
-- A member account of the organization (staff link target).
SELECT u.id, u.uuid, u.name, u.surname, u.email
FROM organization_members om
JOIN users u ON u.id = om.user_id
WHERE om.organization_id = sqlc.arg(organization_id)
  AND u.uuid = sqlc.arg(uuid)
  AND u.deleted_at IS NULL;

-- name: ListLinkableMembers :many
-- Members without a staff record yet.
SELECT u.uuid, u.name, u.surname, u.email
FROM organization_members om
JOIN users u ON u.id = om.user_id
WHERE om.organization_id = sqlc.arg(organization_id)
  AND u.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM staff s
    WHERE s.organization_id = om.organization_id AND s.user_id = om.user_id AND s.deleted_at IS NULL
  )
ORDER BY u.name, u.surname, u.email;

-- name: ListStaff :many
SELECT
    s.*,
    u.uuid AS account_uuid, u.name AS account_name, u.surname AS account_surname, u.email AS account_email,
    COALESCE((
        SELECT jsonb_agg(jsonb_build_object('uuid', b.uuid, 'name', b.name) ORDER BY b.name)
        FROM staff_branches sb
        JOIN branches b ON b.id = sb.branch_id AND b.organization_id = sb.organization_id AND b.deleted_at IS NULL
        WHERE sb.organization_id = s.organization_id AND sb.staff_id = s.id
    ), '[]'::jsonb)::jsonb AS branches
FROM staff s
-- The linked account counts only while the user is a member.
LEFT JOIN organization_members om ON om.organization_id = s.organization_id AND om.user_id = s.user_id
LEFT JOIN users u ON u.id = om.user_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.deleted_at IS NULL
  -- Branch scope of the caller (NULL = every branch): staff assigned to one
  -- of those branches.
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR EXISTS (
    SELECT 1 FROM staff_branches sb
    WHERE sb.organization_id = s.organization_id AND sb.staff_id = s.id
      AND sb.branch_id = ANY(sqlc.narg(branch_ids)::bigint[])))
  -- Branch filter of the list (faceted).
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR EXISTS (
    SELECT 1 FROM staff_branches fb
    WHERE fb.organization_id = s.organization_id AND fb.staff_id = s.id
      AND fb.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
  AND (sqlc.narg(is_active)::boolean IS NULL OR s.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR s.show_online = sqlc.narg(show_online))
  AND (sqlc.narg(has_account)::boolean IS NULL OR (u.id IS NOT NULL) = sqlc.narg(has_account))
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.name ILIKE '%' || sqlc.narg(q) || '%'
    OR s.title ILIKE '%' || sqlc.narg(q) || '%'
    OR s.phone ILIKE '%' || sqlc.narg(q) || '%'
    OR s.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND NOT sqlc.arg(sort_desc)::boolean THEN s.name END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'name' AND sqlc.arg(sort_desc)::boolean THEN s.name END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'title' AND NOT sqlc.arg(sort_desc)::boolean THEN s.title END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'title' AND sqlc.arg(sort_desc)::boolean THEN s.title END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND NOT sqlc.arg(sort_desc)::boolean THEN s.is_active END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'is_active' AND sqlc.arg(sort_desc)::boolean THEN s.is_active END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND NOT sqlc.arg(sort_desc)::boolean THEN s.updated_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'updated_at' AND sqlc.arg(sort_desc)::boolean THEN s.updated_at END DESC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND NOT sqlc.arg(sort_desc)::boolean THEN s.created_at END ASC,
  CASE WHEN sqlc.arg(sort_field)::text = 'created_at' AND sqlc.arg(sort_desc)::boolean THEN s.created_at END DESC,
  s.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountStaff :one
SELECT COUNT(*)::bigint
FROM staff s
-- The linked account counts only while the user is a member.
LEFT JOIN organization_members om ON om.organization_id = s.organization_id AND om.user_id = s.user_id
LEFT JOIN users u ON u.id = om.user_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.deleted_at IS NULL
  -- Branch scope of the caller (NULL = every branch): staff assigned to one
  -- of those branches.
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR EXISTS (
    SELECT 1 FROM staff_branches sb
    WHERE sb.organization_id = s.organization_id AND sb.staff_id = s.id
      AND sb.branch_id = ANY(sqlc.narg(branch_ids)::bigint[])))
  -- Branch filter of the list (faceted).
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR EXISTS (
    SELECT 1 FROM staff_branches fb
    WHERE fb.organization_id = s.organization_id AND fb.staff_id = s.id
      AND fb.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
  AND (sqlc.narg(is_active)::boolean IS NULL OR s.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR s.show_online = sqlc.narg(show_online))
  AND (sqlc.narg(has_account)::boolean IS NULL OR (u.id IS NOT NULL) = sqlc.narg(has_account))
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.name ILIKE '%' || sqlc.narg(q) || '%'
    OR s.title ILIKE '%' || sqlc.narg(q) || '%'
    OR s.phone ILIKE '%' || sqlc.narg(q) || '%'
    OR s.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
  );

-- name: ListStaffUUIDs :many
SELECT s.uuid
FROM staff s
-- The linked account counts only while the user is a member.
LEFT JOIN organization_members om ON om.organization_id = s.organization_id AND om.user_id = s.user_id
LEFT JOIN users u ON u.id = om.user_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.deleted_at IS NULL
  -- Branch scope of the caller (NULL = every branch): staff assigned to one
  -- of those branches.
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR EXISTS (
    SELECT 1 FROM staff_branches sb
    WHERE sb.organization_id = s.organization_id AND sb.staff_id = s.id
      AND sb.branch_id = ANY(sqlc.narg(branch_ids)::bigint[])))
  -- Branch filter of the list (faceted).
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR EXISTS (
    SELECT 1 FROM staff_branches fb
    WHERE fb.organization_id = s.organization_id AND fb.staff_id = s.id
      AND fb.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
  AND (sqlc.narg(is_active)::boolean IS NULL OR s.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR s.show_online = sqlc.narg(show_online))
  AND (sqlc.narg(has_account)::boolean IS NULL OR (u.id IS NOT NULL) = sqlc.narg(has_account))
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.name ILIKE '%' || sqlc.narg(q) || '%'
    OR s.title ILIKE '%' || sqlc.narg(q) || '%'
    OR s.phone ILIKE '%' || sqlc.narg(q) || '%'
    OR s.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY s.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListStaffForExport :many
SELECT
    s.*,
    u.uuid AS account_uuid, u.name AS account_name, u.surname AS account_surname, u.email AS account_email,
    COALESCE((
        SELECT jsonb_agg(jsonb_build_object('uuid', b.uuid, 'name', b.name) ORDER BY b.name)
        FROM staff_branches sb
        JOIN branches b ON b.id = sb.branch_id AND b.organization_id = sb.organization_id AND b.deleted_at IS NULL
        WHERE sb.organization_id = s.organization_id AND sb.staff_id = s.id
    ), '[]'::jsonb)::jsonb AS branches
FROM staff s
-- The linked account counts only while the user is a member.
LEFT JOIN organization_members om ON om.organization_id = s.organization_id AND om.user_id = s.user_id
LEFT JOIN users u ON u.id = om.user_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.deleted_at IS NULL
  -- Branch scope of the caller (NULL = every branch): staff assigned to one
  -- of those branches.
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR EXISTS (
    SELECT 1 FROM staff_branches sb
    WHERE sb.organization_id = s.organization_id AND sb.staff_id = s.id
      AND sb.branch_id = ANY(sqlc.narg(branch_ids)::bigint[])))
  -- Branch filter of the list (faceted).
  AND (sqlc.narg(filter_branch_ids)::bigint[] IS NULL OR EXISTS (
    SELECT 1 FROM staff_branches fb
    WHERE fb.organization_id = s.organization_id AND fb.staff_id = s.id
      AND fb.branch_id = ANY(sqlc.narg(filter_branch_ids)::bigint[])))
  AND (sqlc.narg(is_active)::boolean IS NULL OR s.is_active = sqlc.narg(is_active))
  AND (sqlc.narg(show_online)::boolean IS NULL OR s.show_online = sqlc.narg(show_online))
  AND (sqlc.narg(has_account)::boolean IS NULL OR (u.id IS NOT NULL) = sqlc.narg(has_account))
  AND (
    sqlc.narg(q)::text IS NULL
    OR s.name ILIKE '%' || sqlc.narg(q) || '%'
    OR s.title ILIKE '%' || sqlc.narg(q) || '%'
    OR s.phone ILIKE '%' || sqlc.narg(q) || '%'
    OR s.email ILIKE '%' || sqlc.narg(q) || '%'
    OR u.email ILIKE '%' || sqlc.narg(q) || '%'
  )
ORDER BY s.name ASC, s.id ASC;

-- name: ListStaffBranchFacets :many
-- Branches with their staff count, inside the caller's branch scope.
SELECT b.uuid, b.name, COUNT(DISTINCT s.id)::bigint AS count
FROM staff_branches sb
JOIN staff s ON s.id = sb.staff_id AND s.organization_id = sb.organization_id AND s.deleted_at IS NULL
JOIN branches b ON b.id = sb.branch_id AND b.organization_id = sb.organization_id AND b.deleted_at IS NULL
WHERE sb.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR sb.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
GROUP BY b.uuid, b.name
ORDER BY b.name ASC;

-- name: ListStaffAssignments :many
-- Branch assignments of one person with the shifts and the branch hours.
SELECT sb.branch_id, sb.shifts, b.uuid AS branch_uuid, b.name AS branch_name, b.code AS branch_code,
    b.working_hours AS branch_working_hours, b.timezone AS branch_timezone
FROM staff_branches sb
JOIN branches b ON b.id = sb.branch_id AND b.organization_id = sb.organization_id AND b.deleted_at IS NULL
WHERE sb.organization_id = sqlc.arg(organization_id)
  AND sb.staff_id = sqlc.arg(staff_id)
ORDER BY b.name ASC;

-- name: AddStaffBranch :exec
INSERT INTO staff_branches (organization_id, staff_id, branch_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(staff_id), sqlc.arg(branch_id))
ON CONFLICT (organization_id, staff_id, branch_id) DO NOTHING;

-- name: RemoveStaffBranch :exec
DELETE FROM staff_branches
WHERE organization_id = sqlc.arg(organization_id)
  AND staff_id = sqlc.arg(staff_id)
  AND branch_id = sqlc.arg(branch_id);

-- name: UpdateStaffShifts :execrows
UPDATE staff_branches SET shifts = sqlc.arg(shifts)
WHERE organization_id = sqlc.arg(organization_id)
  AND staff_id = sqlc.arg(staff_id)
  AND branch_id = sqlc.arg(branch_id);

-- name: ListStaffTimeOff :many
SELECT * FROM staff_time_off
WHERE organization_id = sqlc.arg(organization_id)
  AND staff_id = sqlc.arg(staff_id)
  AND (sqlc.narg(range_from)::timestamptz IS NULL OR ends_at > sqlc.narg(range_from))
  AND (sqlc.narg(range_to)::timestamptz IS NULL OR starts_at < sqlc.narg(range_to))
ORDER BY starts_at DESC, id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountStaffTimeOff :one
SELECT COUNT(*)::bigint FROM staff_time_off
WHERE organization_id = sqlc.arg(organization_id)
  AND staff_id = sqlc.arg(staff_id)
  AND (sqlc.narg(range_from)::timestamptz IS NULL OR ends_at > sqlc.narg(range_from))
  AND (sqlc.narg(range_to)::timestamptz IS NULL OR starts_at < sqlc.narg(range_to));

-- name: GetStaffTimeOff :one
SELECT * FROM staff_time_off
WHERE organization_id = sqlc.arg(organization_id)
  AND staff_id = sqlc.arg(staff_id)
  AND uuid = sqlc.arg(uuid);

-- name: CreateStaffTimeOff :one
INSERT INTO staff_time_off (organization_id, staff_id, kind, starts_at, ends_at, note, created_by)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(staff_id), sqlc.arg(kind), sqlc.arg(starts_at),
    sqlc.arg(ends_at), sqlc.arg(note), sqlc.narg(created_by)
)
RETURNING *;

-- name: UpdateStaffTimeOff :one
UPDATE staff_time_off
SET kind = sqlc.arg(kind), starts_at = sqlc.arg(starts_at), ends_at = sqlc.arg(ends_at), note = sqlc.arg(note)
WHERE organization_id = sqlc.arg(organization_id)
  AND staff_id = sqlc.arg(staff_id)
  AND uuid = sqlc.arg(uuid)
RETURNING *;

-- name: DeleteStaffTimeOff :execrows
DELETE FROM staff_time_off
WHERE organization_id = sqlc.arg(organization_id)
  AND staff_id = sqlc.arg(staff_id)
  AND uuid = sqlc.arg(uuid);

-- name: ListStaffSearchDocuments :many
SELECT s.id, s.uuid, s.organization_id, s.name, s.title, s.email, s.phone, o.slug AS organization_slug,
    ARRAY(
        SELECT sb.branch_id FROM staff_branches sb
        WHERE sb.organization_id = s.organization_id AND sb.staff_id = s.id
        ORDER BY sb.branch_id
    )::bigint[] AS branch_ids
FROM staff s
JOIN organizations o ON o.id = s.organization_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.deleted_at IS NULL;

-- name: GetStaffSearchDocument :one
SELECT s.id, s.uuid, s.organization_id, s.name, s.title, s.email, s.phone, o.slug AS organization_slug,
    ARRAY(
        SELECT sb.branch_id FROM staff_branches sb
        WHERE sb.organization_id = s.organization_id AND sb.staff_id = s.id
        ORDER BY sb.branch_id
    )::bigint[] AS branch_ids
FROM staff s
JOIN organizations o ON o.id = s.organization_id
WHERE s.organization_id = sqlc.arg(organization_id)
  AND s.uuid = sqlc.arg(uuid)
  AND s.deleted_at IS NULL;
