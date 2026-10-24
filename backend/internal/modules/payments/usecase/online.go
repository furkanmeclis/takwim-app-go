package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	salesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/testpay"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// StartInput opens the payment page of a reservation for its customer
// (the public booking API checked the reservation's access token).
type StartInput struct {
	OrganizationID  int64
	Slug            string
	ReservationUUID uuid.UUID
	IP              string
	Locale          string
}

// StartOnline opens (or reopens, while it is still valid) the hosted page
// of the business's payment provider for the full amount of an accepted
// request waiting for its online payment. The page expires with the
// reservation's payment window.
func (s *Service) StartOnline(ctx context.Context, in StartInput) (model.OnlineStart, error) {
	orgID := in.OrganizationID
	row, err := s.q.GetReservationRecord(ctx, db.GetReservationRecordParams{OrganizationID: orgID, Uuid: in.ReservationUUID})
	if isNoRows(err) {
		return model.OnlineStart{}, ErrNotFound
	}
	if err != nil {
		return model.OnlineStart{}, err
	}
	res, err := s.reservations.Get(ctx, orgID, in.ReservationUUID, availabilityusecase.Caller{})
	if errors.Is(err, reservationsusecase.ErrNotFound) {
		return model.OnlineStart{}, ErrNotFound
	}
	if err != nil {
		return model.OnlineStart{}, err
	}
	now := s.now()
	if reservationsusecase.OnlinePaymentOutcome(row) != reservationsusecase.PaymentConfirmed || res.HoldExpiresAt == nil ||
		res.HoldExpiresAt.Before(now.Add(minPageMinutes*time.Minute)) || !res.TotalPrice.IsPositive() {
		return model.OnlineStart{}, ErrNotPayable
	}
	acc, prov, err := s.activeProvider(ctx, orgID)
	if err != nil {
		return model.OnlineStart{}, err
	}
	cash, err := s.q.FindOnlineCashAccount(ctx, db.FindOnlineCashAccountParams{
		OrganizationID: orgID, Currency: res.Currency, BranchID: pgtype.Int8{Int64: row.BranchID, Valid: true}, Provider: acc.Provider,
	})
	if isNoRows(err) {
		return model.OnlineStart{}, ErrNotReady
	}
	if err != nil {
		return model.OnlineStart{}, err
	}
	if open, err := s.q.FindOpenPayment(ctx, db.FindOpenPaymentParams{
		OrganizationID: orgID, ReservationID: row.ID, Now: pgtype.Timestamptz{Time: now, Valid: true},
	}); err == nil && amountOf(open.Amount).Equal(res.TotalPrice) {
		return model.OnlineStart{
			PaymentUUID: open.Uuid, RedirectURL: open.PageUrl, Amount: amountOf(open.Amount), Currency: open.Currency, ExpiresAt: open.ExpiresAt.Time,
		}, nil
	} else if err != nil && !isNoRows(err) {
		return model.OnlineStart{}, err
	}

	p, err := s.q.InsertPayment(ctx, db.InsertPaymentParams{
		OrganizationID: orgID, BranchID: row.BranchID, ReservationID: row.ID,
		IntegrationAccountID: pgtype.Int8{Int64: acc.ID, Valid: acc.ID > 0}, CashAccountID: cash.ID, Provider: acc.Provider,
		OrderRef: strings.ReplaceAll(uuid.NewString(), "-", ""), Amount: res.TotalPrice.Numeric(), Currency: res.Currency,
		ExpiresAt: pgtype.Timestamptz{Time: *res.HoldExpiresAt, Valid: true},
	})
	if err != nil {
		return model.OnlineStart{}, err
	}
	back := s.cfg.FrontendURL + "/b/" + url.PathEscape(in.Slug) + "/reservations/" + res.UUID.String()
	names := make([]string, 0, len(res.Lines))
	for _, l := range res.Lines {
		names = append(names, l.ServiceName)
	}
	checkout, err := prov.CreateCheckout(ctx, acc, payments.CheckoutRequest{
		OrderRef: p.OrderRef, Amount: res.TotalPrice, Currency: res.Currency,
		ItemName: clip(fmt.Sprintf("%s · #%d", strings.Join(names, ", "), res.Number), 200),
		Buyer:    payments.Buyer{Name: res.Customer.Name, IP: in.IP}, Locale: in.Locale, ExpiresAt: *res.HoldExpiresAt,
		SuccessURL: back + "?payment=done", FailURL: back + "?payment=failed",
	})
	if err != nil {
		s.log.Warn("payment_checkout_failed", "organization_id", orgID, "payment", p.Uuid, "provider", acc.Provider, "error", err)
		_, _ = s.q.FailPayment(ctx, db.FailPaymentParams{OrganizationID: orgID, ID: p.ID, FailureReason: clip("checkout: "+err.Error(), 500)})
		return model.OnlineStart{}, ErrProvider
	}
	if err := s.q.SetPaymentCheckout(ctx, db.SetPaymentCheckoutParams{
		OrganizationID: orgID, ID: p.ID, PageUrl: checkout.PageURL, ProviderToken: clip(checkout.Token, 200),
	}); err != nil {
		return model.OnlineStart{}, err
	}
	return model.OnlineStart{
		PaymentUUID: p.Uuid, RedirectURL: checkout.PageURL, Amount: res.TotalPrice, Currency: res.Currency, ExpiresAt: *res.HoldExpiresAt,
	}, nil
}

