// Package usecase holds the cash business rules. Every method takes the
// organization id of the caller explicitly (from orgctx.Scope in handlers,
// from the job in workers).
//
// Branch scope: an account belongs to a branch or to the whole business
// (branch NULL). A member limited to some branches sees and uses only the
// accounts of those branches; business-wide accounts need every branch.
// Any other account reads as not found.
//
// The cash book is append-only: balances are opening balance + money in -
// money out, a wrong movement is reversed, a transfer is two linked
// movements written in one transaction, and a closed day locks the account
// for movements dated on or before it.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// SearchSpec is the Cmd+K spec id of cash accounts.
const SearchSpec = "cash_accounts"

// Field limits.
const (
	maxNameLength        = 120
	maxBankNameLength    = 120
	maxProviderLength    = 32
	maxNoteLength        = 2000
	maxDescriptionLength = 500
	maxSortOrder         = 100000
	minReasonLength      = 3
	pageLimitMax         = 200
)

var (
	ErrNotFound          = errors.New("cash account not found")
	ErrMovementNotFound  = errors.New("cash movement not found")
	ErrClosingNotFound   = errors.New("closing not found")
	ErrNameTaken         = errors.New("the branch already has an account with this name")
	ErrInsufficientFunds = errors.New("the till does not hold enough money")
	ErrAccountArchived   = errors.New("the account is archived")
	ErrDayClosed         = errors.New("the day is closed for this account")
	ErrAlreadyReversed   = errors.New("the movement was reversed already")
	ErrNotReversible     = errors.New("the movement cannot be reversed")
	ErrDayAlreadyClosed  = errors.New("the day is closed already")
	ErrNotLatestClosing  = errors.New("only the latest closed day can be reopened")
	ErrClosingReopened   = errors.New("the closing was reopened already")
	ErrBalanceNotZero    = errors.New("an account is archived with a zero balance only")
	ErrCurrencyMismatch  = errors.New("the accounts have different currencies")
	ErrIdempotencyReused = errors.New("the idempotency key was used for another posting")
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

// IsValidation reports whether err is a field validation error.
func IsValidation(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// SearchIndexer schedules Cmd+K index updates (fail-soft).
type SearchIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
	EnqueueDelete(ctx context.Context, spec, id string)
}

// Service implements the cash use cases.
type Service struct {
	repo    *repository.Postgres
	indexer SearchIndexer
	now     func() time.Time
}

// New builds the service. indexer may be nil.
func New(repo *repository.Postgres, indexer SearchIndexer) *Service {
	return &Service{repo: repo, indexer: indexer, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

// AccountSortColumns are the sortable account list fields.
var AccountSortColumns = map[string]string{
	"name": "name", "kind": "kind", "balance": "balance", "status": "status",
	"sort_order": "sort_order", "created_at": "created_at",
}

// Today is the business day in the organization's time zone.
func (s *Service) Today(ctx context.Context) time.Time {
	loc := time.UTC
	if scope, ok := orgctx.From(ctx); ok && scope.Timezone != "" {
		if l, err := time.LoadLocation(scope.Timezone); err == nil {
			loc = l
		}
	}
	y, m, d := s.now().In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func currencyOf(ctx context.Context) string {
	if scope, ok := orgctx.From(ctx); ok && scope.Currency != "" {
		return scope.Currency
	}
	return string(money.TRY)
}

// ---------------------------------------------------------------- accounts

type accountArgs struct {
	scope, branches []int64
	headOffice      bool
	kinds, statuses []string
	q               pgtype.Text
}

func (s *Service) accountArgs(ctx context.Context, q *db.Queries, orgID int64, f model.AccountFilter) (accountArgs, error) {
	branches, err := s.filterBranchIDs(ctx, q, orgID, f.Branches)
	if err != nil {
		return accountArgs{}, err
	}
	if f.HeadOffice && branches == nil {
		branches = []int64{}
	}
	return accountArgs{
		scope: orgctx.BranchScope(ctx), branches: branches, headOffice: f.HeadOffice,
		kinds: nilIfEmpty(f.Kinds), statuses: nilIfEmpty(f.Statuses), q: textArg(f.Q),
	}, nil
}

// ListAccounts returns one page of the organization's accounts with balances.
func (s *Service) ListAccounts(ctx context.Context, orgID int64, f model.AccountFilter) ([]model.Account, int64, error) {
	if _, ok := AccountSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	a, err := s.accountArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListCashAccounts(ctx, db.ListCashAccountsParams{
		OrganizationID: orgID, BranchIds: a.scope, FilterBranchIds: a.branches, FilterHeadOffice: a.headOffice,
		Kinds: a.kinds, Statuses: a.statuses, Q: a.q, SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountCashAccounts(ctx, db.CountCashAccountsParams{
		OrganizationID: orgID, BranchIds: a.scope, FilterBranchIds: a.branches, FilterHeadOffice: a.headOffice,
		Kinds: a.kinds, Statuses: a.statuses, Q: a.q,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Account, 0, len(rows))
	for _, row := range rows {
		out = append(out, MapAccount(db.GetCashAccountRow(row)))
	}
	return out, total, nil
}

// ExportAccounts returns every account matching a filter.
func (s *Service) ExportAccounts(ctx context.Context, orgID int64, f model.AccountFilter) ([]model.Account, error) {
	f.Limit, f.Offset = 100000, 0
	items, _, err := s.ListAccounts(ctx, orgID, f)
	return items, err
}

// AccountUUIDs resolves the accounts matching a filter (bulk "all
// matching" scope), capped at limit.
func (s *Service) AccountUUIDs(ctx context.Context, orgID int64, f model.AccountFilter, limit int32) ([]uuid.UUID, error) {
	q := s.repo.Q()
	a, err := s.accountArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, err
	}
	return q.ListCashAccountUUIDs(ctx, db.ListCashAccountUUIDsParams{
		OrganizationID: orgID, BranchIds: a.scope, FilterBranchIds: a.branches, FilterHeadOffice: a.headOffice,
		Kinds: a.kinds, Statuses: a.statuses, Q: a.q, LimitCount: limit,
	})
}

// AccountFacets returns the branches in use with their account counts.
func (s *Service) AccountFacets(ctx context.Context, orgID int64) (model.AccountFacets, error) {
	out := model.AccountFacets{Branch: []model.Facet{}}
	rows, err := s.repo.Q().ListCashAccountBranchFacets(ctx, db.ListCashAccountBranchFacetsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx),
	})
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		if !row.BranchUuid.Valid {
			out.Branch = append(out.Branch, model.Facet{Value: model.HeadOffice, Count: row.Count})
			continue
		}
		out.Branch = append(out.Branch, model.Facet{Value: uuid.UUID(row.BranchUuid.Bytes).String(), Label: row.BranchName.String, Count: row.Count})
	}
	return out, nil
}

// AccountOptions lists every account of the caller's scope (pickers).
func (s *Service) AccountOptions(ctx context.Context, orgID int64) ([]model.AccountOption, error) {
	rows, err := s.repo.Q().ListCashAccountOptions(ctx, db.ListCashAccountOptionsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx),
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.AccountOption, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.AccountOption{
			UUID: row.Uuid, Name: row.Name, Kind: row.Kind, Status: row.Status, Currency: row.Currency,
			BranchName: row.BranchName.String,
		})
	}
	return out, nil
}

// GetAccount returns one account with its balance.
func (s *Service) GetAccount(ctx context.Context, orgID int64, id uuid.UUID) (model.Account, error) {
	q := s.repo.Q()
	row, err := s.loadAccount(ctx, q, orgID, id)
	if err != nil {
		return model.Account{}, err
	}
	out := MapAccount(row)
	active, err := q.CashAccountHasActivity(ctx, db.CashAccountHasActivityParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return model.Account{}, err
	}
	out.HasActivity = active.Bool
	return out, nil
}

// CreateAccount validates and stores a new account.
func (s *Service) CreateAccount(ctx context.Context, orgID int64, userID *int64, in model.AccountInput) (model.Account, error) {
	q := s.repo.Q()
	ve := &ValidationError{}
	f := accountFields{Status: model.StatusActive, OpeningBalance: money.Zero(), OpeningDate: s.Today(ctx)}
	if in.BranchUUID != nil {
		if err := s.resolveBranch(ctx, q, orgID, *in.BranchUUID, &f, ve); err != nil {
			return model.Account{}, err
		}
	} else if orgctx.RequireAllBranches(ctx) != nil {
		// A branch-limited member opens accounts in its branches only.
		ve.add("branch_uuid", "required", "branch_uuid is required")
	}
	if in.Kind == nil {
		ve.add("kind", "required", "kind is required")
	} else if !slices.Contains(model.Kinds, *in.Kind) {
		ve.add("kind", "invalid", "kind must be one of "+strings.Join(model.Kinds, ", "))
	} else {
		f.Kind = *in.Kind
	}
	f.apply(ve, in, true, s.Today(ctx))
	if err := ve.orNil(); err != nil {
		return model.Account{}, err
	}
	if err := s.checkName(ctx, q, orgID, f, nil); err != nil {
		return model.Account{}, err
	}
	created, err := q.CreateCashAccount(ctx, db.CreateCashAccountParams{
		OrganizationID: orgID, BranchID: f.BranchID, Kind: f.Kind, Name: f.Name, Currency: currencyOf(ctx),
		OpeningBalance: f.OpeningBalance.Numeric(), OpeningDate: dateArg(f.OpeningDate), BankName: f.BankName,
		Iban: f.IBAN, Provider: f.Provider, Note: f.Note, SortOrder: f.SortOrder, CreatedBy: int8Ptr(userID),
	})
	if err != nil {
		return model.Account{}, mapRepoErr(err)
	}
	if f.Status == model.StatusArchived {
		if err := q.SetCashAccountStatus(ctx, db.SetCashAccountStatusParams{OrganizationID: orgID, ID: created.ID, Status: f.Status}); err != nil {
			return model.Account{}, err
		}
	}
	s.index(ctx, orgID, created.Uuid)
	return s.GetAccount(ctx, orgID, created.Uuid)
}

// PatchAccount updates the given fields. Branch, kind and currency are
// fixed; the opening balance and date only change while the account has
// no movement and no closing.
func (s *Service) PatchAccount(ctx context.Context, orgID int64, id uuid.UUID, in model.AccountInput) (model.Account, error) {
	q := s.repo.Q()
	row, err := s.loadAccount(ctx, q, orgID, id)
	if err != nil {
		return model.Account{}, err
	}
	f := fieldsFromRow(row)
	ve := &ValidationError{}
	if in.BranchUUID != nil && (!row.BranchUuid.Valid || uuid.UUID(row.BranchUuid.Bytes) != *in.BranchUUID) {
		// Resolve first: another organization's branch is "not found", not "fixed".
		var probe accountFields
		if err := s.resolveBranch(ctx, q, orgID, *in.BranchUUID, &probe, ve); err != nil {
			return model.Account{}, err
		}
		if len(ve.Details) == 0 {
			ve.add("branch_uuid", "immutable", "the branch of an account cannot change")
		}
	}
	if in.Kind != nil && *in.Kind != row.Kind {
		ve.add("kind", "immutable", "the kind of an account cannot change")
	}
	if in.OpeningBalance != nil || in.OpeningDate != nil {
		active, err := q.CashAccountHasActivity(ctx, db.CashAccountHasActivityParams{OrganizationID: orgID, ID: row.ID})
		if err != nil {
			return model.Account{}, err
		}
		if active.Bool {
			if in.OpeningBalance != nil && !in.OpeningBalance.Equal(f.OpeningBalance) {
				ve.add("opening_balance", "locked", "the opening balance is fixed once the account has movements")
			}
			if in.OpeningDate != nil && strings.TrimSpace(*in.OpeningDate) != formatDate(f.OpeningDate) {
				ve.add("opening_date", "locked", "the opening date is fixed once the account has movements")
			}
		}
	}
	statusBefore := f.Status
	f.apply(ve, in, false, s.Today(ctx))
	if err := ve.orNil(); err != nil {
		return model.Account{}, err
	}
	if err := s.checkName(ctx, q, orgID, f, &row.ID); err != nil {
		return model.Account{}, err
	}
	if f.Status == model.StatusArchived && statusBefore != model.StatusArchived {
		if !money.FromNumericOrZero(row.Balance).IsZero() {
			return model.Account{}, ErrBalanceNotZero
		}
	}
	n, err := q.UpdateCashAccount(ctx, db.UpdateCashAccountParams{
		OrganizationID: orgID, ID: row.ID, Name: f.Name, OpeningBalance: f.OpeningBalance.Numeric(),
		OpeningDate: dateArg(f.OpeningDate), BankName: f.BankName, Iban: f.IBAN, Provider: f.Provider,
		Note: f.Note, SortOrder: f.SortOrder,
	})
	if err != nil {
		return model.Account{}, mapRepoErr(err)
	}
	if n == 0 {
		return model.Account{}, ErrNotFound
	}
	if f.Status != statusBefore {
		if err := q.SetCashAccountStatus(ctx, db.SetCashAccountStatusParams{OrganizationID: orgID, ID: row.ID, Status: f.Status}); err != nil {
			return model.Account{}, err
		}
	}
	s.index(ctx, orgID, id)
	return s.GetAccount(ctx, orgID, id)
}

// SetStatus archives or activates an account and returns the previous
// status (bulk engine). Archiving needs a zero balance.
func (s *Service) SetStatus(ctx context.Context, orgID int64, id uuid.UUID, status string) (string, error) {
	row, err := s.loadAccount(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return "", err
	}
	if row.Status == status {
		return row.Status, nil
	}
	if status == model.StatusArchived && !money.FromNumericOrZero(row.Balance).IsZero() {
		return row.Status, ErrBalanceNotZero
	}
	if err := s.repo.Q().SetCashAccountStatus(ctx, db.SetCashAccountStatusParams{OrganizationID: orgID, ID: row.ID, Status: status}); err != nil {
		return row.Status, err
	}
	s.index(ctx, orgID, id)
	return row.Status, nil
}

// Activity lists the audit events of an account (its movements, transfers
// and closings included).
func (s *Service) Activity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	q := s.repo.Q()
	if _, err := s.loadAccount(ctx, q, orgID, id); err != nil {
		return nil, 0, err
	}
	rows, err := q.ListOrganizationResourceActivity(ctx, db.ListOrganizationResourceActivityParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Resource: model.AccountsSlug,
		ResourceUuid: pgtype.UUID{Bytes: id, Valid: true}, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountOrganizationResourceActivity(ctx, db.CountOrganizationResourceActivityParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Resource: model.AccountsSlug,
		ResourceUuid: pgtype.UUID{Bytes: id, Valid: true},
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ActivityEntry, 0, len(rows))
	for _, row := range rows {
		e := model.ActivityEntry{UUID: row.Uuid, Action: row.Action, Payload: json.RawMessage(row.Payload), CreatedAt: row.CreatedAt.Time}
		if row.ActorUuid.Valid {
			e.Actor = &model.ActivityActor{
				UUID: uuid.UUID(row.ActorUuid.Bytes), Name: row.ActorName.String,
				Surname: row.ActorSurname.String, Email: row.ActorEmail.String,
			}
		}
		out = append(out, e)
	}
	return out, total, nil
}

// loadAccount reads one account; accounts outside the caller's branch
// scope (business-wide ones for a branch-limited member) read as not found.
func (s *Service) loadAccount(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetCashAccountRow, error) {
	row, err := q.GetCashAccount(ctx, db.GetCashAccountParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return db.GetCashAccountRow{}, ErrNotFound
		}
		return db.GetCashAccountRow{}, err
	}
	if !allowsBranch(ctx, row.BranchID) {
		return db.GetCashAccountRow{}, ErrNotFound
	}
	return row, nil
}

