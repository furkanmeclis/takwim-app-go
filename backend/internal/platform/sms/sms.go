// Package sms is the text message port of Takwim. The SMS provider is not
// chosen yet (TEC-884): callers depend on Sender only, and a provider plugs
// in behind it without touching them. LogSender is the development sender.
package sms

import (
	"context"
	"errors"
	"log/slog"
	"strings"
)

// Message purposes (log fields, provider templates later).
const (
	PurposeCustomerOTP       = "customer_otp"
	PurposeCustomerPanelLink = "customer_panel_link"
)

// ErrUnavailable: no provider can deliver messages (callers answer 503 or
// skip the notice).
var ErrUnavailable = errors.New("sms: no provider is configured")

// Message is one text to an E.164 phone.
type Message struct {
	To      string
	Text    string
	Purpose string
}

// Sender delivers a text message.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// LogSender writes messages to the log instead of sending them. The text
// (it may hold a sign-in code) is logged only when RevealText is set, which
// the server does outside production; the phone is always masked.
type LogSender struct {
	log        *slog.Logger
	revealText bool
}

// NewLogSender builds the development sender. log may be nil.
func NewLogSender(log *slog.Logger, revealText bool) *LogSender {
	if log == nil {
		log = slog.Default()
	}
	return &LogSender{log: log, revealText: revealText}
}

// Send logs msg.
func (s *LogSender) Send(ctx context.Context, msg Message) error {
	attrs := []any{"purpose", msg.Purpose, "to", Mask(msg.To)}
	if s.revealText {
		attrs = append(attrs, "text", msg.Text)
	}
	s.log.InfoContext(ctx, "sms_log_sender", attrs...)
	return nil
}

// Mask hides the middle of a phone number for logs and answers:
// "+905321234567" → "+90532*****67".
func Mask(phone string) string {
	phone = strings.TrimSpace(phone)
	if len(phone) <= 6 {
		return strings.Repeat("*", len(phone))
	}
	keepStart, keepEnd := 6, 2
	if len(phone) < keepStart+keepEnd+2 {
		keepStart = 3
	}
	return phone[:keepStart] + strings.Repeat("*", len(phone)-keepStart-keepEnd) + phone[len(phone)-keepEnd:]
}
