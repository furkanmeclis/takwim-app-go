// Package channel is the contract between the customer notification engine
// (TEC-911) and the adapters that deliver its messages: SMS and e-mail now,
// WhatsApp and voice calls with the confirmation module (TEC-921 / 922).
//
// A new channel is one type implementing Channel, registered on the engine
// with Engine.Register. The engine never names a provider: rules list
// channel names (Known); a channel without an adapter, or whose provider is
// not ready, is logged as skipped (never as sent) and the rule's next
// channel runs.
//
// Channels whose result arrives later (a delivery report) answer
// OutcomeAccepted with ExternalUUID set and report the final result through
// the Reporter handed to them by SetReporter (ReportingChannel).
package channel

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// Channel names (rules, templates, deliveries).
const (
	SMS      = "sms"
	WhatsApp = "whatsapp"
	Email    = "email"
)

// Known lists every channel a rule may use, in display order. A known
// channel may have no adapter yet (WhatsApp until TEC-921).
var Known = []string{SMS, WhatsApp, Email}

// IsKnown reports whether name is a known channel.
func IsKnown(name string) bool {
	for _, k := range Known {
		if k == name {
			return true
		}
	}
	return false
}

// Recipient is the customer a message goes to. Contact fields are read
// from the business's customer record at send time; they are never stored
// on the dispatch.
type Recipient struct {
	CustomerID int64
	Name       string
	Phone      string
	Email      string
}

// Message is one rendered message for one channel.
type Message struct {
	// DeliveryUUID identifies the attempt.
	DeliveryUUID uuid.UUID
	Event        string
	Recipient    Recipient
	Subject      string
	Body         string
	// BusinessName is the sender name e-mail and WhatsApp show.
	BusinessName   string
	OrganizationID int64
	ReservationID  int64
}

// Outcome is the result of one send.
type Outcome string

const (
	// OutcomeDelivered: delivered (confirmed by the provider).
	OutcomeDelivered Outcome = "delivered"
	// OutcomeSent: handed to the provider; final, no later report (SMTP).
	OutcomeSent Outcome = "sent"
	// OutcomeAccepted: queued at the provider; the final result arrives
	// through the Reporter (ExternalUUID identifies the message).
	OutcomeAccepted Outcome = "accepted"
	// OutcomeFailed: the send failed; the next channel runs (fallback).
	OutcomeFailed Outcome = "failed"
	// OutcomeSkipped: the channel cannot reach this customer (no ready
	// provider, no number or address); the next channel runs.
	OutcomeSkipped Outcome = "skipped"
)

// Result is what a channel reports for one send.
type Result struct {
	Outcome         Outcome
	Provider        string
	ProviderRef     string
	ExternalUUID    *uuid.UUID
	RecipientMasked string
	Error           string
}

// Skipped answers a skipped send with the reason.
func Skipped(provider, reason string) Result {
	return Result{Outcome: OutcomeSkipped, Provider: provider, Error: reason}
}

// Channel delivers rendered messages.
type Channel interface {
	// Name is one of Known.
	Name() string
	// Ready reports whether a provider can deliver now (shown on the
	// settings page; a channel that is not ready skips every send).
	Ready() bool
	// Send delivers one message. A returned error counts as OutcomeFailed
	// with the error text; it never stops the dispatch.
	Send(ctx context.Context, msg Message) (Result, error)
}

// Report is a channel's final delivery result for an accepted message,
// found by DeliveryUUID or else by ExternalUUID.
type Report struct {
	Channel        string
	OrganizationID int64
	DeliveryUUID   *uuid.UUID
	ExternalUUID   uuid.UUID
	Delivered      bool
	ProviderRef    string
	Error          string
}

// Reporter receives delivery reports (implemented by the engine).
type Reporter interface {
	ReportDelivery(ctx context.Context, r Report)
}

// ReportingChannel is a channel with delivery reports.
type ReportingChannel interface {
	Channel
	SetReporter(r Reporter)
}

// MaskEmail keeps the first letter of the local part and the domain.
func MaskEmail(v string) string {
	v = strings.TrimSpace(v)
	at := strings.LastIndex(v, "@")
	if at <= 0 {
		return ""
	}
	return v[:1] + "***" + v[at:]
}
