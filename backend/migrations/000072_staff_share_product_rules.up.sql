-- TEC-976: a staff member's rate for one product (docs/design/staff-shares.md).
--
-- staff_share_rules gets product_id: NULL is the default of the item type,
-- set is the rate of one product (item_type product), the same way
-- service_id overrides the service default. A product's own rule wins over
-- the default and the latest valid_from on or before the sale day is used,
-- so shares written before keep their values. staff_share_lines records
-- the product scope.

ALTER TABLE staff_share_rules
    ADD COLUMN product_id BIGINT NULL,
    ADD CONSTRAINT ck_staff_share_rules_product CHECK (product_id IS NULL OR item_type = 'product'),
    ADD CONSTRAINT fk_staff_share_rules_product FOREIGN KEY (organization_id, product_id)
        REFERENCES products (organization_id, id) ON DELETE CASCADE;

-- One rule per staff member, item type, service or product and start day.
DROP INDEX uq_staff_share_rules_slot;
CREATE UNIQUE INDEX uq_staff_share_rules_slot
    ON staff_share_rules (
        organization_id, staff_id, item_type, COALESCE(service_id, 0), COALESCE(product_id, 0), valid_from
    );

ALTER TABLE staff_share_lines DROP CONSTRAINT staff_share_lines_rule_scope_check;
ALTER TABLE staff_share_lines
    ADD CONSTRAINT staff_share_lines_rule_scope_check CHECK (rule_scope IN ('default', 'service', 'product'));
