package model

import (
	"time"

	"github.com/google/uuid"
)

// --- tenant member management (/v1/tenant/members, /v1/tenant/roles) -----

// TenantRoleRef is a role held by a member.
type TenantRoleRef struct {
	UUID       uuid.UUID `json:"uuid"`
	Key        string    `json:"key"`
	Name       string    `json:"name"`
	IsTemplate bool      `json:"is_template"`
}

// BranchRef is a branch granted to a member with branch_scope 'selected'.
type BranchRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// TenantMemberUser is the public user part of a tenant member.
type TenantMemberUser struct {
	UUID    uuid.UUID `json:"uuid"`
	Email   string    `json:"email"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Status  string    `json:"status"`
}

// TenantMember is a member as the tenant shell sees it.
type TenantMember struct {
	ID          int64            `json:"-"`
	User        TenantMemberUser `json:"user"`
	Roles       []TenantRoleRef  `json:"roles"`
	BranchScope string           `json:"branch_scope"`
	Branches    []BranchRef      `json:"branches"`
	BranchIDs   []int64          `json:"-"`
	LastLoginAt *time.Time       `json:"last_login_at"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// TenantMemberListFilter filters GET /v1/tenant/members.
type TenantMemberListFilter struct {
	Limit       int32
	Offset      int32
	Q           string
	BranchScope string
	RoleUUIDs   []uuid.UUID
	SortField   string
	SortDesc    bool
}

// PatchTenantMemberInput changes roles and/or branch access. Nil fields are
// left unchanged.
type PatchTenantMemberInput struct {
	RoleUUIDs   *[]uuid.UUID `json:"role_uuids"`
	BranchScope *string      `json:"branch_scope"`
	BranchUUIDs *[]uuid.UUID `json:"branch_uuids"`
}

// TenantRole is a template or organization-specific tenant role.
type TenantRole struct {
	ID          int64     `json:"-"`
	UUID        uuid.UUID `json:"uuid"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsTemplate  bool      `json:"is_template"`
	MemberCount int64     `json:"member_count"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TenantRoleInput creates or patches an organization-specific role.
type TenantRoleInput struct {
	Key         *string   `json:"key"`
	Name        *string   `json:"name"`
	Description *string   `json:"description"`
	Permissions *[]string `json:"permissions"`
}

// TenantPermission is an assignable tenant permission.
type TenantPermission struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}
