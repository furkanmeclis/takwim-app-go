// Package model holds the API shapes of the customer notifications
// (TEC-911).
package model

import (
	"time"

	"github.com/google/uuid"
)

// ChannelInfo is a channel a rule may list.
type ChannelInfo struct {
	Name string `json:"name"`
	// Ready: a provider can deliver now. A rule may list a channel that is
	// not ready; its sends are then logged as skipped.
	Ready bool `json:"ready"`
}

// Rule is an event's rule.
type Rule struct {
	Enabled         bool     `json:"enabled"`
	Channels        []string `json:"channels"`
	Fallback        bool     `json:"fallback"`
	ReminderOffsets []int32  `json:"reminder_offsets"`
}

// Template is the text of an event on a channel.
type Template struct {
	Channel        string `json:"channel"`
	Subject        string `json:"subject"`
	Body           string `json:"body"`
	Custom         bool   `json:"custom"`
	DefaultSubject string `json:"default_subject"`
	DefaultBody    string `json:"default_body"`
}

// Event is a catalog event with the business's rule and texts.
type Event struct {
	Code        string     `json:"code"`
	Reminder    bool       `json:"reminder"`
	Variables   []string   `json:"variables"`
	Rule        Rule       `json:"rule"`
	DefaultRule Rule       `json:"default_rule"`
	Custom      bool       `json:"custom"`
	Templates   []Template `json:"templates"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

// Settings is the notification settings page.
type Settings struct {
	Channels []ChannelInfo `json:"channels"`
	Events   []Event       `json:"events"`
}

// TemplateInput is a business's text; an empty body (or the default text)
// goes back to the default.
type TemplateInput struct {
	Channel string `json:"channel"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// SaveInput replaces an event's rule and texts.
type SaveInput struct {
	Event           string          `json:"event"`
	Enabled         bool            `json:"enabled"`
	Channels        []string        `json:"channels"`
	Fallback        bool            `json:"fallback"`
	ReminderOffsets []int32         `json:"reminder_offsets"`
	Templates       []TemplateInput `json:"templates"`
}

// ResetInput names the event to set back to the defaults.
type ResetInput struct {
	Event string `json:"event"`
}

// PreviewInput renders a text with sample values.
type PreviewInput struct {
	Event   string `json:"event"`
	Channel string `json:"channel"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// Preview is a rendered text.
type Preview struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
	// Unknown lists {{placeholders}} that are not variables of the event.
	Unknown []string `json:"unknown"`
}

// Delivery is one row of the send history (one channel attempt).
type Delivery struct {
	UUID              uuid.UUID  `json:"uuid"`
	DispatchUUID      uuid.UUID  `json:"dispatch_uuid"`
	Event             string     `json:"event"`
	Channel           string     `json:"channel"`
	AttemptNo         int32      `json:"attempt_no"`
	Status            string     `json:"status"`
	DispatchStatus    string     `json:"dispatch_status"`
	Provider          string     `json:"provider"`
	ProviderRef       string     `json:"provider_ref"`
	RecipientMasked   string     `json:"recipient_masked"`
	CustomerUUID      uuid.UUID  `json:"customer_uuid"`
	CustomerName      string     `json:"customer_name"`
	ReservationUUID   *uuid.UUID `json:"reservation_uuid,omitempty"`
	ReservationNumber *int64     `json:"reservation_number,omitempty"`
	Subject           string     `json:"subject"`
	Body              string     `json:"body"`
	Error             string     `json:"error"`
	SentAt            *time.Time `json:"sent_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// DeliveryFilter filters the history.
type DeliveryFilter struct {
	Channels        []string
	Statuses        []string
	Events          []string
	CustomerUUID    *uuid.UUID
	ReservationUUID *uuid.UUID
	CreatedFrom     *time.Time
	CreatedTo       *time.Time
	Q               string
	SortField       string
	SortDesc        bool
	Limit           int32
	Offset          int32
}
