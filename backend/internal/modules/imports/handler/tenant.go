package handler

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// TenantUpload returns a handler that creates an import job for an
// organization-owned resource (stamped with the X-Organization organization).
func (h *Handler) TenantUpload(resource string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.upload(w, r, resource)
	}
}

// TenantSample returns a handler that serves the import sample of a resource.
func (h *Handler) TenantSample(resource string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.sample(w, r, resource)
	}
}

// TenantGet returns an import job of the caller in the current organization.
// Mapping, preview, confirm and rollback reuse the platform handlers: the use
// case only finds jobs of the request's organization.
func (h *Handler) TenantGet(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return
	}
	job, err := h.svc.GetJob(r.Context(), id, p.UserInternal, false)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, job)
}
