// Package handler is the HTTP layer of /v1/tenant/integrations. Every
// handler runs behind RequireOrganization + tenant.integrations.manage; the
// organization comes from orgctx, path uuids are looked up before bodies
// are validated (another organization's uuid answers 404). Secrets are
// accepted, never returned, and never written to the activity log (only
// the names of the changed fields).
package handler

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Resource is the activity resource of integration accounts.
const Resource = "tenant.integrations"

// CodeCheckFailed answers a failed connection test.
const CodeCheckFailed = "INTEGRATION_CHECK_FAILED"

// Sortable list fields.
var Sortable = map[string]string{"display_name": "display_name", "created_at": "created_at"}

// JobSortable are the sortable job list fields.
var JobSortable = map[string]string{"created_at": "created_at", "updated_at": "updated_at"}

// Handler serves the integration accounts.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler (rec may be nil).
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

func (h *Handler) audit(r *http.Request, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, Resource, &id, payload, r)
}

// Meta serves GET /v1/tenant/integrations/meta: the connectable providers.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, map[string]any{"providers": h.svc.Providers()})
}

// List serves GET /v1/tenant/integrations.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, Sortable)
	if !ok {
		return
	}
	f := usecase.ListFilter{
		Q: q.Q, Statuses: httpkit.Values(r, "status"), Kinds: httpkit.Values(r, "kind"),
		Providers: httpkit.Values(r, "provider"), Limit: q.Limit, Offset: q.Offset,
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.List(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Connect serves POST /v1/tenant/integrations.
func (h *Handler) Connect(w http.ResponseWriter, r *http.Request) {
	var in usecase.ConnectInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	a, err := h.svc.Connect(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "integrations.connected", a.UUID, map[string]any{
		"kind": a.Kind, "provider": a.Provider, "mode": a.Mode, "display_name": a.DisplayName,
	})
	response.JSON(w, r, http.StatusCreated, a)
}

// Get serves GET /v1/tenant/integrations/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	a, err := h.svc.Get(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, a)
}

// Update serves PATCH /v1/tenant/integrations/{uuid}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if _, err := h.svc.Get(r.Context(), orgID(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	var in usecase.UpdateInput
	if !httpkit.DecodeJSON(w, r, &in, false) {
		return
	}
	a, changed, err := h.svc.Update(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if len(changed) > 0 {
		h.audit(r, "integrations.updated", a.UUID, map[string]any{"changed": changed, "mode": a.Mode})
	}
	response.JSON(w, r, http.StatusOK, a)
}

// Test serves POST /v1/tenant/integrations/{uuid}/test.
func (h *Handler) Test(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	a, err := h.svc.Test(r.Context(), orgID(r), id)
	var ce *usecase.CheckError
	if errors.As(err, &ce) {
		h.audit(r, "integrations.tested", id, map[string]any{"result": "failed"})
		response.ErrorWithData(w, r, http.StatusUnprocessableEntity, CodeCheckFailed, "Connection test failed",
			[]response.Detail{{Code: "check_failed", Message: ce.Message}}, a)
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "integrations.tested", a.UUID, map[string]any{"result": "ok"})
	response.JSON(w, r, http.StatusOK, a)
}

func (h *Handler) status(status, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpkit.PathUUID(w, r, "uuid")
		if !ok {
			return
		}
		a, err := h.svc.SetStatus(r.Context(), orgID(r), id, status)
		if err != nil {
			writeError(w, r, err)
			return
		}
		h.audit(r, action, a.UUID, map[string]any{"status": a.Status})
		response.JSON(w, r, http.StatusOK, a)
	}
}

// Disable serves POST /v1/tenant/integrations/{uuid}/disable.
func (h *Handler) Disable(w http.ResponseWriter, r *http.Request) {
	h.status(usecase.StatusDisabled, "integrations.disabled")(w, r)
}

// Enable serves POST /v1/tenant/integrations/{uuid}/enable.
func (h *Handler) Enable(w http.ResponseWriter, r *http.Request) {
	h.status(usecase.StatusActive, "integrations.enabled")(w, r)
}

// Jobs serves GET /v1/tenant/integrations/{uuid}/jobs.
func (h *Handler) Jobs(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q, ok := httpkit.ListQuery(w, r, JobSortable)
	if !ok {
		return
	}
	f := usecase.JobFilter{
		Statuses: httpkit.Values(r, "status"), Kinds: httpkit.Values(r, "kind"), Limit: q.Limit, Offset: q.Offset,
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.Jobs(r.Context(), orgID(r), id, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		out := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			out = append(out, response.Detail{Field: d.Field, Code: d.Code, Message: d.Message})
		}
		response.ValidationError(w, r, out)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Integration account was not found")
	case errors.Is(err, usecase.ErrActiveExists):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(),
			[]response.Detail{{Field: "kind", Code: "active_exists", Message: err.Error()}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

// AllJobs serves GET /v1/tenant/integration-jobs: the organization's jobs
// across accounts (status=failed&status=needs_verification is the work list
// of jobs that need a person).
func (h *Handler) AllJobs(w http.ResponseWriter, r *http.Request) {
	q, ok := httpkit.ListQuery(w, r, JobSortable)
	if !ok {
		return
	}
	f := usecase.JobFilter{
		Statuses: httpkit.Values(r, "status"), Kinds: httpkit.Values(r, "kind"), Limit: q.Limit, Offset: q.Offset,
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.AllJobs(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}
