// Package accounts mounts the tenant integration accounts
// (/v1/tenant/integrations, TEC-880): list, connect, rotate, test, disable /
// enable, jobs (per account and organization-wide). Every route needs tenant.integrations.manage. Step-up: the
// tenant shell has no step-up policy yet (the step-up engine guards
// platform routes); when one exists, connect / update / disable get
// middleware.RequireStepUp here (docs/design/integration-jobs.md).
package accounts

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the routes (a nil resolver mounts nothing).
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	manage := middleware.RequireTenantPermission(rbac.PermTenantIntegrationsManage)
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, middleware.Chain(fn, authn, org, manage))
	}
	route("GET /v1/tenant/integrations/meta", h.Meta)
	route("GET /v1/tenant/integrations", h.List)
	route("POST /v1/tenant/integrations", h.Connect)
	route("GET /v1/tenant/integrations/{uuid}", h.Get)
	route("PATCH /v1/tenant/integrations/{uuid}", h.Update)
	route("POST /v1/tenant/integrations/{uuid}/test", h.Test)
	route("POST /v1/tenant/integrations/{uuid}/disable", h.Disable)
	route("POST /v1/tenant/integrations/{uuid}/enable", h.Enable)
	route("GET /v1/tenant/integrations/{uuid}/jobs", h.Jobs)
	route("GET /v1/tenant/integration-jobs", h.AllJobs)
}
