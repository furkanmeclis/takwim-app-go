// Package usecase holds the income / expense rules (TEC-915). Every method
// takes the organization id of the caller explicitly (from orgctx.Scope in
// handlers, from the job in workers).
//
// A record is money in (income) or out (expense) outside sales. It posts
// its cash movement through cash.PostTx in the same transaction, so the
// record and the cash book commit together. Records are append-only: a
// mistake is reversed (the exact negative, dated the day it is posted,
// with the reversal of its movement) or corrected (a reversal plus a
// replacement); only the classification and the document change in place.
//
// Branch scope: a record belongs to a branch or to the whole business
// (branch NULL). A member limited to some branches sees and writes only
// the records of those branches; business-wide records need every branch.
// Any other record reads as not found.
package usecase

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/repository"
	salesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Field limits.
const (
	maxCategoryName      = 120
	maxDescription       = 500
	maxReason            = 500
	minReason            = 3
	maxIdempotencyKey    = 100
	maxSortOrder         = 100000
	maxSummaryDays       = 366
	dateLayout           = "2006-01-02"
	idempotencyPrefix    = "finance_entry:"
	movementSourceType   = "finance_entry"
	correctionReasonNote = "correction: "
)

var (
	ErrCategoryNotFound   = errors.New("category not found")
	ErrEntryNotFound      = errors.New("record not found")
	ErrCategoryNameTaken  = errors.New("the kind has a category with this name already")
	ErrCategoryInUse      = errors.New("a category in use or a default category is archived, not deleted")
	ErrAlreadyReversed    = errors.New("the record was reversed already")
	ErrNotReversible      = errors.New("a reversal cannot be reversed or corrected")
	ErrIdempotencyReused  = errors.New("the idempotency key was used for another record")
	ErrDayClosed          = errors.New("the day is closed for this account")
	ErrInsufficientFunds  = errors.New("the till does not hold enough money")
	ErrAccountArchived    = errors.New("the account is archived")
	ErrNoDocument         = errors.New("the record has no document")
	ErrStorageUnavailable = errors.New("storage is not configured")
)

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Message string
	Code    string
}

// ValidationError carries field details; handlers answer 400 VALIDATION_ERROR.
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

// SalesSummary is the sales ledger summary (revenue, collections,
// transfers, staff payouts and the income / expenses entered here).
type SalesSummary interface {
	Summary(ctx context.Context, orgID int64, from, to *time.Time, branches []uuid.UUID) (salesmodel.Summary, error)
}

// Service implements the income / expense use cases.
type Service struct {
	repo    *repository.Postgres
	cash    *cashusecase.Service
	sales   SalesSummary
	storage storage.Driver
	now     func() time.Time
}

// New builds the service. sales may be nil (no summary).
func New(repo *repository.Postgres, cash *cashusecase.Service, sales SalesSummary) *Service {
	return &Service{repo: repo, cash: cash, sales: sales, now: time.Now}
}

// SetStorage enables documents (private object storage).
func (s *Service) SetStorage(d storage.Driver) {
	s.storage = d
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

// today is the business day in the organization's time zone.
func (s *Service) today(ctx context.Context) time.Time {
	loc := time.UTC
	if scope, ok := orgctx.From(ctx); ok && scope.Timezone != "" {
		if l, err := time.LoadLocation(scope.Timezone); err == nil {
			loc = l
		}
	}
	y, m, d := s.now().In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// allowsBranch applies the branch scope to a record's branch (NULL =
// business-wide: every branch is needed).
func allowsBranch(ctx context.Context, branch pgtype.Int8) bool {
	if !branch.Valid {
		return orgctx.RequireAllBranches(ctx) == nil
	}
	return orgctx.CheckBranch(ctx, branch.Int64) == nil
}

// mapCashErr turns the cash book's answers into this module's errors
// (the movement date is the record's date).
func mapCashErr(err error) error {
	var cve *cashusecase.ValidationError
	if errors.As(err, &cve) {
		ve := &ValidationError{}
		for _, d := range cve.Details {
			field := d.Field
			if field == "movement_date" {
				field = "entry_date"
			}
			ve.add(field, d.Code, d.Message)
		}
		return ve
	}
	switch {
	case errors.Is(err, cashusecase.ErrNotFound):
		return invalid("cash_account_uuid", "not_found", "cash account not found")
	case errors.Is(err, cashusecase.ErrDayClosed):
		return ErrDayClosed
	case errors.Is(err, cashusecase.ErrInsufficientFunds):
		return ErrInsufficientFunds
	case errors.Is(err, cashusecase.ErrAccountArchived):
		return ErrAccountArchived
	case errors.Is(err, cashusecase.ErrAlreadyReversed):
		return ErrAlreadyReversed
	case errors.Is(err, cashusecase.ErrIdempotencyReused):
		return ErrIdempotencyReused
	}
	return err
}

func mapRepoErr(err error) error {
	switch e := repository.MapErr(err); {
	case e == nil:
		return nil
	case errors.Is(e, repository.ErrNameTaken):
		return ErrCategoryNameTaken
	case errors.Is(e, repository.ErrAlreadyReversed):
		return ErrAlreadyReversed
	case errors.Is(e, repository.ErrAlreadyReplaced):
		return ErrAlreadyReversed
	case errors.Is(e, repository.ErrIdempotencyKey):
		return ErrIdempotencyReused
	case errors.Is(e, repository.ErrCategoryKind):
		return invalid("category_uuid", "kind_mismatch", "the category belongs to the other kind")
	default:
		return e
	}
}

func isNotFound(err error) bool {
	return errors.Is(repository.MapErr(err), repository.ErrNotFound)
}

// ------------------------------------------------------------------ helpers

func parseDate(s string) (time.Time, error) {
	return time.Parse(dateLayout, strings.TrimSpace(s))
}

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
	return pgtype.Date{Time: *t, Valid: true}
}

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
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

func ref(id pgtype.UUID, name pgtype.Text) *model.Ref {
	if !id.Valid {
		return nil
	}
	return &model.Ref{UUID: uuid.UUID(id.Bytes), Name: name.String}
}

func checkText(ve *ValidationError, field, v string, max int) string {
	v = strings.TrimSpace(v)
	if utf8.RuneCountInString(v) > max {
		ve.add(field, "too_long", field+" is too long")
	}
	return v
}

func checkReason(raw string) (string, error) {
	reason := strings.TrimSpace(raw)
	switch n := utf8.RuneCountInString(reason); {
	case n < minReason:
		return "", invalid("reason", "required", "a reason of at least 3 characters is required")
	case n > maxReason:
		return "", invalid("reason", "too_long", "reason is too long")
	}
	return reason, nil
}

// lookupBranch resolves a branch of the organization inside the caller's
// scope; anything else is "not found" on the field.
func lookupBranch(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (int64, bool, error) {
	rows, err := q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: []uuid.UUID{id}})
	if err != nil {
		return 0, false, err
	}
	if len(rows) == 0 || orgctx.CheckBranch(ctx, rows[0].ID) != nil {
		return 0, false, nil
	}
	return rows[0].ID, true, nil
}
