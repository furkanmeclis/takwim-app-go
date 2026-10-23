// Package handler is the HTTP layer of the cash module. Every handler runs
// behind RequireOrganization and reads the organization id from
// orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/tenant/cash/*.
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

// ------------------------------------------------------------------ accounts

// AccountsMeta serves GET /v1/tenant/cash/accounts/meta.
func (h *Handler) AccountsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, AccountsMeta())
}

// AccountFacets serves GET /v1/tenant/cash/accounts/facets.
func (h *Handler) AccountFacets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.AccountFacets(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// AccountOptions serves GET /v1/tenant/cash/accounts/options.
func (h *Handler) AccountOptions(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.AccountOptions(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

// ParseAccountFilter reads the account list filters shared by the list,
// export and bulk "all matching" scope.
func ParseAccountFilter(values map[string][]string) (model.AccountFilter, []response.Detail) {
	var f model.AccountFilter
	var details []response.Detail
	for _, raw := range values["branch"] {
		for _, part := range apiquery.SplitCSV(raw) {
			if part == model.HeadOffice {
				f.HeadOffice = true
				continue
			}
			id, err := uuid.Parse(part)
			if err != nil {
				details = append(details, response.Detail{Field: "branch", Message: "branch must list branch uuids", Code: "invalid"})
				break
			}
			f.Branches = append(f.Branches, id)
		}
	}
	for _, raw := range values["kind"] {
		f.Kinds = append(f.Kinds, apiquery.SplitCSV(raw)...)
	}
	for _, raw := range values["status"] {
		f.Statuses = append(f.Statuses, apiquery.SplitCSV(raw)...)
	}
	return f, details
}

// ListAccounts serves GET /v1/tenant/cash/accounts.
func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden"}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(usecase.AccountSortColumns)); err != nil {
		writeError(w, r, err)
		return
	}
	f, details := ParseAccountFilter(values)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListAccounts(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateAccount serves POST /v1/tenant/cash/accounts.
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var in model.AccountInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	acc, err := h.svc.CreateAccount(r.Context(), orgctx.Must(r.Context()).ID, &p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "cash_accounts.created", acc, map[string]any{"name": acc.Name, "kind": acc.Kind}, nil)
	h.publish(r, model.AccountsSlug, resourceevents.ActionCreated, acc.UUID)
	response.JSON(w, r, http.StatusCreated, acc)
}

// GetAccount serves GET /v1/tenant/cash/accounts/{uuid}.
func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	acc, err := h.svc.GetAccount(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, acc)
}

// PatchAccount serves PATCH /v1/tenant/cash/accounts/{uuid}.
func (h *Handler) PatchAccount(w http.ResponseWriter, r *http.Request) {
	orgID := orgctx.Must(r.Context()).ID
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.AccountInput
	if !decodeJSON(w, r, &in) {
		return
	}
	before, beforeErr := h.svc.GetAccount(r.Context(), orgID, id)
	acc, err := h.svc.PatchAccount(r.Context(), orgID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var changes map[string]activity.Change
	if beforeErr == nil {
		changes = activity.Diff(accountDiff(before), accountDiff(acc))
	}
	h.record(r, "cash_accounts.updated", acc, map[string]any{"name": acc.Name, "kind": acc.Kind}, changes)
	h.publish(r, model.AccountsSlug, resourceevents.ActionUpdated, acc.UUID)
	response.JSON(w, r, http.StatusOK, acc)
}

// AccountActivity serves GET /v1/tenant/cash/accounts/{uuid}/activity.
func (h *Handler) AccountActivity(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.Activity(r.Context(), orgctx.Must(r.Context()).ID, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// AccountMovements serves GET /v1/tenant/cash/accounts/{uuid}/movements.
func (h *Handler) AccountMovements(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	values := r.URL.Query()
	q := apiquery.Parse(values)
	f, details := ParseMovementFilter(values)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	f.Limit, f.Offset = q.Limit, q.Offset
	items, total, err := h.svc.ListAccountMovements(r.Context(), orgctx.Must(r.Context()).ID, id, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateMovement serves POST /v1/tenant/cash/accounts/{uuid}/movements.
func (h *Handler) CreateMovement(w http.ResponseWriter, r *http.Request) {
	orgID := orgctx.Must(r.Context()).ID
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.MovementInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	mv, err := h.svc.CreateMovement(r.Context(), orgID, id, &p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordMovement(r, "cash_accounts.movement_created", mv)
	response.JSON(w, r, http.StatusCreated, mv)
}

// ListClosings serves GET /v1/tenant/cash/accounts/{uuid}/closings.
func (h *Handler) ListClosings(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.ListClosings(r.Context(), orgctx.Must(r.Context()).ID, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// PreviewClosing serves GET /v1/tenant/cash/accounts/{uuid}/closings/preview.
func (h *Handler) PreviewClosing(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var date *string
	if v := strings.TrimSpace(r.URL.Query().Get("date")); v != "" {
		date = &v
	}
	out, err := h.svc.PreviewClosing(r.Context(), orgctx.Must(r.Context()).ID, id, date)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CloseDay serves POST /v1/tenant/cash/accounts/{uuid}/closings.
func (h *Handler) CloseDay(w http.ResponseWriter, r *http.Request) {
	orgID := orgctx.Must(r.Context()).ID
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ClosingInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	c, err := h.svc.Close(r.Context(), orgID, id, &p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordClosing(r, "cash_accounts.day_closed", id, c)
	response.JSON(w, r, http.StatusCreated, c)
}

// ReopenDay serves POST /v1/tenant/cash/accounts/{uuid}/closings/{closingUuid}/reopen.
func (h *Handler) ReopenDay(w http.ResponseWriter, r *http.Request) {
	orgID := orgctx.Must(r.Context()).ID
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	closingID, ok := pathUUID(w, r, "closingUuid")
	if !ok {
		return
	}
	var in model.ReopenInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	c, err := h.svc.Reopen(r.Context(), orgID, id, closingID, &p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordClosing(r, "cash_accounts.day_reopened", id, c)
	response.JSON(w, r, http.StatusOK, c)
}

// ----------------------------------------------------------------- movements

// MovementsMeta serves GET /v1/tenant/cash/movements/meta.
func (h *Handler) MovementsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, MovementsMeta())
}

// MovementFacets serves GET /v1/tenant/cash/movements/facets.
func (h *Handler) MovementFacets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.MovementFacets(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ParseMovementFilter reads the movement list filters (list and export).
func ParseMovementFilter(values map[string][]string) (model.MovementFilter, []response.Detail) {
	var f model.MovementFilter
	var details []response.Detail
	for _, raw := range values["account"] {
		for _, part := range apiquery.SplitCSV(raw) {
			id, err := uuid.Parse(part)
			if err != nil {
				details = append(details, response.Detail{Field: "account", Message: "account must list account uuids", Code: "invalid"})
				break
			}
			f.Accounts = append(f.Accounts, id)
		}
	}
	for _, raw := range values["kind"] {
		f.Kinds = append(f.Kinds, apiquery.SplitCSV(raw)...)
	}
	if list := values["direction"]; len(list) > 0 {
		switch v := strings.TrimSpace(list[0]); v {
		case "", model.DirectionIn, model.DirectionOut:
			f.Direction = v
		default:
			details = append(details, response.Detail{Field: "direction", Message: "direction must be in or out", Code: "invalid"})
		}
	}
	for _, p := range []struct {
		key string
		dst **time.Time
	}{{"date_from", &f.DateFrom}, {"date_to", &f.DateTo}} {
		if list := values[p.key]; len(list) > 0 && strings.TrimSpace(list[0]) != "" {
			t, err := time.Parse("2006-01-02", strings.TrimSpace(list[0]))
			if err != nil {
				details = append(details, response.Detail{Field: p.key, Message: p.key + " must be YYYY-MM-DD", Code: "invalid"})
				continue
			}
			*p.dst = &t
		}
	}
	return f, details
}

// ListMovements serves GET /v1/tenant/cash/movements.
func (h *Handler) ListMovements(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden"}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(usecase.MovementSortColumns)); err != nil {
		writeError(w, r, err)
		return
	}
	f, details := ParseMovementFilter(values)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListMovements(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetMovement serves GET /v1/tenant/cash/movements/{uuid}.
func (h *Handler) GetMovement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	mv, err := h.svc.GetMovement(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, mv)
}

// ReverseMovement serves POST /v1/tenant/cash/movements/{uuid}/reverse.
func (h *Handler) ReverseMovement(w http.ResponseWriter, r *http.Request) {
	orgID := orgctx.Must(r.Context()).ID
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ReverseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	items, err := h.svc.Reverse(r.Context(), orgID, id, &p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	for _, mv := range items {
		h.recordMovement(r, "cash_accounts.movement_reversed", mv)
	}
	response.JSON(w, r, http.StatusCreated, map[string]any{"items": items})
}

// Transfer serves POST /v1/tenant/cash/transfers.
func (h *Handler) Transfer(w http.ResponseWriter, r *http.Request) {
	var in model.TransferInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	t, err := h.svc.Transfer(r.Context(), orgctx.Must(r.Context()).ID, &p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordMovement(r, "cash_accounts.transfer_out", t.Out)
	h.recordMovement(r, "cash_accounts.transfer_in", t.In)
	response.JSON(w, r, http.StatusCreated, t)
}

// DailySummary serves GET /v1/tenant/cash/daily-summary.
func (h *Handler) DailySummary(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	f, details := ParseAccountFilter(values)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	out, err := h.svc.DailySummary(r.Context(), orgctx.Must(r.Context()).ID, values.Get("date"), f.Branches, f.HeadOffice)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ------------------------------------------------------------------- helpers

func accountDiff(a model.Account) map[string]any {
	return map[string]any{
		"name": a.Name, "opening_balance": a.OpeningBalance.String(), "opening_date": a.OpeningDate,
		"bank_name": a.BankName, "iban": a.IBAN, "provider": a.Provider, "note": a.Note, "status": a.Status,
		"sort_order": a.SortOrder,
	}
}

// record writes an activity event of an account.
func (h *Handler) record(r *http.Request, action string, acc model.Account, payload map[string]any, changes map[string]activity.Change) {
	if h.activity == nil {
		return
	}
	var branch *uuid.UUID
	if acc.Branch != nil {
		b := acc.Branch.UUID
		branch = &b
	}
	h.recordEntry(r, action, acc.UUID, branch, payload, changes)
}

func (h *Handler) recordEntry(r *http.Request, action string, accountID uuid.UUID, branch *uuid.UUID, payload map[string]any, changes map[string]activity.Change) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	id := accountID
	h.activity.RecordEntry(r.Context(), activity.Entry{
		ActorID: &p.UserInternal, Action: action, Resource: model.AccountsSlug, ResourceUUID: &id,
		BranchUUID: branch, Payload: payload, Changes: changes, Request: r,
	})
}

// recordMovement writes the activity of a movement on its account and
// publishes the account and movement list changes.
func (h *Handler) recordMovement(r *http.Request, action string, mv model.Movement) {
	h.publish(r, model.MovementsSlug, resourceevents.ActionCreated, mv.UUID)
	h.publish(r, model.AccountsSlug, resourceevents.ActionUpdated, mv.Account.UUID)
	payload := map[string]any{
		"movement_uuid": mv.UUID.String(), "kind": mv.Kind, "direction": mv.Direction,
		"amount": mv.Amount.String(), "currency": mv.Currency, "movement_date": mv.MovementDate,
		"description": mv.Description,
	}
	if mv.CounterAccount != nil {
		payload["counter_account"] = mv.CounterAccount.Name
	}
	h.recordEntry(r, action, mv.Account.UUID, nil, payload, nil)
}

func (h *Handler) recordClosing(r *http.Request, action string, accountID uuid.UUID, c model.Closing) {
	h.publish(r, model.AccountsSlug, resourceevents.ActionUpdated, accountID)
	h.publish(r, model.MovementsSlug, resourceevents.ActionUpdated, accountID)
	h.recordEntry(r, action, accountID, nil, map[string]any{
		"closing_uuid": c.UUID.String(), "business_date": c.BusinessDate, "expected_amount": c.Expected.String(),
		"counted_amount": c.Counted.String(), "difference": c.Difference.String(), "reason": c.ReopenReason,
	}, nil)
}

func (h *Handler) publish(r *http.Request, resource, action string, id uuid.UUID) {
	if h.events == nil {
		return
	}
	h.events.Publish(r.Context(), resource, action, id.String())
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: name + " is invalid", Code: "invalid"}})
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
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Account was not found")
	case errors.Is(err, usecase.ErrMovementNotFound):
		response.NotFound(w, r, "Movement was not found")
	case errors.Is(err, usecase.ErrClosingNotFound):
		response.NotFound(w, r, "Closing was not found")
	case errors.Is(err, usecase.ErrNameTaken):
		conflict(w, r, "name", "taken", err)
	case errors.Is(err, usecase.ErrInsufficientFunds):
		conflict(w, r, "amount", "insufficient_funds", err)
	case errors.Is(err, usecase.ErrAccountArchived):
		conflict(w, r, "account", "archived", err)
	case errors.Is(err, usecase.ErrDayClosed):
		conflict(w, r, "movement_date", "day_closed", err)
	case errors.Is(err, usecase.ErrDayAlreadyClosed):
		conflict(w, r, "business_date", "already_closed", err)
	case errors.Is(err, usecase.ErrAlreadyReversed):
		conflict(w, r, "movement", "already_reversed", err)
	case errors.Is(err, usecase.ErrNotReversible):
		conflict(w, r, "movement", "not_reversible", err)
	case errors.Is(err, usecase.ErrNotLatestClosing):
		conflict(w, r, "closing", "not_latest", err)
	case errors.Is(err, usecase.ErrClosingReopened):
		conflict(w, r, "closing", "reopened", err)
	case errors.Is(err, usecase.ErrNotClosable):
		conflict(w, r, "account", "not_closable", err)
	case errors.Is(err, usecase.ErrBalanceNotZero):
		conflict(w, r, "status", "balance_not_zero", err)
	case errors.Is(err, usecase.ErrCurrencyMismatch):
		conflict(w, r, "to_account_uuid", "currency_mismatch", err)
	case errors.Is(err, usecase.ErrIdempotencyReused):
		conflict(w, r, "idempotency_key", "reused", err)
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
