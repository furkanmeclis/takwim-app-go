package usecase

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	reservationmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	salesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// GetCustomerPackage returns a sold package with its sessions. A package
// sold in a branch outside the caller's scope reads as not found.
func (s *Service) GetCustomerPackage(ctx context.Context, orgID int64, id uuid.UUID) (model.CustomerPackage, error) {
	q := s.repo.Q()
	row, err := q.GetCustomerPackage(ctx, db.GetCustomerPackageParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.CustomerPackage{}, mapNotFound(err, ErrCustomerPackage)
	}
	if orgctx.CheckBranch(ctx, row.BranchID) != nil {
		return model.CustomerPackage{}, ErrCustomerPackage
	}
	out, err := s.mapCustomerPackage(ctx, q, orgID, row)
	if err != nil {
		return out, err
	}
	sessions, err := q.ListCustomerPackageSessions(ctx, db.ListCustomerPackageSessionsParams{OrganizationID: orgID, CustomerPackageID: row.ID})
	if err != nil {
		return out, err
	}
	out.Sessions = make([]model.Session, 0, len(sessions))
	for _, r := range sessions {
		ss := model.Session{
			UUID: r.Uuid, SessionNo: int(r.SessionNo), Amount: amountOf(r.Amount), Status: r.Status,
			Reservation: model.ReservationRef{
				UUID: r.ReservationUuid, Number: r.ReservationNumber, StartsAt: r.ReservationStartsAt.Time.UTC(),
				Status: r.ReservationStatus,
			},
			ConsumedOn: datePtr(r.ConsumedOn), CreatedAt: r.CreatedAt.Time.UTC(),
		}
		if r.ReleaseReason.Valid {
			v := r.ReleaseReason.String
			ss.ReleaseReason = &v
		}
		if r.ServiceName.Valid {
			v := r.ServiceName.String
			ss.ServiceName = &v
		}
		if r.StaffName.Valid {
			v := r.StaffName.String
			ss.StaffName = &v
		}
		if r.ConsumedAt.Valid {
			t := r.ConsumedAt.Time.UTC()
			ss.ConsumedAt = &t
		}
		if r.ReleasedAt.Valid {
			t := r.ReleasedAt.Time.UTC()
			ss.ReleasedAt = &t
		}
		out.Sessions = append(out.Sessions, ss)
	}
	return out, nil
}

// resolved list filter ids; empty means "matches nothing".
type resolved struct {
	branches []int64
	customer pgtype.Int8
	packages []int64
	empty    bool
}

func (s *Service) resolveList(ctx context.Context, q *db.Queries, orgID int64, f model.CustomerPackageFilter) (resolved, error) {
	var a resolved
	if len(f.Branches) > 0 {
		rows, err := q.ListPackageBranchesByUUID(ctx, db.ListPackageBranchesByUUIDParams{OrganizationID: orgID, Uuids: f.Branches})
		if err != nil {
			return a, err
		}
		for _, r := range rows {
			a.branches = append(a.branches, r.ID)
		}
		if len(a.branches) == 0 {
			a.empty = true
		}
	}
	if f.Customer != nil {
		c, err := q.GetPackageCustomerByUUID(ctx, db.GetPackageCustomerByUUIDParams{OrganizationID: orgID, Uuid: *f.Customer})
		if err != nil {
			if isNotFound(err) {
				a.empty = true
				return a, nil
			}
			return a, err
		}
		a.customer = int8Of(c.ID)
	}
	if len(f.Packages) > 0 {
		rows, err := q.ListPackageListFilterPackages(ctx, db.ListPackageListFilterPackagesParams{OrganizationID: orgID, Uuids: f.Packages})
		if err != nil {
			return a, err
		}
		for _, r := range rows {
			a.packages = append(a.packages, r.ID)
		}
		if len(a.packages) == 0 {
			a.empty = true
		}
	}
	return a, nil
}

