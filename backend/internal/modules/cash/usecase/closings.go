package usecase

import (
	"context"
	"errors"
	"sort"
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

// dayTotals is the state of one account on one day.
type dayTotals struct {
	opening, in, out, expected, adjustment money.Money
	count                                  int64
}

func (s *Service) dayTotals(ctx context.Context, q *db.Queries, orgID int64, accountID int64, opening money.Money, day time.Time) (dayTotals, error) {
	row, err := q.CashAccountDayTotals(ctx, db.CashAccountDayTotalsParams{OrganizationID: orgID, CashAccountID: accountID, Day: dateArg(day)})
	if err != nil {
		return dayTotals{}, err
	}
	t := dayTotals{
		opening: opening.Add(money.FromNumericOrZero(row.BeforeDay)),
		in:      money.FromNumericOrZero(row.DayIn), out: money.FromNumericOrZero(row.DayOut),
		adjustment: money.FromNumericOrZero(row.DayAdjustment), count: row.DayCount,
	}
	t.expected = t.opening.Add(t.in).Sub(t.out)
	return t, nil
}

func (s *Service) closingDate(ctx context.Context, raw *string, opening time.Time) (time.Time, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return s.Today(ctx), nil
	}
	d, err := parseDate(*raw)
	switch {
	case err != nil:
		return d, invalid("business_date", "invalid", "business_date must be YYYY-MM-DD")
	case d.After(s.Today(ctx)):
		return d, invalid("business_date", "future", "a future day cannot be closed")
	case d.Before(opening):
		return d, invalid("business_date", "before_opening", "the day is before the opening date of the account")
	}
	return d, nil
}

// PreviewClosing returns the expected state of a day of a till.
func (s *Service) PreviewClosing(ctx context.Context, orgID int64, accountID uuid.UUID, rawDate *string) (model.ClosingPreview, error) {
	q := s.repo.Q()
	acc, err := s.loadAccount(ctx, q, orgID, accountID)
	if err != nil {
		return model.ClosingPreview{}, err
	}
	if acc.Kind != model.KindCash {
		return model.ClosingPreview{}, ErrNotClosable
	}
	day, err := s.closingDate(ctx, rawDate, acc.OpeningDate.Time)
	if err != nil {
		return model.ClosingPreview{}, err
	}
	t, err := s.dayTotals(ctx, q, orgID, acc.ID, money.FromNumericOrZero(acc.OpeningBalance), day)
	if err != nil {
		return model.ClosingPreview{}, err
	}
	out := model.ClosingPreview{
		BusinessDate: formatDate(day), Currency: acc.Currency, Opening: t.opening, TotalIn: t.in, TotalOut: t.out,
		Expected: t.expected, MovementCount: t.count,
	}
	if acc.LastClosedDate.Valid {
		d := formatDate(acc.LastClosedDate.Time)
		out.LastClosedDate = &d
		out.Closed = !day.After(acc.LastClosedDate.Time)
	}
	return out, nil
}

// Close closes a day of a till with the counted amount. The difference to
// the expected balance is posted as a closing_difference movement so the
// book follows the count; the day (and every earlier day) is locked for
// movements afterwards.
func (s *Service) Close(ctx context.Context, orgID int64, accountID uuid.UUID, userID *int64, in model.ClosingInput) (model.Closing, error) {
	q := s.repo.Q()
	acc, err := s.loadAccount(ctx, q, orgID, accountID)
	if err != nil {
		return model.Closing{}, err
	}
	if acc.Kind != model.KindCash {
		return model.Closing{}, ErrNotClosable
	}
	day, err := s.closingDate(ctx, in.BusinessDate, acc.OpeningDate.Time)
	if err != nil {
		return model.Closing{}, err
	}
	ve := &ValidationError{}
	if in.Counted == nil {
		ve.add("counted_amount", "required", "counted_amount is required")
	} else if in.Counted.IsNegative() {
		ve.add("counted_amount", "negative", "counted_amount cannot be negative")
	}
	note := strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(note) > maxNoteLength {
		ve.add("note", "too_long", "note is too long")
	}
	if err := ve.orNil(); err != nil {
		return model.Closing{}, err
	}
	counted := *in.Counted
	var closingID uuid.UUID
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		locked, err := s.lock(ctx, q, orgID, acc.ID)
		if err != nil {
			return err
		}
		la := locked[acc.ID]
		if la.Status != model.StatusActive {
			return ErrAccountArchived
		}
		last, err := q.LatestCashClosingDate(ctx, db.LatestCashClosingDateParams{OrganizationID: orgID, CashAccountID: acc.ID})
		if err != nil {
			return err
		}
		if last.Valid && !day.After(last.Time) {
			return ErrDayAlreadyClosed
		}
		t, err := s.dayTotals(ctx, q, orgID, acc.ID, money.FromNumericOrZero(la.OpeningBalance), day)
		if err != nil {
			return err
		}
		diff := counted.Sub(t.expected)
		row, err := q.InsertCashClosing(ctx, db.InsertCashClosingParams{
			OrganizationID: orgID, CashAccountID: acc.ID, BranchID: la.BranchID, BusinessDate: dateArg(day),
			Currency: la.Currency, OpeningAmount: t.opening.Numeric(), TotalIn: t.in.Numeric(), TotalOut: t.out.Numeric(),
			ExpectedAmount: t.expected.Numeric(), CountedAmount: counted.Numeric(), Difference: diff.Numeric(),
			Note: note, ClosedBy: int8Ptr(userID),
		})
		if err != nil {
			return mapRepoErr(err)
		}
		closingID = row.Uuid
		if diff.IsZero() {
			return nil
		}
		dir, amount := model.DirectionIn, diff
		if diff.IsNegative() {
			dir, amount = model.DirectionOut, diff.Neg()
		}
		cid := row.ID
		_, err = s.insert(ctx, q, orgID, la, movementRow{
			Kind: model.MovementClosingDifference, Direction: dir, Amount: amount, Date: day,
			Description: note, ClosingID: &cid, CreatedBy: userID,
		})
		return err
	})
	if err != nil {
		return model.Closing{}, err
	}
	return s.GetClosing(ctx, orgID, accountID, closingID)
}

