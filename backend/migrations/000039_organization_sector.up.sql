-- TEC-882: every business belongs to one of the six sectors. The sector
-- catalog (booking flow, capacity, terms, visible modules) lives in code
-- (internal/platform/sector); this column only stores the choice.
ALTER TABLE organizations
    ADD COLUMN sector VARCHAR(32) NOT NULL DEFAULT 'hairdresser',
    ADD CONSTRAINT organizations_sector_check CHECK (
        sector IN ('hairdresser', 'restaurant', 'gym', 'car_wash', 'clinic', 'beauty_salon')
    );
