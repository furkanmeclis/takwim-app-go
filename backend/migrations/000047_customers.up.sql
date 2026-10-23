-- TEC-896: customers (müşteriler). Two layers, kept apart on purpose:
--
--   * customer_identities is the platform-level person: one row per
--     verified E.164 phone, with an internal permanent uuid. No business
--     reads it through the tenant API.
--   * customers is the business-level relation: name, contact, notes, tags
--     and consent as this business keeps them. Edits stay in the business.
--
-- customer_identity_links joins a local record to an identity only after
-- the phone was proven (OTP, customer panel login) and keeps that trail.
-- A phone typed by staff or imported from a file never links, so a match
-- on an unverified number opens no history.

CREATE TABLE customer_identities (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    phone_e164 VARCHAR(16) NOT NULL UNIQUE,
    -- Last time the person proved the phone.
    phone_verified_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_customer_identities_phone CHECK (phone_e164 ~ '^\+[1-9][0-9]{6,14}$')
);

CREATE TRIGGER trg_customer_identities_updated_at
    BEFORE UPDATE ON customer_identities
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE customers (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    first_name VARCHAR(80) NOT NULL,
    last_name VARCHAR(80) NOT NULL DEFAULT '',
    -- E.164; NULL when the business has no number (walk-in, child).
    phone_e164 VARCHAR(16) NULL CHECK (phone_e164 IS NULL OR phone_e164 ~ '^\+[1-9][0-9]{6,14}$'),
    email VARCHAR(254) NOT NULL DEFAULT '',
    birth_date DATE NULL,
    notes TEXT NOT NULL DEFAULT '',
    tags TEXT[] NOT NULL DEFAULT '{}',
    -- Commercial message consent (İYS); the time of the last change.
    sms_consent BOOLEAN NOT NULL DEFAULT FALSE,
    email_consent BOOLEAN NOT NULL DEFAULT FALSE,
    consent_updated_at TIMESTAMPTZ NULL,
    -- Where the record came from: the panel, a file import or online booking.
    source VARCHAR(16) NOT NULL DEFAULT 'panel' CHECK (source IN ('panel', 'import', 'online')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_customers_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_customers_organization_id UNIQUE (organization_id, id)
);

-- One live record per phone in a business (reception finds the customer
-- by number).
CREATE UNIQUE INDEX uq_customers_phone
    ON customers (organization_id, phone_e164)
    WHERE deleted_at IS NULL AND phone_e164 IS NOT NULL;

CREATE INDEX idx_customers_organization_name
    ON customers (organization_id, lower(first_name), lower(last_name))
    WHERE deleted_at IS NULL;

CREATE INDEX idx_customers_tags ON customers USING GIN (tags);

CREATE TRIGGER trg_customers_updated_at
    BEFORE UPDATE ON customers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A verified link between a local record and the shared identity. One
-- identity has at most one local record per business.
CREATE TABLE customer_identity_links (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL,
    identity_id BIGINT NOT NULL REFERENCES customer_identities (id) ON DELETE CASCADE,
    -- How the phone was proven.
    source VARCHAR(16) NOT NULL CHECK (source IN ('otp', 'panel_login')),
    verified_phone_e164 VARCHAR(16) NOT NULL,
    verified_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_customer_identity_links_customer UNIQUE (organization_id, customer_id),
    CONSTRAINT uq_customer_identity_links_identity UNIQUE (organization_id, identity_id),
    CONSTRAINT fk_customer_identity_links_customer FOREIGN KEY (organization_id, customer_id)
        REFERENCES customers (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_customer_identity_links_identity ON customer_identity_links (identity_id);

INSERT INTO permissions (name, slug) VALUES
    ('Export customers', 'tenant.customers.export'),
    ('Import customers', 'tenant.customers.import'),
    ('Bulk tag customers', 'tenant.customers.bulk.tag'),
    ('Bulk delete customers', 'tenant.customers.bulk.delete')
ON CONFLICT (slug) DO NOTHING;

-- Owner and admin: everything. Branch manager: export and tagging.
-- Reception keeps read / write from TEC-889.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.customers.export', 'tenant.customers.import', 'tenant.customers.bulk.tag', 'tenant.customers.bulk.delete')
WHERE r.slug IN ('owner', 'admin') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.customers.export', 'tenant.customers.bulk.tag')
WHERE r.slug = 'sube_yoneticisi' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
