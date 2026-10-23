ALTER TABLE reservations
    DROP COLUMN IF EXISTS vehicle_plate,
    DROP COLUMN IF EXISTS vehicle_type;

-- Shared session allocations would violate the exclusive constraints.
DELETE FROM capacity_allocations WHERE session_key IS NOT NULL;

ALTER TABLE capacity_allocations
    DROP CONSTRAINT IF EXISTS ex_capacity_allocations_staff,
    DROP CONSTRAINT IF EXISTS ex_capacity_allocations_resource;
ALTER TABLE capacity_allocations
    ADD CONSTRAINT ex_capacity_allocations_staff EXCLUDE USING gist (
        organization_id WITH =, staff_id WITH =, period WITH &&
    ) WHERE (staff_id IS NOT NULL),
    ADD CONSTRAINT ex_capacity_allocations_resource EXCLUDE USING gist (
        organization_id WITH =, resource_id WITH =, period WITH &&
    ) WHERE (resource_id IS NOT NULL);

DROP TRIGGER IF EXISTS trg_capacity_allocations_session_key ON capacity_allocations;
DROP FUNCTION IF EXISTS capacity_allocations_session_key();
ALTER TABLE capacity_allocations DROP COLUMN IF EXISTS session_key;

DROP TRIGGER IF EXISTS trg_capacity_holds_seats ON capacity_holds;
DROP FUNCTION IF EXISTS capacity_holds_check_seats();
DROP INDEX IF EXISTS idx_capacity_holds_session;
ALTER TABLE capacity_holds
    DROP CONSTRAINT IF EXISTS ck_capacity_holds_session,
    DROP COLUMN IF EXISTS session_seats,
    DROP COLUMN IF EXISTS seats,
    DROP COLUMN IF EXISTS session_service_id;

ALTER TABLE services DROP COLUMN IF EXISTS group_capacity;
