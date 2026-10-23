-- TEC-904: group class seats (gym) and the vehicle of a car wash booking.
--
-- A service with group_capacity is a group class: every start of it in a
-- branch is one session with that many seats. The first booking of a
-- session holds its trainer and area like any booking; later bookings of
-- the same session share them. capacity_holds carries the session
-- (service, branch, start) and the seats a booking takes; a trigger counts
-- the seats under a lock of the session, so the last seat is never given
-- to two writers (A03), and fills capacity_allocations.session_key so the
-- allocations of one session may overlap while everything else stays
-- exclusive.

ALTER TABLE services
    ADD COLUMN group_capacity INT NULL CHECK (group_capacity BETWEEN 2 AND 1000);

ALTER TABLE capacity_holds
    ADD COLUMN session_service_id BIGINT NULL,
    ADD COLUMN seats INT NULL,
    ADD COLUMN session_seats INT NULL,
    ADD CONSTRAINT ck_capacity_holds_session CHECK (
        (session_service_id IS NULL AND seats IS NULL AND session_seats IS NULL)
        OR (session_service_id IS NOT NULL AND seats >= 1 AND session_seats >= seats)
    );

CREATE INDEX idx_capacity_holds_session
    ON capacity_holds (organization_id, session_service_id, branch_id, starts_at)
    WHERE session_service_id IS NOT NULL AND status = 'active';

CREATE FUNCTION capacity_holds_check_seats() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    taken INT;
BEGIN
    IF NEW.session_service_id IS NULL OR NEW.status <> 'active' THEN
        RETURN NEW;
    END IF;
    -- One writer per session at a time; the count below then sees every
    -- committed booking of the session.
    PERFORM pg_advisory_xact_lock(hashtextextended(
        'session:' || NEW.organization_id || ':' || NEW.session_service_id || ':' || NEW.branch_id || ':' || extract(epoch FROM NEW.starts_at), 0));
    SELECT COALESCE(SUM(seats), 0) INTO taken
    FROM capacity_holds
    WHERE organization_id = NEW.organization_id AND session_service_id = NEW.session_service_id
      AND branch_id = NEW.branch_id AND starts_at = NEW.starts_at AND status = 'active' AND id <> NEW.id;
    IF taken + NEW.seats > NEW.session_seats THEN
        RAISE EXCEPTION 'group session is full' USING ERRCODE = 'exclusion_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_capacity_holds_seats
    BEFORE INSERT OR UPDATE OF status, seats, session_seats, session_service_id, starts_at ON capacity_holds
    FOR EACH ROW EXECUTE FUNCTION capacity_holds_check_seats();

ALTER TABLE capacity_allocations ADD COLUMN session_key TEXT NULL;

CREATE FUNCTION capacity_allocations_session_key() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    SELECT h.session_service_id || ':' || h.branch_id || ':' || extract(epoch FROM h.starts_at)
    INTO NEW.session_key
    FROM capacity_holds h
    WHERE h.organization_id = NEW.organization_id AND h.id = NEW.hold_id AND h.session_service_id IS NOT NULL;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_capacity_allocations_session_key
    BEFORE INSERT ON capacity_allocations
    FOR EACH ROW EXECUTE FUNCTION capacity_allocations_session_key();

-- Allocations of one group session share the trainer and the area; every
-- other pair of allocations of one holder still never overlaps.
ALTER TABLE capacity_allocations
    DROP CONSTRAINT ex_capacity_allocations_staff,
    DROP CONSTRAINT ex_capacity_allocations_resource;
ALTER TABLE capacity_allocations
    ADD CONSTRAINT ex_capacity_allocations_staff EXCLUDE USING gist (
        organization_id WITH =, staff_id WITH =, period WITH &&,
        (COALESCE(session_key, 'hold:' || hold_id::TEXT)) WITH <>
    ) WHERE (staff_id IS NOT NULL),
    ADD CONSTRAINT ex_capacity_allocations_resource EXCLUDE USING gist (
        organization_id WITH =, resource_id WITH =, period WITH &&,
        (COALESCE(session_key, 'hold:' || hold_id::TEXT)) WITH <>
    ) WHERE (resource_id IS NOT NULL);

-- Car wash: the vehicle of the booking (requirements §5, araç türü/plaka).
ALTER TABLE reservations
    ADD COLUMN vehicle_type VARCHAR(16) NULL
        CHECK (vehicle_type IN ('car', 'suv', 'van', 'motorcycle', 'other')),
    ADD COLUMN vehicle_plate VARCHAR(16) NOT NULL DEFAULT '';
