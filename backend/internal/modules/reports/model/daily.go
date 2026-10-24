package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// DailyAccess is what the caller may see on the daily summary (TEC-926);
// the handler fills it from the tenant permissions and the modules.
type DailyAccess struct {
	// OwnReservations: tenant.reservations.read or .own (the caller's own
	// visits as a staff member).
	OwnReservations bool
	// BusinessReservations: tenant.reservations.read (every visit of the
	// branch scope and the pending approvals).
	BusinessReservations bool
	// Money: tenant.finance.read with the reports module (revenue,
	// collections, staff shares, the business share, the branch rows).
	Money bool
	// Earnings: tenant.earnings.read or .own with the staff shares module
	// (the caller's own sales and share).
	Earnings bool
}

// StaffRef names the staff record of the caller.
type StaffRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// DailyVisit is one line the caller serves on the day.
type DailyVisit struct {
	ReservationUUID    uuid.UUID `json:"reservation_uuid"`
	Number             int64     `json:"number"`
	LineIndex          int32     `json:"line_index"`
	ServiceName        string    `json:"service_name"`
	StartsAt           time.Time `json:"starts_at"`
	EndsAt             time.Time `json:"ends_at"`
	Status             string    `json:"status"`
	ApprovalStatus     string    `json:"approval_status"`
	ConfirmationStatus string    `json:"confirmation_status"`
	PaymentStatus      string    `json:"payment_status"`
	BranchUUID         uuid.UUID `json:"branch_uuid"`
	BranchName         string    `json:"branch_name"`
	CustomerName       string    `json:"customer_name"`
}

// DailyEarning is the caller's own money of the day in one currency:
// the net of the sale lines it served and its share of them.
type DailyEarning struct {
	Currency  string      `json:"currency"`
	Sales     money.Money `json:"sales"`
	Share     money.Money `json:"share"`
	SaleCount int64       `json:"sale_count"`
}

// DailyOwn is the caller's own day as a staff member.
type DailyOwn struct {
	Reservations Reservations `json:"reservations"`
	Visits       []DailyVisit `json:"visits"`
	// Earnings is nil when the caller may not see its earnings (or the
	// staff shares module is off); else one entry per currency, at least
	// the business currency.
	Earnings []DailyEarning `json:"earnings"`
}

// DailyMoney is the money of the day in one currency.
type DailyMoney struct {
	Currency      string      `json:"currency"`
	Revenue       money.Money `json:"revenue"`
	Collections   money.Money `json:"collections"`
	StaffShare    money.Money `json:"staff_share"`
	BusinessShare money.Money `json:"business_share"`
	SaleCount     int64       `json:"sale_count"`
}

// DailyBranch is one branch row of the day (Branch nil = the
// business-wide records).
type DailyBranch struct {
	Branch *BranchRef `json:"branch"`
	// Reservations is nil without the business reservations access.
	Reservations *Reservations `json:"reservations"`
	Money        []DailyMoney  `json:"money"`
}

// DailyBusiness is the day of the whole branch scope.
type DailyBusiness struct {
	// Reservations and Confirmations are nil without the business
	// reservations access.
	Reservations  *Reservations  `json:"reservations"`
	Confirmations *Confirmations `json:"confirmations"`
	// Money and Branches are nil without the money access.
	Money    []DailyMoney  `json:"money"`
	Branches []DailyBranch `json:"branches"`
}

// DailyPending is what waits for the business.
type DailyPending struct {
	// Approvals: visits waiting to be accepted or rejected (any date).
	Approvals int64 `json:"approvals"`
	// Confirmations of the day's visits still waiting for an answer
	// (planned or sent) and those that need a person (a change request,
	// no answer left to the business, unreachable).
	ConfirmationsWaiting   int64 `json:"confirmations_waiting"`
	ConfirmationsAttention int64 `json:"confirmations_attention"`
}

// Daily is the payload of GET /v1/tenant/daily-summary. Sections the
// caller may not see are null.
type Daily struct {
	Date     string         `json:"date"`
	Timezone string         `json:"timezone"`
	Currency string         `json:"currency"`
	Staff    *StaffRef      `json:"staff"`
	Own      *DailyOwn      `json:"own"`
	Business *DailyBusiness `json:"business"`
	Pending  *DailyPending  `json:"pending"`
}
