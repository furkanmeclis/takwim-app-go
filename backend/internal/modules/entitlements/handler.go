package entitlements

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Organizations is what the platform routes need from the organizations
// module.
type Organizations interface {
	PlatformScope(ctx context.Context, orgUUID uuid.UUID) (orgctx.Scope, error)
	SetEntitlements(ctx context.Context, id uuid.UUID, in orgmodel.SetEntitlementsInput) (orgmodel.Organization, error)
}

// ResourcePlatformOrganizations is the activity resource of platform
// organization changes (same as the organizations module).
const ResourcePlatformOrganizations = "platform.organizations"

// Handler serves the entitlement routes.
type Handler struct {
	svc      *Service
	orgs     Organizations
	activity *activity.Recorder
}

// NewHandler builds the handler; rec may be nil.
func NewHandler(svc *Service, orgs Organizations, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, orgs: orgs, activity: rec}
}

// Catalog is the plan and add-on catalog (platform form).
type Catalog struct {
	Plans  []entitlement.Plan `json:"plans"`
	Addons []string           `json:"addons"`
}

func catalog() Catalog {
	addons := make([]string, 0, len(entitlement.Addons))
	for _, a := range entitlement.Addons {
		addons = append(addons, string(a))
	}
	return Catalog{Plans: entitlement.Plans(), Addons: addons}
}

// PlatformSummary is the platform view: the summary, the stored overrides
// and the catalog.
type PlatformSummary struct {
	Summary
	Overrides Overrides `json:"overrides"`
	Catalog   Catalog   `json:"catalog"`
}

// Overrides are the stored limit overrides (nil = plan default).
type Overrides struct {
	BranchLimit *int32 `json:"branch_limit"`
	StaffLimit  *int32 `json:"staff_limit"`
}

// TenantGet serves GET /v1/tenant/entitlements.
func (h *Handler) TenantGet(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Summary(r.Context(), orgctx.Must(r.Context()))
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) platformSummary(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	scope, err := h.orgs.PlatformScope(r.Context(), id)
	if err != nil {
		writeOrgError(w, r, err)
		return
	}
	sum, err := h.svc.Summary(r.Context(), scope)
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	response.JSON(w, r, http.StatusOK, PlatformSummary{
		Summary:   sum,
		Overrides: Overrides{BranchLimit: scope.BranchLimit, StaffLimit: scope.StaffLimit},
		Catalog:   catalog(),
	})
}

// PlatformGet serves GET /v1/platform/organizations/{uuid}/entitlements.
func (h *Handler) PlatformGet(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Organization was not found")
		return
	}
	h.platformSummary(w, r, id)
}

// PlatformPut serves PUT /v1/platform/organizations/{uuid}/entitlements.
func (h *Handler) PlatformPut(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Organization was not found")
		return
	}
	var in struct {
		Plan        string   `json:"plan"`
		Addons      []string `json:"addons"`
		BranchLimit *int32   `json:"branch_limit"`
		StaffLimit  *int32   `json:"staff_limit"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	org, err := h.orgs.SetEntitlements(r.Context(), id, orgmodel.SetEntitlementsInput{
		Plan: in.Plan, Addons: in.Addons, BranchLimit: in.BranchLimit, StaffLimit: in.StaffLimit,
	})
	if err != nil {
		writeOrgError(w, r, err)
		return
	}
	if h.activity != nil {
		if p, ok := authctx.PrincipalFrom(r.Context()); ok {
			h.activity.Record(r.Context(), &p.UserInternal, "organizations.entitlements_updated", ResourcePlatformOrganizations, &org.UUID,
				map[string]any{"slug": org.Slug, "plan": org.Plan, "addons": org.Addons, "branch_limit": org.BranchLimit, "staff_limit": org.StaffLimit}, r)
		}
	}
	h.platformSummary(w, r, id)
}

func writeOrgError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, orgusecase.ErrNotFound):
		response.NotFound(w, r, "Organization was not found")
	case errors.Is(err, orgusecase.ErrInvalidRequest):
		response.ValidationError(w, r, []response.Detail{{Field: "body", Message: err.Error(), Code: "invalid"}})
	default:
		response.InternalErr(w, r, err, "")
	}
}

// RegisterRoutes mounts the tenant route (when tenancy is on) and the
// platform routes.
func RegisterRoutes(mux *http.ServeMux, h *Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	authn := middleware.Authenticate(tokens, loader)
	read := middleware.RequirePermission(rbac.PermPlatformOrganizationsRead)
	write := middleware.RequirePermission(rbac.PermPlatformOrganizationsWrite)
	mux.Handle("GET /v1/platform/organizations/{uuid}/entitlements", middleware.Chain(http.HandlerFunc(h.PlatformGet), authn, read))
	mux.Handle("PUT /v1/platform/organizations/{uuid}/entitlements", middleware.Chain(http.HandlerFunc(h.PlatformPut), authn, write))
	if resolver == nil {
		return
	}
	org := middleware.RequireOrganization(resolver)
	settings := middleware.RequireTenantPermission(rbac.PermTenantSettingsRead)
	mux.Handle("GET /v1/tenant/entitlements", middleware.Chain(http.HandlerFunc(h.TenantGet), authn, org, settings))
}
