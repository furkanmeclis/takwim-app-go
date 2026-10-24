package handler

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// packagesAllowed checks the packages module (TEC-920). Using a session of
// the customer's package is front-desk work: the reservation slugs are
// enough.
func packagesAllowed(w http.ResponseWriter, r *http.Request) bool {
	if !entitlement.ForScope(orgctx.Must(r.Context())).Allows(sector.ModulePackages) {
		response.Error(w, r, http.StatusForbidden, response.CodeModuleNotEnabled, "This module is not enabled for the organization")
		return false
	}
	return true
}

// PackageOptions serves GET /v1/tenant/reservations/{uuid}/package-options:
// the customer's packages that can give a session to the visit's lines.
func (h *Handler) PackageOptions(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok || !packagesAllowed(w, r) {
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsRead)
	if !ok {
		return
	}
	items, err := h.svc.PackageOptions(r.Context(), scope.ID, id, c)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// ApplyPackage serves POST /v1/tenant/reservations/{uuid}/package-sessions.
func (h *Handler) ApplyPackage(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.ApplyPackageInput
	if !packagesAllowed(w, r) || !decodeJSON(w, r, &in) {
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsWrite)
	if !ok {
		return
	}
	out, err := h.svc.ApplyPackage(r.Context(), scope.ID, id, c, authctx.MustPrincipal(r.Context()).UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "reservations.package_applied", out, map[string]any{
		"customer_package_uuid": in.CustomerPackageUUID.String(), "line_index": in.LineIndex,
	})
	h.packageChanged(r, in.CustomerPackageUUID)
	response.JSON(w, r, http.StatusOK, out)
}

// RemovePackageSession serves DELETE
// /v1/tenant/reservations/{uuid}/package-sessions/{sessionUuid}.
func (h *Handler) RemovePackageSession(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	session, err := uuid.Parse(r.PathValue("sessionUuid"))
	if err != nil {
		response.NotFound(w, r, "Package session was not found")
		return
	}
	var in usecase.ChangeInput
	if !packagesAllowed(w, r) || !decodeOptionalJSON(w, r, &in) {
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsWrite)
	if !ok {
		return
	}
	out, err := h.svc.RemovePackageSession(r.Context(), scope.ID, id, session, c, authctx.MustPrincipal(r.Context()).UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "reservations.package_released", out, map[string]any{"session_uuid": session.String()})
	for _, ps := range out.PackageSessions {
		if ps.UUID == session {
			h.packageChanged(r, ps.CustomerPackage.UUID)
		}
	}
	response.JSON(w, r, http.StatusOK, out)
}

// packageChanged tells the customer package lists that a package's
// sessions moved.
func (h *Handler) packageChanged(r *http.Request, pkg uuid.UUID) {
	if h.events != nil {
		h.events.Publish(r.Context(), customerPackagesResource, resourceevents.ActionUpdated, pkg.String())
	}
}

// customerPackagesResource is the live channel of the customer packages.
const customerPackagesResource = "tenant.customer_packages"
