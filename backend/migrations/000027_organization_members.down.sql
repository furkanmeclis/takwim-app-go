DROP TRIGGER IF EXISTS trg_organization_members_role_scope_check ON organization_members;
DROP FUNCTION IF EXISTS organization_members_role_scope_check();
DROP TRIGGER IF EXISTS trg_organization_members_set_updated_at ON organization_members;
DROP TABLE IF EXISTS organization_members;