// Reopen reopens the latest closed day of a till. Its difference movement
// is reversed on the same day, so the day reads as before the closing.
func (s *Service) Reopen(ctx context.Context, orgID int64, accountID, closingID uuid.UUID, userID *int64, in model.ReopenInput) (model.Closing, error) {
	q := s.repo.Q()
	acc, err := s.loadAccount(ctx, q, orgID, accountID)
	if err != nil {
		return model.Closing{}, err
	}
	c, err := q.GetCashClosing(ctx, db.GetCashClosingParams{OrganizationID: orgID, CashAccountID: acc.ID, Uuid: closingID})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return model.Closing{}, ErrClosingNotFound
		}
		return model.Closing{}, err
	}
	reason := strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(reason) < minReasonLength {
		return model.Closing{}, invalid("reason", "required", "a reason of at least 3 characters is required")
	}
	if utf8.RuneCountInString(reason) > maxDescriptionLength {
		return model.Closing{}, invalid("reason", "too_long", "reason is too long")
	}
	if c.Status != "closed" {
		return model.Closing{}, ErrClosingReopened
	}
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		locked, err := s.lock(ctx, q, orgID, acc.ID)
		if err != nil {
			return err
		}
		last, err := q.LatestCashClosingDate(ctx, db.LatestCashClosingDateParams{OrganizationID: orgID, CashAccountID: acc.ID})
		if err != nil {
			return err
		}
		if !last.Valid || !last.Time.Equal(c.BusinessDate.Time) {
			return ErrNotLatestClosing
		}
		n, err := q.ReopenCashClosing(ctx, db.ReopenCashClosingParams{
			OrganizationID: orgID, ID: c.ID, ReopenedBy: int8Ptr(userID), ReopenReason: reason,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrClosingReopened
		}
		if !c.DifferenceMovementID.Valid {
			return nil
		}
		diff := money.FromNumericOrZero(c.Difference)
		dir, amount := model.DirectionOut, diff
		if diff.IsNegative() {
			dir, amount = model.DirectionIn, diff.Neg()
		}
		reverses := c.DifferenceMovementID.Int64
		_, err = s.insert(ctx, q, orgID, locked[acc.ID], movementRow{
			Kind: model.MovementReversal, Direction: dir, Amount: amount, Date: c.BusinessDate.Time,
			Description: reason, ReversesID: &reverses, CreatedBy: userID,
		})
		return err
	})
	if err != nil {
		return model.Closing{}, err
	}
	return s.GetClosing(ctx, orgID, accountID, closingID)
}

// GetClosing returns one closing of an account.
func (s *Service) GetClosing(ctx context.Context, orgID int64, accountID, closingID uuid.UUID) (model.Closing, error) {
	q := s.repo.Q()
	acc, err := s.loadAccount(ctx, q, orgID, accountID)
	if err != nil {
		return model.Closing{}, err
	}
	row, err := q.GetCashClosing(ctx, db.GetCashClosingParams{OrganizationID: orgID, CashAccountID: acc.ID, Uuid: closingID})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return model.Closing{}, ErrClosingNotFound
		}
		return model.Closing{}, err
	}
	return mapClosing(row), nil
}

