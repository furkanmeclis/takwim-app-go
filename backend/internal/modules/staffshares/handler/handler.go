// Package handler is the HTTP layer of the staff shares module. Every
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Handler serves /v1/tenant/staff-shares/*.
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

// viewer resolves the reach of the caller: every share
// (tenant.earnings.read) or its own (tenant.earnings.own).
func viewer(r *http.Request) usecase.Viewer {
	scope := orgctx.Must(r.Context())
	if scope.Reach(rbac.PermTenantEarningsRead, rbac.PermTenantEarningsOwn) == orgctx.ReachOwn {
		return usecase.Viewer{OwnUserID: authctx.MustPrincipal(r.Context()).UserInternal}
	}
	return usecase.Viewer{}
}

// ---------------------------------------------------------------- filters

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
			out = append(out, v)
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

// ------------------------------------------------------------------ rules

// ListRules serves GET /v1/tenant/staff-shares/rules (?staff=&item_type=).
func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, map[string]string{})
	if !ok {
		return
	}
	values := r.URL.Query()
	var details []response.Detail
	f := model.RuleFilter{
		Staff:     parseUUIDList(values, "staff", &details),
		ItemTypes: parseEnum(values, "item_type", model.ItemTypes, &details),
		Limit:     q.Limit, Offset: q.Offset,
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	items, total, err := h.svc.ListRules(r.Context(), orgctx.Must(r.Context()).ID, viewer(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetRule serves GET /v1/tenant/staff-shares/rules/{uuid}.
func (h *Handler) GetRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rule, err := h.svc.GetRule(r.Context(), orgctx.Must(r.Context()).ID, viewer(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rule)
}

// CreateRule serves POST /v1/tenant/staff-shares/rules.
func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	var in model.RuleInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	rule, err := h.svc.CreateRule(r.Context(), orgctx.Must(r.Context()).ID, &p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.publish(r, model.RulesSlug, resourceevents.ActionCreated, rule.UUID)
	h.record(r, "staff_share_rules.created", model.RulesSlug, rule.UUID, nil, rulePayload(rule))
	response.JSON(w, r, http.StatusCreated, rule)
}

// PatchRule serves PATCH /v1/tenant/staff-shares/rules/{uuid}.
func (h *Handler) PatchRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.RuleInput
	if !decodeJSON(w, r, &in) {
		return
	}
	before, after, err := h.svc.PatchRule(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.publish(r, model.RulesSlug, resourceevents.ActionUpdated, id)
	h.record(r, "staff_share_rules.updated", model.RulesSlug, id, nil, map[string]any{
		"before": rulePayload(before), "after": rulePayload(after),
	})
	response.JSON(w, r, http.StatusOK, after)
}

// DeleteRule serves DELETE /v1/tenant/staff-shares/rules/{uuid}.
func (h *Handler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rule, err := h.svc.DeleteRule(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.publish(r, model.RulesSlug, resourceevents.ActionDeleted, id)
	h.record(r, "staff_share_rules.deleted", model.RulesSlug, id, nil, rulePayload(rule))
	w.WriteHeader(http.StatusNoContent)
}

func rulePayload(rule model.Rule) map[string]any {
	out := map[string]any{
		"staff_uuid": rule.Staff.UUID.String(), "staff": rule.Staff.Name, "item_type": rule.ItemType,
		"basis": rule.Basis, "valid_from": rule.ValidFrom,
	}
	if rule.Service != nil {
		out["service"] = rule.Service.Name
	}
	if rule.Rate != nil {
		out["rate"] = rule.Rate.String()
	}
	if rule.FixedAmount != nil {
		out["fixed_amount"] = rule.FixedAmount.String()
	}
	return out
}

// ------------------------------------------------------------- share lines

// LinesMeta serves GET /v1/tenant/staff-shares/lines/meta.
func (h *Handler) LinesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, LinesMeta())
}

// ListLines serves GET /v1/tenant/staff-shares/lines.
func (h *Handler) ListLines(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.LineSortColumns)
	if !ok {
		return
	}
	values := r.URL.Query()
	var details []response.Detail
	f := model.LineFilter{
		Branches:  parseUUIDList(values, "branch", &details),
		Staff:     parseUUIDList(values, "staff", &details),
		Kinds:     parseEnum(values, "kind", model.LineKinds, &details),
		ItemTypes: parseEnum(values, "item_type", model.ItemTypes, &details),
		DateFrom:  parseDate(values, "date_from", &details),
		DateTo:    parseDate(values, "date_to", &details),
		Limit:     q.Limit, Offset: q.Offset, Q: q.Q,
	}
	if st := parseEnum(values, "status", model.Statuses, &details); len(st) == 1 {
		f.Status = st[0]
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListLines(r.Context(), orgctx.Must(r.Context()).ID, viewer(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// ---------------------------------------------------------------- summary

// Summary serves GET /v1/tenant/staff-shares/summary
// (?date_from=&date_to=&branch=&staff=; default this month).
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	var details []response.Detail
	f := model.SummaryFilter{
		DateFrom: parseDate(values, "date_from", &details),
		DateTo:   parseDate(values, "date_to", &details),
		Branches: parseUUIDList(values, "branch", &details),
		Staff:    parseUUIDList(values, "staff", &details),
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	out, err := h.svc.Summary(r.Context(), orgctx.Must(r.Context()).ID, viewer(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Mine serves GET /v1/tenant/staff-shares/me (?date_from=&date_to=): the
// signed-in member's own shares.
func (h *Handler) Mine(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	var details []response.Detail
	from := parseDate(values, "date_from", &details)
	to := parseDate(values, "date_to", &details)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	dates := func(t *time.Time) *pgtype.Date {
		if t == nil {
			return &pgtype.Date{}
		}
		return &pgtype.Date{Time: *t, Valid: true}
	}
	out, err := h.svc.Mine(r.Context(), orgctx.Must(r.Context()).ID, authctx.MustPrincipal(r.Context()).UserInternal, dates(from), dates(to))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ---------------------------------------------------------------- payouts

// PayoutsMeta serves GET /v1/tenant/staff-shares/payouts/meta.
func (h *Handler) PayoutsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, PayoutsMeta())
}

// ListPayouts serves GET /v1/tenant/staff-shares/payouts.
func (h *Handler) ListPayouts(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.PayoutSortColumns)
	if !ok {
		return
	}
	values := r.URL.Query()
	var details []response.Detail
	f := model.PayoutFilter{
		Branches: parseUUIDList(values, "branch", &details),
		Staff:    parseUUIDList(values, "staff", &details),
		Kinds:    parseEnum(values, "kind", model.PayoutKinds, &details),
		DateFrom: parseDate(values, "date_from", &details),
		DateTo:   parseDate(values, "date_to", &details),
		Limit:    q.Limit, Offset: q.Offset, Q: q.Q,
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListPayouts(r.Context(), orgctx.Must(r.Context()).ID, viewer(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetPayout serves GET /v1/tenant/staff-shares/payouts/{uuid}.
func (h *Handler) GetPayout(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.GetPayout(r.Context(), orgctx.Must(r.Context()).ID, viewer(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

// Pay serves POST /v1/tenant/staff-shares/payouts.
func (h *Handler) Pay(w http.ResponseWriter, r *http.Request) {
	var in model.PayoutInput
	if !decodeJSON(w, r, &in) {
		return
	}
	pr := authctx.MustPrincipal(r.Context())
	p, err := h.svc.Pay(r.Context(), orgctx.Must(r.Context()).ID, &pr.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.publish(r, model.PayoutsSlug, resourceevents.ActionCreated, p.UUID)
	h.record(r, "staff_share_payouts.created", model.PayoutsSlug, p.UUID, p.Branch, map[string]any{
		"staff_uuid": p.Staff.UUID.String(), "staff": p.Staff.Name, "amount": p.Amount.String(), "currency": p.Currency,
		"cash_account": p.CashAccount.Name, "payout_date": p.PayoutDate, "note": p.Note,
	})
	response.JSON(w, r, http.StatusCreated, p)
}

// ReversePayout serves POST /v1/tenant/staff-shares/payouts/{uuid}/reverse.
func (h *Handler) ReversePayout(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.ReverseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	pr := authctx.MustPrincipal(r.Context())
	rev, err := h.svc.ReversePayout(r.Context(), orgctx.Must(r.Context()).ID, id, &pr.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.publish(r, model.PayoutsSlug, resourceevents.ActionCreated, rev.UUID)
	h.publish(r, model.PayoutsSlug, resourceevents.ActionUpdated, id)
	h.record(r, "staff_share_payouts.reversed", model.PayoutsSlug, id, rev.Branch, map[string]any{
		"reversal_uuid": rev.UUID.String(), "staff": rev.Staff.Name, "amount": rev.Amount.String(),
		"currency": rev.Currency, "reason": rev.Note,
	})
	response.JSON(w, r, http.StatusCreated, rev)
}

// ------------------------------------------------------------------ helpers

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
	case errors.Is(err, usecase.ErrRuleNotFound):
		response.NotFound(w, r, "Share rule was not found")
	case errors.Is(err, usecase.ErrPayoutNotFound):
		response.NotFound(w, r, "Payout was not found")
	case errors.Is(err, usecase.ErrNoStaffRecord):
		response.NotFound(w, r, "You have no staff record in this business")
	case errors.Is(err, cashusecase.ErrNotFound):
		response.NotFound(w, r, "Cash account was not found")
	case errors.Is(err, usecase.ErrAllBranches):
		response.Forbidden(w, r, err.Error())
	case errors.Is(err, usecase.ErrRuleTaken):
		conflict(w, r, "valid_from", "rule_taken", err)
	case errors.Is(err, usecase.ErrAlreadyReversed):
		conflict(w, r, "uuid", "already_reversed", err)
	case errors.Is(err, usecase.ErrNotReversible):
		conflict(w, r, "uuid", "not_reversible", err)
	case errors.Is(err, usecase.ErrPayoutTooLarge):
		conflict(w, r, "amount", "payout_too_large", err)
	case errors.Is(err, usecase.ErrIdempotencyReused), errors.Is(err, cashusecase.ErrIdempotencyReused):
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
