// Package usecase holds the staff share rules (TEC-917, requirements §11).
// Every method takes the organization id of the caller explicitly.
//
//   - A rule is the agreed rate of a staff member: the default for services,
//     the product sales commission (kept apart, it may be zero) and, when
//     needed, a rate for one service that overrides the service default. A
//     rule is a percentage of the line's net amount (after the discount) or
//     a fixed amount per unit, from a start day on.
//   - When the sales module writes a sale line with a staff member (a
//     completed visit, a stock sale), RecordLineTx writes its share in the
//     same transaction with the rule, rate, base and amount of that moment.
//     A later rate change never rewrites it. Costs and expenses are not
//     deducted. A reversed sale line gets the exact negative share; a
//     corrected one keeps the rate of the line it replaces.
//   - The services of a business owner who works as staff stay in the
//     business share: no share is written for them.
//   - A share is payable once the visit is paid in full (collections minus
//     refunds cover the posted sales); a counter sale is payable at once.
//   - A payout is a staff_payment movement of the cash book (not an
//     expense): it lowers the account and what is owed to the person. It
//     never exceeds what is payable; a wrong payout is reversed.
//
// Reach: tenant.earnings.read sees every staff member inside the branch
// scope, tenant.earnings.own only the caller's own staff record.
package usecase

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Limits.
const (
	maxNoteLength      = 500
	minReasonLength    = 3
	maxIdempotencyKey  = 100
	maxSummaryDays     = 366
	dateLayout         = "2006-01-02"
	payoutSourceType   = "staff_payout"
	payoutIdempotency  = "staff_payout:"
	payoutDescription  = "Personel pay ödemesi"
	reversalPrefixNote = "İptal: "
)

var (
	ErrRuleNotFound      = errors.New("share rule not found")
	ErrPayoutNotFound    = errors.New("payout not found")
	ErrNoStaffRecord     = errors.New("the member has no staff record")
	ErrRuleTaken         = errors.New("the staff member has a rule for this item from this day already")
	ErrAlreadyReversed   = errors.New("the payout was reversed already")
	ErrNotReversible     = errors.New("the record cannot be reversed")
	ErrPayoutTooLarge    = errors.New("the payout is larger than the payable share")
	ErrIdempotencyReused = errors.New("the idempotency key was used for another payout")
	ErrAllBranches       = errors.New("this action needs access to every branch")
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

// Viewer limits a read to the caller's own staff record (OwnUserID set:
// tenant.earnings.own without tenant.earnings.read).
type Viewer struct {
	OwnUserID int64
}

// Service implements the staff share use cases.
type Service struct {
	repo *repository.Postgres
	cash *cashusecase.Service
	now  func() time.Time
}

// New builds the service. cash posts the payouts into the cash book.
func New(repo *repository.Postgres, cash *cashusecase.Service) *Service {
	return &Service{repo: repo, cash: cash, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

// today is the business day in the organization's zone (UTC without a
// request scope).
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

// ownStaff resolves the caller's own staff record for an own-only viewer
// (0 = every staff member; ErrNoStaffRecord when the member has none).
func (s *Service) ownStaff(ctx context.Context, q *db.Queries, orgID int64, v Viewer) (int64, error) {
	if v.OwnUserID == 0 {
		return 0, nil
	}
	st, err := q.GetShareStaffByUser(ctx, db.GetShareStaffByUserParams{OrganizationID: orgID, UserID: int8Of(v.OwnUserID)})
	if err != nil {
		if isNotFound(err) {
			return 0, ErrNoStaffRecord
		}
		return 0, err
	}
	return st.ID, nil
}

// staffIDs resolves staff uuids inside the organization (an unknown uuid
// matches nothing); own narrows them to the caller's record.
func (s *Service) staffIDs(ctx context.Context, q *db.Queries, orgID int64, ids []uuid.UUID, own int64) ([]int64, error) {
	var out []int64
	if len(ids) > 0 {
		rows, err := q.GetStaffIDsByUUIDs(ctx, db.GetStaffIDsByUUIDsParams{OrganizationID: orgID, Uuids: ids})
		if err != nil {
			return nil, err
		}
		out = []int64{}
		for _, r := range rows {
			if own == 0 || r.ID == own {
				out = append(out, r.ID)
			}
		}
	}
	if own != 0 && out == nil {
		out = []int64{own}
	}
	return out, nil
}

// branchIDs resolves branch uuids of a filter (nil = no filter).
func (s *Service) branchIDs(ctx context.Context, q *db.Queries, orgID int64, ids []uuid.UUID) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.ListSalesBranchesByUUID(ctx, db.ListSalesBranchesByUUIDParams{OrganizationID: orgID, Uuids: ids})
	if err != nil {
		return nil, err
	}
	out := []int64{}
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out, nil
}

// scopeArgs is the branch scope of the caller for queries on rows whose
// branch may be NULL (payouts from business-wide accounts).
func scopeArgs(ctx context.Context) (bool, []int64) {
	scope := orgctx.BranchScope(ctx)
	if scope == nil {
		return false, []int64{}
	}
	return true, scope
}

func allowsBranch(ctx context.Context, branch pgtype.Int8) bool {
	if !branch.Valid {
		return orgctx.RequireAllBranches(ctx) == nil
	}
	return orgctx.CheckBranch(ctx, branch.Int64) == nil
}

func checkNote(ve *ValidationError, field, v string) string {
	v = strings.TrimSpace(v)
	if utf8.RuneCountInString(v) > maxNoteLength {
		ve.add(field, "too_long", field+" is too long")
	}
	return v
}

func mapRepoErr(err error) error {
	switch e := repository.MapErr(err); {
	case errors.Is(e, repository.ErrRuleTaken):
		return ErrRuleTaken
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

// time0 is a day before any data (an empty period for totals that only
// need the all-time columns).
var time0 = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// time1st is the first day of t's month.
func time1st(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func parseDate(raw string) (time.Time, error) {
	return time.Parse(dateLayout, strings.TrimSpace(raw))
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

func textArg(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func uuidPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
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

func amountPtr(n pgtype.Numeric) *money.Money {
	if !n.Valid {
		return nil
	}
	m := amountOf(n)
	return &m
}

func ratePtr(n pgtype.Numeric) *money.Rate {
	if !n.Valid {
		return nil
	}
	r, err := money.RateFromNumeric(n)
	if err != nil {
		return nil
	}
	return &r
}

func rateArg(r *money.Rate) pgtype.Numeric {
	if r == nil {
		return pgtype.Numeric{}
	}
	return r.Numeric()
}

func moneyArg(m *money.Money) pgtype.Numeric {
	if m == nil {
		return pgtype.Numeric{}
	}
	return m.Numeric()
}

func userRef(id pgtype.UUID, first, last pgtype.Text) *model.UserRef {
	if !id.Valid {
		return nil
	}
	return &model.UserRef{UUID: uuid.UUID(id.Bytes), Name: first.String, Surname: last.String}
}
