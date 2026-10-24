// Package smsprovider is the SMS channel over the business's own provider
// account (TEC-982, docs/design/confirmations.md "SMS and e-mail"): the
// business's Netgsm account (an integration account of kind sms), texts
// accepted with the provider's job id, and their delivery reports read
// from the provider by the worker.
package smsprovider

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm"
)

// Client is the Netgsm SMS client of this process (nil when
// SMS_NETGSM_ENABLED is off).
func Client(cfg config.Config) *netgsm.Provider {
	if !cfg.SMS.NetgsmEnabled {
		return nil
	}
	return netgsm.New(cfg.SMS.NetgsmBaseURL, nil)
}

// Adapters are the SMS providers registered on the integration kit (a
// business connects its account in Ayarlar → Entegrasyonlar).
func Adapters(c *netgsm.Provider) []integrations.Adapter {
	if c == nil {
		return nil
	}
	return []integrations.Adapter{c}
}
