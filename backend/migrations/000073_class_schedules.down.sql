-- Data-loss note: drops the class timetable, every single-session change
-- and the tenant.classes.* permissions. Bookings of group classes stay.
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('tenant.classes.read', 'tenant.classes.write'));

DELETE FROM permissions WHERE slug IN ('tenant.classes.read', 'tenant.classes.write');

DROP TABLE IF EXISTS class_session_changes;
DROP TABLE IF EXISTS class_schedules;
