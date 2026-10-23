// Package handler is the HTTP layer of the sales module. Every handler runs
// behind RequireOrganization and reads the organization id from
// orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/tenant/sales/*.
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

// ---------------------------------------------------------------- filters

type filterParts struct {
	branches    []uuid.UUID
	kinds       []string
	reservation *uuid.UUID
	customer    *uuid.UUID
	dateFrom    *time.Time
	dateTo      *time.Time
}

func parseUUIDList(values map[string][]string, key string, details *[]response.Detail) []uuid.UUID {
	var out []uuid.UUID
	for _, raw := range values[key] {
		for _, part := range apiquery.SplitCSV(raw) {
			id, err := uuid.Parse(part)
			if err != nil {
				*details = append(*details, response.Detail{Field: key, Message: key + " must list uuids", Code: "invalid"})
				return nil
			}
			out = append(out, id)
		}
	}
	return out
}

func parseUUID(values map[string][]string, key string, details *[]response.Detail) *uuid.UUID {
	list := values[key]
	if len(list) == 0 || strings.TrimSpace(list[0]) == "" {
		return nil
	}
	id, err := uuid.Parse(strings.TrimSpace(list[0]))
	if err != nil {
		*details = append(*details, response.Detail{Field: key, Message: key + " must be a uuid", Code: "invalid"})
		return nil
	}
	return &id
}

func parseDate(values map[string][]string, key string, details *[]response.Detail) *time.Time {
	list := values[key]
	if len(list) == 0 || strings.TrimSpace(list[0]) == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", strings.TrimSpace(list[0]))
	if err != nil {
		*details = append(*details, response.Detail{Field: key, Message: key + " must be YYYY-MM-DD", Code: "invalid"})
		return nil
	}
	return &t
}

func parseEnum(values map[string][]string, key string, allowed []string, details *[]response.Detail) []string {
	var out []string
	for _, raw := range values[key] {
		for _, v := range apiquery.SplitCSV(raw) {
			if !slices.Contains(allowed, v) {
				*details = append(*details, response.Detail{Field: key, Message: key + " must be one of " + strings.Join(allowed, ", "), Code: "invalid"})
				return nil
			}
			out = append(out, v)
		}
	}
	return out
}

func parseCommon(values map[string][]string, kinds []string, details *[]response.Detail) filterParts {
	return filterParts{
		branches:    parseUUIDList(values, "branch", details),
		kinds:       parseEnum(values, "kind", kinds, details),
		reservation: parseUUID(values, "reservation", details),
		customer:    parseUUID(values, "customer", details),
		dateFrom:    parseDate(values, "date_from", details),
		dateTo:      parseDate(values, "date_to", details),
	}
}

// ParseLineFilter reads the sale line list filters.
func ParseLineFilter(values map[string][]string) (model.LineFilter, []response.Detail) {
	var details []response.Detail
	c := parseCommon(values, model.LineKinds, &details)
	f := model.LineFilter{
		Branches: c.branches, Kinds: c.kinds, ReservationUUID: c.reservation, CustomerUUID: c.customer,
		DateFrom: c.dateFrom, DateTo: c.dateTo,
		ItemTypes: parseEnum(values, "item_type", model.ItemTypes, &details),
		Staff:     parseUUIDList(values, "staff", &details),
	}
	return f, details
}

// ParseEntryFilter reads the ledger list filters.
func ParseEntryFilter(values map[string][]string) (model.EntryFilter, []response.Detail) {
	var details []response.Detail
	c := parseCommon(values, model.EntryKinds, &details)
	return model.EntryFilter{
		Branches: c.branches, Kinds: c.kinds, ReservationUUID: c.reservation, CustomerUUID: c.customer,
		DateFrom: c.dateFrom, DateTo: c.dateTo,
	}, details
}

// listQuery reads limit / offset / q / sort; false when it answered.
func listQuery(w http.ResponseWriter, r *http.Request, sortable map[string]string) (apiquery.Query, bool) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden"}})
		return apiquery.Query{}, false
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(sortable)); err != nil {
		writeError(w, r, err)
		return q, false
	}
	return q, true
}

