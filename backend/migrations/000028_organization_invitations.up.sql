CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE organization_invitations (
    id              BIGSERIAL PRIMARY KEY,
    uuid            UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    email           CITEXT       NOT NULL,
    role_id         BIGINT       NOT NULL REFERENCES roles (id),
    token_hash      TEXT         NOT NULL,
    expires_at      TIMESTAMPTZ  NOT NULL,
    accepted_at     TIMESTAMPTZ  NULL,
    revoked_at      TIMESTAMPTZ  NULL,
    invited_by      BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_organization_invitations_uuid UNIQUE (uuid),
    CONSTRAINT uq_organization_invitations_token_hash UNIQUE (token_hash)
);

CREATE INDEX idx_organization_invitations_org_created
    ON organization_invitations (organization_id, created_at DESC);
CREATE INDEX idx_organization_invitations_email
    ON organization_invitations (email);
CREATE INDEX idx_organization_invitations_pending
    ON organization_invitations (organization_id, email)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

CREATE OR REPLACE FUNCTION organization_invitations_role_scope_check()
RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roles r
        WHERE r.id = NEW.role_id
          AND r.scope = 'tenant'
          AND (r.organization_id IS NULL OR r.organization_id = NEW.organization_id)
    ) THEN
        RAISE EXCEPTION 'organization invitation role must be tenant-scoped for this organization'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_organization_invitations_role_scope_check
    BEFORE INSERT OR UPDATE OF role_id, organization_id ON organization_invitations
    FOR EACH ROW
    EXECUTE FUNCTION organization_invitations_role_scope_check();
