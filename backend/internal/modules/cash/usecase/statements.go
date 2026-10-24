package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Reconciliation limits (TEC-965).
const (
	maxStatementNameLength = 160
	maxReferenceLength     = 120
	// autoMatchWindowDays is how far a book movement may be dated from a
	// statement line to be matched without a human (value dates lag).
	autoMatchWindowDays = 3
	// candidateWindowDays is the window of the manual candidate list.
	candidateWindowDays = 15
	candidateLimit      = 20
	exportLimit         = 100000
)

var (
	ErrStatementNotFound    = errors.New("statement not found")
	ErrLineNotFound         = errors.New("statement line not found")
	ErrStatementClosed      = errors.New("the statement belongs to a closed period")
	ErrStatementHasPostings = errors.New("the statement has lines posted into the book")
	ErrLineState            = errors.New("the line is not in a state that allows this action")
	ErrMovementMatched      = errors.New("the movement is matched by another line already")
	ErrReferenceTaken       = errors.New("a line with this reference was imported already")
	ErrUnreconciledLines    = errors.New("the period has statement lines waiting for reconciliation")
)

// StatementSortColumns / LineSortColumns are the sortable list fields.
var (
	StatementSortColumns = map[string]string{"name": "name", "period_to": "period_to", "created_at": "created_at"}
	LineSortColumns      = map[string]string{"line_date": "line_date", "amount": "amount", "status": "status", "line_no": "line_no"}
)

// --------------------------------------------------------------- statements

type statementFields struct {
	Name, Note     string
	PeriodFrom     pgtype.Date
	PeriodTo       pgtype.Date
	ClosingBalance pgtype.Numeric
}

func (f *statementFields) apply(ve *ValidationError, in model.StatementInput, create bool) {
	if in.Name != nil {
		v := strings.TrimSpace(*in.Name)
		switch {
		case v == "":
			ve.add("name", "required", "name is required")
		case utf8.RuneCountInString(v) > maxStatementNameLength:
			ve.add("name", "too_long", fmt.Sprintf("name must be at most %d characters", maxStatementNameLength))
		default:
			f.Name = v
		}
	} else if create {
		ve.add("name", "required", "name is required")
	}
	if in.Note != nil {
		v := strings.TrimSpace(*in.Note)
		if utf8.RuneCountInString(v) > maxNoteLength {
			ve.add("note", "too_long", "note is too long")
		} else {
			f.Note = v
		}
	}
	for _, p := range []struct {
		field string
		src   *string
		dst   *pgtype.Date
	}{{"period_from", in.PeriodFrom, &f.PeriodFrom}, {"period_to", in.PeriodTo, &f.PeriodTo}} {
		if p.src == nil {
			continue
		}
		if strings.TrimSpace(*p.src) == "" {
			*p.dst = pgtype.Date{}
			continue
		}
		d, err := parseDate(*p.src)
		if err != nil {
			ve.add(p.field, "invalid", p.field+" must be YYYY-MM-DD")
			continue
		}
		*p.dst = dateArg(d)
	}
	if f.PeriodFrom.Valid && f.PeriodTo.Valid && f.PeriodFrom.Time.After(f.PeriodTo.Time) {
		ve.add("period_to", "before_from", "period_to is before period_from")
	}
	if in.ClosingBalance != nil {
		f.ClosingBalance = in.ClosingBalance.Numeric()
	}
}

// CreateStatement opens a statement (bank statement or provider settlement
// report) on a bank or online account of the caller's scope.
func (s *Service) CreateStatement(ctx context.Context, orgID int64, userID *int64, in model.StatementInput) (model.Statement, error) {
	q := s.repo.Q()
	if in.AccountUUID == nil || *in.AccountUUID == uuid.Nil {
		return model.Statement{}, invalid("account_uuid", "required", "account_uuid is required")
	}
	acc, err := s.loadAccount(ctx, q, orgID, *in.AccountUUID)
	if errors.Is(err, ErrNotFound) {
		return model.Statement{}, invalid("account_uuid", "not_found", "account not found")
	}
	if err != nil {
		return model.Statement{}, err
	}
	ve := &ValidationError{}
	if acc.Kind == model.KindCash {
		ve.add("account_uuid", "not_reconcilable", "a till is counted, not reconciled: pick a bank or online account")
	}
	source := model.SourceBank
	if acc.Kind == model.KindOnline {
		source = model.SourceProvider
	}
	if in.Source != nil && strings.TrimSpace(*in.Source) != "" {
		if !slices.Contains(model.StatementSources, *in.Source) {
			ve.add("source", "invalid", "source must be bank or provider")
		} else {
			source = *in.Source
		}
	}
	var f statementFields
	f.apply(ve, in, true)
	if err := ve.orNil(); err != nil {
		return model.Statement{}, err
	}
	row, err := q.InsertCashStatement(ctx, db.InsertCashStatementParams{
		OrganizationID: orgID, CashAccountID: acc.ID, BranchID: acc.BranchID, Source: source, Name: f.Name,
		PeriodFrom: f.PeriodFrom, PeriodTo: f.PeriodTo, ClosingBalance: f.ClosingBalance, Note: f.Note,
		CreatedBy: int8Ptr(userID),
	})
	if err != nil {
		return model.Statement{}, mapRepoErr(err)
	}
	return s.GetStatement(ctx, orgID, row.Uuid)
}

// PatchStatement updates the name, period, stated balance and note of an
// open statement.
func (s *Service) PatchStatement(ctx context.Context, orgID int64, id uuid.UUID, in model.StatementInput) (model.Statement, error) {
	q := s.repo.Q()
	row, err := s.loadStatement(ctx, q, orgID, id)
	if err != nil {
		return model.Statement{}, err
	}
	ve := &ValidationError{}
	if in.AccountUUID != nil && *in.AccountUUID != row.AccountUuid {
		ve.add("account_uuid", "immutable", "the account of a statement cannot change")
	}
	if in.Source != nil && *in.Source != row.Source {
		ve.add("source", "immutable", "the source of a statement cannot change")
	}
	f := statementFields{Name: row.Name, Note: row.Note, PeriodFrom: row.PeriodFrom, PeriodTo: row.PeriodTo, ClosingBalance: row.ClosingBalance}
	f.apply(ve, in, false)
	if err := ve.orNil(); err != nil {
		return model.Statement{}, err
	}
	if row.Status == model.StatementClosed {
		return model.Statement{}, ErrStatementClosed
	}
	n, err := q.UpdateCashStatement(ctx, db.UpdateCashStatementParams{
		Name: f.Name, PeriodFrom: f.PeriodFrom, PeriodTo: f.PeriodTo, ClosingBalance: f.ClosingBalance, Note: f.Note,
		OrganizationID: orgID, ID: row.ID,
	})
	if err != nil {
		return model.Statement{}, mapRepoErr(err)
	}
	if n == 0 {
		return model.Statement{}, ErrStatementNotFound
	}
	return s.GetStatement(ctx, orgID, id)
}

