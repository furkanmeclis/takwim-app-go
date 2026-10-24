// Package whatsapp is the WhatsApp channel of the confirmation requests
// (TEC-922, docs/design/confirmations.md "Channels"): the business's own
// WhatsApp Cloud API account (an integration account), the confirmation
// template with Geleceğim / İptal / Değişiklik buttons, delivery reports and
// button replies through the account's webhook.
package whatsapp

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp/meta"
)

// Client is the Cloud API client of this process (nil when
// WHATSAPP_META_ENABLED is off).
func Client(cfg config.Config) *meta.Provider {
	if !cfg.WhatsApp.MetaEnabled {
		return nil
	}
	return meta.New(cfg.WhatsApp.MetaGraphURL, cfg.WhatsApp.MetaAPIVersion, nil)
}

// Adapters are the WhatsApp providers registered on the integration kit
// (a business connects its account in Ayarlar → Entegrasyonlar).
func Adapters(c *meta.Provider) []integrations.Adapter {
	if c == nil {
		return nil
	}
	return []integrations.Adapter{c}
}

// RegisterRoutes mounts the public webhook of an account. The account uuid
// in the path names the business; every call is checked with that
// account's own verify token or app secret.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler) {
	mux.HandleFunc("GET /v1/public/whatsapp/meta/{account}/webhook", h.Verify)
	mux.HandleFunc("POST /v1/public/whatsapp/meta/{account}/webhook", h.Receive)
}
