package payments

import (
	"context"
	"errors"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	paymentsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	productsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	productsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/usecase"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
)

// StockSaleCollector adapts the service to the stock flow (TEC-992): the
// money taken with a counter sale is posted as a ledger collection in the
// sale's transaction, keyed by the sale so a retried sale never collects
// twice.
func StockSaleCollector(svc *paymentsusecase.Service) productsusecase.SaleCollector {
	return func(ctx context.Context, q *db.Queries, orgID int64, sale productsusecase.StockSale, in productsmodel.SaleCollectionInput) error {
		err := svc.CollectStockSaleTx(ctx, q, orgID,
			paymentsusecase.StockSaleParty{MovementUUID: sale.MovementUUID, BranchID: sale.BranchID, BranchUUID: sale.BranchUUID},
			paymentsmodel.CollectInput{Amount: in.Amount, Method: in.Method, CashAccountUUID: in.CashAccountUUID, Note: in.Note},
			"stock_sale:"+sale.MovementUUID.String(), sale.CreatedBy)
		return productMoneyError(err, "collection")
	}
}

// StockSaleRefunder adapts the service to the return flow (TEC-992): the
// money of a returned sale is paid back in the return's transaction, keyed
// by the sale (a sale is returned once).
func StockSaleRefunder(svc *paymentsusecase.Service) productsusecase.SaleRefunder {
	return func(ctx context.Context, q *db.Queries, orgID int64, sale productsusecase.ReturnedSale, in productsmodel.SaleRefundInput) error {
		err := svc.RefundStockSaleTx(ctx, q, orgID,
			paymentsusecase.StockSaleParty{MovementUUID: sale.MovementUUID, BranchID: sale.BranchID},
			paymentsmodel.StockSaleRefundInput{Amount: in.Amount, Method: in.Method, CashAccountUUID: in.CashAccountUUID, Reason: "return: " + sale.Reason},
			"stock_return:"+sale.MovementUUID.String(), sale.ActorID)
		return productMoneyError(err, "refund")
	}
}

// productMoneyError translates the payment layer's answers into the
// products module's: field errors under the body key they came from
// (`collection.amount`, `refund.cash_account_uuid`), cash rules as
// conflicts on the account.
func productMoneyError(err error, prefix string) error {
	if err == nil {
		return nil
	}
	fields := func(details []productsusecase.Detail) error {
		return &productsusecase.ValidationError{Details: details}
	}
	var pve *paymentsusecase.ValidationError
	if errors.As(err, &pve) {
		out := make([]productsusecase.Detail, 0, len(pve.Details))
		for _, d := range pve.Details {
			out = append(out, productsusecase.Detail{Field: prefix + "." + d.Field, Code: d.Code, Message: d.Message})
		}
		return fields(out)
	}
	var sve *salesusecase.ValidationError
	if errors.As(err, &sve) {
		out := make([]productsusecase.Detail, 0, len(sve.Details))
		for _, d := range sve.Details {
			out = append(out, productsusecase.Detail{Field: prefix + "." + d.Field, Code: d.Code, Message: d.Message})
		}
		return fields(out)
	}
	switch {
	case errors.Is(err, cashusecase.ErrDayClosed), errors.Is(err, cashusecase.ErrAccountArchived):
		return &productsusecase.MoneyConflict{Field: prefix + ".cash_account_uuid", Code: "closed", Message: err.Error()}
	case errors.Is(err, cashusecase.ErrInsufficientFunds):
		return &productsusecase.MoneyConflict{Field: prefix + ".cash_account_uuid", Code: "insufficient_funds", Message: err.Error()}
	case errors.Is(err, cashusecase.ErrNotFound):
		return fields([]productsusecase.Detail{{Field: prefix + ".cash_account_uuid", Code: "not_found", Message: "cash account not found"}})
	case errors.Is(err, salesusecase.ErrRefundTooLarge):
		return fields([]productsusecase.Detail{{Field: prefix + ".amount", Code: "exceeds_refundable", Message: err.Error()}})
	case errors.Is(err, salesusecase.ErrCurrencyMismatch):
		return fields([]productsusecase.Detail{{Field: prefix + ".cash_account_uuid", Code: "currency", Message: err.Error()}})
	}
	return err
}
