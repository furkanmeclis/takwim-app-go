package organizations

import (
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// RegisterLiveResources declares the tenant member and role list channels
// (org:{org_uuid}:resource:tenant.members / tenant.roles) and their tenant
// read permissions. reg is the tenant registry checked against the
// organization membership.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(orghandler.ResourceTenantMembers, rbac.PermTenantMembersRead)
	reg.Register(orghandler.ResourceTenantRoles, rbac.PermTenantRolesRead)
}
