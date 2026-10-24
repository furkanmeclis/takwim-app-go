// Package handler is the HTTP layer of the confirmation settings
// (TEC-921). Every handler runs behind RequireOrganization and
// RequireModule(confirmation).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/usecase"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Handler serves /v1/tenant/confirmations.
type Handler struct {
	engine   *usecase.Engine
	activity *activity.Recorder
	events   resourceevents.Publisher
	texts    TextStore
}

// New builds the handler. rec may be nil.
func New(engine *usecase.Engine, rec *activity.Recorder) *Handler {
	return &Handler{engine: engine, activity: rec}
}

// GetSettings serves GET /v1/tenant/confirmations/settings.
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	out, err := h.engine.Settings(r.Context(), orgctx.Must(r.Context()))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PutSettings serves PUT /v1/tenant/confirmations/settings: the
// business-wide rule (no branch_uuid) or one branch's rule.
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var in model.SaveInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, err := h.engine.Save(r.Context(), orgctx.Must(r.Context()), p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "confirmations.updated", map[string]any{
		"branch_uuid": in.BranchUUID, "enabled": in.Enabled, "channels": in.Channels, "fallback": in.Fallback,
		"first_request_minutes": in.FirstRequestMinutes, "follow_up_minutes": in.FollowUpMinutes,
		"max_attempts": in.MaxAttempts, "deadline_minutes": in.DeadlineMinutes,
		"window_start_minute": in.WindowStartMinute, "window_end_minute": in.WindowEndMinute, "on_no_answer": in.OnNoAnswer,
	})
	response.JSON(w, r, http.StatusOK, out)
}

// Reset serves POST /v1/tenant/confirmations/settings/reset.
func (h *Handler) Reset(w http.ResponseWriter, r *http.Request) {
	var in model.ResetInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.engine.Reset(r.Context(), orgctx.Must(r.Context()), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "confirmations.reset", map[string]any{"branch_uuid": in.BranchUUID})
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) record(r *http.Request, action string, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.RecordEntry(r.Context(), activity.Entry{
		ActorID: &p.UserInternal, Action: action, Resource: usecase.Resource, Payload: payload, Request: r,
	})
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
	var qve *apiquery.ValidationError
	if errors.As(err, &qve) {
		details := make([]response.Detail, 0, len(qve.Details))
		for _, d := range qve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrBranchNotFound):
		response.NotFound(w, r, "Branch was not found")
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Reservation was not found")
	case errors.Is(err, usecase.ErrNotOpen):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeNotOpen, err.Error(), []response.Detail{{
			Field: "confirmation", Message: err.Error(), Code: "not_open",
		}})
	case errors.Is(err, reservationsusecase.ErrStaleVersion):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "expected_version", Message: err.Error(), Code: "stale",
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
