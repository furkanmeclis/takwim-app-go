-- TEC-903: starter records a new business gets from its sector template.

-- name: CreateTemplateService :one
INSERT INTO services (
    organization_id, name, description, duration_minutes, price, payment_mode, staff_mode, resource_kinds, show_online,
    group_capacity
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(duration_minutes), 0,
    sqlc.arg(payment_mode), sqlc.arg(staff_mode), sqlc.arg(resource_kinds)::TEXT[], TRUE, sqlc.narg(group_capacity)
)
RETURNING id, uuid;

-- name: CreateTemplateServiceOffer :exec
INSERT INTO service_branches (organization_id, service_id, branch_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(service_id), sqlc.arg(branch_id));

-- name: CreateTemplateResource :one
INSERT INTO resources (
    organization_id, branch_id, kind, name, zone, min_capacity, capacity, combinable, sort_order
)
VALUES (
    sqlc.arg(organization_id), sqlc.arg(branch_id), sqlc.arg(kind), sqlc.arg(name), sqlc.arg(zone),
    sqlc.arg(min_capacity), sqlc.arg(capacity), sqlc.arg(combinable), sqlc.arg(sort_order)
)
RETURNING id, uuid;
