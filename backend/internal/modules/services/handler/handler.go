// Package handler is the HTTP layer of the services tenant resource. Every
// handler runs behind RequireOrganization and reads the organization id from
// orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/tenant/services and /v1/tenant/service-categories.
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

// Meta serves GET /v1/tenant/services/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/services/facets.
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
// bulk "all matching" scope. category takes category uuids and "none".
func ParseListFilter(values map[string][]string) (model.ListFilter, []response.Detail) {
	var f model.ListFilter
	var details []response.Detail
	for _, p := range []struct {
		key  string
		dst  *[]uuid.UUID
		none *bool
	}{{"category", &f.Categories, &f.Uncategorized}, {"branch", &f.Branches, nil}} {
	values:
		for _, raw := range values[p.key] {
			for _, part := range apiquery.SplitCSV(raw) {
				if p.none != nil && part == model.UncategorizedFacet {
					*p.none = true
					continue
				}
				id, err := uuid.Parse(part)
				if err != nil {
					details = append(details, response.Detail{Field: p.key, Message: p.key + " must list uuids", Code: "invalid"})
					break values
				}
				*p.dst = append(*p.dst, id)
			}
		}
	}
	for _, raw := range values["payment_mode"] {
		f.PaymentModes = append(f.PaymentModes, apiquery.SplitCSV(raw)...)
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

// List serves GET /v1/tenant/services.
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

// Create serves POST /v1/tenant/services.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	svc, err := h.svc.Create(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "services.created", svc, nil, nil)
	response.JSON(w, r, http.StatusCreated, svc)
}

// Get serves GET /v1/tenant/services/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	svc, err := h.svc.Get(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, svc)
}

// Patch serves PATCH /v1/tenant/services/{uuid}. branch_uuids is a create
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
	svc, err := h.svc.Patch(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var changes map[string]activity.Change
	if beforeErr == nil {
		changes = activity.Diff(diffView(before), diffView(svc))
	}
	h.record(r, "services.updated", svc, nil, changes)
	response.JSON(w, r, http.StatusOK, svc)
}

// Delete serves DELETE /v1/tenant/services/{uuid} (soft delete).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	svc, err := h.svc.Delete(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "services.deleted", svc, nil, nil)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// Activity serves GET /v1/tenant/services/{uuid}/activity.
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

// PutOffers serves PUT /v1/tenant/services/{uuid}/offers. The body lists
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
	svc, before, err := h.svc.SetOffers(r.Context(), scope.ID, id, in.Offers)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "services.offers_updated", svc, nil,
		activity.Diff(map[string]any{"offers": offerView(before)}, map[string]any{"offers": offerView(svc.Offers)}))
	response.JSON(w, r, http.StatusOK, svc)
}

// PutStaff serves PUT /v1/tenant/services/{uuid}/staff (replaces the list).
func (h *Handler) PutStaff(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.StaffLinksInput
	if !decodeJSON(w, r, &in) {
		return
	}
	svc, before, err := h.svc.SetStaff(r.Context(), scope.ID, id, in.Staff)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "services.staff_updated", svc, nil,
		activity.Diff(map[string]any{"staff": staffView(before)}, map[string]any{"staff": staffView(svc.Staff)}))
	response.JSON(w, r, http.StatusOK, svc)
}

// UploadImage serves PUT /v1/tenant/services/{uuid}/image (multipart
// "file": JPEG, PNG or WebP up to 5 MiB; replaces the cover).
func (h *Handler) UploadImage(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxServiceImageBytes+(1<<20))
	if err := r.ParseMultipartForm(usecase.MaxServiceImageBytes); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "invalid multipart form or file too large", Code: "invalid"}})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "file is required", Code: "required"}})
		return
	}
	defer func() { _ = file.Close() }()
	svc, err := h.svc.UploadImage(r.Context(), scope.ID, id, usecase.ImageUpload{Body: file, Size: header.Size})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "services.image_updated", svc, nil, nil)
	response.JSON(w, r, http.StatusOK, svc)
}

// DeleteImage serves DELETE /v1/tenant/services/{uuid}/image.
func (h *Handler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	svc, err := h.svc.DeleteImage(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "services.image_removed", svc, nil, nil)
	response.JSON(w, r, http.StatusOK, svc)
}

// PublicImage serves GET /v1/public/media/services/{orgUuid}/images/{imageUuid}
// (no authentication: booking pages and the mobile app show service covers).
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

