CREATE TABLE organizations (
    id               BIGSERIAL PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    slug             VARCHAR(64)  NOT NULL,
    name             VARCHAR(255) NOT NULL,
    logo_object_key  TEXT         NULL,
    status           VARCHAR(16)  NOT NULL DEFAULT 'active',
    access_starts_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    access_ends_at   TIMESTAMPTZ  NULL,
    settings         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ  NULL,
    CONSTRAINT uq_organizations_uuid UNIQUE (uuid),
    CONSTRAINT chk_organizations_status CHECK (status IN ('active', 'suspended'))
);

CREATE UNIQUE INDEX uq_organizations_slug_live
    ON organizations (slug)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_organizations_status_live
    ON organizations (status)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_organizations_created_at_live
    ON organizations (created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TRIGGER trg_organizations_set_updated_at
    BEFORE UPDATE ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
