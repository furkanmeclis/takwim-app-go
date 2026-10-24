package organizations

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// PlatformEngineHandlers are the shared engine handlers of the platform
// organizations list (export / bulk). Nil handlers are not mounted.
type PlatformEngineHandlers struct {
	Export http.HandlerFunc
	Bulk   http.HandlerFunc
	// StepUp guards the export like the other platform exports (nil = none).
	StepUp func(http.Handler) http.Handler
}

// RegisterRoutes mounts platform organization management, the public
// invitation accept flow and the organization logo stream.
func RegisterRoutes(
	mux *http.ServeMux,
	h *orghandler.Handler,
	engines PlatformEngineHandlers,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	read := middleware.RequirePermission(rbac.PermPlatformOrganizationsRead)
	write := middleware.RequirePermission(rbac.PermPlatformOrganizationsWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/platform/organizations/meta", h.Meta, read)
	route("GET /v1/platform/organizations", h.List, read)
	route("POST /v1/platform/organizations", h.Create, write)
	if engines.Export != nil {
		guards := []func(http.Handler) http.Handler{read, middleware.RequirePermission(rbac.PermPlatformOrganizationsExport)}
		if engines.StepUp != nil {
			guards = append(guards, engines.StepUp)
		}
		route("POST /v1/platform/organizations/export", engines.Export, guards...)
	}
	if engines.Bulk != nil {
		// The bulk engine checks the action permission.
		route("POST /v1/platform/organizations/bulk", engines.Bulk, read)
	}
	route("GET /v1/platform/organizations/{uuid}", h.Get, read)
	route("PATCH /v1/platform/organizations/{uuid}", h.Patch, write)
	route("DELETE /v1/platform/organizations/{uuid}", h.Delete, write)
	route("POST /v1/platform/organizations/{uuid}/status", h.SetStatus, write)
	route("POST /v1/platform/organizations/{uuid}/access", h.SetAccess, write)
	route("GET /v1/platform/organizations/{uuid}/activity", h.ListActivity, read)
	route("GET /v1/platform/organizations/{uuid}/roles", h.ListRoles, read)
	route("GET /v1/platform/organizations/{uuid}/branches", h.ListBranches, read)
	route("GET /v1/platform/organizations/{uuid}/members", h.ListMembers, read)
	route("POST /v1/platform/organizations/{uuid}/members", h.AddMember, write)
	route("PATCH /v1/platform/organizations/{uuid}/members/{userUuid}", h.PatchMember, write)
	route("DELETE /v1/platform/organizations/{uuid}/members/{userUuid}", h.DeleteMember, write)
	route("GET /v1/platform/organizations/{uuid}/invitations", h.ListPlatformInvitations, read)
	route("POST /v1/platform/organizations/{uuid}/invitations", h.CreatePlatformInvitation, write)
	route("POST /v1/platform/organizations/{uuid}/invitations/{invitationUuid}/revoke", h.RevokePlatformInvitation, write)
	route("POST /v1/platform/organizations/{uuid}/invitations/{invitationUuid}/resend", h.ResendPlatformInvitation, write)

	// Organization logo for any signed-in user (sidebar, switcher, lists).
	route("GET /v1/organizations/{uuid}/logo", h.StreamLogo)

	// Invitation accept flow: public preview / registration, signed-in accept.
	mux.HandleFunc("GET /v1/invitations/{token}", h.PreviewInvitation)
	mux.HandleFunc("POST /v1/invitations/{token}/register", h.RegisterInvitation)
	route("POST /v1/invitations/{token}/accept", h.AcceptInvitation)
}

// RegisterTenantRoutes mounts the tenant shell's member, invitation, role and
// organization profile management (/v1/tenant/members, /v1/tenant/invitations,
// /v1/tenant/roles, /v1/tenant/organization). A nil resolver (tenancy off)
// mounts nothing. Permissions come from the membership only (no super_admin
// bypass).
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
	settingsRead := perm(rbac.PermTenantSettingsRead)
	settingsWrite := perm(rbac.PermTenantSettingsWrite)

	route("GET /v1/tenant/members/meta", h.TenantMembersMetaH, membersRead)
	route("GET /v1/tenant/members", h.TenantListMembers, membersRead)
	route("GET /v1/tenant/members/{userUuid}", h.TenantGetMember, membersRead)
	route("PATCH /v1/tenant/members/{userUuid}", h.TenantPatchMember, membersWrite)
	route("DELETE /v1/tenant/members/{userUuid}", h.TenantDeleteMember, membersWrite)

	route("GET /v1/tenant/invitations/meta", h.TenantInvitationsMetaH, membersRead)
	route("GET /v1/tenant/invitations", h.TenantListInvitations, membersRead)
	route("POST /v1/tenant/invitations", h.TenantCreateInvitation, membersWrite)
	route("POST /v1/tenant/invitations/{uuid}/revoke", h.TenantRevokeInvitation, membersWrite)
	route("POST /v1/tenant/invitations/{uuid}/resend", h.TenantResendInvitation, membersWrite)

	route("GET /v1/tenant/roles/meta", h.TenantRolesMetaH, rolesRead)
	route("GET /v1/tenant/roles/permissions", h.TenantListPermissions, rolesRead)
	route("GET /v1/tenant/roles", h.TenantListRoles, rolesRead)
	route("POST /v1/tenant/roles", h.TenantCreateRole, rolesWrite)
	route("GET /v1/tenant/roles/{uuid}", h.TenantGetRole, rolesRead)
	route("PATCH /v1/tenant/roles/{uuid}", h.TenantPatchRole, rolesWrite)
	route("DELETE /v1/tenant/roles/{uuid}", h.TenantDeleteRole, rolesWrite)

	route("GET /v1/tenant/organization", h.TenantGetOrganization, settingsRead)
	route("PATCH /v1/tenant/organization", h.TenantPatchOrganization, settingsRead, settingsWrite)
	route("PUT /v1/tenant/organization/logo", h.TenantUploadLogo, settingsRead, settingsWrite)
	route("DELETE /v1/tenant/organization/logo", h.TenantDeleteLogo, settingsRead, settingsWrite)
	route("PUT /v1/tenant/organization/cover", h.TenantUploadCover, settingsRead, settingsWrite)
	route("DELETE /v1/tenant/organization/cover", h.TenantDeleteCover, settingsRead, settingsWrite)
}
