package handler

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Confirm serves POST /v1/customer/reservations/{uuid}/confirm (TEC-921).
func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "write", h.limits.Write)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.ConfirmInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.Confirm(r.Context(), p, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "reservations.confirmed", out, nil, "customer_panel")
	response.JSON(w, r, http.StatusOK, out.Reservation)
}

// RequestChange serves POST /v1/customer/reservations/{uuid}/change-request.
func (h *Handler) RequestChange(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "write", h.limits.Write)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.ChangeRequestInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.RequestChange(r.Context(), p, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "reservations.change_requested", out, nil, "customer_panel")
	response.JSON(w, r, http.StatusOK, out.Reservation)
}

// VisitConfirm serves POST /v1/customer/visit/confirm.
func (h *Handler) VisitConfirm(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "visit_ip", httpkit.ClientIP(r), h.limits.VisitIP) {
		return
	}
	var in model.VisitConfirmInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.VisitConfirm(r.Context(), in, meta(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "reservations.confirmed", out, nil, "visit_link")
	response.JSON(w, r, http.StatusOK, out.Reservation)
}

// VisitRequestChange serves POST /v1/customer/visit/change-request.
func (h *Handler) VisitRequestChange(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "visit_ip", httpkit.ClientIP(r), h.limits.VisitIP) {
		return
	}
	var in model.VisitChangeRequestInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.VisitRequestChange(r.Context(), in, meta(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "reservations.change_requested", out, nil, "visit_link")
	response.JSON(w, r, http.StatusOK, out.Reservation)
}
