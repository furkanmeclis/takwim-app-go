-- Data-loss note: removes every per-product staff share rule. Shares
-- already calculated with one keep their amounts; their rule scope becomes
-- 'default' (the append-only trigger is paused for that one update).
ALTER TABLE staff_share_lines DISABLE TRIGGER trg_staff_share_lines_append_only;
UPDATE staff_share_lines SET rule_scope = 'default' WHERE rule_scope = 'product';
ALTER TABLE staff_share_lines ENABLE TRIGGER trg_staff_share_lines_append_only;

ALTER TABLE staff_share_lines DROP CONSTRAINT staff_share_lines_rule_scope_check;
ALTER TABLE staff_share_lines
    ADD CONSTRAINT staff_share_lines_rule_scope_check CHECK (rule_scope IN ('default', 'service'));

DELETE FROM staff_share_rules WHERE product_id IS NOT NULL;

DROP INDEX uq_staff_share_rules_slot;
CREATE UNIQUE INDEX uq_staff_share_rules_slot
    ON staff_share_rules (organization_id, staff_id, item_type, COALESCE(service_id, 0), valid_from);

ALTER TABLE staff_share_rules
    DROP CONSTRAINT fk_staff_share_rules_product,
    DROP CONSTRAINT ck_staff_share_rules_product,
    DROP COLUMN product_id;
