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

// productsAllowed checks what adding products to a visit needs besides
// the reservation slug: the products module and tenant.products.sell.
func productsAllowed(w http.ResponseWriter, r *http.Request) bool {
	scope := orgctx.Must(r.Context())
	if !entitlement.ForScope(scope).Allows(sector.ModuleProducts) {
		response.Error(w, r, http.StatusForbidden, response.CodeModuleNotEnabled, "This module is not enabled for the organization")
		return false
	}
	if !scope.Has(rbac.PermTenantProductsSell) {
		response.Forbidden(w, r, "Missing permission: "+rbac.PermTenantProductsSell)
		return false
	}
	return true
}

// ProductOptions serves GET /v1/tenant/reservations/{uuid}/product-options:
// the stock items of the visit's branch with their available units (?q=
// matches name, SKU or barcode).
func (h *Handler) ProductOptions(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok || !productsAllowed(w, r) {
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsRead)
	if !ok {
		return
	}
	items, err := h.svc.ProductOptions(r.Context(), scope.ID, id, c, r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// BranchProductOptions serves GET /v1/tenant/reservations/product-options
// (TEC-977): the stock items of ?branch_uuid= with their available units
// for the new reservation form (?q= matches name, SKU or barcode).
func (h *Handler) BranchProductOptions(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	if !productsAllowed(w, r) {
		return
	}
	if _, ok := caller(w, r, rbac.PermTenantReservationsRead); !ok {
		return
	}
	raw := r.URL.Query().Get("branch_uuid")
	if raw == "" {
		response.ValidationError(w, r, []response.Detail{{Field: "branch_uuid", Message: "branch is required", Code: "required"}})
		return
	}
	branch, err := uuid.Parse(raw)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "branch_uuid", Message: "branch must be a uuid", Code: "invalid"}})
		return
	}
	items, err := h.svc.BranchProductOptions(r.Context(), scope.ID, branch, r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// AddProduct serves POST /v1/tenant/reservations/{uuid}/products.
func (h *Handler) AddProduct(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.AddProductInput
	if !productsAllowed(w, r) || !decodeJSON(w, r, &in) {
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsWrite)
	if !ok {
		return
	}
	out, err := h.svc.AddProduct(r.Context(), scope.ID, id, c, authctx.MustPrincipal(r.Context()).UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "reservations.product_added", out, map[string]any{"product_uuid": in.ProductUUID.String(), "quantity": in.Quantity})
	h.productChanged(r, in.ProductUUID)
	response.JSON(w, r, http.StatusOK, out)
}

// RemoveProduct serves DELETE /v1/tenant/reservations/{uuid}/products/{productUuid}.
func (h *Handler) RemoveProduct(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	line, err := uuid.Parse(r.PathValue("productUuid"))
	if err != nil {
		response.NotFound(w, r, "Reservation product was not found")
		return
	}
	var in usecase.ChangeInput
	if !productsAllowed(w, r) || !decodeOptionalJSON(w, r, &in) {
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsWrite)
	if !ok {
		return
	}
	out, err := h.svc.RemoveProduct(r.Context(), scope.ID, id, line, c, authctx.MustPrincipal(r.Context()).UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "reservations.product_removed", out, nil)
	for _, p := range out.Products {
		if p.UUID == line {
			h.productChanged(r, p.Product.UUID)
		}
	}
	response.JSON(w, r, http.StatusOK, out)
}

// productChanged tells the product lists that the stock of a product moved.
func (h *Handler) productChanged(r *http.Request, product uuid.UUID) {
	if h.events != nil {
		h.events.Publish(r.Context(), productsResource, resourceevents.ActionUpdated, product.String())
	}
}

// productsResource is the live channel of the product list.
const productsResource = "tenant.products"
