package branches

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// RegisterLiveResources declares the branches list channel
// (org:{org_uuid}:resource:tenant.branches) and its tenant read permission.
// reg is the tenant registry checked against the organization membership.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantBranchesRead)
}
