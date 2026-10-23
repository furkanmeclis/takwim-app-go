ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS organizations_sector_check,
    DROP COLUMN IF EXISTS sector;
