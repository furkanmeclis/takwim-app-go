// Package usecase holds the payment rules of Takwim (TEC-912,
// docs/design/payments.md):
//
//   - Online full payment: the customer of an accepted request waiting
//     for its payment opens the hosted page of the business's own provider
//     (StartOnline). Only the provider's signed server notification records
//     the result (HandleNotification): the attempt succeeds, a ledger
//     collection goes into the provider's receivable cash account and the
//     reservation becomes a booking, in one transaction. A repeated
//     notification produces one collection (integration_jobs idempotency,
//     a conditional update of the attempt, the ledger idempotency key).
//   - Money taken at the business (cash, card, transfer), in one or more
//     parts, is a ledger collection into a till or bank account (Collect).
//
// A payment never confirms the customer's attendance; a payment that
// arrives after the visit was cancelled does not reopen it (refund
// pending, TEC-913).
package usecase

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Limits.
const (
	maxNoteLength = 240
	// minPageMinutes: a payment page is not opened for less time than this
	// (the customer could not finish).
	minPageMinutes = 1
)

var (
	// ErrNotFound: no such reservation / payment for the caller.
	ErrNotFound = errors.New("not found")
	// ErrNotPayable: the reservation does not wait for an online payment
	// (any more): not accepted yet, paid, cancelled or its window lapsed.
	ErrNotPayable = errors.New("the reservation does not wait for an online payment")
	// ErrNotReady: the business has no working payment provider account or
	// no online (provider receivable) cash account for the branch.
	ErrNotReady = errors.New("online payment is not set up for this business")
	// ErrProvider: the provider refused or did not answer the checkout.
	ErrProvider = errors.New("the payment provider did not open the payment page")
	// ErrUnknownProvider: no adapter for the notification's provider.
	ErrUnknownProvider = errors.New("unknown payment provider")
	// ErrSignature: the notification did not verify.
	ErrSignature = errors.New("the notification signature is invalid")
	// ErrMalformed: the notification could not be read.
	ErrMalformed = errors.New("malformed notification")
	// ErrNotCollectable: money is not taken for a cancelled / rejected visit.
	ErrNotCollectable = errors.New("money cannot be taken for this reservation")

	errAlreadyApplied = errors.New("payment already applied")
)

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Message string
	Code    string
}

// ValidationError carries field details; handlers answer 400.
type ValidationError struct {
	Details []Detail
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, Detail{Field: field, Code: code, Message: msg})
}

func invalid(field, code, msg string) error {
	return &ValidationError{Details: []Detail{{Field: field, Code: code, Message: msg}}}
}

// Config is what the service needs from the process configuration.
type Config struct {
	// FrontendURL is the public web origin (return pages).
	FrontendURL string
}

// Service implements the payment use cases.
type Service struct {
	q            *db.Queries
	reservations *reservationsusecase.Service
	sales        *salesusecase.Service
	accounts     *accountsusecase.Service
	runner       *jobsusecase.Runner
	cfg          Config
	log          *slog.Logger
	events       resourceevents.Publisher
	notify       RefundNotifier
	now          func() time.Time
}

// New builds the service.
func New(q *db.Queries, reservations *reservationsusecase.Service, sales *salesusecase.Service,
	accounts *accountsusecase.Service, runner *jobsusecase.Runner, cfg Config, log *slog.Logger,
) *Service {
	if log == nil {
		log = slog.Default()
	}
	cfg.FrontendURL = strings.TrimRight(cfg.FrontendURL, "/")
	return &Service{q: q, reservations: reservations, sales: sales, accounts: accounts, runner: runner, cfg: cfg, log: log, now: time.Now}
}

// SetResourceEvents publishes live list changes (optional).
func (s *Service) SetResourceEvents(pub resourceevents.Publisher) {
	s.events = pub
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

// provider returns the adapter of an account's provider.
func (s *Service) provider(code string) (payments.Provider, bool) {
	return payments.Lookup(s.accounts.Registry(), code)
}

// activeProvider is the business's connected payment account and its
// adapter (ErrNotReady when none works).
func (s *Service) activeProvider(ctx context.Context, orgID int64) (integrations.Account, payments.Provider, error) {
	acc, err := s.accounts.ActiveAccount(ctx, orgID, integrations.KindPayment)
	if errors.Is(err, accountsusecase.ErrNoActiveAccount) {
		return acc, nil, ErrNotReady
	}
	if err != nil {
		return acc, nil, err
	}
	p, ok := s.provider(acc.Provider)
	if !ok {
		return acc, nil, ErrNotReady
	}
	return acc, p, nil
}

// OnlineReady reports whether the business can take online payments: a
// working provider account and an online cash account for its money. The
// public booking page offers online payment only then.
func (s *Service) OnlineReady(ctx context.Context, orgID int64) bool {
	acc, _, err := s.activeProvider(ctx, orgID)
	if err != nil {
		return false
	}
	n, err := s.q.CountOnlineCashAccounts(ctx, db.CountOnlineCashAccountsParams{OrganizationID: orgID, Provider: acc.Provider})
	return err == nil && n > 0
}

func clip(v string, n int) string {
	v = strings.TrimSpace(v)
	if utf8.RuneCountInString(v) <= n {
		return v
	}
	return string([]rune(v)[:n])
}

func amountOf(n pgtype.Numeric) money.Money {
	return money.FromNumericOrZero(n)
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// paidOf is what is paid for a reservation in its currency after the
// postings of the current transaction (collections - refunds).
func paidOf(ctx context.Context, q *db.Queries, orgID, reservationID int64, currency string) (money.Money, error) {
	rows, err := q.LedgerTotals(ctx, db.LedgerTotalsParams{OrganizationID: orgID, ReservationID: pgtype.Int8{Int64: reservationID, Valid: true}})
	if err != nil {
		return money.Zero(), err
	}
	for _, r := range rows {
		if r.Currency == currency {
			return amountOf(r.Collected).Sub(amountOf(r.Refunded)), nil
		}
	}
	return money.Zero(), nil
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func validMethod(m string) bool { return slices.Contains(model.OnSiteMethods, m) }
