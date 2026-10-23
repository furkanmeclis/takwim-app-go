package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	bulkusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// TenantExecute returns a handler that runs a bulk action on an
// organization-owned resource. The action permission is checked against the
// caller's permissions in the X-Organization organization.
func (h *Handler) TenantExecute(resource string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := authctx.MustPrincipal(r.Context())
		scope := orgctx.Must(r.Context())
		var body bulkRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "invalid JSON body")
			return
		}
		action := strings.TrimSpace(body.Action)
		if action == "" {
			response.BadRequest(w, r, response.CodeValidationError, "action is required")
			return
		}
		def, _, err := h.svc.Registry().ActionDef(resource, action)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "unknown bulk action")
			return
		}
		if !scope.Has(def.Permission) {
			response.Forbidden(w, r, "Missing permission: "+def.Permission)
			return
		}
		result, err := h.svc.Execute(r.Context(), p.UserInternal, bulkusecase.ExecuteInput{
			Resource: resource, Action: action, Target: body.Target, Locale: body.Locale,
		})
		if err != nil {
			writeErr(w, r, err)
			return
		}
		if result.Async {
			response.JSON(w, r, http.StatusAccepted, result.Job)
			return
		}
		response.JSON(w, r, http.StatusOK, map[string]any{"sync": true, "summary": result.Summary})
	}
}

// TenantGet returns a bulk job of the caller in the current organization.
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

// TenantRollback undoes a bulk job of the caller in the current organization.
func (h *Handler) TenantRollback(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	scope := orgctx.Must(r.Context())
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
	def, _, err := h.svc.Registry().ActionDef(job.Resource, job.Action)
	if err != nil || !scope.Has(def.Permission) {
		response.Forbidden(w, r, "Missing permission for bulk rollback")
		return
	}
	updated, err := h.svc.Rollback(r.Context(), id, p.UserInternal)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, updated)
}
