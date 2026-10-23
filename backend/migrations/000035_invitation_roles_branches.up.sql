-- Staff invitations with several roles and a branch scope (TEC-671).
--
-- organization_invitations.role_id stays the primary (display) role of the
-- invitation, like organization_members.role_id; organization_invitation_roles
-- holds every role the invitee gets on acceptance and
-- organization_invitation_branches the branches of a 'selected' scope.
-- Tokens are stored as sha256 hex (token_hash); the raw token only travels in
-- the invitation e-mail.

ALTER TABLE organization_invitations
    ADD COLUMN branch_scope VARCHAR(16) NOT NULL DEFAULT 'all',
    ADD COLUMN accepted_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN revoked_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN last_sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN send_count INTEGER NOT NULL DEFAULT 1,
    ADD CONSTRAINT chk_organization_invitations_branch_scope CHECK (branch_scope IN ('all', 'selected')),
    -- Target of the composite foreign keys below.
    ADD CONSTRAINT uq_organization_invitations_organization_id UNIQUE (organization_id, id);

-- One open invitation per e-mail and organization: re-inviting revokes the
-- previous one. Older duplicates (none expected) are revoked first.
UPDATE organization_invitations oi
SET revoked_at = NOW()
WHERE oi.accepted_at IS NULL AND oi.revoked_at IS NULL
  AND EXISTS (
    SELECT 1 FROM organization_invitations newer
    WHERE newer.organization_id = oi.organization_id
      AND newer.email = oi.email
      AND newer.accepted_at IS NULL AND newer.revoked_at IS NULL
      AND (newer.created_at, newer.id) > (oi.created_at, oi.id)
  );

DROP INDEX IF EXISTS idx_organization_invitations_pending;
CREATE UNIQUE INDEX uq_organization_invitations_open
    ON organization_invitations (organization_id, email)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

CREATE TABLE organization_invitation_roles (
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    invitation_id   BIGINT      NOT NULL,
    role_id         BIGINT      NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_organization_invitation_roles PRIMARY KEY (organization_id, invitation_id, role_id),
    CONSTRAINT fk_organization_invitation_roles_invitation
        FOREIGN KEY (organization_id, invitation_id)
        REFERENCES organization_invitations (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_organization_invitation_roles_organization_role
    ON organization_invitation_roles (organization_id, role_id);

-- Same rule as member roles: tenant roles of this organization or templates.
CREATE TRIGGER trg_organization_invitation_roles_scope_check
    BEFORE INSERT OR UPDATE ON organization_invitation_roles
    FOR EACH ROW
    EXECUTE FUNCTION organization_member_roles_scope_check();

CREATE TABLE organization_invitation_branches (
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    invitation_id   BIGINT      NOT NULL,
    branch_id       BIGINT      NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_organization_invitation_branches PRIMARY KEY (organization_id, invitation_id, branch_id),
    CONSTRAINT fk_organization_invitation_branches_invitation
        FOREIGN KEY (organization_id, invitation_id)
        REFERENCES organization_invitations (organization_id, id) ON DELETE CASCADE,
    -- An invitation can only grant branches of its own organization.
    CONSTRAINT fk_organization_invitation_branches_branch
        FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_organization_invitation_branches_organization_branch
    ON organization_invitation_branches (organization_id, branch_id);

-- Existing invitations keep their single role.
INSERT INTO organization_invitation_roles (organization_id, invitation_id, role_id)
SELECT organization_id, id, role_id
FROM organization_invitations
ON CONFLICT DO NOTHING;
