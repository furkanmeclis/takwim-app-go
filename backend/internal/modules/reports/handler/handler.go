// Package handler is the HTTP layer of the reports (TEC-925). Every
// handler runs behind RequireOrganization; the organization and the branch
// scope come from the request context, never from the client.
package handler

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Resource is the activity resource of a report export.
const Resource = "tenant.reports"

// Handler serves /v1/tenant/reports/*.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// parseFilter reads date_from, date_to, branch (uuids and / or
// head_office) and staff (uuids).
func parseFilter(r *http.Request) (model.Filter, []response.Detail) {
	values := r.URL.Query()
	var f model.Filter
	var details []response.Detail
	date := func(key string) *time.Time {
		raw := strings.TrimSpace(values.Get(key))
		if raw == "" {
			return nil
		}
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			details = append(details, response.Detail{Field: key, Message: key + " must be YYYY-MM-DD", Code: "invalid"})
			return nil
		}
		return &t
	}
	f.DateFrom, f.DateTo = date("date_from"), date("date_to")
	ids := func(key string, headOffice *bool) []uuid.UUID {
		var out []uuid.UUID
		for _, raw := range values[key] {
			for _, part := range apiquery.SplitCSV(raw) {
				if headOffice != nil && part == model.HeadOffice {
					*headOffice = true
					continue
				}
				id, err := uuid.Parse(part)
				if err != nil {
					details = append(details, response.Detail{Field: key, Message: key + " must list uuids", Code: "invalid"})
					return nil
				}
				out = append(out, id)
			}
		}
		return out
	}
	f.Branches = ids("branch", &f.HeadOffice)
	f.Staff = ids("staff", nil)
	return f, details
}

// Summary serves GET /v1/tenant/reports/summary.
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	f, details := parseFilter(r)
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	out, err := h.svc.Summary(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Daily serves GET /v1/tenant/daily-summary: the home page summary of
// one business day (date, default today). Each section follows the
// caller's permissions and the modules of the business.
func (h *Handler) Daily(w http.ResponseWriter, r *http.Request) {
	var day *time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("date")); raw != "" {
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "date", Message: "date must be YYYY-MM-DD", Code: "invalid"}})
			return
		}
		day = &t
	}
	scope := orgctx.Must(r.Context())
	modules := entitlement.ForScope(scope)
	access := model.DailyAccess{
		OwnReservations:      scope.Has(rbac.PermTenantReservationsRead) || scope.Has(rbac.PermTenantReservationsOwn),
		BusinessReservations: scope.Has(rbac.PermTenantReservationsRead),
		Money:                scope.Has(rbac.PermTenantFinanceRead) && modules.Allows(sector.ModuleReports),
		Earnings: (scope.Has(rbac.PermTenantEarningsRead) || scope.Has(rbac.PermTenantEarningsOwn)) &&
			modules.Allows(sector.ModuleStaffShares),
	}
	out, err := h.svc.Daily(r.Context(), scope.ID, authctx.MustPrincipal(r.Context()).UserInternal, day, access)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Export serves GET /v1/tenant/reports/export: the days (section=days,
// default) or the branch comparison (section=branches) of the same filter
// as CSV (UTF-8 with BOM), column titles in the locale (tr default).
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	f, details := parseFilter(r)
	section := strings.TrimSpace(r.URL.Query().Get("section"))
	if section == "" {
		section = usecase.SectionDays
	}
	if !slices.Contains(usecase.Sections, section) {
		details = append(details, response.Detail{Field: "section", Message: "section must be one of " + strings.Join(usecase.Sections, ", "), Code: "invalid"})
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	scope := orgctx.Must(r.Context())
	rep, err := h.svc.Summary(r.Context(), scope.ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	body, err := ioengine.EncodeCSV(usecase.Dataset(rep, section, r.URL.Query().Get("locale")), r.URL.Query().Get("locale"))
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	if h.activity != nil {
		if p, ok := authctx.PrincipalFrom(r.Context()); ok {
			h.activity.RecordEntry(r.Context(), activity.Entry{
				ActorID: &p.UserInternal, Action: "reports.exported", Resource: Resource, Request: r,
				Payload: map[string]any{
					"section": section, "date_from": rep.DateFrom, "date_to": rep.DateTo, "format": "csv",
					"branches": len(f.Branches), "head_office": f.HeadOffice, "staff": len(f.Staff),
				},
			})
		}
	}
	name := fmt.Sprintf("rapor_%s_%s_%s.csv", section, strings.ReplaceAll(rep.DateFrom, "-", ""), strings.ReplaceAll(rep.DateTo, "-", ""))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	response.InternalErr(w, r, err, "")
}
