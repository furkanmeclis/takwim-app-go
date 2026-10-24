package handler

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// CodeNotOpen: the booking's confirmation takes no answer now (none
// planned, already answered or closed, the visit is not ahead).
const CodeNotOpen = "CONFIRMATION_NOT_OPEN"

// SetResourceEvents publishes a phone confirmation on the reservations
// live channel (the confirmation list listens there too).
func (h *Handler) SetResourceEvents(pub resourceevents.Publisher) { h.events = pub }

// ListMeta is the table contract of GET /v1/tenant/confirmations: a
// read-only work list with a quick confirm.
func ListMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.ListSlug,
		DefaultSort:   "starts_at",
		DefaultFields: []string{"number", "customer", "starts_at", "approval_status", "state", "attempts", "deadline_at"},
		Capabilities: resourcemeta.Capabilities{
			Read: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"customer.name", "customer.phone", "number"},
		SortableFields:   []string{"starts_at", "deadline_at", "number", "customer", "updated_at"},
		FilterableFields: []string{"branch", "state", "attention", "starts_from", "starts_to"},
		Columns: []resourcemeta.Column{
			{Key: "number", LabelKey: "confirmations.list.columns.number", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "customer", LabelKey: "confirmations.list.columns.customer", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "starts_at", LabelKey: "confirmations.list.columns.starts_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
			{Key: "branch", LabelKey: "confirmations.list.columns.branch", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "approval_status", LabelKey: "confirmations.list.columns.approval_status", Type: resourcemeta.ColumnTypeEnum, DefaultVisible: true},
			{Key: "state", LabelKey: "confirmations.list.columns.state", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "attempts", LabelKey: "confirmations.list.columns.attempts", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "last_send", LabelKey: "confirmations.list.columns.last_send", Type: resourcemeta.ColumnTypeEnum, DefaultVisible: false},
			{Key: "answer", LabelKey: "confirmations.list.columns.answer", Type: resourcemeta.ColumnTypeEnum, DefaultVisible: false},
			{Key: "deadline_at", LabelKey: "confirmations.list.columns.deadline_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
			{Key: "updated_at", LabelKey: "confirmations.list.columns.updated_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "confirmations.list.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "state", LabelKey: "confirmations.list.columns.state", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "confirmation_state"},
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}

// Meta serves GET /v1/tenant/confirmations/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, ListMeta())
}

// parseListFilter reads the branch, state, attention and time filters.
func parseListFilter(values map[string][]string) (model.ListFilter, []response.Detail) {
	var f model.ListFilter
	csv := func(key string) []string {
		var out []string
		for _, raw := range values[key] {
			out = append(out, apiquery.SplitCSV(raw)...)
		}
		return out
	}
	for _, v := range csv("branch") {
		id, err := uuid.Parse(v)
		if err != nil {
			return f, []response.Detail{{Field: "branch", Message: "branch must list uuids", Code: "invalid"}}
		}
		f.Branches = append(f.Branches, id)
	}
	for _, v := range csv("state") {
		if !slices.Contains(model.States, v) {
			return f, []response.Detail{{Field: "state", Message: "state must be one of " + strings.Join(model.States, ", "), Code: "invalid"}}
		}
		f.States = append(f.States, v)
	}
	switch v := first(values, "attention"); v {
	case "":
	case "true", "false":
		b := v == "true"
		f.Attention = &b
	default:
		return f, []response.Detail{{Field: "attention", Message: "attention must be true or false", Code: "invalid"}}
	}
	for _, key := range []string{"starts_from", "starts_to"} {
		raw := strings.TrimSpace(first(values, key))
		if raw == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return f, []response.Detail{{Field: key, Message: key + " must be an RFC 3339 time", Code: "invalid"}}
		}
		if key == "starts_from" {
			f.StartsFrom = &t
		} else {
			f.StartsTo = &t
		}
	}
	return f, nil
}

func first(values map[string][]string, key string) string {
	if list := values[key]; len(list) > 0 {
		return strings.TrimSpace(list[0])
	}
	return ""
}

// List serves GET /v1/tenant/confirmations.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden"}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(usecase.ListSortColumns)); err != nil {
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
	items, total, err := h.engine.List(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Facets serves GET /v1/tenant/confirmations/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	f, details := parseListFilter(values)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	f.Q = first(values, "q")
	out, err := h.engine.Facets(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Summary serves GET /v1/tenant/confirmations/summary.
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	out, err := h.engine.Summary(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Reservation was not found")
		return uuid.Nil, false
	}
	return id, true
}

// Attempts serves GET /v1/tenant/reservations/{uuid}/confirmation/attempts.
func (h *Handler) Attempts(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.engine.Attempts(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

// Confirm serves POST /v1/tenant/reservations/{uuid}/confirmation/confirm:
// the customer confirmed on the phone.
func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.ConfirmInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	if err := h.engine.ConfirmByPhone(r.Context(), orgctx.Must(r.Context()).ID, id, p.UserInternal, in); err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.RecordEntry(r.Context(), activity.Entry{
			ActorID: &p.UserInternal, Action: "reservations.confirmed", Resource: reservationsmodel.Slug, ResourceUUID: &id,
			Payload: map[string]any{"source": model.SourcePhone, "note": strings.TrimSpace(in.Note)},
			Request: r,
		})
	}
	if h.events != nil {
		h.events.Publish(r.Context(), reservationsmodel.Slug, resourceevents.ActionUpdated, id.String())
	}
	response.JSON(w, r, http.StatusOK, ConfirmResult{UUID: id, ConfirmationStatus: reservationsmodel.ConfirmationConfirmed})
}

// ConfirmResult is the answer of a phone confirmation.
type ConfirmResult struct {
	UUID               uuid.UUID `json:"uuid"`
	ConfirmationStatus string    `json:"confirmation_status"`
}
