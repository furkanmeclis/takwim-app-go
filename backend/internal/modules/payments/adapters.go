package payments

import (
	"context"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	customernotifycatalog "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	customernotifyusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/usecase"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
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

// RefundNotices sends the refund notices (TEC-978) through the customer
// notification engine with the business's rule for the event: one notice
// per refund and event.
func RefundNotices(engine *customernotifyusecase.Engine, q *db.Queries) paymentsusecase.RefundNotifier {
	if engine == nil {
		return nil
	}
	return func(ctx context.Context, orgID, reservationID int64, refund uuid.UUID, notice string) error {
		event := customernotifycatalog.RefundCompleted
		if notice == paymentsusecase.NoticeRefundProcessing {
			event = customernotifycatalog.RefundProcessing
		}
		_, err := engine.EmitEvent(ctx, q, orgID, reservationID, event, "refund:"+refund.String())
		return err
	}
}
