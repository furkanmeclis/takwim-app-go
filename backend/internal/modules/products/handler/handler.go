// Package handler is the HTTP layer of the products tenant resource. Every
// handler runs behind RequireOrganization and reads the organization id
// from orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/tenant/products and /v1/tenant/stock-movements.
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

// Meta serves GET /v1/tenant/products/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// MovementMeta serves GET /v1/tenant/stock-movements/meta.
func (h *Handler) MovementMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, MovementMeta())
}

// Facets serves GET /v1/tenant/products/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	out, err := h.svc.Facets(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ParseListFilter reads the list filters shared by the list, export and
// bulk "all matching" scope.
func ParseListFilter(values map[string][]string) (model.ListFilter, []response.Detail) {
	var f model.ListFilter
	var details []response.Detail
	ids, bad := uuidList(values["branch"])
	if bad {
		details = append(details, response.Detail{Field: "branch", Message: "branch must list uuids", Code: "invalid"})
	}
	f.Branches = ids
	for _, raw := range values["stock_status"] {
		f.StockStatus = append(f.StockStatus, apiquery.SplitCSV(raw)...)
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

func uuidList(raws []string) ([]uuid.UUID, bool) {
	var out []uuid.UUID
	for _, raw := range raws {
		for _, part := range apiquery.SplitCSV(raw) {
			id, err := uuid.Parse(part)
			if err != nil {
				return nil, true
			}
			out = append(out, id)
		}
	}
	return out, false
}

// List serves GET /v1/tenant/products.
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
	f, details := ParseListFilter(values)
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

// Lookup serves GET /v1/tenant/products/lookup?code= (barcode / SKU entry).
func (h *Handler) Lookup(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	items, err := h.svc.Lookup(r.Context(), scope.ID, r.URL.Query().Get("code"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Create serves POST /v1/tenant/products.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	p, err := h.svc.Create(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "products.created", p, nil, nil)
	response.JSON(w, r, http.StatusCreated, p)
}

// Get serves GET /v1/tenant/products/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p, err := h.svc.Get(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

// Patch serves PATCH /v1/tenant/products/{uuid}. branch_uuids is a create
// only field; offers change through PUT …/offers.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.BranchUUIDs != nil {
		response.ValidationError(w, r, []response.Detail{{
			Field: "branch_uuids", Message: "use PUT /offers to change branches", Code: "forbidden",
		}})
		return
	}
	before, beforeErr := h.svc.Get(r.Context(), scope.ID, id)
	p, err := h.svc.Patch(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var changes map[string]activity.Change
	if beforeErr == nil {
		changes = activity.Diff(diffView(before), diffView(p))
	}
	h.record(r, "products.updated", p, nil, changes)
	response.JSON(w, r, http.StatusOK, p)
}

// Delete serves DELETE /v1/tenant/products/{uuid} (soft delete; only
// without stock).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p, err := h.svc.Delete(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "products.deleted", p, nil, nil)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// Activity serves GET /v1/tenant/products/{uuid}/activity.
func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
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

// PutOffers serves PUT /v1/tenant/products/{uuid}/offers. The body lists
// the offers of the branches the caller manages; offers in other branches
// are left as they are.
func (h *Handler) PutOffers(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.OffersInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p, before, err := h.svc.SetOffers(r.Context(), scope.ID, id, in.Offers)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "products.offers_updated", p, nil,
		activity.Diff(map[string]any{"offers": offerView(before)}, map[string]any{"offers": offerView(p.Offers)}))
	response.JSON(w, r, http.StatusOK, p)
}

// CreateVariant serves POST /v1/tenant/products/{uuid}/variants.
func (h *Handler) CreateVariant(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.VariantInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p, v, err := h.svc.CreateVariant(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "products.variant_created", p, variantPayload(p, v), nil)
	response.JSON(w, r, http.StatusCreated, p)
}

// PatchVariant serves PATCH /v1/tenant/products/{uuid}/variants/{variantUuid}.
func (h *Handler) PatchVariant(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	variantID, ok := pathUUID(w, r, "variantUuid")
	if !ok {
		return
	}
	var in model.VariantInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p, after, before, err := h.svc.PatchVariant(r.Context(), scope.ID, id, variantID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "products.variant_updated", p, variantPayload(p, after),
		activity.Diff(variantView(before), variantView(after)))
	response.JSON(w, r, http.StatusOK, p)
}

// DeleteVariant serves DELETE /v1/tenant/products/{uuid}/variants/{variantUuid}.
func (h *Handler) DeleteVariant(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	variantID, ok := pathUUID(w, r, "variantUuid")
	if !ok {
		return
	}
	p, v, err := h.svc.DeleteVariant(r.Context(), scope.ID, id, variantID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "products.variant_deleted", p, variantPayload(p, v), nil)
	response.JSON(w, r, http.StatusOK, p)
}

// UploadImage serves PUT /v1/tenant/products/{uuid}/image (multipart
// "file": JPEG, PNG or WebP up to 5 MiB; replaces the cover).
func (h *Handler) UploadImage(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxProductImageBytes+(1<<20))
	if err := r.ParseMultipartForm(usecase.MaxProductImageBytes); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "invalid multipart form or file too large", Code: "invalid"}})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "file is required", Code: "required"}})
		return
	}
	defer func() { _ = file.Close() }()
	p, err := h.svc.UploadImage(r.Context(), scope.ID, id, usecase.ImageUpload{Body: file, Size: header.Size})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "products.image_updated", p, nil, nil)
	response.JSON(w, r, http.StatusOK, p)
}

