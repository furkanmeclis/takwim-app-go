-- TEC-970: the class timetable of a gym (classes module).
--
-- class_schedules: one weekly class of a group service (services with
-- group_capacity) in a branch: a weekday and a start time (branch wall
-- clock), the trainer and the area it runs with, the seats (NULL = the
-- service's group_capacity) and the dates it runs between. Every such
-- weekday in [valid_from, valid_until] is one session. A group class is
-- bookable only at the sessions of its timetable; a scheduled session
-- holds its trainer and area for other bookings even before its first
-- booking. The seats of a session are still guarded by the trigger of
-- 000056 (capacity_holds.session_seats carries the session's seats).
--
-- class_session_changes: one session that differs from its schedule (the
-- local date of the session): cancelled, or with other seats. A row that
-- would change nothing is deleted.

CREATE TABLE class_schedules (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    branch_id BIGINT NOT NULL,
    service_id BIGINT NOT NULL,
    weekday VARCHAR(3) NOT NULL CHECK (weekday IN ('mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun')),
    start_time VARCHAR(5) NOT NULL CHECK (start_time ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'),
    -- The trainer and the area of every session; NULL = picked by the first
    -- booking of the session (staff mode, resource kinds of the service).
    staff_id BIGINT NULL,
    resource_id BIGINT NULL,
    capacity INT NULL CHECK (capacity BETWEEN 1 AND 1000),
    valid_from DATE NOT NULL,
    valid_until DATE NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_class_schedules_organization_uuid UNIQUE (organization_id, uuid),
    CONSTRAINT uq_class_schedules_organization_id UNIQUE (organization_id, id),
    CONSTRAINT ck_class_schedules_validity CHECK (valid_until IS NULL OR valid_until >= valid_from),
    CONSTRAINT fk_class_schedules_branch FOREIGN KEY (organization_id, branch_id)
        REFERENCES branches (organization_id, id),
    CONSTRAINT fk_class_schedules_service FOREIGN KEY (organization_id, service_id)
        REFERENCES services (organization_id, id),
    CONSTRAINT fk_class_schedules_staff FOREIGN KEY (organization_id, staff_id)
        REFERENCES staff (organization_id, id) ON DELETE SET NULL (staff_id),
    CONSTRAINT fk_class_schedules_resource FOREIGN KEY (organization_id, resource_id)
        REFERENCES resources (organization_id, id) ON DELETE SET NULL (resource_id)
);

CREATE INDEX idx_class_schedules_branch ON class_schedules (organization_id, branch_id, service_id)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_class_schedules_staff ON class_schedules (organization_id, staff_id)
    WHERE deleted_at IS NULL AND staff_id IS NOT NULL;
CREATE INDEX idx_class_schedules_resource ON class_schedules (organization_id, resource_id)
    WHERE deleted_at IS NULL AND resource_id IS NOT NULL;

CREATE TRIGGER trg_class_schedules_updated_at
    BEFORE UPDATE ON class_schedules
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE class_session_changes (
    organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    schedule_id BIGINT NOT NULL,
    -- Local date of the session (branch time zone).
    session_date DATE NOT NULL,
    cancelled BOOLEAN NOT NULL DEFAULT FALSE,
    capacity INT NULL CHECK (capacity BETWEEN 1 AND 1000),
    reason TEXT NOT NULL DEFAULT '',
    updated_by BIGINT NULL REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, schedule_id, session_date),
    CONSTRAINT ck_class_session_changes_change CHECK (cancelled OR capacity IS NOT NULL),
    CONSTRAINT fk_class_session_changes_schedule FOREIGN KEY (organization_id, schedule_id)
        REFERENCES class_schedules (organization_id, id) ON DELETE CASCADE
);

CREATE TRIGGER trg_class_session_changes_updated_at
    BEFORE UPDATE ON class_session_changes
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Permissions: read the timetable and the sessions with their attendees;
-- manage the timetable and single sessions (cancel, seats). Attendance is
-- the reservation's own step (tenant.reservations.write).
INSERT INTO permissions (name, slug) VALUES
    ('Read class timetable', 'tenant.classes.read'),
    ('Manage class timetable', 'tenant.classes.write')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN ('tenant.classes.read', 'tenant.classes.write')
WHERE r.slug IN ('owner', 'admin', 'sube_yoneticisi') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug = 'tenant.classes.read'
WHERE r.slug = 'resepsiyon' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