// allowsBranch applies the branch scope to an account's branch (NULL =
// business-wide: every branch is needed).
func allowsBranch(ctx context.Context, branch pgtype.Int8) bool {
	if !branch.Valid {
		return orgctx.RequireAllBranches(ctx) == nil
	}
	return orgctx.CheckBranch(ctx, branch.Int64) == nil
}

func (s *Service) resolveBranch(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID, f *accountFields, ve *ValidationError) error {
	rows, err := q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: []uuid.UUID{id}})
	if err != nil {
		return err
	}
	if len(rows) == 0 || orgctx.CheckBranch(ctx, rows[0].ID) != nil {
		ve.add("branch_uuid", "not_found", "branch not found")
		return nil
	}
	f.BranchID = pgtype.Int8{Int64: rows[0].ID, Valid: true}
	return nil
}

func (s *Service) checkName(ctx context.Context, q *db.Queries, orgID int64, f accountFields, exclude *int64) error {
	taken, err := q.CashAccountNameTaken(ctx, db.CashAccountNameTakenParams{
		OrganizationID: orgID, BranchID: f.BranchID, Name: f.Name, ExcludeID: int8Ptr(exclude),
	})
	if err != nil {
		return err
	}
	if taken {
		return ErrNameTaken
	}
	return nil
}

