// Package handler is the HTTP layer of the income / expense module. Every
// handler runs behind RequireOrganization and reads the organization id
// from orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/tenant/finance/*.
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

func splitCSV(values map[string][]string, key string) []string {
	var out []string
	for _, raw := range values[key] {
		out = append(out, apiquery.SplitCSV(raw)...)
	}
	return out
}

func parseEnum(values map[string][]string, key string, allowed []string, details *[]response.Detail) []string {
	out := splitCSV(values, key)
	for _, v := range out {
		ok := false
		for _, a := range allowed {
			if v == a {
				ok = true
				break
			}
		}
		if !ok {
			*details = append(*details, response.Detail{Field: key, Message: key + " must be one of " + strings.Join(allowed, ", "), Code: "invalid"})
			return nil
		}
	}
	return out
}

func parseUUIDs(values map[string][]string, key string, details *[]response.Detail) []uuid.UUID {
	var out []uuid.UUID
	for _, part := range splitCSV(values, key) {
		id, err := uuid.Parse(part)
		if err != nil {
			*details = append(*details, response.Detail{Field: key, Message: key + " must list uuids", Code: "invalid"})
			return nil
		}
		out = append(out, id)
	}
	return out
}

func parseDate(values map[string][]string, key string, details *[]response.Detail) *time.Time {
	raw := strings.TrimSpace(url.Values(values).Get(key))
	if raw == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		*details = append(*details, response.Detail{Field: key, Message: key + " must be YYYY-MM-DD", Code: "invalid"})
		return nil
	}
	return &t
}

// ParseLedgerFilter reads the list filters shared by the list and the
// export: source, kind, category (uuids, sale_service, sale_product),
// branch (uuids, head_office), staff, account, date_from, date_to.
func ParseLedgerFilter(values map[string][]string) (model.LedgerFilter, []response.Detail) {
	var details []response.Detail
	f := model.LedgerFilter{
		Sources:  parseEnum(values, "source", model.Sources, &details),
		Kinds:    parseEnum(values, "kind", model.Kinds, &details),
		Staff:    parseUUIDs(values, "staff", &details),
		Accounts: parseUUIDs(values, "account", &details),
		DateFrom: parseDate(values, "date_from", &details),
		DateTo:   parseDate(values, "date_to", &details),
	}
	for _, part := range splitCSV(values, "branch") {
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
	for _, part := range splitCSV(values, "category") {
		if part != model.SaleServiceCategory && part != model.SaleProductCategory {
			if _, err := uuid.Parse(part); err != nil {
				details = append(details, response.Detail{Field: "category", Message: "category must list category uuids", Code: "invalid"})
				break
			}
		}
		f.Categories = append(f.Categories, part)
	}
	return f, details
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

// ------------------------------------------------------------- categories

// ListCategories serves GET /v1/tenant/finance/categories.
func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.CategorySortColumns)
	if !ok {
		return
	}
	values := r.URL.Query()
	var details []response.Detail
	f := model.CategoryFilter{
		Kinds:    parseEnum(values, "kind", model.Kinds, &details),
		Statuses: parseEnum(values, "status", model.Statuses, &details),
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListCategories(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetCategory serves GET /v1/tenant/finance/categories/{categoryUuid}.
func (h *Handler) GetCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "categoryUuid")
	if !ok {
		return
	}
	c, err := h.svc.GetCategory(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

// CreateCategory serves POST /v1/tenant/finance/categories.
func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var in model.CategoryInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.CreateCategory(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordCategory(r, "finance_categories.created", c.UUID, map[string]any{"name": c.Name, "kind": c.Kind}, nil)
	h.publish(r, model.CategoriesSlug, resourceevents.ActionCreated, c.UUID)
	response.JSON(w, r, http.StatusCreated, c)
}

// PatchCategory serves PATCH /v1/tenant/finance/categories/{categoryUuid}.
func (h *Handler) PatchCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "categoryUuid")
	if !ok {
		return
	}
	var in model.CategoryInput
	if !decodeJSON(w, r, &in) {
		return
	}
	before, after, err := h.svc.PatchCategory(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	view := func(c model.Category) map[string]any {
		return map[string]any{"name": c.Name, "status": c.Status, "sort_order": c.SortOrder}
	}
	h.recordCategory(r, "finance_categories.updated", id, map[string]any{"name": after.Name}, activity.Diff(view(before), view(after)))
	h.publish(r, model.CategoriesSlug, resourceevents.ActionUpdated, id)
	response.JSON(w, r, http.StatusOK, after)
}

// DeleteCategory serves DELETE /v1/tenant/finance/categories/{categoryUuid}.
func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "categoryUuid")
	if !ok {
		return
	}
	c, err := h.svc.DeleteCategory(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordCategory(r, "finance_categories.deleted", id, map[string]any{"name": c.Name, "kind": c.Kind}, nil)
	h.publish(r, model.CategoriesSlug, resourceevents.ActionDeleted, id)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- entries

// EntriesMeta serves GET /v1/tenant/finance/entries/meta.
func (h *Handler) EntriesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, EntriesMeta())
}