// Notification is what HandleNotification answers: the result and the
// provider to acknowledge with.
type Notification struct {
	Result   model.NotifyResult
	Provider payments.Provider
}

// HandleNotification records a provider's server notification:
//
//  1. parse the body (ErrMalformed);
//  2. our reference → the attempt → its business (ErrNotFound);
//  3. verify the signature with the secret of the account the page was
//     opened with (ErrSignature);
//  4. apply it at most once per notification (integration_jobs, key
//     org:<id>:<provider event>): a failure marks the attempt failed; a
//     success writes the collection and tells the reservation, in one
//     transaction, and the attempt moves only once.
func (s *Service) HandleNotification(ctx context.Context, provider string, header http.Header, body []byte) (Notification, error) {
	prov, ok := s.provider(provider)
	if !ok {
		return Notification{}, ErrUnknownProvider
	}
	n, err := prov.ParseNotification(header, body)
	if err != nil {
		return Notification{}, ErrMalformed
	}
	p, err := s.q.GetPaymentByOrderRef(ctx, n.OrderRef)
	if isNoRows(err) {
		return Notification{}, ErrNotFound
	}
	if err != nil {
		return Notification{}, err
	}
	if p.Provider != provider || !p.IntegrationAccountID.Valid {
		return Notification{}, ErrNotFound
	}
	acc, err := s.accounts.AccountByID(ctx, p.OrganizationID, p.IntegrationAccountID.Int64)
	if err != nil {
		return Notification{}, ErrNotFound
	}
	if err := prov.VerifyNotification(acc, n, header, body); err != nil {
		s.log.Warn("payment_notification_rejected", "organization_id", p.OrganizationID, "payment", p.Uuid, "provider", provider, "error", err)
		return Notification{}, ErrSignature
	}
	ref := p.Uuid
	res, err := s.runner.HandleInbound(ctx, jobsusecase.InboundInput{
		OrganizationID: p.OrganizationID, AccountID: acc.ID, Provider: provider, Kind: model.InboundKind,
		Key:     fmt.Sprintf("org:%d:%s", p.OrganizationID, n.EventKey),
		Payload: map[string]any{"payment_uuid": p.Uuid.String(), "status": n.Status, "test": n.Test},
		RefType: model.SourcePayment, RefUUID: &ref,
	}, func(ctx context.Context) (string, map[string]any, error) {
		return s.apply(ctx, p, n)
	})
	return Notification{Result: model.NotifyResult{Status: res.Status, Duplicate: res.Duplicate}, Provider: prov}, err
}

