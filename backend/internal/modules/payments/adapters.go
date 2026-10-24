package payments

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/paytr"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/testpay"
)

// Adapters are the payment providers of this process, registered on the
// integration kit (API and worker alike): PayTR when
// PAYMENTS_PAYTR_ENABLED is set, the test provider outside production.
func Adapters(cfg config.Config) []integrations.Adapter {
	var out []integrations.Adapter
	if cfg.Payments.PayTREnabled {
		out = append(out, paytr.New(cfg.Payments.PayTRBaseURL, nil))
	}
	if cfg.App.Env == "development" || cfg.App.Env == "test" {
		out = append(out, testpay.New(cfg.Auth.FrontendURL))
	}
	return out
}