func (s *Service) filterBranchIDs(ctx context.Context, q *db.Queries, orgID int64, ids []uuid.UUID) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: ids})
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ID)
	}
	return out, nil
}

func (s *Service) index(ctx context.Context, orgID int64, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
}

// accountFields are the writable columns of an account.
type accountFields struct {
	BranchID                             pgtype.Int8
	Kind, Name, BankName, IBAN, Provider string
	Note, Status                         string
	OpeningBalance                       money.Money
	OpeningDate                          time.Time
	SortOrder                            int32
}

func fieldsFromRow(row db.GetCashAccountRow) accountFields {
	return accountFields{
		BranchID: row.BranchID, Kind: row.Kind, Name: row.Name, BankName: row.BankName, IBAN: row.Iban,
		Provider: row.Provider, Note: row.Note, Status: row.Status,
		OpeningBalance: money.FromNumericOrZero(row.OpeningBalance), OpeningDate: row.OpeningDate.Time,
		SortOrder: row.SortOrder,
	}
}

var ibanRE = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{10,30}$`)

func (f *accountFields) apply(ve *ValidationError, in model.AccountInput, create bool, today time.Time) {
	text := func(field string, src *string, dst *string, max int, required bool) {
		if src == nil {
			if create && required {
				ve.add(field, "required", field+" is required")
			}
			return
		}
		v := strings.TrimSpace(*src)
		switch {
		case required && v == "":
			ve.add(field, "required", field+" is required")
		case utf8.RuneCountInString(v) > max:
			ve.add(field, "too_long", fmt.Sprintf("%s must be at most %d characters", field, max))
		default:
			*dst = v
		}
	}
	text("name", in.Name, &f.Name, maxNameLength, true)
	text("bank_name", in.BankName, &f.BankName, maxBankNameLength, false)
	text("provider", in.Provider, &f.Provider, maxProviderLength, false)
	text("note", in.Note, &f.Note, maxNoteLength, false)
	if in.IBAN != nil {
		v := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(*in.IBAN), " ", ""))
		if v != "" && !ibanRE.MatchString(v) {
			ve.add("iban", "invalid", "iban is not valid")
		} else {
			f.IBAN = v
		}
	}
	if in.OpeningBalance != nil {
		f.OpeningBalance = *in.OpeningBalance
	}
	if f.Kind == model.KindCash && f.OpeningBalance.IsNegative() {
		ve.add("opening_balance", "negative", "a till cannot open with a negative balance")
	}
	if in.OpeningDate != nil {
		d, err := parseDate(*in.OpeningDate)
		switch {
		case err != nil:
			ve.add("opening_date", "invalid", "opening_date must be YYYY-MM-DD")
		case d.After(today):
			ve.add("opening_date", "future", "opening_date cannot be in the future")
		default:
			f.OpeningDate = d
		}
	}
	if in.Status != nil {
		if !slices.Contains(model.Statuses, *in.Status) {
			ve.add("status", "invalid", "status must be active or archived")
		} else {
			f.Status = *in.Status
		}
	}
	if in.SortOrder != nil {
		if *in.SortOrder < 0 || *in.SortOrder > maxSortOrder {
			ve.add("sort_order", "invalid", fmt.Sprintf("sort_order must be between 0 and %d", maxSortOrder))
		} else {
			f.SortOrder = *in.SortOrder
		}
	}
	if f.Kind != model.KindBank && in.IBAN != nil && f.IBAN != "" {
		ve.add("iban", "invalid", "only bank accounts have an iban")
	}
}

// MapAccount maps an account row (list, detail and export share the columns).
func MapAccount(row db.GetCashAccountRow) model.Account {
	a := model.Account{
		UUID: row.Uuid, Kind: row.Kind, Name: row.Name, Currency: row.Currency,
		OpeningBalance: money.FromNumericOrZero(row.OpeningBalance), OpeningDate: formatDate(row.OpeningDate.Time),
		Balance: money.FromNumericOrZero(row.Balance), BankName: row.BankName, IBAN: row.Iban,
		Provider: row.Provider, Note: row.Note, Status: row.Status, SortOrder: row.SortOrder,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.BranchUuid.Valid {
		a.Branch = &model.BranchRef{UUID: uuid.UUID(row.BranchUuid.Bytes), Name: row.BranchName.String, Code: row.BranchCode.String}
	}
	if row.LastClosedDate.Valid {
		d := formatDate(row.LastClosedDate.Time)
		a.LastClosedDate = &d
	}
	return a
}

// ------------------------------------------------------------------ helpers

const dateLayout = "2006-01-02"

func parseDate(s string) (time.Time, error) {
	return time.Parse(dateLayout, strings.TrimSpace(s))
}

func formatDate(t time.Time) string {
	return t.Format(dateLayout)
}

func dateArg(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: true}
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

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func textArg(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func mapRepoErr(err error) error {
	switch e := repository.MapErr(err); {
	case errors.Is(e, repository.ErrNotFound):
		return ErrNotFound
	case errors.Is(e, repository.ErrNameTaken):
		return ErrNameTaken
	case errors.Is(e, repository.ErrAlreadyReversed):
		return ErrAlreadyReversed
	case errors.Is(e, repository.ErrDayClosed):
		return ErrDayClosed
	case errors.Is(e, repository.ErrDayAlreadyClosed):
		return ErrDayAlreadyClosed
	case errors.Is(e, repository.ErrMovementMatched):
		return ErrMovementMatched
	case errors.Is(e, repository.ErrReferenceTaken):
		return ErrReferenceTaken
	default:
		return e
	}
}
