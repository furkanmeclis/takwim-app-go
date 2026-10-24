-- Data-loss note: drops every income / expense record and category. The
-- cash movements they posted stay in the cash book (kind income / expense)
-- and the documents stay in storage.
DROP TABLE IF EXISTS finance_entries;
DROP FUNCTION IF EXISTS finance_entries_insert_rules();
DROP FUNCTION IF EXISTS finance_entries_append_only();
DROP TRIGGER IF EXISTS trg_organizations_finance_categories ON organizations;
DROP FUNCTION IF EXISTS finance_categories_seed_organization();
DROP FUNCTION IF EXISTS finance_seed_default_categories(BIGINT);
DROP TABLE IF EXISTS finance_categories;
