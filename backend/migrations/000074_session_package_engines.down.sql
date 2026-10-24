DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE slug IN (
        'tenant.session_packages.export', 'tenant.session_packages.bulk.activate',
        'tenant.session_packages.bulk.deactivate'
    )
);

DELETE FROM permissions WHERE slug IN (
    'tenant.session_packages.export', 'tenant.session_packages.bulk.activate',
    'tenant.session_packages.bulk.deactivate'
);
