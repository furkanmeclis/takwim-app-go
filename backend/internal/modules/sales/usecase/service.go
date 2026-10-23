// Package usecase holds the sales and ledger rules (TEC-916). Every method
// takes the organization id of the caller explicitly.
//
// Revenue and collection are separate facts:
//
//   - A sale line is written when a service is delivered (a reservation is
//     completed: one line per reservation line) or a product is handed over
//     (a stock sale). Its net amount is the revenue of its business day.
//   - Money taken is a collection entry in the ledger with its cash
//     movement; an online prepayment is a collection on the day it is
//     taken and the service is revenue on the day it is delivered, so the
//     same visit is never income twice.
//   - Each sale line posts a debit to the ledger, so a customer's balance
//     (debits - credits) is what is still owed (or, negative, held for the
//     customer).
//
// Nothing is edited or deleted. A wrong sale line is reversed (the exact
// negative, dated the day of the reversal) and, for a correction, replaced
// by a new line for the same item; its ledger debit is reversed and posted
// again with it. A wrong collection or refund is reversed together with
// its cash movement.
//
// Branch scope: lines and entries belong to a branch. A member limited to
// some branches sees only those; any other row reads as not found. The
// posting methods (…Tx) are called by other modules that authorized the
// caller already and do not check the scope.
package usecase

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Limits.
const (
	minReasonLength      = 3
	maxReasonLength      = 500
	maxDescriptionLength = 240
	maxQuantity          = 100000
	maxSummaryDays       = 366
	dateLayout           = "2006-01-02"
)

var (
	ErrLineNotFound       = errors.New("sale line not found")
	ErrEntryNotFound      = errors.New("ledger entry not found")
	ErrReservationMissing = errors.New("reservation not found")
	ErrCustomerMissing    = errors.New("customer not found")
	ErrAlreadyReversed    = errors.New("the record was reversed already")
	ErrNotReversible      = errors.New("the record cannot be reversed")
	ErrReverseSaleLine    = errors.New("a sale entry is reversed through its sale line")
	ErrHasRefunds         = errors.New("the collection has refunds; reverse them first")
	ErrRefundTooLarge     = errors.New("the refund is larger than what is left of the collection")
	ErrNothingChanged     = errors.New("the correction changes nothing")
	ErrCurrencyMismatch   = errors.New("the cash account has another currency")
	ErrIdempotencyReused  = errors.New("the idempotency key was used for another posting")
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

func (e *ValidationError) orNil() error {
	if len(e.Details) == 0 {
		return nil
	}
	return e
}

func invalid(field, code, msg string) error {
	return &ValidationError{Details: []Detail{{Field: field, Code: code, Message: msg}}}
}

// IsValidation reports whether err is a field validation error.
func IsValidation(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// Service implements the sales and ledger use cases.
type Service struct {
	repo *repository.Postgres
	cash *cashusecase.Service
	now  func() time.Time
}

// New builds the service. cash posts the money of collections and refunds
// into the cash book.
func New(repo *repository.Postgres, cash *cashusecase.Service) *Service {
	return &Service{repo: repo, cash: cash, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

// clock is the time zone and currency of a branch.
type clock struct {
	loc      *time.Location
	currency string
}

func (s *Service) clockOf(ctx context.Context, q *db.Queries, orgID, branchID int64) (clock, error) {
	row, err := q.GetSalesClock(ctx, db.GetSalesClockParams{OrganizationID: orgID, BranchID: branchID})
	if err != nil {
		return clock{}, err
	}
	loc, err := time.LoadLocation(row.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return clock{loc: loc, currency: row.Currency}, nil
}

// day is the business day of t in the clock's zone (a UTC midnight).
func (c clock) day(t time.Time) time.Time {
	y, m, d := t.In(c.loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// today is the business day in the organization's zone (the request scope,
// UTC without one).
func (s *Service) today(ctx context.Context) time.Time {
	loc := time.UTC
	if scope, ok := orgctx.From(ctx); ok && scope.Timezone != "" {
		if l, err := time.LoadLocation(scope.Timezone); err == nil {
			loc = l
		}
	}
	return clock{loc: loc}.day(s.now())
}

func checkReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	switch n := utf8.RuneCountInString(reason); {
	case n < minReasonLength:
		return "", invalid("reason", "required", "a reason of at least 3 characters is required")
	case n > maxReasonLength:
		return "", invalid("reason", "too_long", "reason is too long")
	}
	return reason, nil
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func mapRepoErr(err error) error {
	switch e := repository.MapErr(err); {
	case errors.Is(e, repository.ErrAlreadyReversed):
		return ErrAlreadyReversed
	case errors.Is(e, repository.ErrIdempotencyKey):
		return ErrIdempotencyReused
	default:
		return e
	}
}

func isNotFound(err error) bool {
	return errors.Is(repository.MapErr(err), repository.ErrNotFound)
}

// ------------------------------------------------------------------ helpers

func formatDate(t time.Time) string {
	return t.Format(dateLayout)
}

func dateArg(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: true}
}

func datePtrArg(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return dateArg(*t)
}

func int8Of(v int64) pgtype.Int8 {
	return pgtype.Int8{Int64: v, Valid: true}
}

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil || *v == 0 {
		return pgtype.Int8{}
	}
	return int8Of(*v)
}

func uuidArg(v *uuid.UUID) pgtype.UUID {
	if v == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *v, Valid: true}
}

func uuidPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
}

func textArg(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func amountOf(n pgtype.Numeric) money.Money {
	return money.FromNumericOrZero(n)
}

func name(first, last pgtype.Text) string {
	return strings.TrimSpace(first.String + " " + last.String)
}
