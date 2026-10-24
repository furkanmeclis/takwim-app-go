-- TEC-905: a reservation keeps the business settings version it was booked
-- under and the definitions of the booking form questions it answered
-- (label, type, options), so later changes to the form or the booking rules
-- never rewrite past records (requirements §6, A06).

ALTER TABLE reservations
    ADD COLUMN settings_version INT NULL CHECK (settings_version IS NULL OR settings_version >= 0),
    ADD COLUMN custom_field_defs JSONB NOT NULL DEFAULT '[]'
        CONSTRAINT ck_reservations_custom_field_defs CHECK (jsonb_typeof(custom_field_defs) = 'array');

-- The online booking limit per customer counts the customer's open bookings.
CREATE INDEX idx_reservations_customer_open ON reservations (organization_id, customer_id, ends_at)
    WHERE status IN ('requested', 'active', 'in_service');
