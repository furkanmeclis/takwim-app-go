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
)

// Handler serves /v1/tenant/availability.
type Handler struct {
	svc *usecase.Service
}

// New builds the handler.
func New(svc *usecase.Service) *Handler {
	return &Handler{svc: svc}
}

// Search serves POST /v1/tenant/availability/search. A member holding only
// tenant.reservations.own searches its own calendar.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.SearchInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	caller := usecase.Caller{}
	switch scope.Reach(rbac.PermTenantReservationsRead, rbac.PermTenantReservationsOwn) {
	case orgctx.ReachNone:
		response.Forbidden(w, r, "Missing permission: "+rbac.PermTenantReservationsRead)
		return
	case orgctx.ReachOwn:
		caller.OwnUserID = authctx.MustPrincipal(r.Context()).UserInternal
	}
	out, err := h.svc.Search(r.Context(), scope.ID, in, caller)
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
	case errors.Is(err, usecase.ErrOnlineNotReady):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "channel", Message: err.Error(), Code: "online_not_ready",
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
