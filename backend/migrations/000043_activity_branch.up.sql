-- TEC-893: activity history by business and branch.
--
-- activity_events.branch_id ties an event to the branch it happened in, so
-- the tenant activity list follows the member's branch scope and the
-- header branch picker. Events of the whole business (settings, roles,
-- members) keep it NULL. tenant.activity.read opens the list.

ALTER TABLE activity_events
    ADD COLUMN branch_id BIGINT NULL REFERENCES branches (id) ON DELETE SET NULL;

CREATE INDEX idx_activity_events_organization_branch_created
    ON activity_events (organization_id, branch_id, created_at DESC)
    WHERE organization_id IS NOT NULL;

-- Existing branch events belong to their branch.
UPDATE activity_events e
SET branch_id = b.id
FROM branches b
WHERE e.resource = 'tenant.branches'
  AND e.organization_id = b.organization_id
  AND e.resource_uuid = b.uuid;

INSERT INTO permissions (name, slug) VALUES
    ('Read activity history', 'tenant.activity.read')
ON CONFLICT (slug) DO NOTHING;

-- Owner and admin see every event; branch managers the events of their
-- branches.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug = 'tenant.activity.read'
WHERE r.slug IN ('owner', 'admin', 'sube_yoneticisi') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