// Facets serves GET /v1/tenant/finance/entries/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Facets(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListEntries serves GET /v1/tenant/finance/entries: the records of the
// business and the sale lines (automatic income).
func (h *Handler) ListEntries(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.LedgerSortColumns)
	if !ok {
		return
	}
	f, details := ParseLedgerFilter(r.URL.Query())
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListLedger(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateEntry serves POST /v1/tenant/finance/entries.
func (h *Handler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	var in model.EntryInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	e, err := h.svc.CreateEntry(r.Context(), orgctx.Must(r.Context()).ID, &p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordEntry(r, "finance_entries.created", e, entryPayload(e), nil)
	h.publish(r, model.EntriesSlug, resourceevents.ActionCreated, e.UUID)
	response.JSON(w, r, http.StatusCreated, e)
}

// GetEntry serves GET /v1/tenant/finance/entries/{entryUuid}.
func (h *Handler) GetEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "entryUuid")
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

// PatchEntry serves PATCH /v1/tenant/finance/entries/{entryUuid}.
func (h *Handler) PatchEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "entryUuid")
	if !ok {
		return
	}
	var in model.EntryPatch
	if !decodeJSON(w, r, &in) {
		return
	}
	before, after, err := h.svc.PatchEntry(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordEntry(r, "finance_entries.updated", after, nil, activity.Diff(classification(before), classification(after)))
	h.publish(r, model.EntriesSlug, resourceevents.ActionUpdated, id)
	response.JSON(w, r, http.StatusOK, after)
}

// ReverseEntry serves POST /v1/tenant/finance/entries/{entryUuid}/reverse.
func (h *Handler) ReverseEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "entryUuid")
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
	payload := entryPayload(rev)
	payload["reversal_uuid"] = rev.UUID.String()
	h.recordEntryID(r, "finance_entries.reversed", id, rev.Branch, payload, nil)
	h.recordEntry(r, "finance_entries.created", rev, entryPayload(rev), nil)
	h.publish(r, model.EntriesSlug, resourceevents.ActionUpdated, id)
	h.publish(r, model.EntriesSlug, resourceevents.ActionCreated, rev.UUID)
	response.JSON(w, r, http.StatusCreated, rev)
}

// CorrectEntry serves POST /v1/tenant/finance/entries/{entryUuid}/correct.
func (h *Handler) CorrectEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "entryUuid")
	if !ok {
		return
	}
	var in model.CorrectInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	scope := orgctx.Must(r.Context())
	before, err := h.svc.GetEntry(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	e, err := h.svc.CorrectEntry(r.Context(), scope.ID, id, &p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	view := func(x model.Entry) map[string]any {
		v := classification(x)
		v["amount"], v["entry_date"], v["account"] = x.Amount.String(), x.EntryDate, x.Account.Name
		v["branch"] = ""
		if x.Branch != nil {
			v["branch"] = x.Branch.Name
		}
		return v
	}
	payload := map[string]any{"replacement_uuid": e.UUID.String(), "reason": strings.TrimSpace(in.Reason)}
	h.recordEntryID(r, "finance_entries.corrected", id, before.Branch, payload, activity.Diff(view(before), view(e)))
	created := entryPayload(e)
	created["replaces_uuid"] = id.String()
	h.recordEntry(r, "finance_entries.created", e, created, nil)
	h.publish(r, model.EntriesSlug, resourceevents.ActionUpdated, id)
	h.publish(r, model.EntriesSlug, resourceevents.ActionCreated, e.UUID)
	response.JSON(w, r, http.StatusCreated, e)
}

// EntryActivity serves GET /v1/tenant/finance/entries/{entryUuid}/activity.
func (h *Handler) EntryActivity(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "entryUuid")
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

// UploadDocument serves PUT /v1/tenant/finance/entries/{entryUuid}/document
// (multipart "file": PDF, JPEG, PNG or WebP up to 10 MiB).
func (h *Handler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "entryUuid")
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxDocumentBytes+(1<<20))
	if err := r.ParseMultipartForm(usecase.MaxDocumentBytes); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "invalid multipart form or file too large", Code: "invalid"}})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "file is required", Code: "required"}})
		return
	}
	defer func() { _ = file.Close() }()
	before, after, err := h.svc.UploadDocument(r.Context(), orgctx.Must(r.Context()).ID, id, usecase.FileUpload{
		Body: file, Size: header.Size, Filename: header.Filename,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	payload := map[string]any{"name": after.Document.Name, "size_bytes": after.Document.SizeBytes}
	if before.Document != nil {
		payload["replaced"] = before.Document.Name
	}
	h.recordEntry(r, "finance_entries.document_attached", after, payload, nil)
	h.publish(r, model.EntriesSlug, resourceevents.ActionUpdated, id)
	response.JSON(w, r, http.StatusOK, after)
}

// DownloadDocument serves GET /v1/tenant/finance/entries/{entryUuid}/document.
func (h *Handler) DownloadDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "entryUuid")
	if !ok {
		return
	}
	f, err := h.svc.OpenDocument(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	w.Header().Set("Content-Type", f.ContentType)
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	w.Header().Set("Content-Disposition", contentDisposition(f.Name))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, f.Body)
}

