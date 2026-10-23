package tenant

import (
	"net/http"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

type organizationDTO struct {
	UUID         string     `json:"uuid"`
	Slug         string     `json:"slug"`
	Name         string     `json:"name"`
	Status       string     `json:"status"`
	AccessEndsAt *time.Time `json:"access_ends_at"`
	Sector       string     `json:"sector"`
	Timezone     string     `json:"timezone"`
	Currency     string     `json:"currency"`
}

type contextDTO struct {
	Organization organizationDTO `json:"organization"`
	Role         string          `json:"role"`
	Roles        []string        `json:"roles"`
	Permissions  []string        `json:"permissions"`
	// BranchScope is "all" or "selected" (branch-bound data is limited to
	// the member's branches).
	BranchScope string `json:"branch_scope"`
	// Sector is the definition of the organization's sector: booking flow,
	// capacity, terms and visible modules (TEC-882).
	Sector sector.Definition `json:"sector"`
	// Entitlements: plan, add-ons, effective limits and the modules the
	// organization may use (the sector's modules minus add-ons not
	// granted; TEC-883). The panel shows exactly these modules.
	Entitlements entitlement.Effective `json:"entitlements"`
}

// listSectors serves GET /v1/sectors: the sector catalog (any signed-in
// user; the platform uses it when it opens a business).
func listSectors(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]any{"items": sector.All()})
}

// getContext returns the resolved organization scope of the request.
func getContext(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	perms := s.Permissions
	if perms == nil {
		perms = []string{}
	}
	roles := s.MemberRoles
	if roles == nil {
		roles = []string{s.MemberRole}
	}
	branchScope := orgctx.BranchScopeAll
	if !s.AllBranches() {
		branchScope = orgctx.BranchScopeSelected
	}
	response.JSON(w, r, http.StatusOK, contextDTO{
		Organization: organizationDTO{
			UUID:         s.UUID.String(),
			Slug:         s.Slug,
			Name:         s.Name,
			Status:       s.Status,
			AccessEndsAt: s.AccessEndsAt,
			Sector:       s.Sector,
			Timezone:     s.Timezone,
			Currency:     s.Currency,
		},
		Role:         s.MemberRole,
		Roles:        roles,
		Permissions:  perms,
		BranchScope:  branchScope,
		Sector:       sector.Of(s.Sector),
		Entitlements: entitlement.ForScope(s),
	})
}