// ListCategories serves GET /v1/tenant/service-categories.
func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	items, err := h.svc.Categories(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateCategory serves POST /v1/tenant/service-categories.
func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.CategoryInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.CreateCategory(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordCategory(r, "services.category_created", c, nil)
	response.JSON(w, r, http.StatusCreated, c)
}

// PatchCategory serves PATCH /v1/tenant/service-categories/{uuid}.
func (h *Handler) PatchCategory(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.CategoryInput
	if !decodeJSON(w, r, &in) {
		return
	}
	before, c, err := h.svc.PatchCategory(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordCategory(r, "services.category_updated", c,
		activity.Diff(before, c, "uuid", "services_count", "created_at", "updated_at"))
	response.JSON(w, r, http.StatusOK, c)
}

// DeleteCategory serves DELETE /v1/tenant/service-categories/{uuid}; its
// services become uncategorized.
func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	c, err := h.svc.DeleteCategory(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordCategory(r, "services.category_deleted", c, nil)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// diffView is the part of a service a catalog edit changes.
func diffView(s model.Service) map[string]any {
	category := ""
	if s.Category != nil {
		category = s.Category.Name
	}
	return map[string]any{
		"category": category, "name": s.Name, "description": s.Description,
		"duration_minutes": s.DurationMinutes, "buffer_before_minutes": s.BufferBeforeMinutes,
		"buffer_after_minutes": s.BufferAfterMinutes, "price": s.Price.String(),
		"payment_mode": s.PaymentMode, "staff_mode": s.StaffMode, "resource_kinds": s.ResourceKinds,
		"show_online": s.ShowOnline, "is_active": s.IsActive, "sort_order": s.SortOrder,
	}
}

// offerView reduces offers to "CODE: price / minutes (off)" lines for the
// activity diff.
func offerView(offers []model.Offer) []string {
	out := make([]string, 0, len(offers))
	for _, o := range offers {
		line := o.BranchName + ": " + o.EffectivePrice.String() + " / " + strconv.Itoa(int(o.EffectiveDurationMinutes))
		if !o.IsActive {
			line += " (off)"
		}
		out = append(out, line)
	}
	return out
}

func staffView(staff []model.StaffLink) []string {
	out := make([]string, 0, len(staff))
	for _, s := range staff {
		line := s.Name
		if s.Price != nil {
			line += " · " + s.Price.String()
		}
		if s.DurationMinutes != nil {
			line += " · " + strconv.Itoa(int(*s.DurationMinutes))
		}
		out = append(out, line)
	}
	return out
}

// record writes the activity event of a service (business level: no
// branch) and publishes the list change.
func (h *Handler) record(r *http.Request, action string, s model.Service, payload map[string]any, changes map[string]activity.Change) {
	h.publish(r, action, s.UUID)
	if h.activity == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{"name": s.Name}
	}
	p := authctx.MustPrincipal(r.Context())
	id := s.UUID
	h.activity.RecordEntry(r.Context(), activity.Entry{
		ActorID: &p.UserInternal, Action: action, Resource: model.Slug, ResourceUUID: &id,
		Payload: payload, Changes: changes, Request: r,
	})
}

// recordCategory writes a category event under the organization (no
// service subject).
func (h *Handler) recordCategory(r *http.Request, action string, c model.Category, changes map[string]activity.Change) {
	if h.events != nil {
		// Category names show on every row of the list.
		h.events.Publish(r.Context(), model.Slug, resourceevents.ActionUpdated)
	}
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.RecordEntry(r.Context(), activity.Entry{
		ActorID: &p.UserInternal, Action: action, Resource: model.Slug,
		Payload: map[string]any{"name": c.Name, "category_uuid": c.UUID.String()}, Changes: changes, Request: r,
	})
}

// publish tells open service lists of the organization what changed.
func (h *Handler) publish(r *http.Request, action string, id uuid.UUID) {
	if h.events == nil {
		return
	}
	ev := resourceevents.ActionUpdated
	switch action {
	case "services.created":
		ev = resourceevents.ActionCreated
	case "services.deleted":
		ev = resourceevents.ActionDeleted
	}
	h.events.Publish(r.Context(), model.Slug, ev, id.String())
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
		response.NotFound(w, r, "Service was not found")
	case errors.Is(err, usecase.ErrCategoryNotFound):
		response.NotFound(w, r, "Service category was not found")
	case errors.Is(err, usecase.ErrImageNotFound):
		response.NotFound(w, r, "Image was not found")
	case errors.Is(err, usecase.ErrStorageUnavailable):
		response.ServiceUnavailable(w, r, response.CodeInternalError, "storage is not configured")
	case errors.Is(err, usecase.ErrNameTaken), errors.Is(err, usecase.ErrCategoryNameTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "name", Message: err.Error(), Code: "taken",
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
