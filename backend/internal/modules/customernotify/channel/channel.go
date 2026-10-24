// Package channel is the contract between the customer notification engine
// (TEC-911) and the adapters that deliver its messages: SMS, e-mail and
// WhatsApp (TEC-922, the business's own Meta account) and automatic voice
// calls (TEC-923, the business's own voice provider account).
//
// A new channel is one type implementing Channel, registered on the engine
// with Engine.Register. The engine never names a provider: rules list
// channel names (Known); a channel without an adapter, or whose provider is
// not ready, is logged as skipped (never as sent) and the rule's next
// channel runs.
//
// Channels whose result arrives later (a delivery report) answer
// OutcomeAccepted with ProviderRef (or ExternalUUID) set and report the
// final result through the Reporter handed to them by SetReporter
// (ReportingChannel). A report that never comes fails the attempt after the
// engine's receipt timeout, so the next channel runs. A delivery or read
// report, or a picked up call, is never an answer to the message.
package channel

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Channel names (rules, templates, deliveries).
const (
	SMS      = "sms"
	WhatsApp = "whatsapp"
	Email    = "email"
	// VoiceCall is the automatic call with keys (TEC-923); the name is also
	// its integration kind and usage metric.
	VoiceCall = "voice_call"
)

// Known lists every channel the engine sends on, in display order. A known
// channel may have no adapter in a process (it is then skipped).
var Known = []string{SMS, WhatsApp, Email, VoiceCall}

// Messages are the channels of the notification settings page: a voice
// call carries only the confirmation request, whose channels come from the
// confirmation settings.
var Messages = []string{SMS, WhatsApp, Email}

// IsMessage reports whether name is one of Messages.
func IsMessage(name string) bool {
	for _, k := range Messages {
		if k == name {
			return true
		}
	}
	return false
}

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
	// Vars are the dispatch variables the text was rendered with (a
	// provider template fills its own parameters from them).
	Vars map[string]string
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
	// Read: the customer opened the message (implies Delivered). It is
	// recorded as a receipt time only, never as an answer.
	Read bool
	// Unanswered: the message reached the customer (a call was picked up,
	// implies Delivered) but brought no answer the channel could take (no
	// key or an invalid one). The receipt is recorded and the attempt counts
	// as delivered, yet the rule's next channel still runs.
	Unanswered  bool
	ProviderRef string
	Error       string
	// At is the provider's time of the report (zero = now).
	At time.Time
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

// OrganizationChannel is a channel that sends through the business's own
// provider account (WhatsApp): Ready says the provider is available in this
// process, ReadyFor whether the business has a working account.
type OrganizationChannel interface {
	Channel
	ReadyFor(ctx context.Context, orgID int64) bool
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
