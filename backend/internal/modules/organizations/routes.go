package organizations

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

func RegisterRoutes(
	mux *http.ServeMux,
	h *orghandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	read := middleware.RequirePermission(rbac.PermPlatformOrganizationsRead)
	write := middleware.RequirePermission(rbac.PermPlatformOrganizationsWrite)

	mux.Handle("GET /v1/platform/organizations/meta", middleware.Chain(
		http.HandlerFunc(h.Meta), authn, read,
	))
	mux.Handle("GET /v1/platform/organizations", middleware.Chain(
		http.HandlerFunc(h.List), authn, read,
	))
	mux.Handle("POST /v1/platform/organizations", middleware.Chain(
		http.HandlerFunc(h.Create), authn, write,
	))
	mux.Handle("GET /v1/platform/organizations/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Get), authn, read,
	))
	mux.Handle("PATCH /v1/platform/organizations/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Patch), authn, write,
	))
	mux.Handle("DELETE /v1/platform/organizations/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Delete), authn, write,
	))
	mux.Handle("POST /v1/platform/organizations/{uuid}/status", middleware.Chain(
		http.HandlerFunc(h.SetStatus), authn, write,
	))
	mux.Handle("POST /v1/platform/organizations/{uuid}/access", middleware.Chain(
		http.HandlerFunc(h.SetAccess), authn, write,
	))
	mux.Handle("GET /v1/platform/organizations/{uuid}/members", middleware.Chain(
		http.HandlerFunc(h.ListMembers), authn, read,
	))
	mux.Handle("POST /v1/platform/organizations/{uuid}/members", middleware.Chain(
		http.HandlerFunc(h.AddMember), authn, write,
	))
	mux.Handle("PATCH /v1/platform/organizations/{uuid}/members/{userUuid}", middleware.Chain(
		http.HandlerFunc(h.PatchMember), authn, write,
	))
	mux.Handle("DELETE /v1/platform/organizations/{uuid}/members/{userUuid}", middleware.Chain(
		http.HandlerFunc(h.DeleteMember), authn, write,
	))
}

// RegisterTenantRoutes mounts the tenant shell's member and role management
// (/v1/tenant/members, /v1/tenant/roles). A nil resolver (tenancy off) mounts
// nothing. Permissions come from the membership only (no super_admin bypass).
func RegisterTenantRoutes(
	mux *http.ServeMux,
	h *orghandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	perm := middleware.RequireTenantPermission
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	membersRead := perm(rbac.PermTenantMembersRead)
	membersWrite := perm(rbac.PermTenantMembersWrite)
	rolesRead := perm(rbac.PermTenantRolesRead)
	rolesWrite := perm(rbac.PermTenantRolesWrite)

	route("GET /v1/tenant/members/meta", h.TenantMembersMetaH, membersRead)
	route("GET /v1/tenant/members", h.TenantListMembers, membersRead)
	route("GET /v1/tenant/members/{userUuid}", h.TenantGetMember, membersRead)
	route("PATCH /v1/tenant/members/{userUuid}", h.TenantPatchMember, membersWrite)
	route("DELETE /v1/tenant/members/{userUuid}", h.TenantDeleteMember, membersWrite)

	route("GET /v1/tenant/roles/meta", h.TenantRolesMetaH, rolesRead)
	route("GET /v1/tenant/roles/permissions", h.TenantListPermissions, rolesRead)
	route("GET /v1/tenant/roles", h.TenantListRoles, rolesRead)
	route("POST /v1/tenant/roles", h.TenantCreateRole, rolesWrite)
	route("GET /v1/tenant/roles/{uuid}", h.TenantGetRole, rolesRead)
	route("PATCH /v1/tenant/roles/{uuid}", h.TenantPatchRole, rolesWrite)
	route("DELETE /v1/tenant/roles/{uuid}", h.TenantDeleteRole, rolesWrite)
}
