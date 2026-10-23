DROP TABLE IF EXISTS branch_images;
DROP INDEX IF EXISTS uq_branches_organization_slug;
DROP TRIGGER IF EXISTS trg_branches_default_slug ON branches;
DROP FUNCTION IF EXISTS branches_default_slug();
ALTER TABLE branches
    DROP CONSTRAINT IF EXISTS branches_timezone_check,
    DROP COLUMN IF EXISTS show_online,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS slug;
