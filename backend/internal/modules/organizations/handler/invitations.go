package handler

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"

	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/localtime"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Staff invitations (/v1/tenant/invitations, /v1/invitations/{token}) and
// the organization profile (/v1/tenant/organization).

// ResourceTenantInvitations is the live list slug of the invitations tab
// (org:{org_uuid}:resource:tenant.invitations).
const ResourceTenantInvitations = "tenant.invitations"

// PlatformOrganizationsMeta is the table contract of GET /v1/platform/organizations.
func PlatformOrganizationsMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:    ResourcePlatformOrganizations,
		DefaultSort: "-created_at",
		DefaultFields: []string{
			"name", "slug", "owner", "member_count", "branch_count", "status", "created_at",
		},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Bulk: true,
		},
		SearchableFields: []string{"name", "slug", "owner_email"},
		SortableFields: []string{
			"name", "slug", "status", "member_count", "branch_count", "access_ends_at", "created_at", "updated_at",
		},
		FilterableFields: []string{"status", "access", "window"},
		Columns: []resourcemeta.Column{
			{Key: "name", LabelKey: "organizations.columns.name", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "slug", LabelKey: "organizations.columns.slug", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "owner", LabelKey: "organizations.columns.owner", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "member_count", LabelKey: "organizations.columns.member_count", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "branch_count", LabelKey: "organizations.columns.branch_count", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "status", LabelKey: "organizations.columns.status", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "access", LabelKey: "organizations.columns.access", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "access_ends_at", LabelKey: "organizations.columns.access_ends_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: false},
			{Key: "created_at", LabelKey: "organizations.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
		},
		Filters: []resourcemeta.Filter{
			{Key: "status", LabelKey: "organizations.columns.status", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "organization_status"},
			{Key: "access", LabelKey: "organizations.columns.access", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "organization_access"},
			{Key: "window", LabelKey: "organizations.columns.access_ends_at", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "organization_window"},
		},
		Includes:    []string{"owner", "counts"},
		BulkActions: OrganizationBulkActions(),
	}
}

// OrganizationBulkActions are the bulk actions of the platform list (the
// adapter in organizations/adapters implements them).
func OrganizationBulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: "activate", LabelKey: "bulk.actions.organizations.activate",
			Permission: rbac.PermPlatformOrganizationsBulkActivate, Reversible: true,
		},
		{
			ID: "suspend", LabelKey: "bulk.actions.organizations.suspend",
			Permission: rbac.PermPlatformOrganizationsBulkSuspend, Destructive: true, Reversible: true,
			ConfirmKey: "bulk.confirm.organizations.suspend",
		},
	}
}

// TenantInvitationsMeta is the table contract of GET /v1/tenant/invitations.
func TenantInvitationsMeta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      ResourceTenantInvitations,
		DefaultSort:   "-created_at",
		DefaultFields: []string{"email", "roles", "invited_by", "expires_at", "status"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"email"},
		SortableFields:   []string{"email", "expires_at", "created_at"},
		FilterableFields: []string{"status"},
		Columns: []resourcemeta.Column{
			{Key: "email", LabelKey: "invitations.columns.email", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "roles", LabelKey: "invitations.columns.roles", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "invited_by", LabelKey: "invitations.columns.invited_by", Type: resourcemeta.ColumnTypeString, DefaultVisible: true},
			{Key: "expires_at", LabelKey: "invitations.columns.expires_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
			{Key: "status", LabelKey: "invitations.columns.status", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "created_at", LabelKey: "invitations.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "status", LabelKey: "invitations.columns.status", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "invitation_status"},
		},
		Includes:    []string{},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// TenantInvitationsMetaH serves GET /v1/tenant/invitations/meta.
func (h *Handler) TenantInvitationsMetaH(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, TenantInvitationsMeta())
}

// TenantListInvitations serves GET /v1/tenant/invitations.
func (h *Handler) TenantListInvitations(w http.ResponseWriter, r *http.Request) {
	h.listInvitations(w, r, orgctx.Must(r.Context()).ID)
}

func (h *Handler) listInvitations(w http.ResponseWriter, r *http.Request, orgID int64) {
	q, ok := parseListQuery(w, r, apiquery.SortColumns(orgusecase.InvitationSortColumns))
	if !ok {
		return
	}
	statuses, err := orgmodel.EnumList(r.URL.Query().Get("status"),
		orgmodel.InvitationPending, orgmodel.InvitationExpired, orgmodel.InvitationAccepted, orgmodel.InvitationRevoked)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "status", Message: err.Error(), Code: "invalid"}})
		return
	}
	items, total, err := h.mgmt.ListInvitations(r.Context(), orgID, orgmodel.InvitationListFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q, Statuses: statuses,
		SortField: firstSortField(q, "created_at"), SortDesc: firstSortDesc(q, true),
	})
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// TenantCreateInvitation serves POST /v1/tenant/invitations.
func (h *Handler) TenantCreateInvitation(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in orgmodel.CreateInvitationInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	inv, err := h.mgmt.CreateInvitation(r.Context(), scope, h.actor(r), in)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	h.recordInvitation(r.Context(), r, "invitations.created", inv, resourceevents.ActionCreated)
	response.JSON(w, r, http.StatusCreated, inv)
}

