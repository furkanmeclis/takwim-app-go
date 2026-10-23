// Package model holds the shapes of the reservation record (TEC-899).
package model

import (
	"time"

	"github.com/google/uuid"

	availability "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Lifecycle of the visit (requirements §8).
const (
	StatusRequested = "requested"
	StatusActive    = "active"
	StatusInService = "in_service"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
	StatusRejected  = "rejected"
	StatusNoShow    = "no_show"
)

// The business's decision and how it was made.
const (
	ApprovalPending  = "pending"
	ApprovalAccepted = "accepted"
	ApprovalRejected = "rejected"

	ApprovalModeAuto  = "auto"
	ApprovalModeStaff = "staff"
	ApprovalModePanel = "panel"
)

// Customer confirmation ("Geleceğim"); the confirmation module (TEC-90x)
// moves it, every other booking stays not_required.
const (
	ConfirmationNotRequired = "not_required"
	ConfirmationScheduled   = "scheduled"
	ConfirmationPending     = "pending"
	ConfirmationConfirmed   = "confirmed"
	ConfirmationDeclined    = "declined"
	ConfirmationExpired     = "expired"
)

// Payment of the booking.
const (
	PaymentOnSite            = "on_site"
	PaymentOnlinePending     = "online_pending"
	PaymentPaid              = "paid"
	PaymentRefundPending     = "refund_pending"
	PaymentPartiallyRefunded = "partially_refunded"
	PaymentRefunded          = "refunded"
	PaymentFailed            = "failed"
)

// Payment methods a booking asks for.
const (
	MethodOnSite = "on_site"
	MethodOnline = "online"
)

// Where the booking came from.
const (
	SourcePanel  = "panel"
	SourceOnline = "online"
	SourceEmbed  = "embed"
)

// Cancel reasons set by the system (people give their own).
const (
	ReasonApprovalTimeout = "approval_timeout"
	ReasonPaymentTimeout  = "payment_timeout"
)

// CreateInput books a visit. Lines are the services in the order the
// customer is served (availability.LineInput: service and, optionally, a
// staff member). Payment is on_site or online; empty picks on_site when the
// business and every service allow it, else online.
type CreateInput struct {
	BranchUUID   uuid.UUID                `json:"branch_uuid"`
	CustomerUUID uuid.UUID                `json:"customer_uuid"`
	Lines        []availability.LineInput `json:"lines"`
	StartsAt     *time.Time               `json:"starts_at"`
	Source       string                   `json:"source"`
	Payment      string                   `json:"payment"`
	CustomFields map[string]any           `json:"custom_fields"`
	CustomerNote string                   `json:"customer_note"`
	InternalNote string                   `json:"internal_note"`
}

// Ref names a related record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// Line is one service of the visit as it was booked.
type Line struct {
	Index               int                        `json:"index"`
	ServiceUUID         *uuid.UUID                 `json:"service_uuid"`
	ServiceName         string                     `json:"service_name"`
	Staff               *LineStaff                 `json:"staff"`
	Resources           []availability.ResourceRef `json:"resources"`
	StartsAt            time.Time                  `json:"starts_at"`
	EndsAt              time.Time                  `json:"ends_at"`
	DurationMinutes     int                        `json:"duration_minutes"`
	BufferBeforeMinutes int                        `json:"buffer_before_minutes"`
	BufferAfterMinutes  int                        `json:"buffer_after_minutes"`
	UnitPrice           money.Money                `json:"unit_price"`
}

// LineStaff is the staff member of a line; UUID is nil when the record was
// deleted (the name stays).
type LineStaff struct {
	UUID *uuid.UUID `json:"uuid"`
	Name string     `json:"name"`
}

// Event is one entry of the history.
type Event struct {
	Version    int       `json:"version"`
	Type       string    `json:"type"`
	FromStatus *string   `json:"from_status"`
	ToStatus   *string   `json:"to_status"`
	Actor      *string   `json:"actor"`
	Note       string    `json:"note"`
	CreatedAt  time.Time `json:"created_at"`
}

// Reservation is the record with its lines and history.
type Reservation struct {
	UUID               uuid.UUID      `json:"uuid"`
	Number             int64          `json:"number"`
	Status             string         `json:"status"`
	ApprovalMode       string         `json:"approval_mode"`
	ApprovalStatus     string         `json:"approval_status"`
	ConfirmationStatus string         `json:"confirmation_status"`
	PaymentStatus      string         `json:"payment_status"`
	Source             string         `json:"source"`
	Branch             Ref            `json:"branch"`
	Customer           Ref            `json:"customer"`
	StartsAt           time.Time      `json:"starts_at"`
	EndsAt             time.Time      `json:"ends_at"`
	TotalPrice         money.Money    `json:"total_price"`
	Currency           string         `json:"currency"`
	CustomFields       map[string]any `json:"custom_fields"`
	CustomerNote       string         `json:"customer_note"`
	InternalNote       string         `json:"internal_note"`
	Version            int            `json:"version"`
	CancelReason       *string        `json:"cancel_reason"`
	// HoldExpiresAt is when a waiting request lets its capacity go.
	HoldExpiresAt *time.Time `json:"hold_expires_at"`
	Lines         []Line     `json:"lines"`
	Events        []Event    `json:"events"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}
