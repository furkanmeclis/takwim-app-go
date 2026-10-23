-- Data-loss note: drops the extra roles and the branch selection of every
-- invitation (each keeps its primary role_id), plus who accepted / revoked
-- it and the resend counters. Several open invitations per e-mail become
-- possible again.
DROP TABLE IF EXISTS organization_invitation_branches;
DROP TRIGGER IF EXISTS trg_organization_invitation_roles_scope_check ON organization_invitation_roles;
DROP TABLE IF EXISTS organization_invitation_roles;

DROP INDEX IF EXISTS uq_organization_invitations_open;
CREATE INDEX idx_organization_invitations_pending
    ON organization_invitations (organization_id, email)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

ALTER TABLE organization_invitations
    DROP CONSTRAINT IF EXISTS uq_organization_invitations_organization_id,
    DROP CONSTRAINT IF EXISTS chk_organization_invitations_branch_scope,
    DROP COLUMN IF EXISTS send_count,
    DROP COLUMN IF EXISTS last_sent_at,
    DROP COLUMN IF EXISTS revoked_by,
    DROP COLUMN IF EXISTS accepted_by,
    DROP COLUMN IF EXISTS branch_scope;
