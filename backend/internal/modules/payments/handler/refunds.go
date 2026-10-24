package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

// RefundResponse is the answer of a refund request.
type RefundResponse struct {
	Refund   model.Refund              `json:"refund"`
	Payments model.ReservationPayments `json:"payments"`
	Replayed bool                      `json:"replayed"`
}

// Refund serves POST /v1/tenant/reservations/{uuid}/refunds.
func (h *Handler) Refund(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.RefundInput
	if !decode(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, err := h.svc.Refund(r.Context(), orgctx.Must(r.Context()).ID, id, c, p.UserInternal, viewOptions(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if out.Replayed {
		status = http.StatusOK
	} else if h.activity != nil {
		ref := out.Refund.UUID.String()
		h.activity.RecordEntry(r.Context(), activity.Entry{
			ActorID: &p.UserInternal, Action: "reservations.refund_requested", Resource: reservationsmodel.Slug, ResourceUUID: &id,
			Payload: map[string]any{
				"number": out.Payments.Reservation.Number, "refund_uuid": ref, "amount": in.Amount.String(),
				"currency": out.Payments.Currency, "channel": in.Channel, "status": out.Refund.Status,
				"provider_status": out.Refund.ProviderStatus,
			},
			Request: r,
		})
	}
	response.JSON(w, r, status, RefundResponse{Refund: out.Refund, Payments: out.Payments, Replayed: out.Replayed})
}

// ResolveRefund serves POST /v1/tenant/refunds/{uuid}/resolve: a person
// settles a provider refund whose result was unknown.
func (h *Handler) ResolveRefund(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Refund was not found")
		return
	}
	var in model.ResolveInput
	if !decode(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, err := h.svc.Resolve(r.Context(), orgctx.Must(r.Context()).ID, id, c, p.UserInternal, viewOptions(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		res := out.Payments.Reservation.UUID
		h.activity.RecordEntry(r.Context(), activity.Entry{
			ActorID: &p.UserInternal, Action: "reservations.refund_resolved", Resource: reservationsmodel.Slug, ResourceUUID: &res,
			Payload: map[string]any{
				"number": out.Payments.Reservation.Number, "refund_uuid": id.String(), "outcome": in.Outcome,
				"status": out.Refund.Status,
			},
			Request: r,
		})
	}
	response.JSON(w, r, http.StatusOK, RefundResponse{Refund: out.Refund, Payments: out.Payments})
}

// CloseWithoutRefund serves POST
// /v1/tenant/reservations/{uuid}/close-without-refund: the refund to make
// of a visit is closed with a reason, the money stays with the business
// (TEC-978).
func (h *Handler) CloseWithoutRefund(w http.ResponseWriter, r *http.Request) {
	c, ok := caller(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.CloseInput
	if !decode(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, err := h.svc.CloseWithoutRefund(r.Context(), orgctx.Must(r.Context()).ID, id, c, p.UserInternal, viewOptions(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.RecordEntry(r.Context(), activity.Entry{
			ActorID: &p.UserInternal, Action: "reservations.refund_closed", Resource: reservationsmodel.Slug, ResourceUUID: &id,
			Payload: map[string]any{
				"number": out.Reservation.Number, "currency": out.Currency, "held": out.Refundable.String(),
				"reason": strings.TrimSpace(in.Reason),
			},
			Request: r,
		})
	}
	response.JSON(w, r, http.StatusOK, out)
}

// FollowUpsMeta is the table contract of GET /v1/tenant/payments/follow-ups:
// a read-only work list (refunds to make or settle, unpaid visits).
func FollowUpsMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.FollowUpsSlug,
		DefaultSort:   "-starts_at",
		DefaultFields: []string{"number", "customer", "starts_at", "kind", "payment_status", "amount"},
		Capabilities: resourcemeta.Capabilities{
			Read: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"customer.name", "number"},
		SortableFields:   []string{"starts_at", "amount", "number", "updated_at"},
		FilterableFields: []string{"branch", "kind"},
		Columns: []resourcemeta.Column{
			{Key: "number", LabelKey: "reservations.follow_ups.columns.number", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "customer", LabelKey: "reservations.follow_ups.columns.customer", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "starts_at", LabelKey: "reservations.follow_ups.columns.starts_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
			{Key: "kind", LabelKey: "reservations.follow_ups.columns.kind", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "status", LabelKey: "reservations.follow_ups.columns.status", Type: resourcemeta.ColumnTypeEnum, DefaultVisible: false},
			{Key: "payment_status", LabelKey: "reservations.follow_ups.columns.payment_status", Type: resourcemeta.ColumnTypeEnum, DefaultVisible: true},
			{Key: "branch", LabelKey: "reservations.follow_ups.columns.branch", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "amount", LabelKey: "reservations.follow_ups.columns.amount", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "last_refund_status", LabelKey: "reservations.follow_ups.columns.last_refund", Type: resourcemeta.ColumnTypeEnum, DefaultVisible: false},
			{Key: "updated_at", LabelKey: "reservations.follow_ups.columns.updated_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "reservations.follow_ups.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "kind", LabelKey: "reservations.follow_ups.columns.kind", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "payment_follow_up_kind"},
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}

// FollowUpsMeta serves GET /v1/tenant/payments/follow-ups/meta.
func (h *Handler) FollowUpsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, FollowUpsMeta())
}

// parseFollowUpFilter reads the branch and kind filters.
func parseFollowUpFilter(values map[string][]string) (model.FollowUpFilter, []response.Detail) {
	var f model.FollowUpFilter
	var details []response.Detail
	for _, raw := range values["branch"] {
		for _, part := range apiquery.SplitCSV(raw) {
			id, err := uuid.Parse(part)
			if err != nil {
				return f, []response.Detail{{Field: "branch", Message: "branch must list uuids", Code: "invalid"}}
			}
			f.Branches = append(f.Branches, id)
		}
	}
	for _, raw := range values["kind"] {
		for _, v := range apiquery.SplitCSV(raw) {
			if !slices.Contains(model.FollowUpKinds, v) {
				details = append(details, response.Detail{
					Field: "kind", Message: "kind must be one of " + strings.Join(model.FollowUpKinds, ", "), Code: "invalid",
				})
				return f, details
			}
			f.Kinds = append(f.Kinds, v)
		}
	}
	return f, details
}

// ListFollowUps serves GET /v1/tenant/payments/follow-ups.
func (h *Handler) ListFollowUps(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden"}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(usecase.FollowUpSortColumns)); err != nil {
		writeError(w, r, err)
		return
	}
	f, details := parseFollowUpFilter(values)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListFollowUps(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// FollowUpFacets serves GET /v1/tenant/payments/follow-ups/facets.
func (h *Handler) FollowUpFacets(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	f, details := parseFollowUpFilter(values)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	out, err := h.svc.FollowUpFacets(r.Context(), orgctx.Must(r.Context()).ID, f.Branches, strings.TrimSpace(values.Get("q")))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
