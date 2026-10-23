-- Data-loss note: drops every stored organization settings version.
DROP TRIGGER IF EXISTS trg_organization_settings_versions_immutable ON organization_settings_versions;
DROP FUNCTION IF EXISTS organization_settings_versions_immutable();
DROP TABLE IF EXISTS organization_settings_versions;