// DeleteStatement removes an open statement and its lines. A statement
// whose lines wrote movements is kept: the book is append-only.
func (s *Service) DeleteStatement(ctx context.Context, orgID int64, id uuid.UUID) (model.Statement, error) {
	q := s.repo.Q()
	row, err := s.loadStatement(ctx, q, orgID, id)
	if err != nil {
		return model.Statement{}, err
	}
	if row.Status == model.StatementClosed {
		return model.Statement{}, ErrStatementClosed
	}
	n, err := q.CountCashStatementPostings(ctx, db.CountCashStatementPostingsParams{OrganizationID: orgID, StatementID: row.ID})
	if err != nil {
		return model.Statement{}, err
	}
	if n > 0 {
		return model.Statement{}, ErrStatementHasPostings
	}
	if _, err := q.DeleteCashStatement(ctx, db.DeleteCashStatementParams{OrganizationID: orgID, ID: row.ID}); err != nil {
		return model.Statement{}, mapRepoErr(err)
	}
	return mapStatement(row), nil
}

// GetStatement returns one statement.
func (s *Service) GetStatement(ctx context.Context, orgID int64, id uuid.UUID) (model.Statement, error) {
	row, err := s.loadStatement(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.Statement{}, err
	}
	return mapStatement(row), nil
}

// ListStatements pages the statements of the caller's scope.
func (s *Service) ListStatements(ctx context.Context, orgID int64, f model.StatementFilter) ([]model.Statement, int64, error) {
	if _, ok := StatementSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	accounts, err := s.accountIDs(ctx, q, orgID, f.Accounts)
	if err != nil {
		return nil, 0, err
	}
	scope := orgctx.BranchScope(ctx)
	rows, err := q.ListCashStatements(ctx, db.ListCashStatementsParams{
		OrganizationID: orgID, BranchIds: scope, AccountIds: accounts, Sources: nilIfEmpty(f.Sources),
		Statuses: nilIfEmpty(f.Statuses), Q: textArg(f.Q), SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountCashStatements(ctx, db.CountCashStatementsParams{
		OrganizationID: orgID, BranchIds: scope, AccountIds: accounts, Sources: nilIfEmpty(f.Sources),
		Statuses: nilIfEmpty(f.Statuses), Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Statement, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapStatement(db.GetCashStatementRow(row)))
	}
	return out, total, nil
}

// accountIDs resolves account filter uuids inside the caller's scope; an
// unknown account yields an empty match, never a wider one.
func (s *Service) accountIDs(ctx context.Context, q *db.Queries, orgID int64, ids []uuid.UUID) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out := []int64{}
	for _, id := range ids {
		row, err := s.loadAccount(ctx, q, orgID, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, row.ID)
	}
	return out, nil
}

// StatementSummary returns the reconciliation state of a statement with the
// book balance of its account at the period end against the stated balance.
func (s *Service) StatementSummary(ctx context.Context, orgID int64, id uuid.UUID) (model.StatementSummary, error) {
	q := s.repo.Q()
	row, err := s.loadStatement(ctx, q, orgID, id)
	if err != nil {
		return model.StatementSummary{}, err
	}
	sum, err := q.CashStatementSummary(ctx, db.CashStatementSummaryParams{OrganizationID: orgID, StatementID: row.ID})
	if err != nil {
		return model.StatementSummary{}, err
	}
	out := model.StatementSummary{
		PendingCount: sum.PendingCount, MatchedCount: sum.MatchedCount, DifferenceCount: sum.DifferenceCount,
		PostedCount: sum.PostedCount, IgnoredCount: sum.IgnoredCount,
		TotalIn: money.FromNumericOrZero(sum.TotalIn), TotalOut: money.FromNumericOrZero(sum.TotalOut),
		TotalFee: money.FromNumericOrZero(sum.TotalFee), PendingNet: money.FromNumericOrZero(sum.PendingNet),
		DifferenceTotal: money.FromNumericOrZero(sum.DifferenceTotal),
	}
	if sum.FirstDate.Valid {
		d := formatDate(sum.FirstDate.Time)
		out.FirstDate = &d
	}
	if sum.LastDate.Valid {
		d := formatDate(sum.LastDate.Time)
		out.LastDate = &d
	}
	day := s.Today(ctx)
	switch {
	case row.PeriodTo.Valid:
		day = row.PeriodTo.Time
	case sum.LastDate.Valid:
		day = sum.LastDate.Time
	}
	bal, err := q.CashAccountBalanceAt(ctx, db.CashAccountBalanceAtParams{Day: dateArg(day), OrganizationID: orgID, ID: row.CashAccountID})
	if err != nil {
		return model.StatementSummary{}, err
	}
	out.BalanceDate = formatDate(day)
	out.BookBalance = money.FromNumericOrZero(bal)
	if row.ClosingBalance.Valid {
		stated := money.FromNumericOrZero(row.ClosingBalance)
		diff := stated.Sub(out.BookBalance)
		out.ClosingBalance, out.BalanceDifference = &stated, &diff
	}
	out.Reconciled = sum.PendingCount == 0 && sum.DifferenceCount == 0 &&
		(out.BalanceDifference == nil || out.BalanceDifference.IsZero())
	return out, nil
}

func (s *Service) loadStatement(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetCashStatementRow, error) {
	row, err := q.GetCashStatement(ctx, db.GetCashStatementParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return row, ErrStatementNotFound
		}
		return row, err
	}
	if !allowsBranch(ctx, row.BranchID) {
		return db.GetCashStatementRow{}, ErrStatementNotFound
	}
	return row, nil
}

