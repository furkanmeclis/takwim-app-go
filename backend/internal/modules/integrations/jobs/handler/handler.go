// Package handler is the HTTP layer of /v1/tenant/integration-jobs (TEC-880):
// a job with its attempts, and the staff retry. Behind RequireOrganization +
// tenant.integrations.manage; another organization's uuid answers 404.
package handler

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Resource is the activity resource of integration jobs.
const Resource = "tenant.integration_jobs"

// Error codes.
const (
	CodeNotRetryable = "INTEGRATION_JOB_NOT_RETRYABLE"
	CodeUnverified   = "INTEGRATION_JOB_UNVERIFIED"
	CodeBusy         = "INTEGRATION_JOB_BUSY"
)

// Handler serves integration jobs.
type Handler struct {
	runner   *usecase.Runner
	activity *activity.Recorder
}

// New builds the handler (rec may be nil).
func New(runner *usecase.Runner, rec *activity.Recorder) *Handler {
	return &Handler{runner: runner, activity: rec}
}

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

// Get serves GET /v1/tenant/integration-jobs/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	d, err := h.runner.Detail(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

// Retry serves POST /v1/tenant/integration-jobs/{uuid}/retry: CheckStatus
// first, resubmit only when the provider confirms the work was not done.
func (h *Handler) Retry(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if _, err := h.runner.Detail(r.Context(), orgID(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	res, err := h.runner.Retry(r.Context(), orgID(r), id)
	var ve *usecase.VerifyError
	if errors.As(err, &ve) {
		h.audit(r, id, map[string]any{"result": "unverified", "outcome": string(ve.Outcome)})
		d, _ := h.runner.Detail(r.Context(), orgID(r), id)
		response.ErrorWithData(w, r, http.StatusConflict, CodeUnverified, ve.Error(),
			[]response.Detail{{Code: "unverified", Message: ve.Message}}, d)
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, id, map[string]any{"result": res.Action, "status": res.Status})
	d, err := h.runner.Detail(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"action": res.Action, "job": d})
}

func (h *Handler) audit(r *http.Request, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, "integrations.job_retried", Resource, &id, payload, r)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Integration job was not found")
	case errors.Is(err, usecase.ErrNotRetryable):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeNotRetryable, err.Error(),
			[]response.Detail{{Code: "not_retryable", Message: err.Error()}})
	case errors.Is(err, usecase.ErrBusy):
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeBusy, err.Error(),
			[]response.Detail{{Code: "busy", Message: err.Error()}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
