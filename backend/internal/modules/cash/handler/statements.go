package handler

import (
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------- statements

// StatementsMeta is the table contract of GET /v1/tenant/cash/statements.
func StatementsMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.StatementsSlug,
		DefaultSort:   "-created_at",
		DefaultFields: []string{"name", "account", "source", "period", "line_count", "open_count", "status"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"name", "account.name"},
		SortableFields:   []string{"name", "period_to", "created_at"},
		FilterableFields: []string{"account", "source", "status"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "cash.statements.columns.name", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "account", LabelKey: "cash.statements.columns.account", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "source", LabelKey: "cash.statements.columns.source", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "period", LabelKey: "cash.statements.columns.period", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "closing_balance", LabelKey: "cash.statements.columns.closing_balance", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "line_count", LabelKey: "cash.statements.columns.line_count", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "open_count", LabelKey: "cash.statements.columns.open_count", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "status", LabelKey: "cash.statements.columns.status", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "created_at", LabelKey: "cash.statements.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "cash.statements.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "account", LabelKey: "cash.statements.columns.account", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "cash.movements.facets.account"},
			{Key: "source", LabelKey: "cash.statements.columns.source", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "cash_statement_source"},
			{Key: "status", LabelKey: "cash.statements.columns.status", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "cash_statement_status"},
		},
		Includes: []string{},
	}
}

// StatementLinesMeta is the table contract of the line list of a statement.
// Lines are imported (io engine) or added by hand and reconciled; they are
// never edited in place.
func StatementLinesMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.StatementLinesSlug,
		DefaultSort:   "line_no",
		DefaultFields: []string{"line_no", "line_date", "description", "direction", "amount", "fee", "line_type", "status", "matched_movement"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Search: true, Filter: true, Sort: true, Export: true, Import: true,
		},
		SearchableFields: []string{"description", "reference"},
		SortableFields:   []string{"line_no", "line_date", "amount", "status"},
		FilterableFields: []string{"statement", "status", "line_type", "direction"},
		Columns: []resourcemeta.Column{
			{Key: "line_no", LabelKey: "cash.lines.columns.line_no", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "line_date", LabelKey: "cash.lines.columns.line_date", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "description", LabelKey: "cash.lines.columns.description", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "reference", LabelKey: "cash.lines.columns.reference", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "direction", LabelKey: "cash.lines.columns.direction", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "amount", LabelKey: "cash.lines.columns.amount", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "fee", LabelKey: "cash.lines.columns.fee", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "line_type", LabelKey: "cash.lines.columns.line_type", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "status", LabelKey: "cash.lines.columns.status", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "matched_movement", LabelKey: "cash.lines.columns.matched_movement", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "balance_after", LabelKey: "cash.lines.columns.balance_after", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "note", LabelKey: "cash.lines.columns.note", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "uuid", LabelKey: "cash.lines.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "status", LabelKey: "cash.lines.columns.status", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "cash_statement_line_status"},
			{Key: "line_type", LabelKey: "cash.lines.columns.line_type", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "cash_statement_line_type"},
			{Key: "direction", LabelKey: "cash.lines.columns.direction", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "cash_movement_direction"},
		},
		Includes: []string{},
	}
}

// StatementsMeta serves GET /v1/tenant/cash/statements/meta.
func (h *Handler) StatementsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, StatementsMeta())
}

// StatementLinesMeta serves GET /v1/tenant/cash/statement-lines/meta.
func (h *Handler) StatementLinesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, StatementLinesMeta())
}

// ParseStatementFilter reads the statement list filters.
func ParseStatementFilter(values map[string][]string) (model.StatementFilter, []response.Detail) {
	var f model.StatementFilter
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
	for _, raw := range values["source"] {
		f.Sources = append(f.Sources, apiquery.SplitCSV(raw)...)
	}
	for _, raw := range values["status"] {
		f.Statuses = append(f.Statuses, apiquery.SplitCSV(raw)...)
	}
	return f, details
}