func mapStatement(row db.GetCashStatementRow) model.Statement {
	st := model.Statement{
		UUID:    row.Uuid,
		Account: model.AccountRef{UUID: row.AccountUuid, Name: row.AccountName, Kind: row.AccountKind},
		Source:  row.Source, Name: row.Name, Currency: row.AccountCurrency, Note: row.Note, Status: row.Status,
		ClosingUUID: uuidPtr(row.ClosingUuid), LineCount: row.LineCount, OpenCount: row.OpenCount,
		CreatedBy: userRef(row.CreatorUuid, row.CreatorName, row.CreatorSurname),
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.BranchUuid.Valid {
		st.Branch = &model.BranchRef{UUID: uuid.UUID(row.BranchUuid.Bytes), Name: row.BranchName.String}
	}
	if row.PeriodFrom.Valid {
		d := formatDate(row.PeriodFrom.Time)
		st.PeriodFrom = &d
	}
	if row.PeriodTo.Valid {
		d := formatDate(row.PeriodTo.Time)
		st.PeriodTo = &d
	}
	if row.ClosingBalance.Valid {
		m := money.FromNumericOrZero(row.ClosingBalance)
		st.ClosingBalance = &m
	}
	return st
}

// -------------------------------------------------------------------- lines

// lineFields are the validated columns of a new line.
type lineFields struct {
	Date                   time.Time
	Direction, Type        string
	Amount, Fee            money.Money
	Description, Reference string
	BalanceAfter           pgtype.Numeric
}

func validateLine(in model.LineInput) (lineFields, error) {
	ve := &ValidationError{}
	var f lineFields
	d, err := parseDate(in.LineDate)
	if err != nil {
		ve.add("line_date", "invalid", "line_date must be YYYY-MM-DD")
	} else {
		f.Date = d
	}
	switch in.Direction {
	case model.DirectionIn, model.DirectionOut:
		f.Direction = in.Direction
	default:
		ve.add("direction", "invalid", "direction must be in or out")
	}
	checkAmount(ve, "amount", in.Amount)
	f.Amount = in.Amount
	if in.Fee != nil {
		if in.Fee.IsNegative() {
			ve.add("fee", "negative", "fee cannot be negative")
		} else {
			f.Fee = *in.Fee
		}
	}
	f.Type = model.LineTypeOther
	if t := strings.TrimSpace(in.LineType); t != "" {
		if !slices.Contains(model.LineTypes, t) {
			ve.add("line_type", "invalid", "line_type must be one of "+strings.Join(model.LineTypes, ", "))
		} else {
			f.Type = t
		}
	}
	f.Description = checkDescription(ve, "description", in.Description, false)
	f.Reference = strings.TrimSpace(in.Reference)
	if utf8.RuneCountInString(f.Reference) > maxReferenceLength {
		ve.add("reference", "too_long", "reference is too long")
	}
	if in.BalanceAfter != nil {
		f.BalanceAfter = in.BalanceAfter.Numeric()
	}
	return f, ve.orNil()
}

// LinePreview is the dry run of one line (import preview).
type LinePreview struct {
	Date, Direction, Type, Amount, Fee string
	// Matched is true when the line would be matched with the book on the spot.
	Matched bool
}

// PreviewLine validates a line against its statement without writing it
// (import dry run) and reports whether it would be matched on the spot.
func (s *Service) PreviewLine(ctx context.Context, orgID int64, statementID uuid.UUID, in model.LineInput) (LinePreview, error) {
	q := s.repo.Q()
	st, err := s.loadStatement(ctx, q, orgID, statementID)
	if err != nil {
		return LinePreview{}, err
	}
	f, err := validateLine(in)
	if err != nil {
		return LinePreview{}, err
	}
	if st.Status == model.StatementClosed {
		return LinePreview{}, ErrStatementClosed
	}
	candidate, _, err := s.autoCandidate(ctx, q, orgID, st.CashAccountID, f.Direction, f.Amount.Numeric(), f.Date)
	if err != nil {
		return LinePreview{}, err
	}
	return LinePreview{
		Date: formatDate(f.Date), Direction: f.Direction, Type: f.Type, Amount: f.Amount.String(), Fee: f.Fee.String(),
		Matched: candidate != nil,
	}, nil
}

// AddLine appends a line to an open statement (manual add and import) and
// tries to match it with the book on the spot.
func (s *Service) AddLine(ctx context.Context, orgID int64, userID *int64, statementID uuid.UUID, in model.LineInput) (model.StatementLine, error) {
	q := s.repo.Q()
	st, err := s.loadStatement(ctx, q, orgID, statementID)
	if err != nil {
		return model.StatementLine{}, err
	}
	f, err := validateLine(in)
	if err != nil {
		return model.StatementLine{}, err
	}
	if st.Status == model.StatementClosed {
		return model.StatementLine{}, ErrStatementClosed
	}
	var id uuid.UUID
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		row, err := q.InsertCashStatementLine(ctx, db.InsertCashStatementLineParams{
			OrganizationID: orgID, StatementID: st.ID, CashAccountID: st.CashAccountID, LineDate: dateArg(f.Date),
			Direction: f.Direction, Amount: f.Amount.Numeric(), Fee: f.Fee.Numeric(), LineType: f.Type,
			Description: f.Description, Reference: f.Reference, BalanceAfter: f.BalanceAfter,
		})
		if err != nil {
			return mapRepoErr(err)
		}
		id = row.Uuid
		if err := q.RefreshCashStatementPeriod(ctx, db.RefreshCashStatementPeriodParams{OrganizationID: orgID, StatementID: st.ID}); err != nil {
			return err
		}
		line, err := q.LockCashStatementLine(ctx, db.LockCashStatementLineParams{OrganizationID: orgID, ID: row.ID})
		if err != nil {
			return err
		}
		_, err = s.autoMatchLine(ctx, q, orgID, userID, line)
		return err
	})
	if err != nil {
		return model.StatementLine{}, err
	}
	return s.GetLine(ctx, orgID, id)
}

