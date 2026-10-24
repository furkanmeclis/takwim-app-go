package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// States of a package in the panel (TEC-984), as the business computes
// them on its business day.
const (
	PackageActive   = "active"
	PackageUsedUp   = "used_up"
	PackageExpired  = "expired"
	PackageRefunded = "refunded"
)

// PackageStates lists the package states.
var PackageStates = []string{PackageActive, PackageUsedUp, PackageExpired, PackageRefunded}

// PackageFilter narrows "Paketlerim".
type PackageFilter struct {
	BusinessUUID *uuid.UUID
	States       []string
	Limit        int32
	Offset       int32
}

// PackageBusiness is the business that sold a package.
type PackageBusiness struct {
	UUID uuid.UUID `json:"uuid"`
	Slug string    `json:"slug"`
	Name string    `json:"name"`
}

// PackageRef names a branch or a service.
type PackageRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Package is one session package of the person ("Paketlerim"): what is
// left, until when and where it can be used. No cash, ledger or staff data.
type Package struct {
	UUID          uuid.UUID       `json:"uuid"`
	Business      PackageBusiness `json:"business"`
	Name          string          `json:"name"`
	ServiceName   string          `json:"service_name"`
	ServiceUUID   *uuid.UUID      `json:"service_uuid"`
	TotalSessions int             `json:"total_sessions"`
	Used          int             `json:"used"`
	// Held: sessions booked on visits not completed yet.
	Held int `json:"held"`
	// Remaining: sessions neither used nor booked (0 once refunded).
	Remaining int         `json:"remaining"`
	Price     money.Money `json:"price"`
	Currency  string      `json:"currency"`
	SoldOn    string      `json:"sold_on"`
	// ExpiresOn is the last day the package can be used (null = no end).
	ExpiresOn  *string `json:"expires_on"`
	State      string  `json:"state"`
	RefundedOn *string `json:"refunded_on"`
	// Branch sold it; Branches are where it can be used (empty with
	// AllBranches).
	Branch      PackageRef   `json:"branch"`
	AllBranches bool         `json:"all_branches"`
	Branches    []PackageRef `json:"branches"`
	CreatedAt   time.Time    `json:"created_at"`
	// Sessions: the usage history, on the detail only.
	Sessions []PackageSession `json:"sessions,omitempty"`
}

// PackageSession is one session of a package booked on or used by a visit.
type PackageSession struct {
	UUID              uuid.UUID `json:"uuid"`
	SessionNo         int       `json:"session_no"`
	Status            string    `json:"status"`
	ConsumedOn        *string   `json:"consumed_on"`
	ReservationUUID   uuid.UUID `json:"reservation_uuid"`
	ReservationNumber int64     `json:"reservation_number"`
	StartsAt          time.Time `json:"starts_at"`
	ReservationStatus string    `json:"reservation_status"`
	ServiceName       *string   `json:"service_name"`
	StaffName         *string   `json:"staff_name"`
	BranchName        string    `json:"branch_name"`
	// Timezone: the branch's zone, for showing starts_at as planned.
	Timezone string `json:"timezone"`
}