// apply carries out one verified notification.
func (s *Service) apply(ctx context.Context, p db.GetPaymentByOrderRefRow, n payments.Notification) (string, map[string]any, error) {
	orgID := p.OrganizationID
	result := map[string]any{"payment_uuid": p.Uuid.String()}
	if n.Status == payments.StatusFailed {
		reason := clip(strings.TrimSpace(n.FailureCode+" "+n.FailureMessage), 500)
		rows, err := s.q.FailPayment(ctx, db.FailPaymentParams{OrganizationID: orgID, ID: p.ID, FailureReason: reason})
		if err != nil {
			return "", nil, err
		}
		if rows == 0 {
			result["outcome"] = "already_final"
			return jobsusecase.StatusIgnored, result, nil
		}
		result["outcome"] = model.StatusFailed
		s.changed(ctx, orgID, p.ReservationUuid)
		return jobsusecase.StatusSucceeded, result, nil
	}
	amount := amountOf(p.Amount)
	if n.Amount.Minor() != amount.Minor() || (n.Currency != "" && n.Currency != p.Currency) {
		// Never confirm on an answer that does not match what was asked
		// for; staff checks the provider panel.
		reason := fmt.Sprintf("amount_mismatch: expected %s %s, notified %s %s", amount, p.Currency, n.Amount, n.Currency)
		s.log.Error("payment_notification_mismatch", "organization_id", orgID, "payment", p.Uuid, "reason", reason)
		if _, err := s.q.FailPayment(ctx, db.FailPaymentParams{OrganizationID: orgID, ID: p.ID, FailureReason: reason}); err != nil {
			return "", nil, err
		}
		result["outcome"] = "mismatch"
		s.changed(ctx, orgID, p.ReservationUuid)
		return jobsusecase.StatusIgnored, result, nil
	}
	paid := n.PaidAmount
	if !paid.IsPositive() {
		paid = amount
	}
	_, outcome, err := s.reservations.RecordPayment(ctx, orgID, p.ReservationUuid, availabilityusecase.Caller{}, 0, true,
		"online "+amount.String()+" "+p.Currency, func(ctx context.Context, q *db.Queries, row db.Reservation) (money.Money, error) {
			lp, err := q.LockPayment(ctx, db.LockPaymentParams{OrganizationID: orgID, ID: p.ID})
			if err != nil {
				return money.Zero(), err
			}
			if lp.Status == model.StatusSucceeded {
				return money.Zero(), errAlreadyApplied
			}
			entry, err := s.sales.RecordCollectionTx(ctx, q, orgID, salesmodel.CollectionInput{
				ReservationUUID: &row.Uuid, CashAccountUUID: lp.CashAccountUuid, Amount: amount, Method: salesmodel.MethodOnline,
				Description: "online payment (" + lp.Provider + ")", SourceType: model.SourcePayment, SourceUUID: &lp.Uuid,
				IdempotencyKey: "payment:" + lp.Uuid.String(),
			})
			if err != nil {
				return money.Zero(), err
			}
			e, err := q.GetLedgerEntryRow(ctx, db.GetLedgerEntryRowParams{OrganizationID: orgID, Uuid: entry})
			if err != nil {
				return money.Zero(), err
			}
			rows, err := q.SucceedPayment(ctx, db.SucceedPaymentParams{
				OrganizationID: orgID, ID: lp.ID, PaidAmount: paid.Numeric(), ProviderPaymentID: clip(n.ProviderPaymentID, 200),
				Outcome: reservationsusecase.OnlinePaymentOutcome(row), LedgerEntryID: pgtype.Int8{Int64: e.ID, Valid: true},
			})
			if err != nil {
				return money.Zero(), err
			}
			if rows == 0 {
				return money.Zero(), errAlreadyApplied
			}
			return paidOf(ctx, q, orgID, row.ID, row.Currency)
		})
	if errors.Is(err, errAlreadyApplied) {
		result["outcome"] = "already_applied"
		return jobsusecase.StatusIgnored, result, nil
	}
	if err != nil {
		return "", nil, err
	}
	result["outcome"] = outcome
	if outcome == reservationsusecase.PaymentLate {
		s.log.Warn("payment_late", "organization_id", orgID, "payment", p.Uuid, "reservation", p.ReservationUuid)
	}
	s.changed(ctx, orgID, p.ReservationUuid)
	return jobsusecase.StatusSucceeded, result, nil
}

// TestPage is what the development provider's page shows for a reference
// (ErrNotFound unless the attempt is the test provider's and it is
// registered: never in production).
func (s *Service) TestPage(ctx context.Context, ref string) (model.TestPage, error) {
	if _, ok := s.provider(testpay.ProviderCode); !ok {
		return model.TestPage{}, ErrNotFound
	}
	row, err := s.q.GetTestPaymentPage(ctx, ref)
	if isNoRows(err) || (err == nil && row.Provider != testpay.ProviderCode) {
		return model.TestPage{}, ErrNotFound
	}
	if err != nil {
		return model.TestPage{}, err
	}
	return model.TestPage{
		Business: row.OrganizationName, Slug: row.OrganizationSlug, ReservationUUID: row.ReservationUuid,
		ReservationNumber: row.ReservationNumber, Amount: amountOf(row.Amount), Currency: row.Currency, Status: row.Status,
		ExpiresAt: row.ExpiresAt.Time,
	}, nil
}

// TestComplete is the test page's button: it signs the notification a
// real provider would send with the account's secret and hands it to
// HandleNotification, so the verified path is the one exercised.
func (s *Service) TestComplete(ctx context.Context, ref, outcome string) (Notification, error) {
	page, err := s.TestPage(ctx, ref)
	if err != nil {
		return Notification{}, err
	}
	if outcome != payments.StatusSucceeded && outcome != payments.StatusFailed {
		return Notification{}, invalid("outcome", "invalid", "outcome must be succeeded or failed")
	}
	row, err := s.q.GetPaymentByOrderRef(ctx, ref)
	if err != nil || !row.IntegrationAccountID.Valid {
		return Notification{}, ErrNotFound
	}
	acc, err := s.accounts.AccountByID(ctx, row.OrganizationID, row.IntegrationAccountID.Int64)
	if err != nil {
		return Notification{}, ErrNotFound
	}
	header, body, err := testpay.Notify(acc, ref, outcome, page.Amount, page.Currency)
	if err != nil {
		return Notification{}, err
	}
	return s.HandleNotification(ctx, testpay.ProviderCode, header, body)
}

// changed tells the panel's live lists that a reservation and its ledger
// moved.
func (s *Service) changed(ctx context.Context, orgID int64, reservation uuid.UUID) {
	if s.events == nil {
		return
	}
	ctx = orgctx.WithOrganizationID(ctx, orgID)
	s.events.Publish(ctx, reservationsmodel.Slug, "updated", reservation.String())
	s.events.Publish(ctx, salesmodel.EntriesSlug, "updated")
}