// RemoveLine deletes a line that wrote nothing into the book (import
// rollback). A matched line is unlinked first; a posted line stays.
func (s *Service) RemoveLine(ctx context.Context, orgID int64, userID *int64, id uuid.UUID) error {
	q := s.repo.Q()
	row, err := s.loadLine(ctx, q, orgID, id)
	if err != nil {
		return err
	}
	if row.StatementStatus == model.StatementClosed {
		return ErrStatementClosed
	}
	return s.repo.InTx(ctx, func(q *db.Queries) error {
		line, err := q.LockCashStatementLine(ctx, db.LockCashStatementLineParams{OrganizationID: orgID, ID: row.ID})
		if err != nil {
			return err
		}
		if line.Status == model.LinePosted {
			return ErrLineState
		}
		if row.FeeUuid.Valid {
			if err := s.reverseByUUID(ctx, q, orgID, uuid.UUID(row.FeeUuid.Bytes), "import rollback", userID); err != nil {
				return err
			}
		}
		if _, err := q.ResetCashStatementLine(ctx, db.ResetCashStatementLineParams{Note: "", OrganizationID: orgID, ID: line.ID}); err != nil {
			return err
		}
		n, err := q.DeleteCashStatementLine(ctx, db.DeleteCashStatementLineParams{OrganizationID: orgID, ID: line.ID})
		if err != nil {
			return mapRepoErr(err)
		}
		if n == 0 {
			return ErrLineState
		}
		return nil
	})
}

// GetLine returns one line.
func (s *Service) GetLine(ctx context.Context, orgID int64, id uuid.UUID) (model.StatementLine, error) {
	row, err := s.loadLine(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.StatementLine{}, err
	}
	return mapLine(row), nil
}

type lineArgs struct {
	scope, statements []int64
	statuses, types   []string
	direction, q      pgtype.Text
}

func (s *Service) lineArgs(ctx context.Context, q *db.Queries, orgID int64, f model.LineFilter) (lineArgs, error) {
	a := lineArgs{
		scope: orgctx.BranchScope(ctx), statuses: nilIfEmpty(f.Statuses), types: nilIfEmpty(f.LineTypes),
		direction: textArg(f.Direction), q: textArg(f.Q),
	}
	if len(f.Statements) > 0 {
		a.statements = []int64{}
		for _, id := range f.Statements {
			row, err := s.loadStatement(ctx, q, orgID, id)
			if errors.Is(err, ErrStatementNotFound) {
				continue
			}
			if err != nil {
				return a, err
			}
			a.statements = append(a.statements, row.ID)
		}
	}
	return a, nil
}

