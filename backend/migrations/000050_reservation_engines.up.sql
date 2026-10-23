-- Reservation engines (TEC-963): export, bulk accept / reject / cancel.
-- Reservations are never imported (a booking goes through the capacity
-- engine), so there is no import permission.
INSERT INTO permissions (name, slug) VALUES
    ('Export reservations', 'tenant.reservations.export'),
    ('Bulk accept reservations', 'tenant.reservations.bulk.accept'),
    ('Bulk reject reservations', 'tenant.reservations.bulk.reject'),
    ('Bulk cancel reservations', 'tenant.reservations.bulk.cancel')
ON CONFLICT (slug) DO NOTHING;

-- Owner, admin and branch manager: everything (inside their branch scope).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.reservations.export', 'tenant.reservations.bulk.accept',
    'tenant.reservations.bulk.reject', 'tenant.reservations.bulk.cancel')
WHERE r.slug IN ('owner', 'admin', 'sube_yoneticisi') AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;

-- Reception answers the request inbox in bulk; cancelling many bookings
-- and exporting customer data stay with the managers.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.slug IN (
    'tenant.reservations.bulk.accept', 'tenant.reservations.bulk.reject')
WHERE r.slug = 'resepsiyon' AND r.scope = 'tenant' AND r.organization_id IS NULL
ON CONFLICT DO NOTHING;
