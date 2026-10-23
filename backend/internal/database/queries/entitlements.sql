-- Entitlements and usage (TEC-883).

-- name: UpdateOrganizationEntitlements :one
-- PUT semantics: every value is replaced; NULL limits mean the plan default.
UPDATE organizations
SET plan = sqlc.arg(plan),
    addons = sqlc.arg(addons)::text[],
    branch_limit = sqlc.narg(branch_limit)::int,
    staff_limit = sqlc.narg(staff_limit)::int
WHERE uuid = sqlc.arg(uuid) AND deleted_at IS NULL
RETURNING *;

-- name: CountEntitlementUsage :one
-- Live branches and staff seats (members plus open invitations) of a
-- business.
SELECT
    (SELECT COUNT(*) FROM branches b
     WHERE b.organization_id = sqlc.arg(organization_id) AND b.deleted_at IS NULL)::bigint AS branches,
    ((SELECT COUNT(*) FROM organization_members om
      WHERE om.organization_id = sqlc.arg(organization_id))
     + (SELECT COUNT(*) FROM organization_invitations oi
        WHERE oi.organization_id = sqlc.arg(organization_id)
          AND oi.revoked_at IS NULL AND oi.accepted_at IS NULL AND oi.expires_at > NOW()))::bigint AS staff;

-- name: AddOrganizationUsage :one
INSERT INTO organization_usage (organization_id, metric, period, quantity)
VALUES (sqlc.arg(organization_id), sqlc.arg(metric), sqlc.arg(period), sqlc.arg(quantity))
ON CONFLICT (organization_id, metric, period)
DO UPDATE SET quantity = organization_usage.quantity + EXCLUDED.quantity, updated_at = NOW()
RETURNING *;

-- name: ListOrganizationUsage :many
SELECT * FROM organization_usage
WHERE organization_id = sqlc.arg(organization_id) AND period = sqlc.arg(period)
ORDER BY metric;