// ListLines pages statement lines (one statement, or every statement of
// the scope).
func (s *Service) ListLines(ctx context.Context, orgID int64, f model.LineFilter) ([]model.StatementLine, int64, error) {
	if _, ok := LineSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	a, err := s.lineArgs(ctx, q, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListCashStatementLines(ctx, db.ListCashStatementLinesParams{
		OrganizationID: orgID, BranchIds: a.scope, StatementIds: a.statements, Statuses: a.statuses, LineTypes: a.types,
		Direction: a.direction, Q: a.q, SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountCashStatementLines(ctx, db.CountCashStatementLinesParams{
		OrganizationID: orgID, BranchIds: a.scope, StatementIds: a.statements, Statuses: a.statuses, LineTypes: a.types,
		Direction: a.direction, Q: a.q,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.StatementLine, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapLine(db.GetCashStatementLineRow(row)))
	}
	return out, total, nil
}

// ExportLines returns every line matching a filter.
func (s *Service) ExportLines(ctx context.Context, orgID int64, f model.LineFilter) ([]model.StatementLine, error) {
	f.Limit, f.Offset = exportLimit, 0
	items, _, err := s.ListLines(ctx, orgID, f)
	return items, err
}

// LineFacets returns the status counts of a statement's lines.
func (s *Service) LineFacets(ctx context.Context, orgID int64, statementID uuid.UUID) (model.LineFacets, error) {
	out := model.LineFacets{Status: []model.Facet{}}
	q := s.repo.Q()
	st, err := s.loadStatement(ctx, q, orgID, statementID)
	if err != nil {
		return out, err
	}
	rows, err := q.CashStatementLineStatusFacets(ctx, db.CashStatementLineStatusFacetsParams{OrganizationID: orgID, StatementID: st.ID})
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		out.Status = append(out.Status, model.Facet{Value: row.Value, Count: row.Count})
	}
	return out, nil
}

// Candidates lists the book movements a line may be matched with: same
// account and direction, dated within ±candidateWindowDays, closest amount
// first.
func (s *Service) Candidates(ctx context.Context, orgID int64, id uuid.UUID, search string) ([]model.MatchCandidate, error) {
	q := s.repo.Q()
	row, err := s.loadLine(ctx, q, orgID, id)
	if err != nil {
		return nil, err
	}
	day := row.LineDate.Time
	rows, err := q.ListCashMatchCandidates(ctx, db.ListCashMatchCandidatesParams{
		Amount: row.Amount, LineDate: row.LineDate, OrganizationID: orgID, CashAccountID: row.CashAccountID,
		Direction: row.Direction, DateFrom: dateArg(day.AddDate(0, 0, -candidateWindowDays)),
		DateTo: dateArg(day.AddDate(0, 0, candidateWindowDays)), ExactAmount: false, Q: textArg(strings.TrimSpace(search)),
		LimitCount: candidateLimit,
	})
	if err != nil {
		return nil, err
	}
	amount := money.FromNumericOrZero(row.Amount)
	out := make([]model.MatchCandidate, 0, len(rows))
	for _, c := range rows {
		m := money.FromNumericOrZero(c.Amount)
		out = append(out, model.MatchCandidate{
			UUID: c.Uuid, Kind: c.Kind, Direction: c.Direction, Amount: m, Currency: c.Currency,
			MovementDate: formatDate(c.MovementDate.Time), Description: c.Description, SourceType: c.SourceType,
			CounterAccount: c.CounterAccountName.String, AmountDelta: money.FromNumericOrZero(c.AmountDelta),
			DayDelta: c.DayDelta, Exact: m.Equal(amount),
		})
	}
	return out, nil
}

// autoCandidate finds the one book movement of the same amount dated
// within the auto-match window; two movements of equal closeness leave the
// decision to a human.
func (s *Service) autoCandidate(ctx context.Context, q *db.Queries, orgID, accountID int64, direction string, amount pgtype.Numeric, day time.Time) (*db.ListCashMatchCandidatesRow, int, error) {
	rows, err := q.ListCashMatchCandidates(ctx, db.ListCashMatchCandidatesParams{
		Amount: amount, LineDate: dateArg(day), OrganizationID: orgID, CashAccountID: accountID, Direction: direction,
		DateFrom: dateArg(day.AddDate(0, 0, -autoMatchWindowDays)), DateTo: dateArg(day.AddDate(0, 0, autoMatchWindowDays)),
		ExactAmount: true, LimitCount: 2,
	})
	if err != nil {
		return nil, 0, err
	}
	switch {
	case len(rows) == 0:
		return nil, 0, nil
	case len(rows) == 1, rows[0].DayDelta < rows[1].DayDelta:
		return &rows[0], len(rows), nil
	default:
		return nil, len(rows), nil
	}
}

// autoMatchLine matches a pending line with its single candidate. The fee
// of a provider line is posted with the match; when the book refuses it
// (closed day, archived account) the line stays pending for a human.
func (s *Service) autoMatchLine(ctx context.Context, q *db.Queries, orgID int64, userID *int64, line db.LockCashStatementLineRow) (bool, error) {
	if line.Status != model.LinePending {
		return false, nil
	}
	c, _, err := s.autoCandidate(ctx, q, orgID, line.CashAccountID, line.Direction, line.Amount, line.LineDate.Time)
	if err != nil || c == nil {
		return false, err
	}
	attempt, feeID := line.PostCount, line.FeeMovementID
	if money.FromNumericOrZero(line.Fee).IsPositive() && !feeID.Valid {
		if err := s.checkPostable(ctx, q, orgID, line.CashAccountID, line.LineDate.Time); err != nil {
			if IsValidation(err) || errors.Is(err, ErrDayClosed) || errors.Is(err, ErrAccountArchived) {
				return false, nil
			}
			return false, err
		}
		attempt++
		feeID, err = s.postFee(ctx, q, orgID, userID, line, attempt)
		if err != nil {
			return false, err
		}
	}
	_, err = q.SetCashStatementLineMatched(ctx, db.SetCashStatementLineMatchedParams{
		Status: model.LineMatched, MatchMethod: model.MatchAuto, MatchedMovementID: pgtype.Int8{Int64: c.ID, Valid: true},
		DifferenceAmount: money.Zero().Numeric(), FeeMovementID: feeID, PostCount: attempt, Note: "",
		ResolvedBy: int8Ptr(userID), OrganizationID: orgID, ID: line.ID,
	})
	if err != nil {
		return false, mapRepoErr(err)
	}
	return true, nil
}

// checkPostable runs the writable checks of a movement dated on a day
// without inserting anything.
func (s *Service) checkPostable(ctx context.Context, q *db.Queries, orgID, accountID int64, day time.Time) error {
	locked, err := s.lock(ctx, q, orgID, accountID)
	if err != nil {
		return err
	}
	return s.checkWritable(ctx, q, orgID, locked[accountID], day)
}

// AutoMatch re-runs the automatic matching over the pending lines of a
// statement (after new movements were posted into the book).
func (s *Service) AutoMatch(ctx context.Context, orgID int64, userID *int64, statementID uuid.UUID) (model.AutoMatchResult, error) {
	q := s.repo.Q()
	st, err := s.loadStatement(ctx, q, orgID, statementID)
	if err != nil {
		return model.AutoMatchResult{}, err
	}
	if st.Status == model.StatementClosed {
		return model.AutoMatchResult{}, ErrStatementClosed
	}
	ids, err := q.ListCashStatementLineIDs(ctx, db.ListCashStatementLineIDsParams{OrganizationID: orgID, StatementID: st.ID})
	if err != nil {
		return model.AutoMatchResult{}, err
	}
	out := model.AutoMatchResult{Checked: len(ids)}
	for _, id := range ids {
		err := s.repo.InTx(ctx, func(q *db.Queries) error {
			line, err := q.LockCashStatementLine(ctx, db.LockCashStatementLineParams{OrganizationID: orgID, ID: id.ID})
			if err != nil {
				return err
			}
			matched, err := s.autoMatchLine(ctx, q, orgID, userID, line)
			if matched {
				out.Matched++
			}
			return err
		})
		if err != nil {
			return out, err
		}
	}
	out.Pending = out.Checked - out.Matched
	return out, nil
}

// MatchLine links a line to a movement of its account by hand. A movement
// of another amount leaves the line in difference with the gap recorded.
func (s *Service) MatchLine(ctx context.Context, orgID int64, userID *int64, id uuid.UUID, in model.MatchInput) (model.StatementLine, error) {
	q := s.repo.Q()
	row, err := s.loadLine(ctx, q, orgID, id)
	if err != nil {
		return model.StatementLine{}, err
	}
	if in.MovementUUID == uuid.Nil {
		return model.StatementLine{}, invalid("movement_uuid", "required", "movement_uuid is required")
	}
	ve := &ValidationError{}
	note := checkDescription(ve, "note", in.Note, false)
	mv, err := s.loadMovement(ctx, q, orgID, in.MovementUUID)
	switch {
	case errors.Is(err, ErrMovementNotFound):
		ve.add("movement_uuid", "not_found", "movement not found")
	case err != nil:
		return model.StatementLine{}, err
	case mv.CashAccountID != row.CashAccountID:
		ve.add("movement_uuid", "other_account", "the movement belongs to another account")
	case mv.Direction != row.Direction:
		ve.add("movement_uuid", "direction", "the movement goes the other way")
	case mv.Kind == model.MovementReversal || mv.ReversedByUuid.Valid:
		ve.add("movement_uuid", "reversed", "a reversal or a reversed movement cannot be matched")
	}
	if err := ve.orNil(); err != nil {
		return model.StatementLine{}, err
	}
	if row.StatementStatus == model.StatementClosed {
		return model.StatementLine{}, ErrStatementClosed
	}
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		line, err := q.LockCashStatementLine(ctx, db.LockCashStatementLineParams{OrganizationID: orgID, ID: row.ID})
		if err != nil {
			return err
		}
		if !slices.Contains([]string{model.LinePending, model.LineMatched, model.LineDifference}, line.Status) {
			return ErrLineState
		}
		amount, mvAmount := money.FromNumericOrZero(line.Amount), money.FromNumericOrZero(mv.Amount)
		status, diff := model.LineMatched, money.Zero()
		if !amount.Equal(mvAmount) {
			status, diff = model.LineDifference, amount.Sub(mvAmount)
		}
		attempt, feeID := line.PostCount, line.FeeMovementID
		if money.FromNumericOrZero(line.Fee).IsPositive() && !feeID.Valid {
			attempt++
			if feeID, err = s.postFee(ctx, q, orgID, userID, line, attempt); err != nil {
				return err
			}
		}
		_, err = q.SetCashStatementLineMatched(ctx, db.SetCashStatementLineMatchedParams{
			Status: status, MatchMethod: model.MatchManual, MatchedMovementID: pgtype.Int8{Int64: mv.ID, Valid: true},
			DifferenceAmount: diff.Numeric(), FeeMovementID: feeID, PostCount: attempt, Note: note,
			ResolvedBy: int8Ptr(userID), OrganizationID: orgID, ID: line.ID,
		})
		return mapRepoErr(err)
	})
	if err != nil {
		return model.StatementLine{}, err
	}
	return s.GetLine(ctx, orgID, id)
}

