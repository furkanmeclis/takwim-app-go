package orgoverview

import (
	"context"
	"errors"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Organizations resolves the business of a platform route.
type Organizations interface {
	PlatformScope(ctx context.Context, orgUUID uuid.UUID) (orgctx.Scope, error)
}

// Handler serves the overview route.
type Handler struct {
	svc  *Service
	orgs Organizations
}

// NewHandler builds the handler.
func NewHandler(svc *Service, orgs Organizations) *Handler {
	return &Handler{svc: svc, orgs: orgs}
}

// Get serves GET /v1/platform/organizations/{uuid}/overview.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Organization was not found")
		return
	}
	scope, err := h.orgs.PlatformScope(r.Context(), id)
	if errors.Is(err, orgusecase.ErrNotFound) {
		response.NotFound(w, r, "Organization was not found")
		return
	}
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	out, err := h.svc.Get(r.Context(), scope)
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// RegisterRoutes mounts the platform overview (platform organizations
// read).
func RegisterRoutes(mux *http.ServeMux, h *Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)
	read := middleware.RequirePermission(rbac.PermPlatformOrganizationsRead)
	mux.Handle("GET /v1/platform/organizations/{uuid}/overview", middleware.Chain(http.HandlerFunc(h.Get), authn, read))
}
