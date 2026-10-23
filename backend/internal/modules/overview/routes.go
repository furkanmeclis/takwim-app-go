package overview

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Handler serves GET /v1/tenant/overview.
type Handler struct {
	svc *Service
}

// NewHandler returns a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Get serves GET /v1/tenant/overview.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Get(r.Context(), orgctx.Must(r.Context()))
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// RegisterRoutes mounts the overview route. A nil resolver (tenancy off)
// registers nothing.
func RegisterRoutes(mux *http.ServeMux, h *Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantBranchesRead)
	mux.Handle("GET /v1/tenant/overview", middleware.Chain(http.HandlerFunc(h.Get), authn, org, read))
}
