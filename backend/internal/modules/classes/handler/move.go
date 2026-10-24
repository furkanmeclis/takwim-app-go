package handler

import (
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

// sessionMoveBody is the body of POST …/sessions/{date}/move.
type sessionMoveBody struct {
	model.SessionMoveInput
	// Locale is the language of the run's report (tr when empty).
	Locale string `json:"locale"`
}

// MoveSessionBookings serves POST /v1/tenant/classes/{uuid}/sessions/{date}/move
// (TEC-994): the open bookings of the session are moved to another session
// of the class in one bulk run (each through the reservation's own
// reschedule: seats held in the target, package sessions kept, customers
// told their time changed), which can be rolled back in the bulk window
// (POST /v1/tenant/bulk/{uuid}/rollback). A booking that finds no seat
// stays and is listed in the run's issues.
func (h *Handler) MoveSessionBookings(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in sessionMoveBody
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
	from, to, targets, err := h.svc.MoveTargets(r.Context(), orgID, id, date, in.SessionMoveInput)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var mv *model.SessionMove
	if len(targets) > 0 {
		ids := make([]string, 0, len(targets))
		for _, t := range targets {
			ids = append(ids, t.String())
		}
		res, err := h.bulk.Execute(r.Context(), pr.UserInternal, bulkusecase.ExecuteInput{
			Resource: model.SessionsSlug, Action: model.MoveBookingsAction, Locale: in.Locale, Record: true,
			Target: bulkengine.BulkTarget{Scope: "ids", IDs: ids, Params: map[string]string{"class": id.String(), "date": date, "to_date": to.Date}},
		})
		if errors.Is(err, bulkusecase.ErrInvalidRequest) {
			response.BadRequest(w, r, response.CodeValidationError, err.Error())
			return
		}
		if err != nil {
			writeError(w, r, err)
			return
		}
		m := runMove(res, to.Date, len(targets))
		mv = &m
	}
	h.publish(r, resourceevents.ActionUpdated, id)
	payload := map[string]any{
		"date": date, "to_date": to.Date, "service": from.Service.Name, "starts_at": from.StartsAt, "to_starts_at": to.StartsAt,
		"booked": from.Booked, "bookings": len(targets),
	}
	if mv != nil {
		payload["job"] = mv.JobUUID
		payload["moved_bookings"] = mv.Succeeded
	}
	branch := from.Branch
	h.record(r, "classes.bookings_moved", id, &branch, payload)
	detail, err := h.sessionDetail(r, id, date)
	if err != nil {
		writeError(w, r, err)
		return
	}
	target, err := h.sessionDetail(r, id, to.Date)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, model.SessionMoveResult{Session: detail, Target: target, Move: mv})
}

// runMove is the answer of the bulk run: the summary of an inline run, or
// the queued job of a large one (its counts come with the job).
func runMove(res bulkusecase.ExecuteResult, toDate string, total int) model.SessionMove {
	m := model.SessionMove{
		JobUUID: res.Job.UUID, ToDate: toDate, Total: total, Issues: []model.SessionMoveIssue{},
		RollbackUntil: res.Job.RollbackUntil, CreatedAt: res.Job.CreatedAt,
	}
	if !res.Async {
		m.Total, m.Succeeded, m.Failed, m.Skipped = res.Summary.Total, res.Summary.Succeeded, res.Summary.Failed, res.Summary.Skipped
		for _, i := range res.Summary.Issues {
			m.Issues = append(m.Issues, model.SessionMoveIssue{ReservationUUID: i.EntityUUID, Label: i.Label, Error: i.Error, Skipped: i.Skipped})
		}
	}
	if m.JobUUID == uuid.Nil {
		m.RollbackUntil = nil
	}
	return m
}
