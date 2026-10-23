-- TEC-883: plan and add-on entitlements, assigned by the platform. The
-- plan catalog (default limits) lives in code (internal/platform/
-- entitlement); a business stores its plan, its granted add-ons and
-- optional limit overrides (NULL = the plan default).
ALTER TABLE organizations
    ADD COLUMN plan VARCHAR(32) NOT NULL DEFAULT 'starter',
    ADD COLUMN addons TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN branch_limit INT,
    ADD COLUMN staff_limit INT,
    ADD CONSTRAINT organizations_plan_check CHECK (plan IN ('starter', 'growth', 'enterprise')),
    ADD CONSTRAINT organizations_addons_check CHECK (addons <@ ARRAY['confirmation']::text[]),
    ADD CONSTRAINT organizations_branch_limit_check CHECK (branch_limit IS NULL OR branch_limit >= 1),
    ADD CONSTRAINT organizations_staff_limit_check CHECK (staff_limit IS NULL OR staff_limit >= 1);

-- Businesses that exist before plans keep working as they did: no limits
-- until the platform assigns a plan. New businesses start on 'starter'.
UPDATE organizations SET plan = 'enterprise';

-- Monthly communication usage per business (SMS, WhatsApp, e-mail, voice
-- calls). Konfirmasyon right and channel usage are tracked separately
-- (requirements §16): the add-on is a column above, the usage is here.
-- period is the first day of the month in the business time zone.
CREATE TABLE organization_usage (
    organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    metric VARCHAR(32) NOT NULL CHECK (metric IN ('sms', 'whatsapp', 'email', 'voice_call')),
    period DATE NOT NULL CHECK (EXTRACT(DAY FROM period) = 1),
    quantity BIGINT NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, metric, period)
);
