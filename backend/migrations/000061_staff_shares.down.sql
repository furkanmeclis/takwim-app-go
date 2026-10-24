-- Data-loss note: drops every staff share rule, calculated share and share
-- payout (the payouts' cash movements stay in the cash book), and the
-- tenant.earnings.manage / tenant.earnings.pay permissions.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN ('tenant.earnings.manage', 'tenant.earnings.pay')
);

DELETE FROM permissions WHERE slug IN ('tenant.earnings.manage', 'tenant.earnings.pay');

DROP TABLE IF EXISTS staff_share_payouts;
DROP FUNCTION IF EXISTS staff_share_payouts_append_only();
DROP TABLE IF EXISTS staff_share_lines;
DROP FUNCTION IF EXISTS staff_share_lines_insert_rules();
DROP FUNCTION IF EXISTS staff_share_lines_append_only();
DROP TABLE IF EXISTS staff_share_rules;
