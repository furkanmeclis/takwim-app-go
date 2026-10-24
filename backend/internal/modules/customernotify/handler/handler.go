// Package handler is the HTTP layer of the customer notifications
// (TEC-911): the business's rules and texts and the send history. Every
// handler runs behind RequireOrganization and reads the organization from
// orgctx.Scope.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Handler serves /v1/tenant/customer-notifications.
type Handler struct {
	engine   *usecase.Engine
	activity *activity.Recorder
}

// New builds the handler. rec may be nil.
func New(engine *usecase.Engine, rec *activity.Recorder) *Handler {
	return &Handler{engine: engine, activity: rec}
}

// GetSettings serves GET /v1/tenant/customer-notifications/settings.
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	out, err := h.engine.Settings(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PutSettings serves PUT /v1/tenant/customer-notifications/settings: one
// event's rule and texts.
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var in model.SaveInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, err := h.engine.Save(r.Context(), orgctx.Must(r.Context()).ID, p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	channels := make([]string, 0, len(in.Templates))
	for _, t := range in.Templates {
		channels = append(channels, t.Channel)
	}
	h.record(r, "customer_notifications.updated", map[string]any{
		"event": in.Event, "enabled": in.Enabled, "channels": out.Rule.Channels, "fallback": in.Fallback,
		"reminder_offsets": out.Rule.ReminderOffsets, "templates": channels,
	})
	response.JSON(w, r, http.StatusOK, out)
}

// Reset serves POST /v1/tenant/customer-notifications/settings/reset.
func (h *Handler) Reset(w http.ResponseWriter, r *http.Request) {
	var in model.ResetInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.engine.Reset(r.Context(), orgctx.Must(r.Context()).ID, in.Event)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "customer_notifications.reset", map[string]any{"event": in.Event})
	response.JSON(w, r, http.StatusOK, out)
}

// Preview serves POST /v1/tenant/customer-notifications/preview.
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	var in model.PreviewInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.engine.Preview(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// DeliveryStatuses are the attempt statuses (filter values).
var DeliveryStatuses = []string{usecase.DeliverySending, usecase.DeliverySent, usecase.DeliveryDelivered, usecase.DeliveryFailed, usecase.DeliverySkipped}

// EventCodes are the catalog events (filter values).
func EventCodes() []string {
	out := []string{}
	for _, e := range catalog.All() {
		out = append(out, e.Code)
	}
	return out
}

// Deliveries serves GET /v1/tenant/customer-notifications/deliveries.
func (h *Handler) Deliveries(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	q := apiquery.Parse(values)
	sorts := apiquery.SortColumns{}
	for _, s := range usecase.DeliverySortFields() {
		sorts[s] = s
	}
	if err := apiquery.ValidateSort(q.Sort, sorts); err != nil {
		writeError(w, r, err)
		return
	}
	var details []response.Detail
	enum := func(key string, allowed []string) []string {
		var out []string
		for _, raw := range values[key] {
			for _, v := range apiquery.SplitCSV(raw) {
				ok := false
				for _, a := range allowed {
					ok = ok || a == v
				}
				if !ok {
					details = append(details, response.Detail{Field: key, Message: key + " has an unknown value", Code: "invalid"})
					return nil
				}
				out = append(out, v)
			}
		}
		return out
	}
	one := func(key string) *uuid.UUID {
		raw := strings.TrimSpace(values.Get(key))
		if raw == "" {
			return nil
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			details = append(details, response.Detail{Field: key, Message: key + " must be a uuid", Code: "invalid"})
			return nil
		}
		return &id
	}
	instant := func(key string) *time.Time {
		raw := strings.TrimSpace(values.Get(key))
		if raw == "" {
			return nil
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			details = append(details, response.Detail{Field: key, Message: key + " must be an RFC 3339 time", Code: "invalid"})
			return nil
		}
		return &t
	}
	f := model.DeliveryFilter{
		Channels: enum("channel", channel.Known), Statuses: enum("status", DeliveryStatuses), Events: enum("event", EventCodes()),
		CustomerUUID: one("customer_uuid"), ReservationUUID: one("reservation_uuid"),
		CreatedFrom: instant("created_from"), CreatedTo: instant("created_to"),
		Q: q.Q, Limit: q.Limit, Offset: q.Offset,
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.engine.Deliveries(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// DeliveriesMeta serves GET /v1/tenant/customer-notifications/deliveries/meta.
func (h *Handler) DeliveriesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Meta is the table contract of the send history.
func Meta() resourcemeta.ResourceMeta {
	label := func(k string) string { return "customer_notifications.columns." + k }
	enum := func(key string) resourcemeta.Column {
		return resourcemeta.Column{
			Key: key, LabelKey: label(key), Type: resourcemeta.ColumnTypeEnum, Sortable: true,
			Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true,
		}
	}
	return resourcemeta.ResourceMeta{
		Resource:      usecase.Resource,
		DefaultSort:   "-created_at",
		DefaultFields: []string{"created_at", "event", "customer", "reservation", "channel", "recipient", "status"},
		Capabilities: resourcemeta.Capabilities{
			Read: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"customer", "reservation"},
		SortableFields:   usecase.DeliverySortFields(),
		FilterableFields: []string{"channel", "status", "event", "customer_uuid", "reservation_uuid", "created_from", "created_to"},
		Columns: []resourcemeta.Column{
			{Key: "created_at", LabelKey: label("created_at"), Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
			enum("event"),
			{Key: "customer", LabelKey: label("customer"), Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "reservation", LabelKey: label("reservation"), Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			enum("channel"),
			{Key: "recipient", LabelKey: label("recipient"), Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			enum("status"),
			{Key: "error", LabelKey: label("error"), Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "provider", LabelKey: label("provider"), Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "event", LabelKey: label("event"), Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "customer_notifications.events"},
			{Key: "channel", LabelKey: label("channel"), Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "customer_notifications.channels"},
			{Key: "status", LabelKey: label("status"), Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "customer_notifications.statuses"},
		},
		Includes: []string{},
	}
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
	var qve *apiquery.ValidationError
	if errors.As(err, &qve) {
		details := make([]response.Detail, 0, len(qve.Details))
		for _, d := range qve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	if ve, ok := usecase.IsValidation(err); ok {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	response.InternalErr(w, r, err, "Unexpected server error")
}