// ResetLine takes a matched, difference or ignored line back to pending. A
// fee posted with the match is reversed. A posted line is undone with
// ReverseLinePosting.
func (s *Service) ResetLine(ctx context.Context, orgID int64, userID *int64, id uuid.UUID, in model.ReverseInput) (model.StatementLine, error) {
	q := s.repo.Q()
	row, err := s.loadLine(ctx, q, orgID, id)
	if err != nil {
		return model.StatementLine{}, err
	}
	ve := &ValidationError{}
	reason := checkDescription(ve, "reason", in.Reason, false)
	if err := ve.orNil(); err != nil {
		return model.StatementLine{}, err
	}
	if row.StatementStatus == model.StatementClosed {
		return model.StatementLine{}, ErrStatementClosed
	}
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		line, err := q.LockCashStatementLine(ctx, db.LockCashStatementLineParams{OrganizationID: orgID, ID: row.ID})
		if err != nil {
			return err
		}
		if !slices.Contains([]string{model.LineMatched, model.LineDifference, model.LineIgnored}, line.Status) {
			return ErrLineState
		}
		if line.FeeMovementID.Valid && row.FeeUuid.Valid {
			if err := s.reverseByUUID(ctx, q, orgID, uuid.UUID(row.FeeUuid.Bytes), reason, userID); err != nil {
				return err
			}
		}
		_, err = q.ResetCashStatementLine(ctx, db.ResetCashStatementLineParams{Note: reason, OrganizationID: orgID, ID: line.ID})
		return err
	})
	if err != nil {
		return model.StatementLine{}, err
	}
	return s.GetLine(ctx, orgID, id)
}

// IgnoreLine leaves a pending or difference line out of the reconciliation
// with a reason (a duplicate, a provider's own bookkeeping row).
func (s *Service) IgnoreLine(ctx context.Context, orgID int64, userID *int64, id uuid.UUID, in model.ReverseInput) (model.StatementLine, error) {
	q := s.repo.Q()
	row, err := s.loadLine(ctx, q, orgID, id)
	if err != nil {
		return model.StatementLine{}, err
	}
	reason, err := requireReason(in.Reason)
	if err != nil {
		return model.StatementLine{}, err
	}
	if row.StatementStatus == model.StatementClosed {
		return model.StatementLine{}, ErrStatementClosed
	}
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		line, err := q.LockCashStatementLine(ctx, db.LockCashStatementLineParams{OrganizationID: orgID, ID: row.ID})
		if err != nil {
			return err
		}
		if !slices.Contains([]string{model.LinePending, model.LineMatched, model.LineDifference}, line.Status) {
			return ErrLineState
		}
		if line.FeeMovementID.Valid && row.FeeUuid.Valid {
			if err := s.reverseByUUID(ctx, q, orgID, uuid.UUID(row.FeeUuid.Bytes), reason, userID); err != nil {
				return err
			}
			if _, err := q.ResetCashStatementLine(ctx, db.ResetCashStatementLineParams{Note: "", OrganizationID: orgID, ID: line.ID}); err != nil {
				return err
			}
		}
		_, err = q.SetCashStatementLineIgnored(ctx, db.SetCashStatementLineIgnoredParams{
			Note: reason, ResolvedBy: int8Ptr(userID), OrganizationID: orgID, ID: line.ID,
		})
		return err
	})
	if err != nil {
		return model.StatementLine{}, err
	}
	return s.GetLine(ctx, orgID, id)
}

// postKinds are the module kinds a line of each direction may be posted as.
var postKinds = map[string][]string{
	model.DirectionIn:  {model.MovementIncome, model.MovementCollection},
	model.DirectionOut: {model.MovementExpense, model.MovementRefund, model.MovementStaffPayment},
}

// PostLine writes a pending line into the book through the module posting
// path (PostTx) with an idempotency key per attempt, so a retried request
// never doubles the movement. A payout line, or a counter account, posts a
// transfer between the statement account and the counter account instead.
// The provider fee of the line is posted as an expense beside it.
func (s *Service) PostLine(ctx context.Context, orgID int64, userID *int64, id uuid.UUID, in model.PostLineInput) (model.StatementLine, error) {
	q := s.repo.Q()
	row, err := s.loadLine(ctx, q, orgID, id)
	if err != nil {
		return model.StatementLine{}, err
	}
	ve := &ValidationError{}
	desc := checkDescription(ve, "description", in.Description, false)
	note := checkDescription(ve, "note", in.Note, false)
	if desc == "" {
		desc = row.Description
		if desc == "" {
			desc = row.Reference
		}
	}
	transfer := row.LineType == model.LineTypePayout || in.CounterAccountUUID != nil
	var counter db.GetCashAccountRow
	kind := strings.TrimSpace(in.Kind)
	switch {
	case transfer:
		if in.CounterAccountUUID == nil || *in.CounterAccountUUID == uuid.Nil {
			ve.add("counter_account_uuid", "required", "a payout is posted as a transfer: counter_account_uuid is required")
			break
		}
		counter, err = s.loadAccount(ctx, q, orgID, *in.CounterAccountUUID)
		switch {
		case errors.Is(err, ErrNotFound):
			ve.add("counter_account_uuid", "not_found", "account not found")
		case err != nil:
			return model.StatementLine{}, err
		case counter.ID == row.CashAccountID:
			ve.add("counter_account_uuid", "same_account", "the counter account must be another account")
		case counter.Currency != row.AccountCurrency:
			ve.add("counter_account_uuid", "currency_mismatch", "the accounts have different currencies")
		}
		if kind != "" {
			ve.add("kind", "invalid", "a transfer has no kind")
		}
	case kind == "":
		kind = model.MovementIncome
		if row.Direction == model.DirectionOut {
			kind = model.MovementExpense
			if row.LineType == model.LineTypeRefund {
				kind = model.MovementRefund
			}
		}
	case !slices.Contains(postKinds[row.Direction], kind):
		ve.add("kind", "invalid", "kind must be one of "+strings.Join(postKinds[row.Direction], ", ")+" for this line")
	}
	if err := ve.orNil(); err != nil {
		return model.StatementLine{}, err
	}
	if row.StatementStatus == model.StatementClosed {
		return model.StatementLine{}, ErrStatementClosed
	}
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		line, err := q.LockCashStatementLine(ctx, db.LockCashStatementLineParams{OrganizationID: orgID, ID: row.ID})
		if err != nil {
			return err
		}
		if line.Status != model.LinePending {
			return ErrLineState
		}
		attempt := line.PostCount + 1
		amount := money.FromNumericOrZero(line.Amount)
		lineUUID := line.Uuid
		var posted pgtype.Int8
		if transfer {
			posted, err = s.postTransferLeg(ctx, q, orgID, userID, line, counter, attempt, desc)
		} else {
			mvID, err := s.PostTx(ctx, q, orgID, PostInput{
				AccountUUID: row.AccountUuid, Kind: kind, Amount: amount, Date: line.LineDate.Time, Description: desc,
				SourceType: model.StatementLineSourceType, SourceUUID: &lineUUID,
				IdempotencyKey: lineKey(lineUUID, attempt, ""), CreatedBy: userID,
			})
			if err != nil {
				return err
			}
			posted, err = s.movementID(ctx, q, orgID, mvID)
			if err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
		feeID := line.FeeMovementID
		if money.FromNumericOrZero(line.Fee).IsPositive() && !feeID.Valid {
			if feeID, err = s.postFee(ctx, q, orgID, userID, line, attempt); err != nil {
				return err
			}
		}
		_, err = q.SetCashStatementLinePosted(ctx, db.SetCashStatementLinePostedParams{
			PostedMovementID: posted, FeeMovementID: feeID, PostCount: attempt, Note: note,
			ResolvedBy: int8Ptr(userID), OrganizationID: orgID, ID: line.ID,
		})
		return mapRepoErr(err)
	})
	if err != nil {
		return model.StatementLine{}, err
	}
	return s.GetLine(ctx, orgID, id)
}

