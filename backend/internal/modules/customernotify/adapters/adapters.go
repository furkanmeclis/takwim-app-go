// Package adapters connects the customer notification engine (TEC-911) to
// the platform senders: SMS through sms.Sender, e-mail through the SMTP
// mailer. WhatsApp and voice calls register their own channels with the
// confirmation module (TEC-921 / 922).
package adapters

import (
	"context"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/mail"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
)

// SMS sends notices as text messages. A sender that does not really
// deliver (the log sender until the provider is chosen, TEC-884) still
// gets the message (the development log shows it) but the attempt is
// recorded as skipped, never as sent.
type SMS struct {
	Sender sms.Sender
}

// Name implements channel.Channel.
func (SMS) Name() string { return channel.SMS }

// Ready implements channel.Channel.
func (a SMS) Ready() bool {
	_, ready := sms.Describe(a.Sender)
	return ready
}

// Send implements channel.Channel.
func (a SMS) Send(ctx context.Context, msg channel.Message) (channel.Result, error) {
	provider, ready := sms.Describe(a.Sender)
	phone := strings.TrimSpace(msg.Recipient.Phone)
	if a.Sender == nil {
		return channel.Skipped("", "provider not configured"), nil
	}
	if phone == "" {
		return channel.Skipped(provider, "no phone number"), nil
	}
	res := channel.Result{Provider: provider, RecipientMasked: sms.Mask(phone)}
	err := a.Sender.Send(ctx, sms.Message{To: phone, Text: msg.Body, Purpose: sms.PurposeCustomerNotice})
	if err != nil {
		res.Outcome, res.Error = channel.OutcomeFailed, err.Error()
		return res, nil
	}
	if !ready {
		res.Outcome, res.Error = channel.OutcomeSkipped, "provider not configured (logged only)"
		return res, nil
	}
	res.Outcome = channel.OutcomeSent
	return res, nil
}

// Email sends notices through the mailer. Without an SMTP host (or with a
// driver other than smtp) the channel is not ready and every attempt is
// skipped.
type Email struct {
	Mail mail.Sender
	// Configured: the mailer has a real server (SMTP_HOST set, driver
	// smtp).
	Configured bool
}

// Name implements channel.Channel.
func (Email) Name() string { return channel.Email }

// Ready implements channel.Channel.
func (a Email) Ready() bool { return a.Mail != nil && a.Configured }

// Send implements channel.Channel.
func (a Email) Send(ctx context.Context, msg channel.Message) (channel.Result, error) {
	if !a.Ready() {
		return channel.Skipped("", "provider not configured"), nil
	}
	to := strings.TrimSpace(msg.Recipient.Email)
	if to == "" || !strings.Contains(to, "@") {
		return channel.Skipped("smtp", "no e-mail address"), nil
	}
	res := channel.Result{Provider: "smtp", RecipientMasked: channel.MaskEmail(to)}
	if err := a.Mail.Send(ctx, mail.Message{To: []string{to}, Subject: msg.Subject, Body: msg.Body}); err != nil {
		res.Outcome, res.Error = channel.OutcomeFailed, err.Error()
		return res, nil
	}
	res.Outcome = channel.OutcomeSent
	return res, nil
}

// PanelLinker issues the single-use panel link of a phone
// (customerpanel.Service.PanelLink, TEC-910).
type PanelLinker interface {
	PanelLink(ctx context.Context, phoneE164 string) string
}

// PanelLinks is the engine's link issuer: SMS notices carry the phone's
// single-use panel link. Other channels keep the public address (the link
// opens the panel of the phone, and an e-mail address is no proof of it).
func PanelLinks(p PanelLinker) func(ctx context.Context, ch string, to channel.Recipient, orgID, reservationID int64) (string, error) {
	return func(ctx context.Context, ch string, to channel.Recipient, _, _ int64) (string, error) {
		if p == nil || ch != channel.SMS || to.Phone == "" {
			return "", nil
		}
		return p.PanelLink(ctx, to.Phone), nil
	}
}

var (
	_ channel.Channel = SMS{}
	_ channel.Channel = Email{}
)
