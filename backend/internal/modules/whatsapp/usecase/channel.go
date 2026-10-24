package usecase

import (
	"context"
	"errors"
	"strings"

	confirmationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp/meta"
)

// Channel is the customer notification engine's WhatsApp channel
// (channel.ReportingChannel and channel.OrganizationChannel). It sends
// through the business's own Cloud API account and only the confirmation
// request: a business-initiated WhatsApp message needs a template Meta
// approved, and the confirmation template is the one Takwim asks the
// business to register. Any other notice is skipped, so the rule's next
// channel (SMS, e-mail) runs.
type Channel struct{ svc *Service }

// NewChannel builds the channel on the service.
func NewChannel(svc *Service) *Channel { return &Channel{svc: svc} }

// TemplateParams are the body parameters of the confirmation template, in
// order ({{1}} … {{5}}).
var TemplateParams = []string{"customer_name", "business_name", "starts_at", "deadline", "reference"}

// QuickReplies are the payloads of the template's buttons, in order
// (Geleceğim, İptal, Değişiklik).
var QuickReplies = []string{
	PayloadPrefix + confirmationsmodel.AnswerConfirm,
	PayloadPrefix + confirmationsmodel.AnswerCancel,
	PayloadPrefix + confirmationsmodel.AnswerChange,
}

// Name implements channel.Channel.
func (*Channel) Name() string { return channel.WhatsApp }

// Ready implements channel.Channel: the Cloud API is enabled here.
func (c *Channel) Ready() bool { return c.svc.enabled() }

// SetReporter implements channel.ReportingChannel: the engine that
// registers the channel records its webhook's delivery reports.
func (c *Channel) SetReporter(r channel.Reporter) {
	if rec, ok := r.(Recorder); ok {
		c.svc.reports = rec
	}
}

// account is the business's active WhatsApp account on the Cloud API.
func (c *Channel) account(ctx context.Context, orgID int64) (integrations.Account, string, error) {
	acc, err := c.svc.accounts.ActiveAccount(ctx, orgID, integrations.KindWhatsApp)
	if errors.Is(err, accountsusecase.ErrNoActiveAccount) {
		return acc, "WhatsApp account not connected", nil
	}
	if err != nil {
		return acc, "", err
	}
	if acc.Provider != meta.ProviderCode {
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

// Send implements channel.Channel.
func (c *Channel) Send(ctx context.Context, m channel.Message) (channel.Result, error) {
	if !c.Ready() {
		return channel.Skipped("", "provider not configured"), nil
	}
	if m.Event != catalog.ConfirmationRequest {
		return channel.Skipped(meta.ProviderCode, "no approved WhatsApp template for this notice"), nil
	}
	acc, reason, err := c.account(ctx, m.OrganizationID)
	if err != nil {
		return channel.Result{}, err
	}
	if reason != "" {
		return channel.Skipped(acc.Provider, reason), nil
	}
	phone := strings.TrimSpace(m.Recipient.Phone)
	if len(meta.Recipient(phone)) < 8 {
		return channel.Skipped(meta.ProviderCode, "no phone number"), nil
	}
	vars := map[string]string{}
	for k, v := range m.Vars {
		vars[k] = v
	}
	if vars["business_name"] == "" {
		vars["business_name"] = m.BusinessName
	}
	params := make([]string, 0, len(TemplateParams))
	for _, k := range TemplateParams {
		params = append(params, vars[k])
	}
	name, lang := meta.Template(acc)
	res := channel.Result{Provider: meta.ProviderCode, RecipientMasked: sms.Mask(phone)}
	id, err := c.svc.client.SendTemplate(ctx, acc, meta.TemplateMessage{
		To: phone, Name: name, Language: lang, BodyParams: params, QuickReplies: QuickReplies,
	})
	if err != nil {
		res.Outcome, res.Error = channel.OutcomeFailed, err.Error()
		return res, nil
	}
	// Accepted by the Cloud API: delivery comes with the webhook status.
	res.Outcome, res.ProviderRef = channel.OutcomeAccepted, id
	return res, nil
}

var (
	_ channel.ReportingChannel    = (*Channel)(nil)
	_ channel.OrganizationChannel = (*Channel)(nil)
)
