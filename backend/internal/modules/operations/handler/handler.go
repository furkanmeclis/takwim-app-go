// Package handler is the HTTP layer of /v1/platform/operations (TEC-930):
// the background work summary and the failed work lists of the platform
// operator. Reads need platform.settings.read, actions (run again, delete,
// retry) platform.settings.write; every action is written to the activity
// log.
package handler

import (
	"errors"
	"net/http"

	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/operations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Resource is the activity resource of the operations actions.
const Resource = "platform.operations"

// Error codes.
const (
	CodeQueueUnavailable = "QUEUE_UNAVAILABLE"
	CodeTaskNotFailed    = "QUEUE_TASK_NOT_FAILED"
	CodeNotRetryable     = "INTEGRATION_JOB_NOT_RETRYABLE"
	CodeUnverified       = "INTEGRATION_JOB_UNVERIFIED"
	CodeBusy             = "INTEGRATION_JOB_BUSY"
)

// IntegrationJobSortable are the sortable provider error list fields.
var IntegrationJobSortable = map[string]string{"created_at": "created_at", "updated_at": "updated_at"}

// NoticeFailureSortable are the sortable failed send list fields.
var NoticeFailureSortable = map[string]string{"created_at": "created_at"}

// Handler serves the operations routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler (rec may be nil).
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// Summary serves GET /v1/platform/operations/summary.
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.Summary(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, s)
}

// QueueTasks serves GET /v1/platform/operations/queue-tasks.
func (h *Handler) QueueTasks(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, map[string]string{})
	if !ok {
		return
	}
	state := r.URL.Query().Get("state")
	if state != "" && state != usecase.StateArchived && state != usecase.StateRetry {
		response.ValidationError(w, r, []response.Detail{{Field: "state", Message: "state must be archived or retry", Code: "invalid"}})
		return
	}
	items, total, err := h.svc.QueueTasks(r.Context(), usecase.QueueTaskFilter{
		State: state, Queues: httpkit.Values(r, "queue"), Types: httpkit.Values(r, "type"),
		Limit: q.Limit, Offset: q.Offset,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// RunQueueTask serves POST /v1/platform/operations/queue-tasks/{queue}/{id}/run.
func (h *Handler) RunQueueTask(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.RunQueueTask(r.Context(), r.PathValue("queue"), r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "operations.queue_task_run", nil, taskPayload(t))
	response.JSON(w, r, http.StatusOK, t)
}

// DeleteQueueTask serves DELETE /v1/platform/operations/queue-tasks/{queue}/{id}.
func (h *Handler) DeleteQueueTask(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.DeleteQueueTask(r.Context(), r.PathValue("queue"), r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "operations.queue_task_deleted", nil, taskPayload(t))
	response.JSON(w, r, http.StatusOK, t)
}

// IntegrationJobs serves GET /v1/platform/operations/integration-jobs.
func (h *Handler) IntegrationJobs(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, IntegrationJobSortable)
	if !ok {
		return
	}
	f := usecase.IntegrationJobFilter{
		Statuses: httpkit.Values(r, "status"), Providers: httpkit.Values(r, "provider"), Kinds: httpkit.Values(r, "kind"),
		Q: q.Q, Limit: q.Limit, Offset: q.Offset,
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.IntegrationJobs(r.Context(), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// IntegrationJob serves GET /v1/platform/operations/integration-jobs/{uuid}.
func (h *Handler) IntegrationJob(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	d, err := h.svc.IntegrationJob(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

// RetryIntegrationJob serves POST /v1/platform/operations/integration-jobs/{uuid}/retry.
func (h *Handler) RetryIntegrationJob(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if _, err := h.svc.IntegrationJob(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	res, err := h.svc.RetryIntegrationJob(r.Context(), id)
	var ve *jobsusecase.VerifyError
	if errors.As(err, &ve) {
		h.audit(r, "operations.integration_job_retried", &id, map[string]any{"result": "unverified", "outcome": string(ve.Outcome)})
		d, _ := h.svc.IntegrationJob(r.Context(), id)
		response.ErrorWithData(w, r, http.StatusConflict, CodeUnverified, ve.Error(),
			[]response.Detail{{Code: "unverified", Message: ve.Message}}, d)
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "operations.integration_job_retried", &id, map[string]any{"result": res.Action, "status": res.Status})
	d, err := h.svc.IntegrationJob(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"action": res.Action, "job": d})
}

// NoticeFailures serves GET /v1/platform/operations/notice-failures.
func (h *Handler) NoticeFailures(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, NoticeFailureSortable)
	if !ok {
		return
	}
	f := usecase.NoticeFailureFilter{Channels: httpkit.Values(r, "channel"), Q: q.Q, SortDesc: true, Limit: q.Limit, Offset: q.Offset}
	if len(q.Sort) > 0 {
		f.SortDesc = q.Sort[0].Desc
	}
	items, total, err := h.svc.NoticeFailures(r.Context(), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func taskPayload(t usecase.QueueTask) map[string]any {
	return map[string]any{"queue": t.Queue, "task_id": t.ID, "type": t.Type, "state": t.State}
}

func (h *Handler) audit(r *http.Request, action string, id *uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, Resource, id, payload, r)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, usecase.ErrQueueUnavailable):
		response.ServiceUnavailable(w, r, CodeQueueUnavailable, "The task queue cannot be inspected right now")
	case errors.Is(err, usecase.ErrInvalidState):
		response.ValidationError(w, r, []response.Detail{{Field: "state", Message: err.Error(), Code: "invalid"}})
	case errors.Is(err, usecase.ErrTaskNotFound):
		response.NotFound(w, r, "Queue task was not found")
	case errors.Is(err, usecase.ErrTaskNotFailed):
		response.Conflict(w, r, CodeTaskNotFailed, err.Error())
	case errors.Is(err, jobsusecase.ErrNotFound):
		response.NotFound(w, r, "Integration job was not found")
	case errors.Is(err, jobsusecase.ErrNotRetryable):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeNotRetryable, err.Error(),
			[]response.Detail{{Code: "not_retryable", Message: err.Error()}})
	case errors.Is(err, jobsusecase.ErrBusy):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeBusy, err.Error(),
			[]response.Detail{{Code: "busy", Message: err.Error()}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
