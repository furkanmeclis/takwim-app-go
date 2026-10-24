-- TEC-966: business page theme (docs/design/business-page.md, "Theme").
--
-- The colours, font and texts of the page live in the versioned settings
-- document (organization_settings_versions, section "theme"). The cover
-- image is an object in storage like the logo; its key is a column of the
-- organization so an upload does not create a settings version and the
-- public profile can stream it by slug.

ALTER TABLE organizations
    ADD COLUMN cover_object_key TEXT NULL;
