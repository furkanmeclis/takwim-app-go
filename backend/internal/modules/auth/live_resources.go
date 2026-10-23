package auth

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// RegisterLiveResources declares the live list resources of this module and
// the permission a client needs to subscribe to their channels.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(usecase.ResourceUsers, rbac.PermPlatformUsersRead)
	reg.Register(usecase.ResourceRoles, rbac.PermPlatformRolesRead)
}