// ------------------------------------------------------------- sale lines

// LinesMeta serves GET /v1/tenant/sales/lines/meta.
func (h *Handler) LinesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, LinesMeta())
}

// ListLines serves GET /v1/tenant/sales/lines.
func (h *Handler) ListLines(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.LineSortColumns)
	if !ok {
		return
	}
	f, details := ParseLineFilter(r.URL.Query())
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListLines(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetLine serves GET /v1/tenant/sales/lines/{uuid}.
func (h *Handler) GetLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	line, err := h.svc.GetLine(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, line)
}

// ReverseLine serves POST /v1/tenant/sales/lines/{uuid}/reverse.
func (h *Handler) ReverseLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.ReverseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	rev, err := h.svc.ReverseLine(r.Context(), orgctx.Must(r.Context()).ID, id, &p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordLine(r, "sale_lines.reversed", id, rev, map[string]any{
		"reversal_uuid": rev.UUID.String(), "net_amount": rev.NetAmount.String(), "currency": rev.Currency, "reason": rev.Reason,
	})
	response.JSON(w, r, http.StatusCreated, rev)
}

// CorrectLine serves POST /v1/tenant/sales/lines/{uuid}/correct. A new
// unit price needs tenant.price.override, a new discount
// tenant.discount.apply.
func (h *Handler) CorrectLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.CorrectInput
	if !decodeJSON(w, r, &in) {
		return
	}
	scope := orgctx.Must(r.Context())
	if in.UnitPrice != nil && !scope.Has(rbac.PermTenantPriceOverride) {
		response.Forbidden(w, r, "Changing the price needs the price override permission")
		return
	}
	if in.DiscountAmount != nil && !scope.Has(rbac.PermTenantDiscountApply) {
		response.Forbidden(w, r, "Changing the discount needs the discount permission")
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, err := h.svc.CorrectLine(r.Context(), scope.ID, id, &p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordLine(r, "sale_lines.corrected", id, out.Replacement, map[string]any{
		"reversal_uuid": out.Reversal.UUID.String(), "replacement_uuid": out.Replacement.UUID.String(),
		"old_net_amount": out.Reversal.NetAmount.Neg().String(), "net_amount": out.Replacement.NetAmount.String(),
		"currency": out.Replacement.Currency, "reason": out.Replacement.Reason,
	})
	response.JSON(w, r, http.StatusCreated, out)
}

// --------------------------------------------------------- ledger entries

// EntriesMeta serves GET /v1/tenant/sales/entries/meta.
func (h *Handler) EntriesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, EntriesMeta())
}

// ListEntries serves GET /v1/tenant/sales/entries.
func (h *Handler) ListEntries(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.EntrySortColumns)
	if !ok {
		return
	}
	f, details := ParseEntryFilter(r.URL.Query())
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListEntries(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetEntry serves GET /v1/tenant/sales/entries/{uuid}.
func (h *Handler) GetEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	e, err := h.svc.GetEntry(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, e)
}

// ReverseEntry serves POST /v1/tenant/sales/entries/{uuid}/reverse: a
// collection or refund posted by mistake, with its cash movement.
func (h *Handler) ReverseEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.ReverseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	rev, err := h.svc.ReverseEntry(r.Context(), orgctx.Must(r.Context()).ID, id, &p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.publish(r, model.EntriesSlug, resourceevents.ActionCreated, rev.UUID)
	h.publish(r, model.EntriesSlug, resourceevents.ActionUpdated, id)
	if h.activity != nil {
		branch := rev.Branch.UUID
		h.activity.RecordEntry(r.Context(), activity.Entry{
			ActorID: &p.UserInternal, Action: "ledger_entries.reversed", Resource: model.EntriesSlug, ResourceUUID: &id,
			BranchUUID: &branch, Request: r, Payload: map[string]any{
				"reversal_uuid": rev.UUID.String(), "kind": rev.EffectiveKind, "amount": rev.Amount.String(),
				"currency": rev.Currency, "reason": rev.Description,
			},
		})
	}
	response.JSON(w, r, http.StatusCreated, rev)
}

