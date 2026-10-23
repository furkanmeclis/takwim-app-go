package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	StatusActive    = "active"
	StatusSuspended = "suspended"

	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

type Organization struct {
	ID             int64           `json:"-"`
	UUID           uuid.UUID       `json:"uuid"`
	Slug           string          `json:"slug"`
	Name           string          `json:"name"`
	LogoURL        *string         `json:"logo_url,omitempty"`
	Status         string          `json:"status"`
	Timezone       string          `json:"timezone"`
	Currency       string          `json:"currency"`
	AccessStartsAt time.Time       `json:"access_starts_at"`
	AccessEndsAt   *time.Time      `json:"access_ends_at,omitempty"`
	Settings       json.RawMessage `json:"settings,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type User struct {
	ID      int64
	UUID    uuid.UUID `json:"uuid"`
	Email   string    `json:"email"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Status  string    `json:"status"`
}

type RoleSummary struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type Member struct {
	ID        int64       `json:"-"`
	User      User        `json:"user"`
	Role      RoleSummary `json:"role"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type Membership struct {
	Organization Organization `json:"organization"`
	Role         RoleSummary  `json:"role"`
	Permissions  []string     `json:"permissions"`
}

type CreateOrganizationInput struct {
	Name string
	Slug string
	// OwnerUserUUID selects the owner; when zero, OwnerUserID is used.
	OwnerUserUUID uuid.UUID
	OwnerUserID   int64
	// NoOwner creates the organization without a member (the owner joins
	// later through an invitation).
	NoOwner bool
}

type PatchOrganizationInput struct {
	Name     *string
	Slug     *string
	Settings []byte
	// Timezone is an IANA zone name ("Europe/Istanbul").
	Timezone *string
	// Currency is an ISO 4217 code ("TRY").
	Currency *string
}

type ListFilter struct {
	Limit        int32
	Offset       int32
	Q            string
	Statuses     []string
	AccessStates []string
	Windows      []string
	SortField    string
	SortDesc     bool
}

type MemberListFilter struct {
	Limit     int32
	Offset    int32
	Q         string
	SortField string
	SortDesc  bool
}
