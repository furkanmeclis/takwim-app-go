package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	activityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/activity/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// tenantSort is the only sortable column of the tenant activity list.
var tenantSort = apiquery.SortColumns{"created_at": "created_at"}

// TenantList serves GET /v1/tenant/activity.
func (h *Handler) TenantList(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	values := r.URL.Query()
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, tenantSort); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "sort", Message: err.Error(), Code: "invalid"}})
		return
	}
	f := activityusecase.TenantFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q, SortDesc: true,
		Resources: apiquery.SplitCSV(values.Get("resource")),
		Actions:   apiquery.SplitCSV(values.Get("action")),
	}
	if len(q.Sort) > 0 {
		f.SortDesc = q.Sort[0].Desc
	}
	var details []response.Detail
	for _, p := range []struct {
		key string
		dst **uuid.UUID
	}{{"resource_uuid", &f.ResourceUUID}, {"actor_uuid", &f.ActorUUID}} {
		if raw := strings.TrimSpace(values.Get(p.key)); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				details = append(details, response.Detail{Field: p.key, Message: "must be a uuid", Code: "invalid"})
				continue
			}
			*p.dst = &id
		}
	}
	for _, p := range []struct {
		key string
		dst **time.Time
	}{{"from", &f.From}, {"to", &f.To}} {
		if raw := strings.TrimSpace(values.Get(p.key)); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				details = append(details, response.Detail{Field: p.key, Message: "must be an RFC 3339 time", Code: "invalid"})
				continue
			}
			*p.dst = &t
		}
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	items, total, err := h.svc.TenantList(r.Context(), scope.ID, f)
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// TenantFacets serves GET /v1/tenant/activity/facets.
func (h *Handler) TenantFacets(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	out, err := h.svc.TenantFacets(r.Context(), scope.ID)
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
