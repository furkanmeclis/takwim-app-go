package handler

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	productsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Money of walk-in product sales (TEC-992): the payment view of a stock
// sale, a collection after the sale and a refund after the return. The
// routes sit under /v1/tenant/stock-movements (products module,
// tenant.products.read); the sale-time collection and the return-time
// refund are part of the products routes.

func stockUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Stock movement was not found")
		return uuid.Nil, false
	}
	return id, true
}

// StockSalePayments serves GET /v1/tenant/stock-movements/{uuid}/payments.
func (h *Handler) StockSalePayments(w http.ResponseWriter, r *http.Request) {
	id, ok := stockUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.StockSalePayments(r.Context(), orgctx.Must(r.Context()).ID, id, viewOptions(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// StockPaymentAccounts serves GET /v1/tenant/stock-movements/payment-accounts
// ?branch_uuid: the accounts a counter sale of the branch may use.
func (h *Handler) StockPaymentAccounts(w http.ResponseWriter, r *http.Request) {
	branch, err := uuid.Parse(r.URL.Query().Get("branch_uuid"))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "branch_uuid", Code: "invalid", Message: "branch_uuid must be a uuid"}})
		return
	}
	out, err := h.svc.StockPaymentAccounts(r.Context(), orgctx.Must(r.Context()).ID, branch, viewOptions(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CollectStockSale serves POST /v1/tenant/stock-movements/{uuid}/collections.
func (h *Handler) CollectStockSale(w http.ResponseWriter, r *http.Request) {
	id, ok := stockUUID(w, r)
	if !ok {
		return
	}
	var in model.CollectInput
	if !decode(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, err := h.svc.CollectStockSale(r.Context(), orgctx.Must(r.Context()).ID, id, p.UserInternal, viewOptions(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordStock(r, "products.sale_collected", out, map[string]any{
		"amount": in.Amount.String(), "currency": out.Currency, "method": in.Method,
	})
	response.JSON(w, r, http.StatusCreated, out)
}

// RefundStockSale serves POST /v1/tenant/stock-movements/{uuid}/refunds.
func (h *Handler) RefundStockSale(w http.ResponseWriter, r *http.Request) {
	id, ok := stockUUID(w, r)
	if !ok {
		return
	}
	var in model.StockSaleRefundInput
	if !decode(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, err := h.svc.RefundStockSale(r.Context(), orgctx.Must(r.Context()).ID, id, p.UserInternal, viewOptions(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if out.Replayed {
		status = http.StatusOK
	} else {
		h.recordStock(r, "products.sale_refunded", out.Payments, map[string]any{
			"amount": in.Amount.String(), "currency": out.Payments.Currency, "method": in.Method, "reason": in.Reason,
		})
	}
	response.JSON(w, r, status, out)
}

// recordStock writes the activity row of a money event on a stock sale,
// under the product like every stock movement.
func (h *Handler) recordStock(r *http.Request, action string, view model.StockSalePayments, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	payload["name"] = view.Movement.Product
	payload["movement_uuid"] = view.Movement.UUID.String()
	payload["branch"] = view.Movement.Branch.Name
	if view.Movement.Variant != "" {
		payload["variant"] = view.Movement.Variant
	}
	h.activity.RecordEntry(r.Context(), activity.Entry{
		ActorID: &p.UserInternal, Action: action, Resource: productsmodel.Slug, Payload: payload, Request: r,
	})
}
