// Package handler serves /v1/tenant/settings. The organization comes from
// orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Handler serves the settings routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// VersionsMeta is the table contract of GET /v1/tenant/settings/versions.
func VersionsMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      "tenant.settings.versions",
		DefaultSort:   "-version",
		DefaultFields: []string{"version", "created_at", "created_by", "ready_for_online_sales"},
		Capabilities:  resourcemeta.Capabilities{Read: true, Sort: true},
		SortableFields: []string{
			"version",
		},
		SearchableFields: []string{},
		FilterableFields: []string{},
		Columns: []resourcemeta.Column{
			{Key: "version", LabelKey: "tenant_settings.versions.columns.version", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "created_at", LabelKey: "tenant_settings.versions.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, DefaultVisible: true},
			{Key: "created_by", LabelKey: "tenant_settings.versions.columns.created_by", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "ready_for_online_sales", LabelKey: "tenant_settings.versions.columns.ready", Type: resourcemeta.ColumnTypeBoolean, DefaultVisible: true},
		},
		Filters:     []resourcemeta.Filter{},
		Includes:    []string{},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// Get serves GET /v1/tenant/settings.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	v, err := h.svc.Current(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// Put serves PUT /v1/tenant/settings: stores a new version.
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	p := authctx.MustPrincipal(r.Context())
	var in model.SaveInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	v, err := h.svc.Save(r.Context(), scope.ID, p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), &p.UserInternal, "settings.updated", model.Resource, nil, map[string]any{
			"version": v.Version, "ready_for_online_sales": v.ReadyForOnlineSales,
		}, r)
	}
	response.JSON(w, r, http.StatusOK, v)
}

// VersionsMetaH serves GET /v1/tenant/settings/versions/meta.
func (h *Handler) VersionsMetaH(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, VersionsMeta())
}

// ListVersions serves GET /v1/tenant/settings/versions.
func (h *Handler) ListVersions(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns{"version": "version"}); err != nil {
		writeError(w, r, err)
		return
	}
	desc := true
	if len(q.Sort) > 0 {
		desc = q.Sort[0].Desc
	}
	items, total, err := h.svc.List(r.Context(), scope.ID, desc, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetVersion serves GET /v1/tenant/settings/versions/{version}.
func (h *Handler) GetVersion(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	n, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || n <= 0 {
		response.ValidationError(w, r, []response.Detail{{Field: "version", Message: "version must be a positive number", Code: "invalid"}})
		return
	}
	v, err := h.svc.Get(r.Context(), scope.ID, n)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var qve *apiquery.ValidationError
	if errors.As(err, &qve) {
		details := make([]response.Detail, 0, len(qve.Details))
		for _, d := range qve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
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
	case errors.Is(err, usecase.ErrVersionNotFound):
		response.NotFound(w, r, "Settings version was not found")
	case errors.Is(err, usecase.ErrVersionConflict):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "base_version", Message: err.Error(), Code: "stale",
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