// ListClosings pages the closings of an account, newest day first.
func (s *Service) ListClosings(ctx context.Context, orgID int64, accountID uuid.UUID, limit, offset int32) ([]model.Closing, int64, error) {
	q := s.repo.Q()
	acc, err := s.loadAccount(ctx, q, orgID, accountID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListCashClosings(ctx, db.ListCashClosingsParams{OrganizationID: orgID, CashAccountID: acc.ID, LimitCount: limit, OffsetCount: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountCashClosings(ctx, db.CountCashClosingsParams{OrganizationID: orgID, CashAccountID: acc.ID})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Closing, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapClosing(db.GetCashClosingRow(row)))
	}
	return out, total, nil
}

// DailySummary is the günlük kapanış özeti: every account of the scope on
// one day with its opening, money in / out, transfers, closing difference
// and closing balance, plus totals per currency. Transfers are listed
// apart: they move money between accounts and are not revenue.
func (s *Service) DailySummary(ctx context.Context, orgID int64, rawDate string, branches []uuid.UUID, headOffice bool) (model.DailySummary, error) {
	day := s.Today(ctx)
	if strings.TrimSpace(rawDate) != "" {
		d, err := parseDate(rawDate)
		if err != nil {
			return model.DailySummary{}, invalid("date", "invalid", "date must be YYYY-MM-DD")
		}
		day = d
	}
	q := s.repo.Q()
	filter, err := s.filterBranchIDs(ctx, q, orgID, branches)
	if err != nil {
		return model.DailySummary{}, err
	}
	if headOffice && filter == nil {
		filter = []int64{}
	}
	rows, err := q.CashDailySummary(ctx, db.CashDailySummaryParams{
		OrganizationID: orgID, Day: dateArg(day), BranchIds: orgctx.BranchScope(ctx),
		FilterBranchIds: filter, FilterHeadOffice: headOffice,
	})
	if err != nil {
		return model.DailySummary{}, err
	}
	out := model.DailySummary{Date: formatDate(day), Accounts: []model.DailyAccount{}, Totals: []model.DailyTotals{}}
	totals := map[string]*model.DailyTotals{}
	for _, row := range rows {
		a := model.DailyAccount{
			Account:  model.AccountRef{UUID: row.Uuid, Name: row.Name, Kind: row.Kind},
			Currency: row.Currency, Status: row.Status,
			Opening: money.FromNumericOrZero(row.OpeningBalance).Add(money.FromNumericOrZero(row.BeforeDay)),
			MoneyIn: money.FromNumericOrZero(row.DayIn), MoneyOut: money.FromNumericOrZero(row.DayOut),
			TransferIn: money.FromNumericOrZero(row.TransferIn), TransferOut: money.FromNumericOrZero(row.TransferOut),
			Difference: money.FromNumericOrZero(row.Difference),
		}
		a.Closing = a.Opening.Add(a.MoneyIn).Sub(a.MoneyOut).Add(a.TransferIn).Sub(a.TransferOut).Add(a.Difference)
		if row.BranchUuid.Valid {
			a.Branch = &model.BranchRef{UUID: uuid.UUID(row.BranchUuid.Bytes), Name: row.BranchName.String}
		}
		if row.ClosingUuid.Valid {
			a.Closed = true
			a.ClosingUUID = uuidPtr(row.ClosingUuid)
			t := row.ClosingClosedAt.Time
			a.ClosedAt = &t
		}
		out.Accounts = append(out.Accounts, a)
		t := totals[row.Currency]
		if t == nil {
			t = &model.DailyTotals{Currency: row.Currency}
			totals[row.Currency] = t
		}
		t.Opening = t.Opening.Add(a.Opening)
		t.MoneyIn = t.MoneyIn.Add(a.MoneyIn)
		t.MoneyOut = t.MoneyOut.Add(a.MoneyOut)
		t.TransferIn = t.TransferIn.Add(a.TransferIn)
		t.TransferOut = t.TransferOut.Add(a.TransferOut)
		t.Difference = t.Difference.Add(a.Difference)
		t.Closing = t.Closing.Add(a.Closing)
	}
	for _, t := range totals {
		out.Totals = append(out.Totals, *t)
	}
	sort.Slice(out.Totals, func(i, j int) bool { return out.Totals[i].Currency < out.Totals[j].Currency })
	return out, nil
}

func userRef(id pgtype.UUID, name, surname pgtype.Text) *model.UserRef {
	if !id.Valid {
		return nil
	}
	return &model.UserRef{UUID: uuid.UUID(id.Bytes), Name: name.String, Surname: surname.String}
}

func mapClosing(row db.GetCashClosingRow) model.Closing {
	c := model.Closing{
		UUID: row.Uuid, BusinessDate: formatDate(row.BusinessDate.Time), Currency: row.Currency,
		Opening: money.FromNumericOrZero(row.OpeningAmount), TotalIn: money.FromNumericOrZero(row.TotalIn),
		TotalOut: money.FromNumericOrZero(row.TotalOut), Expected: money.FromNumericOrZero(row.ExpectedAmount),
		Counted: money.FromNumericOrZero(row.CountedAmount), Difference: money.FromNumericOrZero(row.Difference),
		Note: row.Note, Status: row.Status, ClosedAt: row.ClosedAt.Time, ReopenReason: row.ReopenReason,
		ClosedBy:   userRef(row.ClosedByUuid, row.ClosedByName, row.ClosedBySurname),
		ReopenedBy: userRef(row.ReopenedByUuid, row.ReopenedByName, row.ReopenedBySurname),
	}
	if row.ReopenedAt.Valid {
		t := row.ReopenedAt.Time
		c.ReopenedAt = &t
	}
	return c
}
