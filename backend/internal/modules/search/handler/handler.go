package handler

import (
	"net/http"
	"strconv"
	"strings"

	searchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/search/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

type Handler struct {
	svc *searchusecase.Service
}

func New(svc *searchusecase.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Specs(w http.ResponseWriter, r *http.Request) {
	specs := h.svc.ListSpecs(r.Context())
	response.JSON(w, r, http.StatusOK, map[string]any{
		"items":   specs,
		"enabled": h.svc.Enabled(),
	})
}

// TenantSpecs lists the tenant specs of the current organization.
func (h *Handler) TenantSpecs(w http.ResponseWriter, r *http.Request) {
	specs := h.svc.ListTenantSpecs(r.Context())
	if specs == nil {
		specs = []searchengine.Spec{}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{
		"items":   specs,
		"enabled": h.svc.Enabled(),
	})
}

// TenantSearch searches the current organization's records.
func (h *Handler) TenantSearch(w http.ResponseWriter, r *http.Request) {
	q, spec, limit := searchParams(r)
	items := h.svc.SearchTenant(r.Context(), q, spec, limit)
	if items == nil {
		items = []searchengine.Hit{}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func searchParams(r *http.Request) (string, string, int) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	spec := strings.TrimSpace(r.URL.Query().Get("spec"))
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	return q, spec, limit
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	spec := strings.TrimSpace(r.URL.Query().Get("spec"))
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	items := h.svc.Search(r.Context(), q, spec, limit)
	if items == nil {
		items = []searchengine.Hit{}
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}
