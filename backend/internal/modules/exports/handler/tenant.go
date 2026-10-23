package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// TenantExport returns a handler that queues an export of an
// organization-owned resource. The route guards the tenant permission; the
// job is stamped with the X-Organization organization by the use case.
func (h *Handler) TenantExport(resource string, allowedQuery ...string) http.HandlerFunc {
	allowed := map[string]bool{"q": true, "sort": true}
	for _, k := range allowedQuery {
		allowed[k] = true
	}
	return func(w http.ResponseWriter, r *http.Request) {
		p := authctx.MustPrincipal(r.Context())
		var in struct {
			Format string            `json:"format"`
			Query  map[string]string `json:"query"`
			Locale string            `json:"locale"`
		}
		if err := decodeJSON(w, r, &in); err != nil {
			return
		}
		query := ioengine.ExportQuery{}
		for k, v := range in.Query {
			if allowed[k] {
				query[k] = v
			}
		}
		if in.Locale == "" {
			in.Locale = "tr"
		}
		format := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(in.Format)))
		job, err := h.svc.RequestExport(r.Context(), p.UserInternal, resource, format, query, in.Locale)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		response.JSON(w, r, http.StatusAccepted, job)
	}
}

// TenantGet returns an export job of the caller in the current organization.
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

// TenantDownload streams a completed export of the caller in the current organization.
func (h *Handler) TenantDownload(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "uuid is invalid")
		return
	}
	file, ct, filename, err := h.svc.Download(r.Context(), id, p.UserInternal, false)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	defer func() { _ = file.Close() }()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", contentDispositionAttachment(filename))
	_, _ = io.Copy(w, file)
}
