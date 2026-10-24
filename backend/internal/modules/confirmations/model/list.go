package model

import (
	"time"

	"github.com/google/uuid"
)

// SourcePhone: a member of the business recorded the customer's answer
// given on the phone (TEC-924, "telefonla teyit edildi").
const SourcePhone = "phone"

// List states (TEC-924): the task status, with change_requested for an
// open task the customer asked to move, and an expired task split by its
// outcome.
const (
	StateScheduled       = TaskScheduled
	StatePending         = TaskPending
	StateConfirmed       = TaskConfirmed
	StateDeclined        = TaskDeclined
	StateChangeRequested = "change_requested"
	StateFlagged         = OutcomeFlagged
	StateUnreachable     = OutcomeUnreachable
	StateCancelled       = OutcomeCancelled
	StateDropped         = TaskDropped
)

// States lists the list states in the order a desk handles them.
var States = []string{
	StateChangeRequested, StateUnreachable, StateFlagged, StatePending, StateScheduled,
	StateConfirmed, StateDeclined, StateCancelled, StateDropped,
}

// AttentionStates need a person while the visit is ahead.
var AttentionStates = []string{StateChangeRequested, StateUnreachable, StateFlagged}

// ListSlug is the table slug of the confirmation list.
const ListSlug = "tenant.confirmations"

// ListFilter narrows the confirmation list.
type ListFilter struct {
	Q        string
	Branches []uuid.UUID
	// States: empty = every state but dropped.
	States []string
	// Attention: only (true) or never (false) the bookings that need a
	// person; nil = both.
	Attention     *bool
	StartsFrom    *time.Time
	StartsTo      *time.Time
	SortField     string
	SortDesc      bool
	Limit, Offset int32
}

// Customer is the customer of a list row (the phone for the call link).
type Customer struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Phone *string   `json:"phone"`
}

// LastSend is the latest request of the current round.
type LastSend struct {
	// Status of the notice: pending, sending, delivered, failed, skipped,
	// cancelled.
	Status string    `json:"status"`
	Error  string    `json:"error"`
	At     time.Time `json:"at"`
}

// Item is one booking of the confirmation list: the visit, the business's
// approval and the customer's confirmation side by side, the requests, the
// answer and what needs a person.
type Item struct {
	UUID               uuid.UUID `json:"uuid"`
	Number             int64     `json:"number"`
	Status             string    `json:"status"`
	ApprovalStatus     string    `json:"approval_status"`
	ConfirmationStatus string    `json:"confirmation_status"`
	Version            int       `json:"version"`
	Branch             Ref       `json:"branch"`
	Customer           Customer  `json:"customer"`
	Services           string    `json:"services"`
	Staff              string    `json:"staff"`
	StartsAt           time.Time `json:"starts_at"`
	EndsAt             time.Time `json:"ends_at"`
	// State is the list state (States).
	State string `json:"state"`
	// TaskStatus is the task's own status (scheduled … dropped).
	TaskStatus        string     `json:"task_status"`
	Channels          []string   `json:"channels"`
	OnNoAnswer        string     `json:"on_no_answer"`
	AttemptsSent      int        `json:"attempts_sent"`
	AttemptsPlanned   int        `json:"attempts_planned"`
	NextAttemptAt     *time.Time `json:"next_attempt_at"`
	DeadlineAt        time.Time  `json:"deadline_at"`
	Answer            *string    `json:"answer"`
	AnswerChannel     *string    `json:"answer_channel"`
	AnsweredAt        *time.Time `json:"answered_at"`
	ChangeRequestedAt *time.Time `json:"change_requested_at"`
	ChangeNote        string     `json:"change_note"`
	Outcome           *string    `json:"outcome"`
	LastError         string     `json:"last_error"`
	// FailedSends counts the requests of the current round no channel
	// could deliver; LastSend is the latest one (nil: none yet).
	FailedSends int       `json:"failed_sends"`
	SendFailed  bool      `json:"send_failed"`
	LastSend    *LastSend `json:"last_send"`
	// Attention: the visit is ahead and a person has to act.
	Attention bool `json:"attention"`
	// CanConfirm: the booking takes a "confirmed by phone" now.
	CanConfirm bool      `json:"can_confirm"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Facet is one state with its count.
type Facet struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// Facets are the state counts of the list (the other filters applied)
// and how many of the rows need a person.
type Facets struct {
	State     []Facet `json:"state"`
	Attention int64   `json:"attention"`
}

// Summary is the follow-up alert of the panel: upcoming visits whose
// confirmation needs a person, by reason (the nav badge and the alert).
type Summary struct {
	// Attention is the number of bookings that need a person.
	Attention       int64 `json:"attention"`
	ChangeRequested int64 `json:"change_requested"`
	Unreachable     int64 `json:"unreachable"`
	Flagged         int64 `json:"flagged"`
	SendFailed      int64 `json:"send_failed"`
	// Waiting: upcoming visits still waiting for an answer.
	Waiting int64 `json:"waiting"`
}

// Attempt is one channel tried for one request of a booking's
// confirmation (or a request no channel took yet: Channel empty).
type Attempt struct {
	// Round and Request name the request ("r2:a1": second planning, first
	// request).
	Round   int `json:"round"`
	Request int `json:"request"`
	// Call numbers a call made again after a missed one (TEC-923); 0 = the
	// request itself.
	Call   int    `json:"call"`
	Status string `json:"status"`
	// DispatchStatus is the request's own status (pending, sending,
	// delivered, failed, skipped, cancelled).
	DispatchStatus string    `json:"dispatch_status"`
	Channel        *string   `json:"channel"`
	Try            int       `json:"try"`
	Provider       string    `json:"provider"`
	Recipient      string    `json:"recipient"`
	Error          string    `json:"error"`
	ScheduledAt    time.Time `json:"scheduled_at"`
	// SentAt: the provider accepted the message; DeliveredAt / ReadAt come
	// from its delivery reports (WhatsApp). None of them is an answer.
	SentAt          *time.Time `json:"sent_at"`
	DeliveredAt     *time.Time `json:"delivered_at"`
	ReadAt          *time.Time `json:"read_at"`
	FinishedAt      *time.Time `json:"finished_at"`
	CreatedAt       time.Time  `json:"created_at"`
	DeliveryUUID    *uuid.UUID `json:"delivery_uuid"`
	DispatchUUID    uuid.UUID  `json:"dispatch_uuid"`
	DispatchMessage string     `json:"dispatch_error"`
	// Responses are the customer's replies to this message on its channel
	// (a WhatsApp button, a key of the call), with what was done with each.
	Responses []Response `json:"responses"`
}

// Response is one reply on a channel (TEC-922): the answer it asked for
// (confirm, cancel, change, unknown) and the result (applied, rejected,
// ignored) with the reason of one that was not applied.
type Response struct {
	UUID        uuid.UUID `json:"uuid"`
	Channel     string    `json:"channel"`
	Answer      string    `json:"answer"`
	Result      string    `json:"result"`
	Reason      string    `json:"reason"`
	RespondedAt time.Time `json:"responded_at"`
}

// ConfirmInput records a confirmation the customer gave on the phone.
type ConfirmInput struct {
	Note            string `json:"note"`
	ExpectedVersion *int   `json:"expected_version"`
}
