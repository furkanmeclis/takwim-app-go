package handler

// Access security routes of the customer panel (TEC-910): panel links,
// devices, number change, reassigned numbers and visit links. Link tokens
// travel in JSON bodies only, never in a URL, so no access log holds one.

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/usecase"
	customersmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Link serves POST /v1/customer/auth/link: a panel link becomes a session.
func (h *Handler) Link(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "link_ip", httpkit.ClientIP(r), h.limits.LinkIP) {
		return
	}
	var in model.LinkInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.RedeemLink(r.Context(), in, meta(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Sessions serves GET /v1/customer/sessions (the device list).
func (h *Handler) Sessions(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "read", h.limits.Read)
	if !ok {
		return
	}
	out, err := h.svc.Sessions(r.Context(), p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// RevokeSession serves DELETE /v1/customer/sessions/{uuid}.
func (h *Handler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "security", h.limits.Security)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Session was not found")
		return
	}
	if err := h.svc.RevokeSession(r.Context(), p, id, meta(r)); err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			response.NotFound(w, r, "Session was not found")
			return
		}
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PhoneStart serves POST /v1/customer/phone/start: a code to the new number.
func (h *Handler) PhoneStart(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "security", h.limits.Security)
	if !ok {
		return
	}
	var in model.PhoneStartInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if np := usecase.NormalizePhone(in.Phone); np != "" && !h.allow(w, r, "start_phone", np, h.limits.StartPhone) {
		return
	}
	out, err := h.svc.StartPhoneChange(r.Context(), p, in, meta(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, out)
}

// PhoneVerify serves POST /v1/customer/phone/verify: the profile moves to
// the confirmed number.
func (h *Handler) PhoneVerify(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "phone_verify", h.limits.VerifyPhone)
	if !ok {
		return
	}
	var in model.VerifyInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.VerifyPhoneChange(r.Context(), p, in, meta(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Reassigned serves POST /v1/customer/phone/reassigned: "this number was
// recently given to me".
func (h *Handler) Reassigned(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "security", h.limits.Security)
	if !ok {
		return
	}
	var in model.ReassignedInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.ReportReassigned(r.Context(), p, in, meta(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// RequestHistory serves POST /v1/customer/businesses/{slug}/history-request
// (TEC-991): "reopen my history at this business". The business's activity
// gets the request (no actor: the customer acted) so the reception sees it
// on the record.
func (h *Handler) RequestHistory(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "security", h.limits.Security)
	if !ok {
		return
	}
	req, out, err := h.svc.RequestHistory(r.Context(), p, r.PathValue("slug"), meta(r))
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			response.NotFound(w, r, "Business was not found")
			return
		}
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		id := req.CustomerUUID
		h.activity.RecordEntry(orgctx.WithScope(r.Context(), req.Scope), activity.Entry{
			Action: "customers.history_requested", Resource: customersmodel.Slug, ResourceUUID: &id,
			Payload: map[string]any{"source": "customer_panel", "requested_at": req.RequestedAt}, Request: r,
		})
	}
	if h.events != nil {
		h.events.Publish(orgctx.WithScope(r.Context(), req.Scope), customersmodel.Slug, resourceevents.ActionUpdated, req.CustomerUUID.String())
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Visit serves POST /v1/customer/visit: the reservation of a visit link.
func (h *Handler) Visit(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "visit_ip", httpkit.ClientIP(r), h.limits.VisitIP) {
		return
	}
	var in model.VisitInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.Visit(r.Context(), in, meta(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// VisitCancel serves POST /v1/customer/visit/cancel.
func (h *Handler) VisitCancel(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "visit_ip", httpkit.ClientIP(r), h.limits.VisitIP) {
		return
	}
	var in model.VisitCancelInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.VisitCancel(r.Context(), in, meta(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "reservations.cancelled", out, map[string]any{"reason": usecase.CancelReason}, "visit_link")
	response.JSON(w, r, http.StatusOK, out.Reservation)
}
