package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/usecase"
	customernotifymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/model"
	customernotifyusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// TextStore keeps the business's texts of the confirmation request
// (TEC-979): the customer notification engine, where the texts of every
// event live and are rendered.
type TextStore interface {
	ManagedTexts(ctx context.Context, orgID int64, code string) (customernotifymodel.ManagedTexts, error)
	SaveManagedTexts(ctx context.Context, orgID, actorID int64, code string, in customernotifymodel.ManagedTextsInput) (customernotifymodel.ManagedTexts, error)
	ResetManagedTexts(ctx context.Context, orgID int64, code string) (customernotifymodel.ManagedTexts, error)
}

// SetTexts wires the text store of the request texts.
func (h *Handler) SetTexts(t TextStore) { h.texts = t }

// GetTexts serves GET /v1/tenant/confirmations/texts: the request's SMS
// and e-mail texts with their defaults, variables and the call text.
func (h *Handler) GetTexts(w http.ResponseWriter, r *http.Request) {
	if h.texts == nil {
		response.NotFound(w, r, "Confirmation texts are not available")
		return
	}
	out, err := h.texts.ManagedTexts(r.Context(), orgctx.Must(r.Context()).ID, usecase.Event)
	if err != nil {
		writeTextError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PutTexts serves PUT /v1/tenant/confirmations/texts. The rule (channels,
// timing) stays in the confirmation settings.
func (h *Handler) PutTexts(w http.ResponseWriter, r *http.Request) {
	if h.texts == nil {
		response.NotFound(w, r, "Confirmation texts are not available")
		return
	}
	var in customernotifymodel.ManagedTextsInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, err := h.texts.SaveManagedTexts(r.Context(), orgctx.Must(r.Context()).ID, p.UserInternal, usecase.Event, in)
	if err != nil {
		writeTextError(w, r, err)
		return
	}
	channels := make([]string, 0, len(in.Templates))
	for _, t := range in.Templates {
		channels = append(channels, t.Channel)
	}
	h.record(r, "confirmations.texts_updated", map[string]any{"event": usecase.Event, "templates": channels})
	response.JSON(w, r, http.StatusOK, out)
}

// ResetTexts serves POST /v1/tenant/confirmations/texts/reset: every
// request text back to the default.
func (h *Handler) ResetTexts(w http.ResponseWriter, r *http.Request) {
	if h.texts == nil {
		response.NotFound(w, r, "Confirmation texts are not available")
		return
	}
	out, err := h.texts.ResetManagedTexts(r.Context(), orgctx.Must(r.Context()).ID, usecase.Event)
	if err != nil {
		writeTextError(w, r, err)
		return
	}
	h.record(r, "confirmations.texts_reset", map[string]any{"event": usecase.Event})
	response.JSON(w, r, http.StatusOK, out)
}

func writeTextError(w http.ResponseWriter, r *http.Request, err error) {
	if ve, ok := customernotifyusecase.IsValidation(err); ok {
		response.RecordFailure(w, err)
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	if errors.Is(err, customernotifyusecase.ErrUnknownEvent) {
		response.RecordFailure(w, err)
		response.NotFound(w, r, "Confirmation texts are not available")
		return
	}
	writeError(w, r, err)
}