// RemoveDocument serves DELETE /v1/tenant/finance/entries/{entryUuid}/document.
func (h *Handler) RemoveDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "entryUuid")
	if !ok {
		return
	}
	before, after, err := h.svc.RemoveDocument(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordEntry(r, "finance_entries.document_removed", after, map[string]any{"name": before.Document.Name}, nil)
	h.publish(r, model.EntriesSlug, resourceevents.ActionUpdated, id)
	response.JSON(w, r, http.StatusOK, after)
}

// Summary serves GET /v1/tenant/finance/summary.
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	var details []response.Detail
	from := parseDate(values, "date_from", &details)
	to := parseDate(values, "date_to", &details)
	branches := parseUUIDs(values, "branch", &details)
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

// ---------------------------------------------------------------- helpers

func classification(e model.Entry) map[string]any {
	v := map[string]any{"category": e.Category.Name, "description": e.Description, "staff": ""}
	if e.Staff != nil {
		v["staff"] = e.Staff.Name
	}
	return v
}

func entryPayload(e model.Entry) map[string]any {
	p := map[string]any{
		"entry_type": e.EntryType, "kind": e.Kind, "category": e.Category.Name, "amount": e.Amount.String(),
		"currency": e.Currency, "entry_date": e.EntryDate, "account": e.Account.Name, "description": e.Description,
	}
	if e.Reason != "" {
		p["reason"] = e.Reason
	}
	if e.ReversesUUID != nil {
		p["reverses_uuid"] = e.ReversesUUID.String()
	}
	return p
}

func (h *Handler) recordEntry(r *http.Request, action string, e model.Entry, payload map[string]any, changes map[string]activity.Change) {
	h.recordEntryID(r, action, e.UUID, e.Branch, payload, changes)
}

func (h *Handler) recordEntryID(r *http.Request, action string, id uuid.UUID, branch *model.Ref, payload map[string]any, changes map[string]activity.Change) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	entry := activity.Entry{
		ActorID: &p.UserInternal, Action: action, Resource: model.EntriesSlug, ResourceUUID: &id,
		Payload: payload, Changes: changes, Request: r,
	}
	if branch != nil {
		b := branch.UUID
		entry.BranchUUID = &b
	}
	h.activity.RecordEntry(r.Context(), entry)
}

func (h *Handler) recordCategory(r *http.Request, action string, id uuid.UUID, payload map[string]any, changes map[string]activity.Change) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.RecordEntry(r.Context(), activity.Entry{
		ActorID: &p.UserInternal, Action: action, Resource: model.CategoriesSlug, ResourceUUID: &id,
		Payload: payload, Changes: changes, Request: r,
	})
}

func (h *Handler) publish(r *http.Request, resource, action string, id uuid.UUID) {
	if h.events == nil {
		return
	}
	h.events.Publish(r.Context(), resource, action, id.String())
}

func contentDisposition(filename string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, filename)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safe, url.PathEscape(filename))
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
	case errors.Is(err, usecase.ErrCategoryNotFound):
		response.NotFound(w, r, "Category was not found")
	case errors.Is(err, usecase.ErrEntryNotFound):
		response.NotFound(w, r, "Record was not found")
	case errors.Is(err, usecase.ErrNoDocument):
		response.NotFound(w, r, "Document was not found")
	case errors.Is(err, orgctx.ErrAllBranchesRequired):
		response.Forbidden(w, r, "This action needs access to every branch")
	case errors.Is(err, usecase.ErrCategoryNameTaken):
		conflict(w, r, "name", "taken", err)
	case errors.Is(err, usecase.ErrCategoryInUse):
		conflict(w, r, "category", "in_use", err)
	case errors.Is(err, usecase.ErrAlreadyReversed):
		conflict(w, r, "entry", "already_reversed", err)
	case errors.Is(err, usecase.ErrNotReversible):
		conflict(w, r, "entry", "not_reversible", err)
	case errors.Is(err, usecase.ErrIdempotencyReused):
		conflict(w, r, "idempotency_key", "reused", err)
	case errors.Is(err, usecase.ErrDayClosed):
		conflict(w, r, "entry_date", "day_closed", err)
	case errors.Is(err, usecase.ErrInsufficientFunds):
		conflict(w, r, "amount", "insufficient_funds", err)
	case errors.Is(err, usecase.ErrAccountArchived):
		conflict(w, r, "cash_account_uuid", "archived", err)
	case errors.Is(err, usecase.ErrStorageUnavailable):
		response.Error(w, r, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "Storage is not configured")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