// ListStatements serves GET /v1/tenant/cash/statements.
func (h *Handler) ListStatements(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden"}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(usecase.StatementSortColumns)); err != nil {
		writeError(w, r, err)
		return
	}
	f, details := ParseStatementFilter(values)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListStatements(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateStatement serves POST /v1/tenant/cash/statements.
func (h *Handler) CreateStatement(w http.ResponseWriter, r *http.Request) {
	var in model.StatementInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	st, err := h.svc.CreateStatement(r.Context(), orgctx.Must(r.Context()).ID, &p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordStatement(r, "cash_accounts.statement_created", st, nil)
	h.publish(r, model.StatementsSlug, resourceevents.ActionCreated, st.UUID)
	response.JSON(w, r, http.StatusCreated, st)
}

// GetStatement serves GET /v1/tenant/cash/statements/{uuid}.
func (h *Handler) GetStatement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	st, err := h.svc.GetStatement(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// PatchStatement serves PATCH /v1/tenant/cash/statements/{uuid}.
func (h *Handler) PatchStatement(w http.ResponseWriter, r *http.Request) {
	orgID := orgctx.Must(r.Context()).ID
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.StatementInput
	if !decodeJSON(w, r, &in) {
		return
	}
	before, beforeErr := h.svc.GetStatement(r.Context(), orgID, id)
	st, err := h.svc.PatchStatement(r.Context(), orgID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var changes map[string]activity.Change
	if beforeErr == nil {
		changes = activity.Diff(statementDiff(before), statementDiff(st))
	}
	h.recordStatement(r, "cash_accounts.statement_updated", st, changes)
	h.publish(r, model.StatementsSlug, resourceevents.ActionUpdated, st.UUID)
	response.JSON(w, r, http.StatusOK, st)
}

// DeleteStatement serves DELETE /v1/tenant/cash/statements/{uuid}.
func (h *Handler) DeleteStatement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	st, err := h.svc.DeleteStatement(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordStatement(r, "cash_accounts.statement_deleted", st, nil)
	h.publish(r, model.StatementsSlug, resourceevents.ActionDeleted, st.UUID)
	w.WriteHeader(http.StatusNoContent)
}

// StatementSummary serves GET /v1/tenant/cash/statements/{uuid}/summary.
func (h *Handler) StatementSummary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.StatementSummary(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// StatementLineFacets serves GET /v1/tenant/cash/statements/{uuid}/lines/facets.
func (h *Handler) StatementLineFacets(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.LineFacets(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// AutoMatch serves POST /v1/tenant/cash/statements/{uuid}/auto-match.
func (h *Handler) AutoMatch(w http.ResponseWriter, r *http.Request) {
	orgID := orgctx.Must(r.Context()).ID
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, err := h.svc.AutoMatch(r.Context(), orgID, &p.UserInternal, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if st, err := h.svc.GetStatement(r.Context(), orgID, id); err == nil {
		h.recordStatement(r, "cash_accounts.statement_auto_matched", st, nil)
		h.publish(r, model.StatementsSlug, resourceevents.ActionUpdated, st.UUID)
		h.publish(r, model.StatementLinesSlug, resourceevents.ActionUpdated, st.UUID)
	}
	response.JSON(w, r, http.StatusOK, out)
}

// --------------------------------------------------------------------- lines

// ParseLineFilter reads the line list filters (list and export).
func ParseLineFilter(values map[string][]string) (model.LineFilter, []response.Detail) {
	var f model.LineFilter
	var details []response.Detail
	for _, raw := range values["statement"] {
		for _, part := range apiquery.SplitCSV(raw) {
			id, err := uuid.Parse(part)
			if err != nil {
				details = append(details, response.Detail{Field: "statement", Message: "statement must list statement uuids", Code: "invalid"})
				break
			}
			f.Statements = append(f.Statements, id)
		}
	}
	for _, raw := range values["status"] {
		f.Statuses = append(f.Statuses, apiquery.SplitCSV(raw)...)
	}
	for _, raw := range values["line_type"] {
		f.LineTypes = append(f.LineTypes, apiquery.SplitCSV(raw)...)
	}
	if list := values["direction"]; len(list) > 0 {
		switch v := strings.TrimSpace(list[0]); v {
		case "", model.DirectionIn, model.DirectionOut:
			f.Direction = v
		default:
			details = append(details, response.Detail{Field: "direction", Message: "direction must be in or out", Code: "invalid"})
		}
	}
	return f, details
}

func (h *Handler) listLines(w http.ResponseWriter, r *http.Request, statement *uuid.UUID) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden"}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(usecase.LineSortColumns)); err != nil {
		writeError(w, r, err)
		return
	}
	f, details := ParseLineFilter(values)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	orgID := orgctx.Must(r.Context()).ID
	if statement != nil {
		// The statement must exist in the caller's scope (404), then its
		// lines only.
		if _, err := h.svc.GetStatement(r.Context(), orgID, *statement); err != nil {
			writeError(w, r, err)
			return
		}
		f.Statements = []uuid.UUID{*statement}
	}
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListLines(r.Context(), orgID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// ListLines serves GET /v1/tenant/cash/statement-lines (every statement of
// the scope).
func (h *Handler) ListLines(w http.ResponseWriter, r *http.Request) {
	h.listLines(w, r, nil)
}

// StatementLines serves GET /v1/tenant/cash/statements/{uuid}/lines.
func (h *Handler) StatementLines(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	h.listLines(w, r, &id)
}

// AddLine serves POST /v1/tenant/cash/statements/{uuid}/lines.
func (h *Handler) AddLine(w http.ResponseWriter, r *http.Request) {
	orgID := orgctx.Must(r.Context()).ID
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.LineInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	line, err := h.svc.AddLine(r.Context(), orgID, &p.UserInternal, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordLine(r, "cash_accounts.statement_line_added", line)
	response.JSON(w, r, http.StatusCreated, line)
}

// GetLine serves GET /v1/tenant/cash/statement-lines/{lineUuid}.
func (h *Handler) GetLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "lineUuid")
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

// LineCandidates serves GET /v1/tenant/cash/statement-lines/{lineUuid}/candidates.
func (h *Handler) LineCandidates(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "lineUuid")
	if !ok {
		return
	}
	items, err := h.svc.Candidates(r.Context(), orgctx.Must(r.Context()).ID, id, r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// MatchLine serves POST /v1/tenant/cash/statement-lines/{lineUuid}/match.
func (h *Handler) MatchLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "lineUuid")
	if !ok {
		return
	}
	var in model.MatchInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	line, err := h.svc.MatchLine(r.Context(), orgctx.Must(r.Context()).ID, &p.UserInternal, id, in)
	h.lineResult(w, r, "cash_accounts.statement_line_matched", line, err)
}

// ResetLine serves POST /v1/tenant/cash/statement-lines/{lineUuid}/reset.
func (h *Handler) ResetLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "lineUuid")
	if !ok {
		return
	}
	var in model.ReverseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	line, err := h.svc.ResetLine(r.Context(), orgctx.Must(r.Context()).ID, &p.UserInternal, id, in)
	h.lineResult(w, r, "cash_accounts.statement_line_reset", line, err)
}

// PostLine serves POST /v1/tenant/cash/statement-lines/{lineUuid}/post.
func (h *Handler) PostLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "lineUuid")
	if !ok {
		return
	}
	var in model.PostLineInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	line, err := h.svc.PostLine(r.Context(), orgctx.Must(r.Context()).ID, &p.UserInternal, id, in)
	if err == nil {
		h.publish(r, model.MovementsSlug, resourceevents.ActionCreated, line.UUID)
		h.publish(r, model.AccountsSlug, resourceevents.ActionUpdated, line.Account.UUID)
	}
	h.lineResult(w, r, "cash_accounts.statement_line_posted", line, err)
}

// IgnoreLine serves POST /v1/tenant/cash/statement-lines/{lineUuid}/ignore.
func (h *Handler) IgnoreLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "lineUuid")
	if !ok {
		return
	}
	var in model.ReverseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	line, err := h.svc.IgnoreLine(r.Context(), orgctx.Must(r.Context()).ID, &p.UserInternal, id, in)
	h.lineResult(w, r, "cash_accounts.statement_line_ignored", line, err)
}

// ReverseLine serves POST /v1/tenant/cash/statement-lines/{lineUuid}/reverse.
func (h *Handler) ReverseLine(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "lineUuid")
	if !ok {
		return
	}
	var in model.ReverseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	line, err := h.svc.ReverseLinePosting(r.Context(), orgctx.Must(r.Context()).ID, &p.UserInternal, id, in)
	if err == nil {
		h.publish(r, model.MovementsSlug, resourceevents.ActionCreated, line.UUID)
		h.publish(r, model.AccountsSlug, resourceevents.ActionUpdated, line.Account.UUID)
	}
	h.lineResult(w, r, "cash_accounts.statement_line_reversed", line, err)
}

