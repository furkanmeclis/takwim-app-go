// Package jobs mounts the integration job routes (TEC-880,
// docs/design/integration-jobs.md):
//
//	GET  /v1/tenant/integration-jobs/{uuid}         job + attempts
//	POST /v1/tenant/integration-jobs/{uuid}/retry   CheckStatus first, resubmit only if not done
//
// Both need tenant.integrations.manage (the account job lists are
// GET /v1/tenant/integrations/{uuid}/jobs). Step-up: no tenant step-up
// policy exists yet; retry joins the integration account routes when one
// does (docs/design/integration-jobs.md).
package jobs

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// RegisterRoutes mounts the routes (a nil resolver mounts nothing).
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantIntegrationsRead)
	manage := middleware.RequireTenantPermission(rbac.PermTenantIntegrationsManage)
	mux.Handle("GET /v1/tenant/integration-jobs/{uuid}", middleware.Chain(http.HandlerFunc(h.Get), authn, org, read))
	mux.Handle("POST /v1/tenant/integration-jobs/{uuid}/retry", middleware.Chain(http.HandlerFunc(h.Retry), authn, org, manage))
}

// RegisterLiveResources declares the live job list channel.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(usecase.ResourceJobs, rbac.PermTenantIntegrationsRead)
}
