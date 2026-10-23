ALTER TABLE branches DROP COLUMN IF EXISTS timezone;

ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS organizations_currency_check,
    DROP COLUMN IF EXISTS currency,
    DROP COLUMN IF EXISTS timezone;