// TenantRevokeInvitation serves POST /v1/tenant/invitations/{uuid}/revoke.
func (h *Handler) TenantRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := parsePathUUID(w, r, "uuid", "invitation uuid is invalid")
	if !ok {
		return
	}
	inv, err := h.mgmt.RevokeInvitation(r.Context(), scope, h.actor(r), id)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	h.recordInvitation(r.Context(), r, "invitations.revoked", inv, resourceevents.ActionUpdated)
	response.JSON(w, r, http.StatusOK, inv)
}

// TenantResendInvitation serves POST /v1/tenant/invitations/{uuid}/resend.
func (h *Handler) TenantResendInvitation(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := parsePathUUID(w, r, "uuid", "invitation uuid is invalid")
	if !ok {
		return
	}
	inv, err := h.mgmt.ResendInvitation(r.Context(), scope, h.actor(r), id)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	h.recordInvitation(r.Context(), r, "invitations.resent", inv, resourceevents.ActionUpdated)
	response.JSON(w, r, http.StatusOK, inv)
}

// recordInvitation records the activity (organization stamped from ctx) and
// refreshes the invitations tab and the platform list.
func (h *Handler) recordInvitation(ctx context.Context, r *http.Request, action string, inv orgmodel.Invitation, event string) {
	if h.events != nil {
		h.events.Publish(ctx, ResourceTenantInvitations, event, inv.UUID.String())
		if scope, ok := orgctx.From(ctx); ok {
			h.publishPlatform(ctx, resourceevents.ActionUpdated, scope.UUID)
		}
	}
	if h.activity == nil {
		return
	}
	var actor *int64
	if p, ok := authctx.PrincipalFrom(ctx); ok {
		actor = &p.UserInternal
	}
	h.activity.Record(ctx, actor, action, ResourceTenantInvitations, &inv.UUID, map[string]any{
		"email": inv.Email, "roles": roleKeys(inv.Roles), "branch_scope": inv.BranchScope,
	}, r)
}

// publishPlatformInvitation refreshes the organization's invitations tab
// after a platform operator changed an invitation.
func (h *Handler) publishPlatformInvitation(r *http.Request, orgUUID uuid.UUID, action string, invUUID uuid.UUID) {
	if h.events == nil {
		return
	}
	org, err := h.svc.GetOrganization(r.Context(), orgUUID)
	if err != nil {
		return
	}
	ctx := orgctx.WithScope(r.Context(), orgctx.Scope{ID: org.ID, UUID: org.UUID, Slug: org.Slug})
	h.events.Publish(ctx, ResourceTenantInvitations, action, invUUID.String())
	h.publishPlatform(r.Context(), resourceevents.ActionUpdated, orgUUID)
}

// --- public accept flow ------------------------------------------------------

// PreviewInvitation serves GET /v1/invitations/{token} (public): organization
// name, inviter, masked e-mail and state. No e-mail address leaks.
func (h *Handler) PreviewInvitation(w http.ResponseWriter, r *http.Request) {
	preview, err := h.mgmt.PreviewInvitation(r.Context(), r.PathValue("token"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, preview)
}

// AcceptInvitation serves POST /v1/invitations/{token}/accept (signed in).
func (h *Handler) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	if p.ImpersonatorUserID != nil {
		response.Forbidden(w, r, "invitations cannot be accepted while impersonating")
		return
	}
	result, err := h.mgmt.AcceptInvitation(r.Context(), r.PathValue("token"), p.UserInternal)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordAccepted(r, &p.UserInternal, result)
	response.JSON(w, r, http.StatusOK, result)
}

