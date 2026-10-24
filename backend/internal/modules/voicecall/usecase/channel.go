package usecase

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/voice/netgsm"
)

// RingSeconds is how long the customer's phone rings.
const RingSeconds = 30

// Channel is the customer notification engine's voice call channel
// (channel.ReportingChannel and channel.OrganizationChannel). It calls
// through the business's own voice account, and only for the confirmation
// request: any other notice is skipped, so the rule's next channel runs.
type Channel struct{ svc *Service }

// NewChannel builds the channel on the service.
func NewChannel(svc *Service) *Channel { return &Channel{svc: svc} }

// Name implements channel.Channel.
func (*Channel) Name() string { return channel.VoiceCall }

// Ready implements channel.Channel: the voice provider is enabled here.
func (c *Channel) Ready() bool { return c.svc.enabled() }

// SetReporter implements channel.ReportingChannel: the engine that
// registers the channel records the webhook's call results.
func (c *Channel) SetReporter(r channel.Reporter) {
	if rec, ok := r.(Recorder); ok {
		c.svc.reports = rec
	}
}

// account is the business's active voice account.
func (c *Channel) account(ctx context.Context, orgID int64) (integrations.Account, string, error) {
	acc, err := c.svc.accounts.ActiveAccount(ctx, orgID, integrations.KindVoiceCall)
	if errors.Is(err, accountsusecase.ErrNoActiveAccount) {
		return acc, "voice account not connected", nil
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
	if !c.Ready() {
		return false
	}
	_, reason, err := c.account(ctx, orgID)
	return err == nil && reason == ""
}

// Keys are the keys of the confirmation call with what each says back.
func Keys() []netgsm.Key {
	digits := make([]int, 0, len(KeyTexts))
	for d := range KeyTexts {
		digits = append(digits, d)
	}
	sort.Ints(digits)
	out := make([]netgsm.Key, 0, len(digits))
	for _, d := range digits {
		out = append(out, netgsm.Key{Digit: d, Text: KeyTexts[d]})
	}
	return out
}

// Send implements channel.Channel: orders the call; its result comes with
// the webhook.
func (c *Channel) Send(ctx context.Context, m channel.Message) (channel.Result, error) {
	if !c.Ready() {
		return channel.Skipped("", "provider not configured"), nil
	}
	if m.Event != catalog.ConfirmationRequest {
		return channel.Skipped(netgsm.ProviderCode, "calls carry only the confirmation request"), nil
	}
	acc, reason, err := c.account(ctx, m.OrganizationID)
	if err != nil {
		return channel.Result{}, err
	}
	if reason != "" {
		return channel.Skipped(acc.Provider, reason), nil
	}
	phone := strings.TrimSpace(m.Recipient.Phone)
	if netgsm.Recipient(phone) == "" {
		return channel.Skipped(netgsm.ProviderCode, "no Turkish phone number"), nil
	}
	if strings.TrimSpace(m.Body) == "" {
		return channel.Skipped(netgsm.ProviderCode, "no text to read out"), nil
	}
	now := c.svc.now()
	res := channel.Result{Provider: netgsm.ProviderCode, RecipientMasked: sms.Mask(phone)}
	id, err := c.svc.client.Send(ctx, acc, netgsm.Call{
		To: phone, Text: m.Body, Keys: Keys(), Start: now, Stop: now.Add(netgsm.MinWindow), RingSeconds: RingSeconds,
		URL: c.svc.WebhookURL(acc.UUID, m.DeliveryUUID),
	})
	if err != nil {
		res.Outcome, res.Error = channel.OutcomeFailed, err.Error()
		return res, nil
	}
	// Ordered: the call's result comes with the webhook.
	res.Outcome, res.ProviderRef = channel.OutcomeAccepted, id
	return res, nil
}

var (
	_ channel.ReportingChannel    = (*Channel)(nil)
	_ channel.OrganizationChannel = (*Channel)(nil)
)
