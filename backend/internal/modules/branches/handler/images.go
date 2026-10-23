package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// UploadImage serves POST /v1/tenant/branches/{uuid}/images (multipart
// "file": JPEG, PNG or WebP up to 5 MiB; appended to the gallery).
func (h *Handler) UploadImage(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxBranchImageBytes+(1<<20))
	if err := r.ParseMultipartForm(usecase.MaxBranchImageBytes); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "invalid multipart form or file too large", Code: "invalid"}})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "file is required", Code: "required"}})
		return
	}
	defer func() { _ = file.Close() }()
	img, err := h.svc.UploadImage(r.Context(), scope.ID, id, usecase.ImageUpload{
		Body: file, Size: header.Size, Filename: header.Filename,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "branches.image_added", id, map[string]any{"image_uuid": img.UUID.String()})
	response.JSON(w, r, http.StatusCreated, img)
}

// ReorderImages serves PUT /v1/tenant/branches/{uuid}/images/order.
func (h *Handler) ReorderImages(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ImageOrderInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "body", Message: "invalid JSON body", Code: "invalid"}})
		return
	}
	images, err := h.svc.ReorderImages(r.Context(), scope.ID, id, in.ImageUUIDs)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "branches.images_reordered", id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]any{"items": images})
}

// DeleteImage serves DELETE /v1/tenant/branches/{uuid}/images/{imageUuid}.
func (h *Handler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	imageID, ok := pathUUID(w, r, "imageUuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteImage(r.Context(), scope.ID, id, imageID); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "branches.image_removed", id, map[string]any{"image_uuid": imageID.String()})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// PublicImage serves GET /v1/public/media/branches/{orgUuid}/images/{imageUuid}
// (no authentication: booking pages and the mobile app show branch photos).
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