// RegisterInvitation serves POST /v1/invitations/{token}/register (public):
// creates the invitee's account for the invited e-mail and accepts.
func (h *Handler) RegisterInvitation(w http.ResponseWriter, r *http.Request) {
	if h.limiter != nil {
		if ok, _ := h.limiter.AllowRegister(r.Context(), clientIP(r)); !ok {
			response.TooManyRequests(w, r, "")
			return
		}
	}
	var in orgmodel.RegisterInvitedInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	result, err := h.mgmt.RegisterAndAccept(r.Context(), r.PathValue("token"), in)
	if err != nil {
		writeTenantError(w, r, err)
		return
	}
	h.recordAccepted(r, nil, result)
	response.JSON(w, r, http.StatusCreated, result)
}

func (h *Handler) recordAccepted(r *http.Request, actor *int64, result orgmodel.AcceptInvitationResult) {
	org, err := h.svc.GetOrganization(r.Context(), result.OrganizationUUID)
	if err != nil {
		return
	}
	ctx := orgctx.WithScope(r.Context(), orgctx.Scope{ID: org.ID, UUID: org.UUID, Slug: org.Slug})
	if h.events != nil {
		h.events.Publish(ctx, ResourceTenantInvitations, resourceevents.ActionUpdated)
		h.events.Publish(ctx, resourceTenantMembers, resourceevents.ActionCreated)
		h.events.Publish(ctx, resourceTenantRoles, resourceevents.ActionBulk)
		h.publishPlatform(r.Context(), resourceevents.ActionUpdated, org.UUID)
	}
	if h.activity != nil {
		h.activity.Record(ctx, actor, "invitations.accepted", ResourceTenantInvitations, nil, map[string]any{
			"email": result.Email, "already_member": result.AlreadyMember,
		}, r)
	}
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// --- organization profile (name + logo) ---------------------------------------

// TenantGetOrganization serves GET /v1/tenant/organization.
func (h *Handler) TenantGetOrganization(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	org, err := h.svc.GetOrganization(r.Context(), scope.UUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, tenantOrganization(org))
}

// TenantPatchOrganization serves PATCH /v1/tenant/organization (name, time
// zone and currency; slug, status and access window stay platform-managed).
func (h *Handler) TenantPatchOrganization(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in struct {
		Name     *string `json:"name"`
		Timezone *string `json:"timezone"`
		Currency *string `json:"currency"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	var details []response.Detail
	if in.Name == nil && in.Timezone == nil && in.Currency == nil {
		details = append(details, response.Detail{Field: "name", Message: "at least one field is required", Code: "required"})
	}
	if in.Name != nil && (strings.TrimSpace(*in.Name) == "" || len([]rune(strings.TrimSpace(*in.Name))) > 255) {
		details = append(details, response.Detail{Field: "name", Message: "name is required (max 255 characters)", Code: "required"})
	}
	if in.Timezone != nil && !localtime.ValidZone(strings.TrimSpace(*in.Timezone)) {
		details = append(details, response.Detail{Field: "timezone", Message: "timezone must be an IANA zone such as Europe/Istanbul", Code: "invalid"})
	}
	if in.Currency != nil {
		if _, err := money.ParseCurrency(strings.ToUpper(strings.TrimSpace(*in.Currency))); err != nil {
			details = append(details, response.Detail{Field: "currency", Message: "currency must be a 3-letter ISO 4217 code", Code: "invalid"})
		}
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	org, err := h.svc.PatchOrganization(r.Context(), scope.UUID, orgmodel.PatchOrganizationInput{
		Name: in.Name, Timezone: in.Timezone, Currency: in.Currency,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	// The scope holds the profile before the edit.
	after := map[string]any{"name": org.Name, "timezone": org.Timezone, "currency": org.Currency}
	changes := activity.Diff(map[string]any{"name": scope.Name, "timezone": scope.Timezone, "currency": scope.Currency}, after)
	h.recordProfileChanges(r, "organization.updated", org, after, changes)
	response.JSON(w, r, http.StatusOK, tenantOrganization(org))
}

// TenantUploadLogo serves PUT /v1/tenant/organization/logo (multipart
// "logo": PNG, JPEG, WebP or SVG up to 2 MB).
func (h *Handler) TenantUploadLogo(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	if h.storage == nil {
		response.ServiceUnavailable(w, r, response.CodeInternalError, "storage is not configured")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, (3<<20)+(1<<20))
	if err := r.ParseMultipartForm(3 << 20); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "logo", Message: "invalid multipart form", Code: "invalid"}})
		return
	}
	file, header, err := r.FormFile("logo")
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "logo", Message: "logo is required", Code: "required"}})
		return
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	mime, err := storage.DetectLogoMIME(header.Header.Get("Content-Type"), head[:n])
	if err == nil {
		err = storage.ValidateLogoSize(header.Size)
	}
	var ext string
	if err == nil {
		ext, err = storage.LogoExtForMIME(mime)
	}
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "logo", Message: err.Error(), Code: "invalid"}})
		return
	}
	key := storage.OrganizationLogoObjectKey(scope.UUID.String(), uuid.NewString(), ext)
	body := io.MultiReader(strings.NewReader(string(head[:n])), file)
	if err := h.storage.Upload(r.Context(), storage.File{
		Body: body, Size: header.Size, ContentType: mime, Filename: header.Filename,
	}, key); err != nil {
		response.InternalErr(w, r, err, "logo upload failed")
		return
	}
	org, previous, err := h.mgmt.SetLogo(r.Context(), scope.ID, &key)
	if err != nil {
		_ = h.storage.Delete(r.Context(), key)
		writeError(w, r, err)
		return
	}
	if previous != "" && previous != key {
		_ = h.storage.Delete(r.Context(), previous)
	}
	h.recordProfile(r, "organization.logo_updated", org, map[string]any{})
	response.JSON(w, r, http.StatusOK, tenantOrganization(org))
}

// TenantDeleteLogo serves DELETE /v1/tenant/organization/logo.
func (h *Handler) TenantDeleteLogo(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	org, previous, err := h.mgmt.SetLogo(r.Context(), scope.ID, nil)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if previous != "" && h.storage != nil {
		_ = h.storage.Delete(r.Context(), previous)
	}
	h.recordProfile(r, "organization.logo_removed", org, map[string]any{})
	response.JSON(w, r, http.StatusOK, tenantOrganization(org))
}

func (h *Handler) recordProfile(r *http.Request, action string, org orgmodel.Organization, payload map[string]any) {
	h.recordProfileChanges(r, action, org, payload, nil)
}

func (h *Handler) recordProfileChanges(r *http.Request, action string, org orgmodel.Organization, payload map[string]any, changes map[string]activity.Change) {
	h.publishPlatform(r.Context(), resourceevents.ActionUpdated, org.UUID)
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.RecordEntry(r.Context(), activity.Entry{
		ActorID: &p.UserInternal, Action: action, Resource: "tenant.organization", ResourceUUID: &org.UUID,
		Payload: payload, Changes: changes, Request: r,
	})
}

// TenantOrganization is the organization profile as the tenant shell sees it.
type TenantOrganization struct {
	UUID           uuid.UUID `json:"uuid"`
	Slug           string    `json:"slug"`
	Name           string    `json:"name"`
	LogoURL        *string   `json:"logo_url"`
	Status         string    `json:"status"`
	Timezone       string    `json:"timezone"`
	Currency       string    `json:"currency"`
	Sector         string    `json:"sector"`
	AccessStartsAt string    `json:"access_starts_at"`
	AccessEndsAt   *string   `json:"access_ends_at"`
	UpdatedAt      string    `json:"updated_at"`
}

func tenantOrganization(org orgmodel.Organization) TenantOrganization {
	out := TenantOrganization{
		UUID: org.UUID, Slug: org.Slug, Name: org.Name, LogoURL: org.LogoURL, Status: org.Status,
		Timezone: org.Timezone, Currency: org.Currency, Sector: org.Sector,
		AccessStartsAt: org.AccessStartsAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:      org.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if org.AccessEndsAt != nil {
		v := org.AccessEndsAt.Format("2006-01-02T15:04:05Z07:00")
		out.AccessEndsAt = &v
	}
	return out
}

func copyBody(w io.Writer, r io.Reader) (int64, error) {
	return io.Copy(w, r)
}
