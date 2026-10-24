package handler

// Platform routes of the customer panel (TEC-972): numbers that changed
// hands, and reassignment notices processed from Takwim management. Staff
// tokens with the platform.customer_phones permissions only.

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// ResourceCustomerPhones is the activity resource of the platform routes.
const ResourceCustomerPhones = "platform.customer_phones"

// CodeAlreadyReleased: nobody proved the number since its last release.
const CodeAlreadyReleased = "PHONE_ALREADY_RELEASED"

// releaseSort are the sort columns of the platform list.
var releaseSort = apiquery.SortColumns{"released_at": "released_at"}

// PlatformHandler serves /v1/platform/customer-phone-releases.
type PlatformHandler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// NewPlatform builds the platform handler. rec may be nil.
func NewPlatform(svc *usecase.Service, rec *activity.Recorder) *PlatformHandler {
	return &PlatformHandler{svc: svc, activity: rec}
}

// ListReleases serves GET /v1/platform/customer-phone-releases.
func (h *PlatformHandler) ListReleases(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, releaseSort); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "sort", Message: err.Error(), Code: "invalid"}})
		return
	}
	f := model.ReleaseFilter{
		Q: q.Q, Reasons: apiquery.SplitCSV(values.Get("reason")), Sources: apiquery.SplitCSV(values.Get("source")),
		Limit: q.Limit, Offset: q.Offset,
	}
	if len(q.Sort) > 0 {
		f.OldestFirst = !q.Sort[0].Desc
	}
	items, total, err := h.svc.ListReleases(r.Context(), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateReassignment serves POST /v1/platform/customer-phone-releases: a
// reassignment notice of a number, processed at once.
func (h *PlatformHandler) CreateReassignment(w http.ResponseWriter, r *http.Request) {
	var in model.ReassignmentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, err := h.svc.ProcessReassignment(r.Context(), in, p.UserInternal)
	if errors.Is(err, usecase.ErrAlreadyReleased) {
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeAlreadyReleased, err.Error(), []response.Detail{{
			Field: "phone", Message: err.Error(), Code: "already_released",
		}})
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), &p.UserInternal, "customer_phones.reassigned", ResourceCustomerPhones, &out.UUID, map[string]any{
			"phone": out.PhoneMasked, "has_profile": out.HasProfile,
			"businesses_hidden": out.BusinessesHidden, "sessions_revoked": out.SessionsRevoked,
		}, r)
	}
	response.JSON(w, r, http.StatusCreated, out)
}
