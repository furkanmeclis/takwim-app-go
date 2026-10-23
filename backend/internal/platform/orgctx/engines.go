package orgctx

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// TenantResourcePrefix marks organization-owned engine resources
// ("tenant.branches"). Platform resources use "platform.".
const TenantResourcePrefix = "tenant."

// IsTenantResource reports whether an engine resource slug is organization-owned.
func IsTenantResource(resource string) bool {
	return strings.HasPrefix(resource, TenantResourcePrefix)
}

// EngineScope returns the organization_id an io / bulk job for resource must
// be stamped with. Tenant resources need an organization on the context;
// platform resources must not run inside one, so a tenant route can never
// reach platform data through the shared engines and vice versa.
func EngineScope(ctx context.Context, resource string) (pgtype.Int8, error) {
	orgID, scoped := OrganizationID(ctx)
	switch {
	case IsTenantResource(resource) && !scoped:
		return pgtype.Int8{}, fmt.Errorf("resource %q needs an organization", resource)
	case !IsTenantResource(resource) && scoped:
		return pgtype.Int8{}, fmt.Errorf("resource %q is not organization-scoped", resource)
	case scoped:
		return pgtype.Int8{Int64: orgID, Valid: true}, nil
	default:
		return pgtype.Int8{}, nil
	}
}

// SameEngineScope reports whether a job stamped with jobOrg is visible from
// ctx: platform contexts see platform jobs only, tenant contexts see jobs of
// their own organization only.
func SameEngineScope(ctx context.Context, jobOrg pgtype.Int8) bool {
	orgID, scoped := OrganizationID(ctx)
	if !scoped {
		return !jobOrg.Valid
	}
	return jobOrg.Valid && jobOrg.Int64 == orgID
}
