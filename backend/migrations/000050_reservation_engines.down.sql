-- Drops the reservation engine permissions (and every role grant of them).
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE slug IN (
        'tenant.reservations.export',
        'tenant.reservations.bulk.accept',
        'tenant.reservations.bulk.reject',
        'tenant.reservations.bulk.cancel'
    )
);

DELETE FROM permissions
WHERE slug IN (
    'tenant.reservations.export',
    'tenant.reservations.bulk.accept',
    'tenant.reservations.bulk.reject',
    'tenant.reservations.bulk.cancel'
);