// ------------------------------------------------------ balances, summary

// Summary serves GET /v1/tenant/sales/summary.
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	var details []response.Detail
	from := parseDate(values, "date_from", &details)
	to := parseDate(values, "date_to", &details)
	branches := parseUUIDList(values, "branch", &details)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	out, err := h.svc.Summary(r.Context(), orgctx.Must(r.Context()).ID, from, to, branches)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ReservationBalance serves GET /v1/tenant/sales/reservations/{uuid}/balance.
func (h *Handler) ReservationBalance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.ReservationBalance(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CustomerBalance serves GET /v1/tenant/sales/customers/{uuid}/balance.
func (h *Handler) CustomerBalance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.CustomerBalance(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ------------------------------------------------------------------ helpers

// recordLine writes the activity of a sale line change and publishes the
// line and ledger list changes.
func (h *Handler) recordLine(r *http.Request, action string, id uuid.UUID, line model.SaleLine, payload map[string]any) {
	h.publish(r, model.LinesSlug, resourceevents.ActionUpdated, id)
	h.publish(r, model.LinesSlug, resourceevents.ActionCreated, line.UUID)
	h.publish(r, model.EntriesSlug, resourceevents.ActionUpdated, id)
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	branch := line.Branch.UUID
	payload["description"] = line.Description
	if line.Reservation != nil {
		payload["reservation_uuid"] = line.Reservation.UUID.String()
	}
	h.activity.RecordEntry(r.Context(), activity.Entry{
		ActorID: &p.UserInternal, Action: action, Resource: model.LinesSlug, ResourceUUID: &id,
		BranchUUID: &branch, Payload: payload, Request: r,
	})
}

func (h *Handler) publish(r *http.Request, resource, action string, id uuid.UUID) {
	if h.events == nil {
		return
	}
	h.events.Publish(r.Context(), resource, action, id.String())
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

func conflict(w http.ResponseWriter, r *http.Request, field, code string, err error) {
	response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
		Field: field, Message: err.Error(), Code: code,
	}})
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
	case errors.Is(err, usecase.ErrLineNotFound):
		response.NotFound(w, r, "Sale line was not found")
	case errors.Is(err, usecase.ErrEntryNotFound):
		response.NotFound(w, r, "Ledger entry was not found")
	case errors.Is(err, usecase.ErrReservationMissing):
		response.NotFound(w, r, "Reservation was not found")
	case errors.Is(err, usecase.ErrCustomerMissing):
		response.NotFound(w, r, "Customer was not found")
	case errors.Is(err, usecase.ErrAlreadyReversed):
		conflict(w, r, "uuid", "already_reversed", err)
	case errors.Is(err, usecase.ErrNotReversible):
		conflict(w, r, "uuid", "not_reversible", err)
	case errors.Is(err, usecase.ErrReverseSaleLine):
		conflict(w, r, "uuid", "reverse_sale_line", err)
	case errors.Is(err, usecase.ErrHasRefunds):
		conflict(w, r, "uuid", "has_refunds", err)
	case errors.Is(err, usecase.ErrRefundTooLarge):
		conflict(w, r, "amount", "refund_too_large", err)
	case errors.Is(err, usecase.ErrNothingChanged):
		conflict(w, r, "uuid", "nothing_changed", err)
	case errors.Is(err, usecase.ErrCurrencyMismatch):
		conflict(w, r, "cash_account_uuid", "currency_mismatch", err)
	case errors.Is(err, usecase.ErrIdempotencyReused):
		conflict(w, r, "idempotency_key", "reused", err)
	case errors.Is(err, cashusecase.ErrDayClosed):
		conflict(w, r, "cash_account_uuid", "day_closed", err)
	case errors.Is(err, cashusecase.ErrInsufficientFunds):
		conflict(w, r, "amount", "insufficient_funds", err)
	case errors.Is(err, cashusecase.ErrAccountArchived):
		conflict(w, r, "cash_account_uuid", "archived", err)
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
