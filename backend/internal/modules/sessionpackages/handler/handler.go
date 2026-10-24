// Package handler is the HTTP layer of the session packages module. Every
// handler runs behind RequireOrganization and reads the organization id
// from orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/tenant/session-packages/* and
// /v1/tenant/customer-packages/*.
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

// ------------------------------------------------------------- definitions

// PackagesMeta serves GET /v1/tenant/session-packages/meta.
func (h *Handler) PackagesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, PackagesMeta())
}

// ListPackages serves GET /v1/tenant/session-packages.
func (h *Handler) ListPackages(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.PackageSortColumns)
	if !ok {
		return
	}
	values := r.URL.Query()
	var details []response.Detail
	f := model.PackageFilter{
		Q: q.Q, Services: parseUUIDList(values, "service", &details), Limit: q.Limit, Offset: q.Offset,
	}
	if b := parseUUIDList(values, "branch", &details); len(b) > 0 {
		f.Branch = &b[0]
	}
	if active := parseEnum(values, "is_active", []string{"true", "false"}, &details); len(active) == 1 {
		v := active[0] == "true"
		f.IsActive = &v
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListPackages(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetPackage serves GET /v1/tenant/session-packages/{uuid}.
func (h *Handler) GetPackage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.GetPackage(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

// CreatePackage serves POST /v1/tenant/session-packages.
func (h *Handler) CreatePackage(w http.ResponseWriter, r *http.Request) {
	var in model.PackageInput
	if !decodeJSON(w, r, &in) {
		return
	}
	pr := authctx.MustPrincipal(r.Context())
	p, err := h.svc.CreatePackage(r.Context(), orgctx.Must(r.Context()).ID, &pr.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.publish(r, model.PackagesSlug, resourceevents.ActionCreated, p.UUID)
	h.record(r, "session_packages.created", model.PackagesSlug, p.UUID, nil, packagePayload(p))
	response.JSON(w, r, http.StatusCreated, p)
}

// PatchPackage serves PATCH /v1/tenant/session-packages/{uuid} (the whole
// definition).
func (h *Handler) PatchPackage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.PackageInput
	if !decodeJSON(w, r, &in) {
		return
	}
	before, after, err := h.svc.UpdatePackage(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.publish(r, model.PackagesSlug, resourceevents.ActionUpdated, id)
	h.record(r, "session_packages.updated", model.PackagesSlug, id, nil, map[string]any{
		"before": packagePayload(before), "after": packagePayload(after),
	})
	response.JSON(w, r, http.StatusOK, after)
}

// DeletePackage serves DELETE /v1/tenant/session-packages/{uuid}.
func (h *Handler) DeletePackage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.DeletePackage(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.publish(r, model.PackagesSlug, resourceevents.ActionDeleted, id)
	h.record(r, "session_packages.deleted", model.PackagesSlug, id, nil, packagePayload(p))
	w.WriteHeader(http.StatusNoContent)
}

func packagePayload(p model.Package) map[string]any {
	out := map[string]any{
		"name": p.Name, "service": p.Service.Name, "total_sessions": p.TotalSessions, "price": p.Price.String(),
		"all_branches": p.AllBranches, "is_active": p.IsActive,
	}
	if p.ValidityDays != nil {
		out["validity_days"] = *p.ValidityDays
	}
	return out
}

// ------------------------------------------------------ customer packages

// CustomerPackagesMeta serves GET /v1/tenant/customer-packages/meta.
func (h *Handler) CustomerPackagesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, CustomerPackagesMeta())
}

// CustomerPackagesFacets serves GET /v1/tenant/customer-packages/facets.
func (h *Handler) CustomerPackagesFacets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Facets(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListCustomerPackages serves GET /v1/tenant/customer-packages.
func (h *Handler) ListCustomerPackages(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.CustomerPackageSortColumns)
	if !ok {
		return
	}
	values := r.URL.Query()
	var details []response.Detail
	f := model.CustomerPackageFilter{
		Q: q.Q, Branches: parseUUIDList(values, "branch", &details), Packages: parseUUIDList(values, "package", &details),
		States: parseEnum(values, "state", model.States, &details), DateFrom: parseDate(values, "date_from", &details),
		DateTo: parseDate(values, "date_to", &details), Limit: q.Limit, Offset: q.Offset,
	}
	if c := parseUUIDList(values, "customer", &details); len(c) > 0 {
		f.Customer = &c[0]
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListCustomerPackages(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetCustomerPackage serves GET /v1/tenant/customer-packages/{uuid}.
func (h *Handler) GetCustomerPackage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	cp, err := h.svc.GetCustomerPackage(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, cp)
}

// Sell serves POST /v1/tenant/customer-packages.
func (h *Handler) Sell(w http.ResponseWriter, r *http.Request) {
	var in model.SellInput
	if !decodeJSON(w, r, &in) {
		return
	}
	pr := authctx.MustPrincipal(r.Context())
	cp, replayed, err := h.svc.Sell(r.Context(), orgctx.Must(r.Context()).ID, &pr.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if replayed {
		response.JSON(w, r, http.StatusOK, cp)
		return
	}
	h.publish(r, model.CustomerPackagesSlug, resourceevents.ActionCreated, cp.UUID)
	branch := cp.Branch
	h.record(r, "customer_packages.sold", model.CustomerPackagesSlug, cp.UUID, &branch, map[string]any{
		"name": cp.Name, "customer_uuid": cp.Customer.UUID.String(), "customer": cp.Customer.Name,
		"price": cp.Price.String(), "currency": cp.Currency, "total_sessions": cp.TotalSessions, "method": in.Method,
	})
	response.JSON(w, r, http.StatusCreated, cp)
}

// Refund serves POST /v1/tenant/customer-packages/{uuid}/refund.
func (h *Handler) Refund(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.RefundInput
	if !decodeJSON(w, r, &in) {
		return
	}
	pr := authctx.MustPrincipal(r.Context())
	cp, err := h.svc.Refund(r.Context(), orgctx.Must(r.Context()).ID, id, &pr.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.publish(r, model.CustomerPackagesSlug, resourceevents.ActionUpdated, id)
	branch := cp.Branch
	h.record(r, "customer_packages.refunded", model.CustomerPackagesSlug, id, &branch, map[string]any{
		"name": cp.Name, "customer": cp.Customer.Name, "amount": in.Amount.String(), "currency": cp.Currency,
		"reason": strings.TrimSpace(in.Reason), "used": cp.Used, "total_sessions": cp.TotalSessions,
	})
	response.JSON(w, r, http.StatusOK, cp)
}

// Summary serves GET /v1/tenant/customer-packages/summary
// (?date_from=&date_to=&branch=; default this month).
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

// ------------------------------------------------------------------ helpers

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
			if !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	return out
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

func (h *Handler) record(r *http.Request, action, resource string, id uuid.UUID, branch *model.Ref, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	e := activity.Entry{ActorID: &p.UserInternal, Action: action, Resource: resource, ResourceUUID: &id, Payload: payload, Request: r}
	if branch != nil {
		b := branch.UUID
		e.BranchUUID = &b
	}
	h.activity.RecordEntry(r.Context(), e)
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
	var sve *salesusecase.ValidationError
	if errors.As(err, &sve) {
		details := make([]response.Detail, 0, len(sve.Details))
		for _, d := range sve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	var cve *cashusecase.ValidationError
	if errors.As(err, &cve) {
		details := make([]response.Detail, 0, len(cve.Details))
		for _, d := range cve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrPackageNotFound):
		response.NotFound(w, r, "Session package was not found")
	case errors.Is(err, usecase.ErrCustomerPackage):
		response.NotFound(w, r, "Customer package was not found")
	case errors.Is(err, cashusecase.ErrNotFound):
		response.ValidationError(w, r, []response.Detail{{Field: "cash_account_uuid", Message: "cash account not found", Code: "not_found"}})
	case errors.Is(err, usecase.ErrAllBranches):
		response.Forbidden(w, r, err.Error())
	case errors.Is(err, usecase.ErrNameTaken):
		conflict(w, r, "name", "taken", err)
	case errors.Is(err, usecase.ErrAlreadyRefunded):
		conflict(w, r, "status", "already_refunded", err)
	case errors.Is(err, usecase.ErrSessionsHeld):
		conflict(w, r, "sessions", "held", err)
	case errors.Is(err, usecase.ErrIdempotencyReused), errors.Is(err, salesusecase.ErrIdempotencyReused),
		errors.Is(err, cashusecase.ErrIdempotencyReused):
		conflict(w, r, "idempotency_key", "reused", err)
	case errors.Is(err, salesusecase.ErrCurrencyMismatch):
		conflict(w, r, "cash_account_uuid", "currency_mismatch", err)
	case errors.Is(err, salesusecase.ErrRefundTooLarge):
		response.ValidationError(w, r, []response.Detail{{Field: "amount", Message: err.Error(), Code: "exceeds_refundable"}})
	case errors.Is(err, salesusecase.ErrAlreadyReversed):
		conflict(w, r, "collection", "reversed", err)
	case errors.Is(err, cashusecase.ErrDayClosed):
		conflict(w, r, "cash_account_uuid", "day_closed", err)
	case errors.Is(err, cashusecase.ErrInsufficientFunds):
		conflict(w, r, "cash_account_uuid", "insufficient_funds", err)
	case errors.Is(err, cashusecase.ErrAccountArchived):
		conflict(w, r, "cash_account_uuid", "archived", err)
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
