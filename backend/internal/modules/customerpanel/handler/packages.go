package handler

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Packages serves GET /v1/customer/packages (TEC-984, "Paketlerim"):
// ?business=&state=&limit=&offset=.
func (h *Handler) Packages(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "read", h.limits.Read)
	if !ok {
		return
	}
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset instead of " + banned[0], Code: "forbidden"}})
		return
	}
	q := apiquery.Parse(values)
	f := model.PackageFilter{Limit: q.Limit, Offset: q.Offset}
	if raw := strings.TrimSpace(values.Get("business")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "business", Message: "business must be a uuid", Code: "invalid"}})
			return
		}
		f.BusinessUUID = &id
	}
	for _, s := range apiquery.SplitCSV(values.Get("state")) {
		if !slices.Contains(model.PackageStates, s) {
			response.ValidationError(w, r, []response.Detail{{Field: "state", Message: "unknown state " + s, Code: "invalid"}})
			return
		}
		f.States = append(f.States, s)
	}
	items, total, err := h.svc.Packages(r.Context(), p, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Package serves GET /v1/customer/packages/{uuid}: one package with its
// usage history. Another person's package answers 404.
func (h *Handler) Package(w http.ResponseWriter, r *http.Request) {
	p, ok := h.session(w, r, "read", h.limits.Read)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Package was not found")
		return
	}
	out, err := h.svc.Package(r.Context(), p, id)
	if errors.Is(err, usecase.ErrNotFound) {
		response.NotFound(w, r, "Package was not found")
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CustomerOf returns the business's customer record of the request's
// customer session (uuid.Nil when the business does not know the person).
// The public booking module books a signed-in person with it (TEC-984).
func (h *Handler) CustomerOf(r *http.Request, orgID int64) (uuid.UUID, error) {
	p := principalFrom(r.Context())
	if p.IdentityID == 0 {
		return uuid.Nil, usecase.ErrUnauthorized
	}
	id, err := h.svc.LinkedCustomer(r.Context(), p, orgID)
	if errors.Is(err, usecase.ErrNotFound) {
		return uuid.Nil, nil
	}
	return id, err
}