// ListCustomerPackages returns one page of sold packages in the caller's
// branch scope (the branch that sold them).
func (s *Service) ListCustomerPackages(ctx context.Context, orgID int64, f model.CustomerPackageFilter) ([]model.CustomerPackage, int64, error) {
	if _, ok := CustomerPackageSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	a, err := s.resolveList(ctx, q, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	if a.empty {
		return []model.CustomerPackage{}, 0, nil
	}
	today := dateArg(s.today(ctx))
	scope := orgctx.BranchScope(ctx)
	var states []string
	if len(f.States) > 0 {
		states = f.States
	}
	rows, err := q.ListCustomerPackages(ctx, db.ListCustomerPackagesParams{
		OrganizationID: orgID, BranchIds: scope, FilterBranchIds: a.branches, CustomerID: a.customer,
		PackageIds: a.packages, DateFrom: datePtrArg(f.DateFrom), DateTo: datePtrArg(f.DateTo), States: states,
		Today: today, Q: textArg(f.Q), SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountCustomerPackages(ctx, db.CountCustomerPackagesParams{
		OrganizationID: orgID, BranchIds: scope, FilterBranchIds: a.branches, CustomerID: a.customer,
		PackageIds: a.packages, DateFrom: datePtrArg(f.DateFrom), DateTo: datePtrArg(f.DateTo), States: states,
		Today: today, Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.CustomerPackage, 0, len(rows))
	for _, r := range rows {
		cp, err := s.mapCustomerPackage(ctx, q, orgID, db.GetCustomerPackageRow(r))
		if err != nil {
			return nil, 0, err
		}
		out = append(out, cp)
	}
	return out, total, nil
}

// Facets returns the package and branch options of the list.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	q := s.repo.Q()
	scope := orgctx.BranchScope(ctx)
	out := model.Facets{Package: []model.Facet{}, Branch: []model.Facet{}}
	pk, err := q.CustomerPackageFacetPackages(ctx, db.CustomerPackageFacetPackagesParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, r := range pk {
		out.Package = append(out.Package, model.Facet{Value: r.Uuid.String(), Label: r.Name, Count: r.Count})
	}
	br, err := q.CustomerPackageFacetBranches(ctx, db.CustomerPackageFacetBranchesParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, r := range br {
		out.Branch = append(out.Branch, model.Facet{Value: r.Uuid.String(), Label: r.Name, Count: r.Count})
	}
	return out, nil
}

// stateOf is the state of a sold package on day today.
func stateOf(status string, used, total int, expires pgtype.Date, today time.Time) string {
	switch {
	case status == "refunded":
		return model.StateRefunded
	case used >= total:
		return model.StateUsedUp
	case expires.Valid && expires.Time.Before(today):
		return model.StateExpired
	}
	return model.StateActive
}

func (s *Service) mapCustomerPackage(ctx context.Context, q *db.Queries, orgID int64, r db.GetCustomerPackageRow) (model.CustomerPackage, error) {
	price := amountOf(r.Price)
	used, held := int(r.ConsumedCount), int(r.HeldCount)
	usedAmount, heldAmount := amountOf(r.ConsumedAmount), amountOf(r.HeldAmount)
	cp := model.CustomerPackage{
		UUID: r.Uuid, Name: r.Name, ServiceName: r.ServiceName, ServiceUUID: uuidPtr(r.ServiceUuid),
		PackageUUID: uuidPtr(r.PackageUuid),
		Customer: model.CustomerRef{
			UUID: r.CustomerUuid, Name: strings.TrimSpace(r.CustomerFirstName + " " + r.CustomerLastName),
		},
		Branch: model.Ref{UUID: r.BranchUuid, Name: r.BranchName}, AllBranches: r.BranchIds == nil, Branches: []model.Ref{},
		TotalSessions: int(r.TotalSessions), Used: used, Held: held, Remaining: max(int(r.TotalSessions)-used-held, 0),
		Price: price, Currency: r.Currency, SessionValue: reservationmodel.SessionValue(price, int(r.TotalSessions), 1),
		UsedAmount: usedAmount, HeldAmount: heldAmount, RemainingAmount: price.Sub(usedAmount).Sub(heldAmount).ClampZero(),
		SoldOn: r.SoldOn.Time.Format(dateLayout), ExpiresOn: datePtr(r.ExpiresOn),
		State: stateOf(r.Status, used, int(r.TotalSessions), r.ExpiresOn, s.today(ctx)), Note: r.Note,
		CollectionUUID: uuidPtr(r.CollectionUuid), CreatedAt: r.CreatedAt.Time.UTC(),
	}
	if r.Status == "refunded" {
		cp.RemainingAmount, cp.Remaining = money.Zero(), 0
	}
	if r.CustomerPhone.Valid {
		v := r.CustomerPhone.String
		cp.Customer.Phone = &v
	}
	if r.StaffUuid.Valid {
		cp.Staff = &model.Ref{UUID: uuid.UUID(r.StaffUuid.Bytes), Name: r.StaffName.String}
	}
	if r.RefundedAt.Valid {
		cp.Refund = &model.Refund{
			Amount: amountOf(r.RefundAmount), Reason: r.RefundReason.String, RefundedOn: r.RefundedOn.Time.Format(dateLayout),
			RefundedAt: r.RefundedAt.Time.UTC(), EntryUUID: uuidPtr(r.RefundEntryUuid),
		}
	}
	if len(r.BranchIds) > 0 {
		rows, err := q.ListBranchRefsByIDs(ctx, db.ListBranchRefsByIDsParams{OrganizationID: orgID, Ids: r.BranchIds})
		if err != nil {
			return cp, err
		}
		for _, b := range rows {
			cp.Branches = append(cp.Branches, model.Ref{UUID: b.Uuid, Name: b.Name})
		}
	}
	return cp, nil
}

// ------------------------------------------------------------------- sell

// Sell sells a package to a customer in a branch: the customer package
// with the definition's terms, the money taken into the cash or bank
// account (collection) and the package debit, in one transaction. The
// same idempotency key answers the first sale.
func (s *Service) Sell(ctx context.Context, orgID int64, userID *int64, in model.SellInput) (model.CustomerPackage, bool, error) {
	ve := &ValidationError{}
	key := strings.TrimSpace(in.IdempotencyKey)
	if len(key) > model.MaxIdempotencyKey {
		ve.add("idempotency_key", "too_long", "idempotency_key is too long")
	}
	note := strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(note) > model.MaxNoteLength {
		ve.add("note", "too_long", "note is too long")
	}
	if in.PackageUUID == uuid.Nil {
		ve.add("package_uuid", "required", "package_uuid is required")
	}
	if in.CustomerUUID == uuid.Nil {
		ve.add("customer_uuid", "required", "customer_uuid is required")
	}
	if in.BranchUUID == uuid.Nil {
		ve.add("branch_uuid", "required", "branch_uuid is required")
	}
	if err := ve.orNil(); err != nil {
		return model.CustomerPackage{}, false, err
	}
	q := s.repo.Q()
	if key != "" {
		prev, err := q.GetCustomerPackageByIdempotencyKey(ctx, db.GetCustomerPackageByIdempotencyKeyParams{OrganizationID: orgID, IdempotencyKey: textArg(key)})
		if err == nil {
			out, gerr := s.GetCustomerPackage(ctx, orgID, prev.Uuid)
			if gerr != nil {
				return out, false, gerr
			}
			if out.PackageUUID == nil || *out.PackageUUID != in.PackageUUID || out.Customer.UUID != in.CustomerUUID {
				return model.CustomerPackage{}, false, ErrIdempotencyReused
			}
			return out, true, nil
		}
		if !isNotFound(err) {
			return model.CustomerPackage{}, false, err
		}
	}
	id := uuid.New()
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		pkg, err := q.GetSessionPackage(ctx, db.GetSessionPackageParams{OrganizationID: orgID, Uuid: in.PackageUUID})
		if err != nil {
			if isNotFound(err) {
				return invalid("package_uuid", "not_found", "package not found")
			}
			return err
		}
		if !pkg.IsActive {
			return invalid("package_uuid", "inactive", "the package is not on sale")
		}
		branches, err := q.ListPackageBranchesByUUID(ctx, db.ListPackageBranchesByUUIDParams{OrganizationID: orgID, Uuids: []uuid.UUID{in.BranchUUID}})
		if err != nil {
			return err
		}
		if len(branches) == 0 || orgctx.CheckBranch(ctx, branches[0].ID) != nil {
			return invalid("branch_uuid", "not_found", "branch not found")
		}
		branchID := branches[0].ID
		var allowed []int64
		if !pkg.AllBranches {
			rows, err := q.ListSessionPackageBranches(ctx, db.ListSessionPackageBranchesParams{OrganizationID: orgID, PackageIds: []int64{pkg.ID}})
			if err != nil {
				return err
			}
			allowed = []int64{}
			for _, r := range rows {
				allowed = append(allowed, r.ID)
			}
			if !slices.Contains(allowed, branchID) {
				return invalid("branch_uuid", "not_offered", "the package is not sold in this branch")
			}
		}
		customer, err := q.GetPackageCustomerByUUID(ctx, db.GetPackageCustomerByUUIDParams{OrganizationID: orgID, Uuid: in.CustomerUUID})
		if err != nil {
			if isNotFound(err) {
				return invalid("customer_uuid", "not_found", "customer not found")
			}
			return err
		}
		staff := pgtype.Int8{}
		if in.StaffUUID != nil {
			st, err := q.GetPackageStaffByUUID(ctx, db.GetPackageStaffByUUIDParams{OrganizationID: orgID, Uuid: *in.StaffUUID})
			if err != nil {
				if isNotFound(err) {
					return invalid("staff_uuid", "not_found", "staff member not found")
				}
				return err
			}
			staff = int8Of(st.ID)
		}
		price := amountOf(pkg.Price)
		if price.IsPositive() {
			mve := &ValidationError{}
			checkMethod(mve, in.Method, in.CashAccountUUID)
			if err := mve.orNil(); err != nil {
				return err
			}
		}
		clock, err := q.GetPackageClock(ctx, db.GetPackageClockParams{OrganizationID: orgID, BranchID: branchID})
		if err != nil {
			return err
		}
		loc, lerr := time.LoadLocation(clock.Timezone)
		if lerr != nil {
			loc = time.UTC
		}
		soldOn := dayIn(s.now(), loc)
		expires := pgtype.Date{}
		if pkg.ValidityDays.Valid {
			expires = dateArg(soldOn.AddDate(0, 0, int(pkg.ValidityDays.Int32)))
		}
		row, err := q.InsertCustomerPackage(ctx, db.InsertCustomerPackageParams{
			Uuid: id, OrganizationID: orgID, PackageID: int8Of(pkg.ID), CustomerID: customer.ID, BranchID: branchID,
			ServiceID: int8Of(pkg.ServiceID), Name: pkg.Name, ServiceName: pkg.ServiceName,
			TotalSessions: pkg.TotalSessions, Price: pkg.Price, Currency: clock.Currency, BranchIds: allowed,
			SoldOn: dateArg(soldOn), ExpiresOn: expires, StaffID: staff, Note: note, IdempotencyKey: textArg(key),
			CreatedBy: int8Ptr(userID),
		})
		if err != nil {
			return mapRepoErr(err)
		}
		if !price.IsPositive() {
			return nil
		}
		desc := pkg.Name
		ledgerKey := "package:" + id.String()
		if key != "" {
			ledgerKey = "package:" + key
		}
		branchUUID, customerUUID := in.BranchUUID, in.CustomerUUID
		colUUID, err := s.sales.RecordCollectionTx(ctx, q, orgID, salesmodel.CollectionInput{
			BranchUUID: &branchUUID, CustomerUUID: &customerUUID, CashAccountUUID: *in.CashAccountUUID, Amount: price,
			Method: in.Method, Date: soldOn, Description: desc, SourceType: salesusecase.SourceCustomerPackage,
			SourceUUID: &id, IdempotencyKey: ledgerKey, CreatedBy: userID,
		})
		if err != nil {
			return err
		}
		colID, err := s.sales.EntryID(ctx, q, orgID, colUUID)
		if err != nil {
			return err
		}
		chargeID, err := s.sales.RecordPackageChargeTx(ctx, q, orgID, salesusecase.PackageCharge{
			BranchID: branchID, CustomerID: customer.ID, Amount: price, Currency: clock.Currency, Date: soldOn,
			PackageUUID: id, Description: desc, CreatedBy: userID,
		})
		if err != nil {
			return err
		}
		return q.SetCustomerPackageEntries(ctx, db.SetCustomerPackageEntriesParams{
			OrganizationID: orgID, ID: row.ID, CollectionEntryID: int8Of(colID), ChargeEntryID: int8Of(chargeID),
		})
	})
	if err != nil {
		if errors.Is(err, ErrIdempotencyReused) && key != "" {
			// A concurrent sale with the same key won: answer it.
			prev, gerr := s.repo.Q().GetCustomerPackageByIdempotencyKey(ctx, db.GetCustomerPackageByIdempotencyKeyParams{OrganizationID: orgID, IdempotencyKey: textArg(key)})
			if gerr == nil {
				out, gerr := s.GetCustomerPackage(ctx, orgID, prev.Uuid)
				return out, gerr == nil, gerr
			}
		}
		return model.CustomerPackage{}, false, err
	}
	out, err := s.GetCustomerPackage(ctx, orgID, id)
	return out, false, err
}

// ----------------------------------------------------------------- refund

// Refund closes a package (a manager's operation): amount is paid back
// from the cash or bank account (nothing when zero), the package debit is
// reversed and what the business keeps charged again, so the customer's
// ledger is settled; the sessions left can no longer be used. Sessions
// booked on open visits must be released first.
func (s *Service) Refund(ctx context.Context, orgID int64, id uuid.UUID, userID *int64, in model.RefundInput) (model.CustomerPackage, error) {
	if _, err := s.GetCustomerPackage(ctx, orgID, id); err != nil {
		return model.CustomerPackage{}, err
	}
	ve := &ValidationError{}
	reason := strings.TrimSpace(in.Reason)
	switch n := utf8.RuneCountInString(reason); {
	case n < minReasonLength:
		ve.add("reason", "required", "a reason of at least 3 characters is required")
	case n > model.MaxNoteLength:
		ve.add("reason", "too_long", "reason is too long")
	}
	if in.Amount.IsNegative() {
		ve.add("amount", "invalid", "amount must not be negative")
	}
	if in.Amount.IsPositive() {
		checkMethod(ve, in.Method, in.CashAccountUUID)
	}
	key := strings.TrimSpace(in.IdempotencyKey)
	if len(key) > model.MaxIdempotencyKey {
		ve.add("idempotency_key", "too_long", "idempotency_key is too long")
	}
	if err := ve.orNil(); err != nil {
		return model.CustomerPackage{}, err
	}
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		row, err := q.GetCustomerPackageRow(ctx, db.GetCustomerPackageRowParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			return mapNotFound(err, ErrCustomerPackage)
		}
		cp, err := q.LockCustomerPackage(ctx, db.LockCustomerPackageParams{OrganizationID: orgID, ID: row.ID})
		if err != nil {
			return err
		}
		if cp.Status != "active" {
			return ErrAlreadyRefunded
		}
		detail, err := q.GetCustomerPackage(ctx, db.GetCustomerPackageParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			return err
		}
		if detail.HeldCount > 0 {
			return ErrSessionsHeld
		}
		price := amountOf(cp.Price)
		if in.Amount.Cmp(price) > 0 {
			return invalid("amount", "exceeds_price", "the refund is larger than the package price")
		}
		clock, err := q.GetPackageClock(ctx, db.GetPackageClockParams{OrganizationID: orgID, BranchID: cp.BranchID})
		if err != nil {
			return err
		}
		loc, lerr := time.LoadLocation(clock.Timezone)
		if lerr != nil {
			loc = time.UTC
		}
		day := dayIn(s.now(), loc)
		refundEntry := pgtype.Int8{}
		if in.Amount.IsPositive() {
			if !cp.CollectionEntryID.Valid {
				return invalid("amount", "exceeds_price", "nothing was paid for the package")
			}
			col, err := q.GetLedgerEntryRowByID(ctx, db.GetLedgerEntryRowByIDParams{OrganizationID: orgID, ID: cp.CollectionEntryID.Int64})
			if err != nil {
				return err
			}
			ledgerKey := "package_refund:" + id.String()
			if key != "" {
				ledgerKey = "package_refund:" + key
			}
			src := cp.Uuid
			refUUID, err := s.sales.RecordRefundTx(ctx, q, orgID, salesmodel.RefundInput{
				CollectionUUID: col.Uuid, CashAccountUUID: *in.CashAccountUUID, Amount: in.Amount, Method: in.Method,
				Date: day, Description: reason, SourceType: salesusecase.SourceCustomerPackage, SourceUUID: &src,
				IdempotencyKey: ledgerKey, CreatedBy: userID,
			})
			if err != nil {
				return err
			}
			refID, err := s.sales.EntryID(ctx, q, orgID, refUUID)
			if err != nil {
				return err
			}
			refundEntry = int8Of(refID)
		}
		charge := cp.ChargeEntryID
		if charge.Valid {
			if err := s.sales.ReversePackageChargeTx(ctx, q, orgID, charge.Int64, reason, day, userID); err != nil {
				return err
			}
			kept := price.Sub(in.Amount)
			chargeID, err := s.sales.RecordPackageChargeTx(ctx, q, orgID, salesusecase.PackageCharge{
				BranchID: cp.BranchID, CustomerID: cp.CustomerID, Amount: kept, Currency: cp.Currency, Date: day,
				PackageUUID: cp.Uuid, Description: cp.Name, CreatedBy: userID,
			})
			if err != nil {
				return err
			}
			charge = pgtype.Int8{Int64: chargeID, Valid: chargeID > 0}
		}
		return q.RefundCustomerPackage(ctx, db.RefundCustomerPackageParams{
			OrganizationID: orgID, ID: cp.ID, RefundAmount: in.Amount.Numeric(), RefundEntryID: refundEntry,
			ChargeEntryID: charge, RefundReason: pgtype.Text{String: reason, Valid: true}, RefundedOn: dateArg(day), RefundedBy: int8Ptr(userID),
		})
	})
	if err != nil {
		return model.CustomerPackage{}, err
	}
	return s.GetCustomerPackage(ctx, orgID, id)
}

// ---------------------------------------------------------------- summary

// Summary is the package report of a date range (default: this month):
// sales (money taken, apart from revenue), sessions used (revenue through
// the visits), refunds, and today's open, held and expired sessions.
func (s *Service) Summary(ctx context.Context, orgID int64, from, to *time.Time, branches []uuid.UUID) (model.Summary, error) {
	today := s.today(ctx)
	df := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	dt := today
	if from != nil {
		df = *from
	}
	if to != nil {
		dt = *to
	}
	if dt.Before(df) {
		return model.Summary{}, invalid("date_to", "invalid", "date_to must not be before date_from")
	}
	if dt.Sub(df) > maxSummaryDays*24*time.Hour {
		return model.Summary{}, invalid("date_to", "too_long", "the range is at most 366 days")
	}
	q := s.repo.Q()
	a, err := s.resolveList(ctx, q, orgID, model.CustomerPackageFilter{Branches: branches})
	if err != nil {
		return model.Summary{}, err
	}
	out := model.Summary{DateFrom: df.Format(dateLayout), DateTo: dt.Format(dateLayout), Today: today.Format(dateLayout), Totals: []model.Figures{}}
	if a.empty {
		out.Totals = append(out.Totals, emptyFigures(currencyOf(ctx)))
		return out, nil
	}
	scope := orgctx.BranchScope(ctx)
	figs := map[string]*model.Figures{}
	at := func(c string) *model.Figures {
		f := figs[c]
		if f == nil {
			e := emptyFigures(c)
			f = &e
			figs[c] = f
		}
		return f
	}
	sold, err := q.PackageSalesTotals(ctx, db.PackageSalesTotalsParams{
		OrganizationID: orgID, DateFrom: dateArg(df), DateTo: dateArg(dt), BranchIds: scope, FilterBranchIds: a.branches,
	})
	if err != nil {
		return out, err
	}
	for _, r := range sold {
		at(r.Currency).Sold = model.Count{Count: r.SoldCount, Sessions: r.SoldSessions, Amount: amountOf(r.SoldAmount)}
	}
	used, err := q.PackageUsageTotals(ctx, db.PackageUsageTotalsParams{
		OrganizationID: orgID, DateFrom: dateArg(df), DateTo: dateArg(dt), BranchIds: scope, FilterBranchIds: a.branches,
	})
	if err != nil {
		return out, err
	}
	for _, r := range used {
		at(r.Currency).Used = model.Count{Count: r.UsedCount, Sessions: r.UsedCount, Amount: amountOf(r.UsedAmount)}
	}
	refunded, err := q.PackageRefundTotals(ctx, db.PackageRefundTotalsParams{
		OrganizationID: orgID, DateFrom: dateArg(df), DateTo: dateArg(dt), BranchIds: scope, FilterBranchIds: a.branches,
	})
	if err != nil {
		return out, err
	}
	for _, r := range refunded {
		at(r.Currency).Refunded = model.Count{Count: r.RefundedCount, Sessions: r.RefundedSessions, Amount: amountOf(r.RefundedAmount)}
	}
	open, err := q.PackageOutstanding(ctx, db.PackageOutstandingParams{
		OrganizationID: orgID, Today: dateArg(today), BranchIds: scope, FilterBranchIds: a.branches,
	})
	if err != nil {
		return out, err
	}
	for _, r := range open {
		f := at(r.Currency)
		f.Held.Count += r.HeldSessions
		f.Held.Sessions += r.HeldSessions
		f.Held.Amount = f.Held.Amount.Add(amountOf(r.HeldAmount))
		switch r.State {
		case model.StateActive:
			f.Open = model.Count{Count: r.PackageCount, Sessions: r.OpenSessions, Amount: amountOf(r.OpenAmount)}
		case model.StateExpired:
			f.Expired = model.Count{Count: r.PackageCount, Sessions: r.OpenSessions, Amount: amountOf(r.OpenAmount)}
		case model.StateUsedUp:
			f.UsedUp = r.PackageCount
		}
	}
	currencies := make([]string, 0, len(figs))
	for c := range figs {
		currencies = append(currencies, c)
	}
	slices.Sort(currencies)
	for _, c := range currencies {
		out.Totals = append(out.Totals, *figs[c])
	}
	if len(out.Totals) == 0 {
		out.Totals = append(out.Totals, emptyFigures(currencyOf(ctx)))
	}
	return out, nil
}

func emptyFigures(currency string) model.Figures {
	z := money.Zero()
	c := model.Count{Amount: z}
	return model.Figures{Currency: currency, Sold: c, Used: c, Refunded: c, Open: c, Held: c, Expired: c}
}
