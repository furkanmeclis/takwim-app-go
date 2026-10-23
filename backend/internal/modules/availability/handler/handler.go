// Package handler is the HTTP layer of the availability engine. Every
// handler runs behind RequireOrganization and reads the organization id from
// orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/tenant/availability.
type Handler struct {
	svc   *usecase.Service
	holds *usecase.Holds
}

// New builds the handler.
func New(svc *usecase.Service, holds *usecase.Holds) *Handler {
	return &Handler{svc: svc, holds: holds}
}

// caller resolves the reach of a full / own slug pair; false = answered 403.
func caller(w http.ResponseWriter, r *http.Request, full, own string) (usecase.Caller, bool) {
	scope := orgctx.Must(r.Context())
	switch scope.Reach(full, own) {
	case orgctx.ReachNone:
		response.Forbidden(w, r, "Missing permission: "+full)
		return usecase.Caller{}, false
	case orgctx.ReachOwn:
		return usecase.Caller{OwnUserID: authctx.MustPrincipal(r.Context()).UserInternal}, true
	}
	return usecase.Caller{}, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Hold was not found")
		return uuid.Nil, false
	}
	return id, true
}

// Search serves POST /v1/tenant/availability/search. A member holding only
// tenant.reservations.own searches its own calendar.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.SearchInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsRead, rbac.PermTenantReservationsOwn)
	if !ok {
		return
	}
	out, err := h.svc.Search(r.Context(), scope.ID, in, c)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CreateHold serves POST /v1/tenant/availability/holds.
func (h *Handler) CreateHold(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.HoldInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsOwn)
	if !ok {
		return
	}
	out, err := h.holds.Create(r.Context(), scope.ID, in, c, authctx.MustPrincipal(r.Context()).UserInternal)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// GetHold serves GET /v1/tenant/availability/holds/{uuid}.
func (h *Handler) GetHold(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsRead, rbac.PermTenantReservationsOwn)
	if !ok {
		return
	}
	out, err := h.holds.Get(r.Context(), scope.ID, id, c)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ExtendHold serves POST /v1/tenant/availability/holds/{uuid}/extend.
func (h *Handler) ExtendHold(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.ExtendInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsOwn)
	if !ok {
		return
	}
	out, err := h.holds.Extend(r.Context(), scope.ID, id, c, in.Minutes)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ReleaseHold serves DELETE /v1/tenant/availability/holds/{uuid}.
func (h *Handler) ReleaseHold(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	c, ok := caller(w, r, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsOwn)
	if !ok {
		return
	}
	out, err := h.holds.Release(r.Context(), scope.ID, id, c, "released")
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Branch was not found")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, err.Error())
	case errors.Is(err, usecase.ErrHoldNotFound):
		response.NotFound(w, r, "Hold was not found")
	case errors.Is(err, usecase.ErrSlotTaken):
		conflict(w, r, "starts_at", "taken", err)
	case errors.Is(err, usecase.ErrHoldExpired):
		conflict(w, r, "uuid", "expired", err)
	case errors.Is(err, usecase.ErrNotActive):
		conflict(w, r, "uuid", "not_active", err)
	case errors.Is(err, usecase.ErrNotHold):
		conflict(w, r, "uuid", "not_hold", err)
	case errors.Is(err, usecase.ErrOnlineNotReady):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "channel", Message: err.Error(), Code: "online_not_ready",
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

func conflict(w http.ResponseWriter, r *http.Request, field, code string, err error) {
	response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
		Field: field, Message: err.Error(), Code: code,
	}})
}