// DeleteImage serves DELETE /v1/tenant/products/{uuid}/image.
func (h *Handler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p, err := h.svc.DeleteImage(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "products.image_removed", p, nil, nil)
	response.JSON(w, r, http.StatusOK, p)
}

// PublicImage serves GET /v1/public/media/products/{orgUuid}/images/{imageUuid}
// (no authentication: booking pages and the mobile app show product covers).
func (h *Handler) PublicImage(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(r.PathValue("orgUuid"))
	if err != nil {
		response.NotFound(w, r, "Image was not found")
		return
	}
	imageID, err := uuid.Parse(r.PathValue("imageUuid"))
	if err != nil {
		response.NotFound(w, r, "Image was not found")
		return
	}
	rc, contentType, size, err := h.svc.PublicImage(r.Context(), orgID, imageID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Image objects are immutable: a new upload gets a new uuid.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

// MovementFacets serves GET /v1/tenant/stock-movements/facets.
func (h *Handler) MovementFacets(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	out, err := h.svc.MovementFacets(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListMovements serves GET /v1/tenant/stock-movements.
func (h *Handler) ListMovements(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(usecase.MovementSortColumns)); err != nil {
		writeError(w, r, err)
		return
	}
	f := model.MovementFilter{Limit: q.Limit, Offset: q.Offset, Q: q.Q}
	var details []response.Detail
	for _, p := range []struct {
		key string
		dst *[]uuid.UUID
	}{{"branch", &f.Branches}, {"product", &f.Products}, {"staff", &f.Staff}} {
		ids, bad := uuidList(values[p.key])
		if bad {
			details = append(details, response.Detail{Field: p.key, Message: p.key + " must list uuids", Code: "invalid"})
		}
		*p.dst = ids
	}
	for _, raw := range values["kind"] {
		f.Kinds = append(f.Kinds, apiquery.SplitCSV(raw)...)
	}
	for _, t := range []struct {
		key string
		dst **time.Time
	}{{"from", &f.From}, {"to", &f.To}} {
		raw := strings.TrimSpace(values.Get(t.key))
		if raw == "" {
			continue
		}
		v, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			details = append(details, response.Detail{Field: t.key, Message: t.key + " must be an RFC 3339 time", Code: "invalid"})
			continue
		}
		*t.dst = &v
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListMovements(r.Context(), scope.ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateMovement serves POST /v1/tenant/stock-movements. A sale needs
// tenant.products.sell; receipts, returns, counts and transfers need
// tenant.products.stock.
func (h *Handler) CreateMovement(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.MovementInput
	if !decodeJSON(w, r, &in) {
		return
	}
	need := rbac.PermTenantProductsStock
	if in.Kind == model.KindSale {
		need = rbac.PermTenantProductsSell
	}
	if !scope.Has(need) {
		response.Forbidden(w, r, "Missing permission "+need)
		return
	}
	p := authctx.MustPrincipal(r.Context())
	res, err := h.svc.RecordMovement(r.Context(), scope.ID, p.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if len(res.Movements) > 0 {
		m := res.Movements[0]
		payload := map[string]any{
			"name": res.Product.Name, "kind": in.Kind, "branch": m.Branch.Name,
			"movement_uuid": m.UUID.String(), "quantity": m.Quantity, "balance_after": m.BalanceAfter,
		}
		if m.Variant != nil {
			payload["variant"] = m.Variant.Name
		}
		if m.CounterpartBranch != nil {
			payload["to_branch"] = m.CounterpartBranch.Name
		}
		if m.Reason != "" {
			payload["reason"] = m.Reason
		}
		h.record(r, "products.stock_moved", res.Product, payload, nil)
		if in.Kind == model.KindSale {
			h.salesChanged(r)
		}
	}
	response.JSON(w, r, http.StatusCreated, map[string]any{"items": res.Movements, "product": res.Product})
}

// ReturnMovement serves POST /v1/tenant/stock-movements/{uuid}/return:
// the product of a stock sale is taken back (TEC-913) and its revenue line
// reversed.
func (h *Handler) ReturnMovement(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Stock movement was not found")
		return
	}
	var in model.ReturnInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	res, err := h.svc.ReturnSale(r.Context(), scope.ID, p.UserInternal, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	m := res.Movements[0]
	payload := map[string]any{
		"name": res.Product.Name, "kind": model.KindReturn, "branch": m.Branch.Name, "movement_uuid": m.UUID.String(),
		"sale_uuid": id.String(), "quantity": m.Quantity, "balance_after": m.BalanceAfter, "reason": m.Reason,
	}
	if m.Variant != nil {
		payload["variant"] = m.Variant.Name
	}
	h.record(r, "products.sale_returned", res.Product, payload, nil)
	h.salesChanged(r)
	response.JSON(w, r, http.StatusCreated, map[string]any{"items": res.Movements, "product": res.Product})
}

// diffView is the part of a product a catalog edit changes.
func diffView(p model.Product) map[string]any {
	return map[string]any{
		"name": p.Name, "description": p.Description, "sku": p.SKU, "barcode": p.Barcode, "unit": p.Unit,
		"price": p.Price.String(), "cost": p.Cost.String(), "low_stock_threshold": p.LowStockThreshold,
		"show_online": p.ShowOnline, "is_active": p.IsActive,
	}
}

func variantView(v model.Variant) map[string]any {
	price, cost := "", ""
	if v.Price != nil {
		price = v.Price.String()
	}
	if v.Cost != nil {
		cost = v.Cost.String()
	}
	return map[string]any{
		"name": v.Name, "sku": v.SKU, "barcode": v.Barcode, "price": price, "cost": cost,
		"is_active": v.IsActive, "sort_order": v.SortOrder,
	}
}

func variantPayload(p model.Product, v model.Variant) map[string]any {
	return map[string]any{"name": p.Name, "variant": v.Name, "variant_uuid": v.UUID.String(), "sku": v.SKU}
}

// offerView reduces offers to "Branch: price (off)" lines for the activity diff.
func offerView(offers []model.Offer) []string {
	out := make([]string, 0, len(offers))
	for _, o := range offers {
		line := o.BranchName + ": " + o.EffectivePrice.String()
		if !o.IsActive {
			line += " (off)"
		}
		out = append(out, line)
	}
	return out
}

// record writes the activity event of a product (business level: no
// branch) and publishes the list change.
func (h *Handler) record(r *http.Request, action string, p model.Product, payload map[string]any, changes map[string]activity.Change) {
	h.publish(r, action, p.UUID)
	if h.activity == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{"name": p.Name}
	}
	principal := authctx.MustPrincipal(r.Context())
	id := p.UUID
	h.activity.RecordEntry(r.Context(), activity.Entry{
		ActorID: &principal.UserInternal, Action: action, Resource: model.Slug, ResourceUUID: &id,
		Payload: payload, Changes: changes, Request: r,
	})
}

// publish tells open product and movement lists of the organization what
// changed.
func (h *Handler) publish(r *http.Request, action string, id uuid.UUID) {
	if h.events == nil {
		return
	}
	ev := resourceevents.ActionUpdated
	switch action {
	case "products.created":
		ev = resourceevents.ActionCreated
	case "products.deleted":
		ev = resourceevents.ActionDeleted
	}
	h.events.Publish(r.Context(), model.Slug, ev, id.String())
}

// salesChanged tells the open sale line and ledger lists that a stock sale
// or its return posted (or reversed) revenue in the movement's transaction.
func (h *Handler) salesChanged(r *http.Request) {
	if h.events == nil {
		return
	}
	h.events.Publish(r.Context(), saleLinesResource, resourceevents.ActionUpdated)
	h.events.Publish(r.Context(), ledgerEntriesResource, resourceevents.ActionUpdated)
}

// Live channels of the sales ledger (TEC-916).
const (
	saleLinesResource     = "tenant.sale_lines"
	ledgerEntriesResource = "tenant.ledger_entries"
)

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
		response.NotFound(w, r, "Product was not found")
	case errors.Is(err, usecase.ErrMovementNotFound):
		response.NotFound(w, r, "Stock movement was not found")
	case errors.Is(err, usecase.ErrNotReturnable):
		response.Error(w, r, http.StatusConflict, "STOCK_NOT_RETURNABLE", err.Error())
	case errors.Is(err, usecase.ErrAlreadyReturned):
		response.Error(w, r, http.StatusConflict, "STOCK_ALREADY_RETURNED", err.Error())
	case errors.Is(err, usecase.ErrVariantNotFound):
		response.NotFound(w, r, "Product variant was not found")
	case errors.Is(err, usecase.ErrImageNotFound):
		response.NotFound(w, r, "Image was not found")
	case errors.Is(err, usecase.ErrStorageUnavailable):
		response.ServiceUnavailable(w, r, response.CodeInternalError, "storage is not configured")
	case errors.Is(err, usecase.ErrCodeTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "sku", Message: err.Error(), Code: "taken",
		}})
	case errors.Is(err, usecase.ErrVariantNameTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "name", Message: err.Error(), Code: "taken",
		}})
	case errors.Is(err, usecase.ErrHasStock):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict,
			"Stock is not zero; move or count it out first", []response.Detail{{
				Field: "stock", Message: err.Error(), Code: "has_stock",
			}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
