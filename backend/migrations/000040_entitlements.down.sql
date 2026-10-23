DROP TABLE IF EXISTS organization_usage;

ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS organizations_staff_limit_check,
    DROP CONSTRAINT IF EXISTS organizations_branch_limit_check,
    DROP CONSTRAINT IF EXISTS organizations_addons_check,
    DROP CONSTRAINT IF EXISTS organizations_plan_check,
    DROP COLUMN IF EXISTS staff_limit,
    DROP COLUMN IF EXISTS branch_limit,
    DROP COLUMN IF EXISTS addons,
    DROP COLUMN IF EXISTS plan;
