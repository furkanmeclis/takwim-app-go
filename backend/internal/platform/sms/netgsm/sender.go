package netgsm

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
)

// Sender is the sms.Sender port over one fixed Netgsm account (the
// platform's own, for sign-in codes once TEC-884 names it). Business
// notices do not use it: they go through the business's account
// (modules/smsprovider).
type Sender struct {
	Client  *Provider
	Account integrations.Account
}

var (
	_ sms.Sender    = (*Sender)(nil)
	_ sms.Describer = (*Sender)(nil)
)

// Send implements sms.Sender.
func (s *Sender) Send(ctx context.Context, msg sms.Message) error {
	if s == nil || s.Client == nil {
		return sms.ErrUnavailable
	}
	_, err := s.Client.Send(ctx, s.Account, Message{To: msg.To, Text: msg.Text})
	return err
}

// Provider implements sms.Describer.
func (s *Sender) Provider() string { return ProviderCode }

// Ready implements sms.Describer: the account's credentials are set.
func (s *Sender) Ready() bool {
	if s == nil || s.Client == nil {
		return false
	}
	_, _, err := credentials(s.Account)
	return err == nil && s.Account.Credentials[FieldHeader] != ""
}
