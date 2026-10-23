CREATE TABLE organization_members (
    id              BIGSERIAL PRIMARY KEY,
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id         BIGINT      NOT NULL REFERENCES roles (id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_organization_members_org_user UNIQUE (organization_id, user_id)
);

CREATE INDEX idx_organization_members_user_id ON organization_members (user_id);
CREATE INDEX idx_organization_members_role_id ON organization_members (role_id);

CREATE TRIGGER trg_organization_members_set_updated_at
    BEFORE UPDATE ON organization_members
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE OR REPLACE FUNCTION organization_members_role_scope_check()
RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM roles r
        WHERE r.id = NEW.role_id
          AND r.scope = 'tenant'
          AND (r.organization_id IS NULL OR r.organization_id = NEW.organization_id)
    ) THEN
        RAISE EXCEPTION 'organization member role must be tenant-scoped for this organization'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_organization_members_role_scope_check
    BEFORE INSERT OR UPDATE OF role_id, organization_id ON organization_members
    FOR EACH ROW
    EXECUTE FUNCTION organization_members_role_scope_check();
