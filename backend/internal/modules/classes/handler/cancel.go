package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	bulkusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// BulkRunner runs a bulk action (the bulk engine's service).
type BulkRunner interface {
	Execute(ctx context.Context, actorID int64, in bulkusecase.ExecuteInput) (bulkusecase.ExecuteResult, error)
}

// SetBulk installs the bulk engine that cancels a session's bookings.
func (h *Handler) SetBulk(b BulkRunner) {
	h.bulk = b
}

// sessionCancelBody is the body of POST …/sessions/{date}/cancel.
type sessionCancelBody struct {
	model.SessionCancelInput
	// Locale is the language of the run's report (tr when empty).
	Locale string `json:"locale"`
}

// CancelSession serves POST /v1/tenant/classes/{uuid}/sessions/{date}/cancel
// (TEC-989): the session is cancelled and its open bookings are cancelled
// in one bulk run (customer notices, refunds waiting, package sessions and
// seats freed), which can be rolled back in the bulk window
// (POST /v1/tenant/bulk/{uuid}/rollback) to open the session again with
// its bookings.
func (h *Handler) CancelSession(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in sessionCancelBody
	if !decodeJSON(w, r, &in) {
		return
	}
	if h.bulk == nil {
		response.InternalErr(w, r, errors.New("bulk engine is not configured"), "Unexpected server error")
		return
	}
	pr := authctx.MustPrincipal(r.Context())
	orgID := orgctx.Must(r.Context()).ID
	date := r.PathValue("date")
	before, targets, err := h.svc.CancelWithBookings(r.Context(), orgID, id, date, &pr.UserInternal, in.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var cancellation *model.SessionCancellation
	if len(targets) > 0 {
		ids := make([]string, 0, len(targets))
		for _, t := range targets {
			ids = append(ids, t.String())
		}
		res, err := h.bulk.Execute(r.Context(), pr.UserInternal, bulkusecase.ExecuteInput{
			Resource: model.SessionsSlug, Action: model.CancelBookingsAction, Locale: in.Locale, Record: true,
			Target: bulkengine.BulkTarget{Scope: "ids", IDs: ids, Params: map[string]string{"class": id.String(), "date": date}},
		})
		if errors.Is(err, bulkusecase.ErrInvalidRequest) {
			response.BadRequest(w, r, response.CodeValidationError, err.Error())
			return
		}
		if err != nil {
			writeError(w, r, err)
			return
		}
		c := runCancellation(res, len(targets))
		cancellation = &c
	}
	h.publish(r, resourceevents.ActionUpdated, id)
	payload := map[string]any{
		"date": date, "service": before.Service.Name, "starts_at": before.StartsAt, "booked": before.Booked,
		"reason": in.Reason, "bookings": len(targets),
	}
	if cancellation != nil {
		payload["job"] = cancellation.JobUUID
		payload["cancelled_bookings"] = cancellation.Succeeded
	}
	branch := before.Branch
	h.record(r, "classes.session_cancelled", id, &branch, payload)
	detail, err := h.sessionDetail(r, id, date)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, model.SessionCancelResult{Session: detail, Cancellation: cancellation})
}

// runCancellation is the answer of the bulk run: the summary of an inline
// run, or the queued job of a large one (its counts come with the job).
func runCancellation(res bulkusecase.ExecuteResult, total int) model.SessionCancellation {
	c := model.SessionCancellation{
		JobUUID: res.Job.UUID, Total: total, RollbackUntil: res.Job.RollbackUntil, CreatedAt: res.Job.CreatedAt,
	}
	if !res.Async {
		c.Total, c.Succeeded, c.Failed, c.Skipped = res.Summary.Total, res.Summary.Succeeded, res.Summary.Failed, res.Summary.Skipped
	}
	if c.JobUUID == uuid.Nil {
		c.RollbackUntil = nil
	}
	return c
}
