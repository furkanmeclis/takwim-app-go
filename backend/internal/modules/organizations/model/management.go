package model

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Access states of an organization (list facet "access").
const (
	AccessOpen     = "open"
	AccessUpcoming = "upcoming"
	AccessExpired  = "expired"

	WindowUnlimited = "unlimited"
	WindowLimited   = "limited"
)

// Invitation states.
const (
	InvitationPending  = "pending"
	InvitationExpired  = "expired"
	InvitationAccepted = "accepted"
	InvitationRevoked  = "revoked"
)

// OrganizationFilterFromValues parses the platform list filters status /
// access / window (comma separated multi values) from a query lookup. The
// list handler, export and bulk "all matching" share it. On error it names
// the offending field.
func OrganizationFilterFromValues(get func(string) string) (ListFilter, string, error) {
	var f ListFilter
	var err error
	if f.Statuses, err = EnumList(get("status"), StatusActive, StatusSuspended); err != nil {
		return f, "status", err
	}
	if f.AccessStates, err = EnumList(get("access"), AccessOpen, AccessUpcoming, AccessExpired); err != nil {
		return f, "access", err
	}
	if f.Windows, err = EnumList(get("window"), WindowUnlimited, WindowLimited); err != nil {
		return f, "window", err
	}
	f.Q = strings.TrimSpace(get("q"))
	return f, "", nil
}

// EnumList splits a comma separated value and checks every part.
func EnumList(raw string, allowed ...string) ([]string, error) {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !slices.Contains(allowed, part) {
			return nil, errors.New("must be one of " + strings.Join(allowed, ", "))
		}
		out = append(out, part)
	}
	return out, nil
}

// OwnerRef is the longest-standing owner of an organization.
type OwnerRef struct {
	UUID    uuid.UUID `json:"uuid"`
	Email   string    `json:"email"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
}

// OrganizationSummary is one row of the platform organizations list.
type OrganizationSummary struct {
	Organization
	Owner                  *OwnerRef `json:"owner"`
	AccessState            string    `json:"access_state"`
	MemberCount            int64     `json:"member_count"`
	BranchCount            int64     `json:"branch_count"`
	PendingInvitationCount int64     `json:"pending_invitation_count"`
}

// UserRef is a person shown next to a record (inviter).
type UserRef struct {
	UUID    uuid.UUID `json:"uuid"`
	Email   string    `json:"email"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
}

// Invitation is a staff invitation as the organization sees it. The token
// is never part of it: it only travels in the invitation e-mail.
type Invitation struct {
	ID          int64           `json:"-"`
	UUID        uuid.UUID       `json:"uuid"`
	Email       string          `json:"email"`
	Status      string          `json:"status"`
	Roles       []TenantRoleRef `json:"roles"`
	BranchScope string          `json:"branch_scope"`
	Branches    []BranchRef     `json:"branches"`
	InvitedBy   *UserRef        `json:"invited_by"`
	ExpiresAt   time.Time       `json:"expires_at"`
	AcceptedAt  *time.Time      `json:"accepted_at"`
	RevokedAt   *time.Time      `json:"revoked_at"`
	LastSentAt  time.Time       `json:"last_sent_at"`
	SendCount   int32           `json:"send_count"`
	CreatedAt   time.Time       `json:"created_at"`
}

// InvitationListFilter filters an organization's invitations.
type InvitationListFilter struct {
	Limit     int32
	Offset    int32
	Q         string
	Statuses  []string
	SortField string
	SortDesc  bool
}

// CreateInvitationInput is the body of POST /v1/tenant/invitations.
type CreateInvitationInput struct {
	Email       string      `json:"email"`
	RoleUUIDs   []uuid.UUID `json:"role_uuids"`
	BranchScope string      `json:"branch_scope"`
	BranchUUIDs []uuid.UUID `json:"branch_uuids"`
}

// InvitationPreviewOrganization is the public part of the inviting
// organization.
type InvitationPreviewOrganization struct {
	UUID    uuid.UUID `json:"uuid"`
	Slug    string    `json:"slug"`
	Name    string    `json:"name"`
	LogoURL *string   `json:"logo_url"`
}

// InvitationPreview is the public answer of GET /v1/invitations/{token}.
type InvitationPreview struct {
	Organization InvitationPreviewOrganization `json:"organization"`
	InviterName  string                        `json:"inviter_name"`
	EmailMasked  string                        `json:"email_masked"`
	Status       string                        `json:"status"`
	ExpiresAt    time.Time                     `json:"expires_at"`
}

// AcceptInvitationResult tells the client where to go after accepting.
type AcceptInvitationResult struct {
	OrganizationUUID uuid.UUID `json:"organization_uuid"`
	OrganizationSlug string    `json:"organization_slug"`
	AlreadyMember    bool      `json:"already_member"`
	Email            string    `json:"email"`
}

// RegisterInvitedInput creates the account of an invitee (the e-mail comes
// from the invitation).
type RegisterInvitedInput struct {
	Name     string `json:"name"`
	Surname  string `json:"surname"`
	Password string `json:"password"`
}

// PlatformMemberInput adds a member or changes one from the platform. Role
// UUIDs win over the legacy single role key.
type PlatformMemberInput struct {
	UserUUID    uuid.UUID    `json:"user_uuid"`
	RoleKey     string       `json:"role_key"`
	RoleUUIDs   *[]uuid.UUID `json:"role_uuids"`
	BranchScope *string      `json:"branch_scope"`
	BranchUUIDs *[]uuid.UUID `json:"branch_uuids"`
}

// BranchOption is a branch offered in member / invitation forms.
type BranchOption struct {
	UUID     uuid.UUID `json:"uuid"`
	Code     string    `json:"code"`
	Name     string    `json:"name"`
	IsActive bool      `json:"is_active"`
}

// ActivityEntry is one audit row of an organization.
type ActivityEntry struct {
	UUID         uuid.UUID      `json:"uuid"`
	Action       string         `json:"action"`
	Resource     string         `json:"resource"`
	ResourceUUID *uuid.UUID     `json:"resource_uuid"`
	Payload      map[string]any `json:"payload"`
	Actor        *UserRef       `json:"actor"`
	CreatedAt    time.Time      `json:"created_at"`
}