// postTransferLeg writes a payout as a transfer: the statement account's
// leg (returned) and the counter account's leg share a transfer group.
func (s *Service) postTransferLeg(ctx context.Context, q *db.Queries, orgID int64, userID *int64, line db.LockCashStatementLineRow, counter db.GetCashAccountRow, attempt int32, desc string) (pgtype.Int8, error) {
	locked, err := s.lock(ctx, q, orgID, line.CashAccountID, counter.ID)
	if err != nil {
		return pgtype.Int8{}, err
	}
	own, other := locked[line.CashAccountID], locked[counter.ID]
	date := line.LineDate.Time
	for _, a := range []lockedAccount{own, other} {
		if err := s.checkWritable(ctx, q, orgID, a, date); err != nil {
			return pgtype.Int8{}, err
		}
	}
	amount := money.FromNumericOrZero(line.Amount)
	src, dst := own, other
	if line.Direction == model.DirectionIn {
		src, dst = other, own
	}
	if err := s.checkFunds(ctx, q, orgID, src, model.DirectionOut, amount); err != nil {
		return pgtype.Int8{}, err
	}
	group := uuid.New()
	lineUUID := line.Uuid
	legs := []struct {
		acc       lockedAccount
		kind, dir string
		counterID int64
	}{
		{src, model.MovementTransferOut, model.DirectionOut, dst.ID},
		{dst, model.MovementTransferIn, model.DirectionIn, src.ID},
	}
	var ownID uuid.UUID
	for _, leg := range legs {
		suffix := "counter"
		if leg.acc.ID == own.ID {
			suffix = ""
		}
		id, err := s.insert(ctx, q, orgID, leg.acc, movementRow{
			Kind: leg.kind, Direction: leg.dir, Amount: amount, Date: date, Description: desc, TransferGroup: &group,
			CounterAccountID: pgtype.Int8{Int64: leg.counterID, Valid: true}, SourceType: model.StatementLineSourceType,
			SourceUUID: &lineUUID, IdempotencyKey: lineKey(lineUUID, attempt, suffix), CreatedBy: userID,
		})
		if errors.Is(err, repository.ErrIdempotencyKey) {
			return pgtype.Int8{}, ErrIdempotencyReused
		}
		if err != nil {
			return pgtype.Int8{}, err
		}
		if leg.acc.ID == own.ID {
			ownID = id
		}
	}
	return s.movementID(ctx, q, orgID, ownID)
}

// postFee writes the provider commission of a line as an expense of its
// account, dated on the line.
func (s *Service) postFee(ctx context.Context, q *db.Queries, orgID int64, userID *int64, line db.LockCashStatementLineRow, attempt int32) (pgtype.Int8, error) {
	acc, err := q.GetCashAccountByID(ctx, db.GetCashAccountByIDParams{OrganizationID: orgID, ID: line.CashAccountID})
	if err != nil {
		return pgtype.Int8{}, err
	}
	desc := strings.TrimSpace("Komisyon · " + firstNonEmpty(line.Description, line.Reference))
	lineUUID := line.Uuid
	id, err := s.PostTx(ctx, q, orgID, PostInput{
		AccountUUID: acc.Uuid, Kind: model.MovementExpense, Amount: money.FromNumericOrZero(line.Fee), Date: line.LineDate.Time,
		Description: desc, SourceType: model.StatementLineSourceType, SourceUUID: &lineUUID,
		IdempotencyKey: lineKey(lineUUID, attempt, "fee"), CreatedBy: userID,
	})
	if err != nil {
		return pgtype.Int8{}, err
	}
	return s.movementID(ctx, q, orgID, id)
}

