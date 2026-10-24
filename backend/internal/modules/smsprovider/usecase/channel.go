package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm"
)

// Channel is the customer notification engine's SMS channel
// (channel.ReportingChannel and channel.OrganizationChannel). With the
// provider enabled it sends every notice through the business's own SMS
// account and reports delivery later; without it the platform sender it
// was given stands in (the development log sender, never recorded as
// sent).
type Channel struct {
	svc      *Service
	fallback channel.Channel
}

// NewChannel builds the channel on the service. fallback may be nil.
func NewChannel(svc *Service, fallback channel.Channel) *Channel {
	if fallback != nil && fallback.Name() != channel.SMS {
		fallback = nil
	}
	return &Channel{svc: svc, fallback: fallback}
}

// Name implements channel.Channel.
func (*Channel) Name() string { return channel.SMS }

// Ready implements channel.Channel: the provider is enabled here, or the
// fallback sender really delivers.
func (c *Channel) Ready() bool {
	if c.svc.enabled() {
		return true
	}
	return c.fallback != nil && c.fallback.Ready()
}

// SetReporter implements channel.ReportingChannel: the engine that
// registers the channel records the polled delivery reports.
func (c *Channel) SetReporter(r channel.Reporter) {
	if rec, ok := r.(Recorder); ok {
		c.svc.reports = rec
	}
}

// account is the business's active SMS account on the provider.
func (c *Channel) account(ctx context.Context, orgID int64) (integrations.Account, string, error) {
	acc, err := c.svc.accounts.ActiveAccount(ctx, orgID, integrations.KindSMS)
	if errors.Is(err, accountsusecase.ErrNoActiveAccount) {
		return acc, "SMS account not connected", nil
	}
	if err != nil {
		return acc, "", err
	}
	if acc.Provider != netgsm.ProviderCode {
		return acc, "provider not configured (logged only)", nil
	}
	return acc, "", nil
}

// ReadyFor implements channel.OrganizationChannel.
func (c *Channel) ReadyFor(ctx context.Context, orgID int64) bool {
	if !c.svc.enabled() {
		return c.Ready()
	}
	_, reason, err := c.account(ctx, orgID)
	return err == nil && reason == ""
}

// Send implements channel.Channel: hands the text to the provider; its
// delivery report comes with the poll.
func (c *Channel) Send(ctx context.Context, m channel.Message) (channel.Result, error) {
	if !c.svc.enabled() {
		if c.fallback == nil {
			return channel.Skipped("", "provider not configured"), nil
		}
		return c.fallback.Send(ctx, m)
	}
	acc, reason, err := c.account(ctx, m.OrganizationID)
	if err != nil {
		return channel.Result{}, err
	}
	if reason != "" {
		if c.fallback != nil && acc.Provider != "" {
			// A development (log) account: the text still shows in the log.
			_, _ = c.fallback.Send(ctx, m)
		}
		return channel.Skipped(acc.Provider, reason), nil
	}
	phone := strings.TrimSpace(m.Recipient.Phone)
	if phone == "" {
		return channel.Skipped(netgsm.ProviderCode, "no phone number"), nil
	}
	if netgsm.Recipient(phone) == "" {
		return channel.Skipped(netgsm.ProviderCode, "no Turkish phone number"), nil
	}
	if strings.TrimSpace(m.Body) == "" {
		return channel.Skipped(netgsm.ProviderCode, "no text to send"), nil
	}
	res := channel.Result{Provider: netgsm.ProviderCode, RecipientMasked: sms.Mask(phone)}
	id, err := c.svc.client.Send(ctx, acc, netgsm.Message{To: phone, Text: m.Body})
	if err != nil {
		res.Outcome, res.Error = channel.OutcomeFailed, err.Error()
		return res, nil
	}
	// Taken by the provider: delivery comes with the report poll.
	res.Outcome, res.ProviderRef = channel.OutcomeAccepted, id
	return res, nil
}

var (
	_ channel.ReportingChannel    = (*Channel)(nil)
	_ channel.OrganizationChannel = (*Channel)(nil)
)
