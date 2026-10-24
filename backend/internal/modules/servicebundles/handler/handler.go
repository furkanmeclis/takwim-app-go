// Package handler is the HTTP layer of the combined service bundles
// (/v1/tenant/service-bundles, TEC-895). Every handler runs behind
// RequireOrganization and reads the organization id from orgctx.Scope.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Handler serves /v1/tenant/service-bundles.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	events   resourceevents.Publisher
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// SetResourceEvents publishes live list changes (optional).
func (h *Handler) SetResourceEvents(pub resourceevents.Publisher) {
	h.events = pub
}

// Meta is the table contract of GET /v1/tenant/service-bundles.
func Meta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.Slug,
		DefaultFields: []string{"uuid", "name", "items", "duration_minutes", "price", "list_price", "show_online", "is_active"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"name", "description"},
		SortableFields:   []string{"name", "price", "sort_order", "is_active", "created_at", "updated_at"},
		FilterableFields: []string{"branch", "is_active", "show_online"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "service_bundles.columns.name", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "items", LabelKey: "service_bundles.columns.items", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "duration_minutes", LabelKey: "service_bundles.columns.duration", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "price", LabelKey: "service_bundles.columns.price", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "list_price", LabelKey: "service_bundles.columns.list_price", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "branch", LabelKey: "service_bundles.columns.branches", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "show_online", LabelKey: "service_bundles.columns.show_online", Type: resourcemeta.ColumnTypeBoolean, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "is_active", LabelKey: "service_bundles.columns.is_active", Type: resourcemeta.ColumnTypeBoolean, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "sort_order", LabelKey: "service_bundles.columns.sort_order", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: false},
			{Key: "created_at", LabelKey: "service_bundles.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "service_bundles.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "branch", LabelKey: "service_bundles.columns.branches", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "service_bundles.facets.branch"},
			{Key: "is_active", LabelKey: "service_bundles.columns.is_active", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
			{Key: "show_online", LabelKey: "service_bundles.columns.show_online", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
		},
		Includes:    []string{},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// Meta serves GET /v1/tenant/service-bundles/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/service-bundles/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	out, err := h.svc.Facets(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// parseListFilter reads branch (uuids), is_active and show_online.
func parseListFilter(values map[string][]string) (model.ListFilter, []response.Detail) {
	var f model.ListFilter
	var details []response.Detail
branches:
	for _, raw := range values["branch"] {
		for _, part := range apiquery.SplitCSV(raw) {
			id, err := uuid.Parse(part)
			if err != nil {
				details = append(details, response.Detail{Field: "branch", Message: "branch must list uuids", Code: "invalid"})
				break branches
			}
			f.Branches = append(f.Branches, id)
		}
	}
	for _, b := range []struct {
		key string
		dst **bool
	}{{"is_active", &f.IsActive}, {"show_online", &f.ShowOnline}} {
		if list := values[b.key]; len(list) > 0 && strings.TrimSpace(list[0]) != "" {
			v, err := strconv.ParseBool(strings.TrimSpace(list[0]))
			if err != nil {
				details = append(details, response.Detail{Field: b.key, Message: b.key + " must be true or false", Code: "invalid"})
				continue
			}
			*b.dst = &v
		}
	}
	return f, details
}

// List serves GET /v1/tenant/service-bundles.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(usecase.SortColumns)); err != nil {
		writeError(w, r, err)
		return
	}
	f, details := parseListFilter(values)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.List(r.Context(), scope.ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Create serves POST /v1/tenant/service-bundles.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	b, err := h.svc.Create(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "service_bundles.created", b, nil)
	response.JSON(w, r, http.StatusCreated, b)
}

// Get serves GET /v1/tenant/service-bundles/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	b, err := h.svc.Get(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, b)
}

// Patch serves PATCH /v1/tenant/service-bundles/{uuid}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	before, beforeErr := h.svc.Get(r.Context(), scope.ID, id)
	b, err := h.svc.Patch(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var changes map[string]activity.Change
	if beforeErr == nil {
		changes = activity.Diff(diffView(before), diffView(b))
	}
	h.record(r, "service_bundles.updated", b, changes)
	response.JSON(w, r, http.StatusOK, b)
}

// Delete serves DELETE /v1/tenant/service-bundles/{uuid} (soft delete).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	b, err := h.svc.Delete(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "service_bundles.deleted", b, nil)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// Activity serves GET /v1/tenant/service-bundles/{uuid}/activity.
func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.Activity(r.Context(), scope.ID, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// diffView is the part of a bundle an edit changes.
func diffView(b model.Bundle) map[string]any {
	items := make([]string, 0, len(b.Items))
	for _, it := range b.Items {
		items = append(items, it.ServiceName)
	}
	branches := make([]string, 0, len(b.Branches))
	for _, br := range b.Branches {
		branches = append(branches, br.Name)
	}
	return map[string]any{
		"name": b.Name, "description": b.Description, "price": b.Price.String(), "items": items,
		"branches": branches, "show_online": b.ShowOnline, "is_active": b.IsActive, "sort_order": b.SortOrder,
	}
}

// record writes the activity event (business level: no branch) and
// publishes the list change.
func (h *Handler) record(r *http.Request, action string, b model.Bundle, changes map[string]activity.Change) {
	if h.events != nil {
		ev := resourceevents.ActionUpdated
		switch action {
		case "service_bundles.created":
			ev = resourceevents.ActionCreated
		case "service_bundles.deleted":
			ev = resourceevents.ActionDeleted
		}
		h.events.Publish(r.Context(), model.Slug, ev, b.UUID.String())
	}
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	id := b.UUID
	h.activity.RecordEntry(r.Context(), activity.Entry{
		ActorID: &p.UserInternal, Action: action, Resource: model.Slug, ResourceUUID: &id,
		Payload: map[string]any{"name": b.Name}, Changes: changes, Request: r,
	})
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "uuid", Message: "uuid is invalid", Code: "invalid"}})
		return uuid.Nil, false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
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
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Service bundle was not found")
	case errors.Is(err, usecase.ErrNameTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "name", Message: err.Error(), Code: "taken",
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