// ReverseLinePosting undoes a posted line: every movement it wrote (both
// legs of a payout transfer, the fee) is reversed and the line goes
// back to pending. The next posting uses a new idempotency key.
func (s *Service) ReverseLinePosting(ctx context.Context, orgID int64, userID *int64, id uuid.UUID, in model.ReverseInput) (model.StatementLine, error) {
	q := s.repo.Q()
	row, err := s.loadLine(ctx, q, orgID, id)
	if err != nil {
		return model.StatementLine{}, err
	}
	reason, err := requireReason(in.Reason)
	if err != nil {
		return model.StatementLine{}, err
	}
	if row.StatementStatus == model.StatementClosed {
		return model.StatementLine{}, ErrStatementClosed
	}
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		line, err := q.LockCashStatementLine(ctx, db.LockCashStatementLineParams{OrganizationID: orgID, ID: row.ID})
		if err != nil {
			return err
		}
		if line.Status != model.LinePosted || !row.PostedUuid.Valid {
			return ErrLineState
		}
		ids := []uuid.UUID{uuid.UUID(row.PostedUuid.Bytes)}
		if row.PostedTransferGroup.Valid {
			legs, err := q.ListCashTransferLegs(ctx, db.ListCashTransferLegsParams{OrganizationID: orgID, TransferGroup: row.PostedTransferGroup})
			if err != nil {
				return err
			}
			ids = ids[:0]
			for _, leg := range legs {
				ids = append(ids, leg.Uuid)
			}
		}
		if row.FeeUuid.Valid {
			ids = append(ids, uuid.UUID(row.FeeUuid.Bytes))
		}
		for _, mv := range ids {
			if err := s.reverseByUUID(ctx, q, orgID, mv, reason, userID); err != nil {
				return err
			}
		}
		_, err = q.ResetCashStatementLine(ctx, db.ResetCashStatementLineParams{Note: reason, OrganizationID: orgID, ID: line.ID})
		return err
	})
	if err != nil {
		return model.StatementLine{}, err
	}
	return s.GetLine(ctx, orgID, id)
}

// reverseByUUID posts the opposite of a movement (a movement reversed
// already is left alone). The caller authorized the line, so the branch
// scope is not checked again.
func (s *Service) reverseByUUID(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID, reason string, userID *int64) error {
	mv, err := q.GetCashMovement(ctx, db.GetCashMovementParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return ErrMovementNotFound
		}
		return err
	}
	if mv.ReversedByUuid.Valid {
		return nil
	}
	locked, err := s.lock(ctx, q, orgID, mv.CashAccountID)
	if err != nil {
		return err
	}
	acc := locked[mv.CashAccountID]
	// The reversal takes the day of the posting while that day is open, so
	// the book balance of the statement period loses the wrong entry; a
	// closed day pushes it to today, like any other correction.
	date := mv.MovementDate.Time
	if err := s.checkOpenDay(ctx, q, orgID, acc, date); err != nil {
		if !errors.Is(err, ErrDayClosed) {
			return err
		}
		date = s.Today(ctx)
		if err := s.checkOpenDay(ctx, q, orgID, acc, date); err != nil {
			return err
		}
	}
	dir, amount := opposite(mv.Direction), money.FromNumericOrZero(mv.Amount)
	if err := s.checkFunds(ctx, q, orgID, acc, dir, amount); err != nil {
		return err
	}
	reverses := mv.ID
	_, err = s.insert(ctx, q, orgID, acc, movementRow{
		Kind: model.MovementReversal, Direction: dir, Amount: amount, Date: date, Description: reason,
		ReversesID: &reverses, SourceType: mv.SourceType, SourceUUID: uuidPtr(mv.SourceUuid), CreatedBy: userID,
	})
	return err
}

func (s *Service) movementID(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (pgtype.Int8, error) {
	mv, err := q.GetCashMovement(ctx, db.GetCashMovementParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return pgtype.Int8{}, err
	}
	return pgtype.Int8{Int64: mv.ID, Valid: true}, nil
}

// lineKey is the idempotency key of one posting attempt of a line
// (cash_statement_line:{uuid}:{attempt}[:fee|:counter]).
func lineKey(id uuid.UUID, attempt int32, suffix string) string {
	key := fmt.Sprintf("cash_statement_line:%s:%d", id, attempt)
	if suffix != "" {
		key += ":" + suffix
	}
	return key
}

func requireReason(raw string) (string, error) {
	reason := strings.TrimSpace(raw)
	if utf8.RuneCountInString(reason) < minReasonLength {
		return "", invalid("reason", "required", "a reason of at least 3 characters is required")
	}
	if utf8.RuneCountInString(reason) > maxDescriptionLength {
		return "", invalid("reason", "too_long", "reason is too long")
	}
	return reason, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (s *Service) loadLine(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetCashStatementLineRow, error) {
	row, err := q.GetCashStatementLine(ctx, db.GetCashStatementLineParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return row, ErrLineNotFound
		}
		return row, err
	}
	if !allowsBranch(ctx, row.BranchID) {
		return db.GetCashStatementLineRow{}, ErrLineNotFound
	}
	return row, nil
}

func mapLine(row db.GetCashStatementLineRow) model.StatementLine {
	l := model.StatementLine{
		UUID:      row.Uuid,
		Statement: model.StatementRef{UUID: row.StatementUuid, Status: row.StatementStatus},
		Account:   model.AccountRef{UUID: row.AccountUuid, Name: row.AccountName, Kind: row.AccountKind},
		LineNo:    row.LineNo, LineDate: formatDate(row.LineDate.Time), Direction: row.Direction,
		Amount: money.FromNumericOrZero(row.Amount), Fee: money.FromNumericOrZero(row.Fee), LineType: row.LineType,
		Description: row.Description, Reference: row.Reference, Currency: row.AccountCurrency, Status: row.Status,
		MatchMethod: row.MatchMethod, DifferenceAmount: money.FromNumericOrZero(row.DifferenceAmount),
		FeeMovementUUID: uuidPtr(row.FeeUuid), PostCount: row.PostCount, Note: row.Note,
		ResolvedBy: userRef(row.ResolverUuid, row.ResolverName, row.ResolverSurname),
		CreatedAt:  row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.BalanceAfter.Valid {
		m := money.FromNumericOrZero(row.BalanceAfter)
		l.BalanceAfter = &m
	}
	if row.MatchedUuid.Valid {
		m := money.FromNumericOrZero(row.MatchedAmount)
		l.MatchedMovement = &model.MovementRef{
			UUID: uuid.UUID(row.MatchedUuid.Bytes), Kind: row.MatchedKind.String, MovementDate: formatDate(row.MatchedDate.Time),
			Amount: &m, Description: row.MatchedDescription.String,
		}
	}
	if row.PostedUuid.Valid {
		l.PostedMovement = &model.MovementRef{
			UUID: uuid.UUID(row.PostedUuid.Bytes), Kind: row.PostedKind.String, TransferGroup: uuidPtr(row.PostedTransferGroup),
		}
	}
	if row.ResolvedAt.Valid {
		t := row.ResolvedAt.Time
		l.ResolvedAt = &t
	}
	return l
}
