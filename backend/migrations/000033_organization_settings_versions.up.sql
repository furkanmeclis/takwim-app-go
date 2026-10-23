-- Versioned organization settings (TEC-690). Every save inserts a new row
-- with version = previous + 1; rows are immutable and the current settings
-- are the row with the highest version.

CREATE TABLE organization_settings_versions (
    id              BIGSERIAL PRIMARY KEY,
    organization_id BIGINT      NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    version         INTEGER     NOT NULL CHECK (version > 0),
    data            JSONB       NOT NULL,
    created_by      BIGINT      NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_organization_settings_versions_organization_version UNIQUE (organization_id, version)
);

CREATE OR REPLACE FUNCTION organization_settings_versions_immutable()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'organization settings versions are immutable'
        USING ERRCODE = '55000';
END;
$$ LANGUAGE plpgsql;

-- Content columns never change. created_by may still be cleared by the
-- ON DELETE SET NULL of a removed user.
CREATE TRIGGER trg_organization_settings_versions_immutable
    BEFORE UPDATE OF organization_id, version, data, created_at ON organization_settings_versions
    FOR EACH ROW
    EXECUTE FUNCTION organization_settings_versions_immutable();