// ------------------------------------------------------------------- helpers

func (h *Handler) lineResult(w http.ResponseWriter, r *http.Request, action string, line model.StatementLine, err error) {
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordLine(r, action, line)
	response.JSON(w, r, http.StatusOK, line)
}

func statementDiff(st model.Statement) map[string]any {
	out := map[string]any{"name": st.Name, "note": st.Note, "period_from": "", "period_to": "", "closing_balance": ""}
	if st.PeriodFrom != nil {
		out["period_from"] = *st.PeriodFrom
	}
	if st.PeriodTo != nil {
		out["period_to"] = *st.PeriodTo
	}
	if st.ClosingBalance != nil {
		out["closing_balance"] = st.ClosingBalance.String()
	}
	return out
}

// recordStatement writes the activity of a statement on its account.
func (h *Handler) recordStatement(r *http.Request, action string, st model.Statement, changes map[string]activity.Change) {
	var branch *uuid.UUID
	if st.Branch != nil {
		b := st.Branch.UUID
		branch = &b
	}
	h.recordEntry(r, action, st.Account.UUID, branch, map[string]any{
		"statement_uuid": st.UUID.String(), "name": st.Name, "source": st.Source, "line_count": st.LineCount,
	}, changes)
}

// recordLine writes the activity of a line decision on its account and
// publishes the statement and line list changes.
func (h *Handler) recordLine(r *http.Request, action string, line model.StatementLine) {
	h.publish(r, model.StatementLinesSlug, resourceevents.ActionUpdated, line.UUID)
	h.publish(r, model.StatementsSlug, resourceevents.ActionUpdated, line.Statement.UUID)
	payload := map[string]any{
		"statement_uuid": line.Statement.UUID.String(), "line_uuid": line.UUID.String(), "line_no": line.LineNo,
		"line_date": line.LineDate, "direction": line.Direction, "amount": line.Amount.String(), "fee": line.Fee.String(),
		"status": line.Status, "description": line.Description, "reason": line.Note,
	}
	if line.MatchedMovement != nil {
		payload["movement_uuid"] = line.MatchedMovement.UUID.String()
	}
	if line.PostedMovement != nil {
		payload["movement_uuid"] = line.PostedMovement.UUID.String()
	}
	h.recordEntry(r, action, line.Account.UUID, nil, payload, nil)
}
