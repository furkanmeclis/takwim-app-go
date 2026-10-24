// Package voicecall is the automatic voice call of the confirmation
// requests (TEC-923, docs/design/confirmations.md "Voice call"): the
// business's own Netgsm account (an integration account), a call that reads
// out the visit and takes a key (1 Geleceğim, 2 cancel, 3 change), and the
// call results on the account's webhook.
package voicecall

import (
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/voicecall/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/voice/netgsm"
)

// Client is the Netgsm voice client of this process (nil when
// VOICE_NETGSM_ENABLED is off).
func Client(cfg config.Config) *netgsm.Provider {
	if !cfg.Voice.NetgsmEnabled {
		return nil
	}
	return netgsm.New(cfg.Voice.NetgsmBaseURL, nil)
}

// WebhookBase is the public API origin the provider posts call results to.
func WebhookBase(cfg config.Config) string {
	if v := strings.TrimSpace(cfg.Voice.WebhookBaseURL); v != "" {
		return strings.TrimRight(v, "/")
	}
	return strings.TrimRight(cfg.Auth.FrontendURL, "/") + "/api"
}

// Adapters are the voice providers registered on the integration kit (a
// business connects its account in Ayarlar → Entegrasyonlar).
func Adapters(c *netgsm.Provider) []integrations.Adapter {
	if c == nil {
		return nil
	}
	return []integrations.Adapter{c}
}

// RegisterRoutes mounts the public webhook of an account. The account uuid
// in the path names the business; every call result carries the attempt
// and a token only this server can make.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler) {
	mux.HandleFunc("POST /v1/public/voice/netgsm/{account}/webhook", h.Receive)
}
