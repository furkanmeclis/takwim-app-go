-- Capacity holds and allocations (TEC-898). Every query filters by
-- organization_id; joins by alias.organization_id too.

-- name: LockCapacity :exec
-- Serialises the writers of one organization inside a transaction (the
-- exclusion constraints stay the correctness argument).
SELECT pg_advisory_xact_lock(hashtextextended('capacity:' || sqlc.arg(organization_id)::BIGINT::TEXT, 0));

-- name: ExpireDueCapacityHolds :many
UPDATE capacity_holds
SET status = 'expired', released_at = sqlc.arg(now), release_reason = 'expired'
WHERE organization_id = sqlc.arg(organization_id) AND status = 'active' AND kind = 'hold'
  AND expires_at <= sqlc.arg(now)
RETURNING id;

-- name: DeleteCapacityAllocations :exec
DELETE FROM capacity_allocations
WHERE organization_id = sqlc.arg(organization_id) AND hold_id = ANY(sqlc.arg(hold_ids)::BIGINT[]);

-- name: ListDueCapacityHoldOrganizations :many
-- Organizations with holds past their expiry (the minute sweep).
SELECT DISTINCT organization_id FROM capacity_holds
WHERE status = 'active' AND kind = 'hold' AND expires_at <= sqlc.arg(now)
ORDER BY organization_id;

-- name: CreateCapacityHold :one
INSERT INTO capacity_holds (
    organization_id, branch_id, kind, reason, channel, starts_at, ends_at, expires_at, ref_type, ref_uuid, created_by
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(branch_id), sqlc.arg(kind), sqlc.narg(reason), sqlc.arg(channel),
    sqlc.arg(starts_at), sqlc.arg(ends_at), sqlc.narg(expires_at), sqlc.narg(ref_type), sqlc.narg(ref_uuid),
    sqlc.narg(created_by)
)
RETURNING *;

-- name: CreateCapacityAllocation :exec
INSERT INTO capacity_allocations (organization_id, hold_id, line_index, staff_id, resource_id, period)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(hold_id), sqlc.arg(line_index), sqlc.narg(staff_id), sqlc.narg(resource_id),
    tstzrange(sqlc.arg(starts_at)::TIMESTAMPTZ, sqlc.arg(ends_at)::TIMESTAMPTZ, '[)')
);

-- name: GetCapacityHold :one
SELECT h.*, b.uuid AS branch_uuid, b.name AS branch_name
FROM capacity_holds h
JOIN branches b ON b.organization_id = h.organization_id AND b.id = h.branch_id
WHERE h.organization_id = sqlc.arg(organization_id) AND h.uuid = sqlc.arg(uuid);

-- name: GetCapacityHoldForUpdate :one
SELECT * FROM capacity_holds
WHERE organization_id = sqlc.arg(organization_id) AND uuid = sqlc.arg(uuid)
FOR UPDATE;

-- name: ListCapacityHoldAllocations :many
SELECT a.line_index, lower(a.period)::TIMESTAMPTZ AS starts_at, upper(a.period)::TIMESTAMPTZ AS ends_at,
       st.uuid AS staff_uuid, st.name AS staff_name,
       r.uuid AS resource_uuid, r.name AS resource_name, r.kind AS resource_kind
FROM capacity_allocations a
LEFT JOIN staff st ON st.organization_id = a.organization_id AND st.id = a.staff_id
LEFT JOIN resources r ON r.organization_id = a.organization_id AND r.id = a.resource_id
WHERE a.organization_id = sqlc.arg(organization_id) AND a.hold_id = sqlc.arg(hold_id)
ORDER BY a.line_index, lower(a.period), a.id;

-- name: ListCapacityHoldStaffIDs :many
SELECT DISTINCT staff_id::BIGINT FROM capacity_allocations
WHERE organization_id = sqlc.arg(organization_id) AND hold_id = sqlc.arg(hold_id) AND staff_id IS NOT NULL;

-- name: ConfirmCapacityHold :one
UPDATE capacity_holds
SET kind = 'booking', expires_at = NULL,
    ref_type = COALESCE(sqlc.narg(ref_type), ref_type), ref_uuid = COALESCE(sqlc.narg(ref_uuid), ref_uuid)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'active' AND kind = 'hold'
RETURNING *;

-- name: ExtendCapacityHold :one
UPDATE capacity_holds SET expires_at = sqlc.arg(expires_at)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'active' AND kind = 'hold'
RETURNING *;

-- name: ReleaseCapacityHold :one
UPDATE capacity_holds
SET status = 'released', released_at = sqlc.arg(now), release_reason = sqlc.arg(release_reason)
WHERE organization_id = sqlc.arg(organization_id) AND id = sqlc.arg(id) AND status = 'active'
RETURNING *;

-- name: ListBusyCapacityAllocations :many
-- Commitments of the given staff and resources overlapping [since, until):
-- active holds that have not lapsed (a lapsed hold stops counting at its
-- expiry, before the sweep) and confirmed bookings. exclude_hold_id
-- leaves one commitment out (0 = none), for re-planning it.
SELECT a.staff_id, a.resource_id, lower(a.period)::TIMESTAMPTZ AS starts_at, upper(a.period)::TIMESTAMPTZ AS ends_at
FROM capacity_allocations a
JOIN capacity_holds h ON h.organization_id = a.organization_id AND h.id = a.hold_id
WHERE a.organization_id = sqlc.arg(organization_id)
  AND h.status = 'active' AND (h.expires_at IS NULL OR h.expires_at > sqlc.arg(now))
  AND h.id <> sqlc.arg(exclude_hold_id)
  AND (a.staff_id = ANY(sqlc.arg(staff_ids)::BIGINT[]) OR a.resource_id = ANY(sqlc.arg(resource_ids)::BIGINT[]))
  AND a.period && tstzrange(sqlc.arg(since)::TIMESTAMPTZ, sqlc.arg(until)::TIMESTAMPTZ, '[)');
