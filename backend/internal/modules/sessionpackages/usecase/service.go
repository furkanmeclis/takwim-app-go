// Package usecase holds the rules of multi-session packages (TEC-920,
// requirements §13 "Çok seanslı paket", A19). Every method takes the
// organization id of the caller explicitly.
//
//   - A definition is N sessions of one service for a price, usable in
//     every branch or the listed ones, valid for some days after the sale.
//   - Selling a package to a customer takes its price into a cash or bank
//     account (a ledger collection: kasa girişi) and charges the customer
//     with a "package" debit. Neither is revenue.
//   - Its sessions are given to visits by the reservations module; a
//     completed visit consumes its session and its sale line carries the
//     session's value, so the revenue of a package is its use, once.
//   - A refund closes the package: the amount paid back leaves the cash
//     book, the package debit is replaced by what the business keeps, the
//     sessions left are gone. Sessions still booked on visits must be
//     released first.
package usecase

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	reservationmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	salesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	dateLayout      = "2006-01-02"
	maxSummaryDays  = 366
	minReasonLength = 3
)

var (
	ErrPackageNotFound   = errors.New("session package not found")
	ErrCustomerPackage   = errors.New("customer package not found")
	ErrAllBranches       = errors.New("session packages are managed with access to every branch")
	ErrNameTaken         = errors.New("a package with this name exists")
	ErrAlreadyRefunded   = errors.New("the package was refunded already")
	ErrSessionsHeld      = errors.New("sessions of the package are booked on visits; release them first")
	ErrIdempotencyReused = errors.New("the idempotency key was used for another sale")
)

// PackageSortColumns are the sortable package list fields.
var PackageSortColumns = map[string]string{
	"name": "name", "price": "price", "total_sessions": "total_sessions", "created_at": "created_at",
}

// CustomerPackageSortColumns are the sortable customer package list fields.
var CustomerPackageSortColumns = map[string]string{
	"sold_on": "sold_on", "expires_on": "expires_on", "price": "price", "created_at": "created_at",
}

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

// Service implements the session package use cases.
type Service struct {
	repo  *repository.Postgres
	sales *salesusecase.Service
	now   func() time.Time
}

// New builds the service. sales posts the money and the ledger entries.
func New(repo *repository.Postgres, sales *salesusecase.Service) *Service {
	return &Service{repo: repo, sales: sales, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

// ------------------------------------------------------------ definitions

// GetPackage returns one package definition.
func (s *Service) GetPackage(ctx context.Context, orgID int64, id uuid.UUID) (model.Package, error) {
	q := s.repo.Q()
	row, err := q.GetSessionPackage(ctx, db.GetSessionPackageParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return model.Package{}, ErrPackageNotFound
		}
		return model.Package{}, err
	}
	out := []model.Package{mapPackage(ctx, row)}
	if err := s.fillBranches(ctx, q, orgID, out, []int64{row.ID}); err != nil {
		return model.Package{}, err
	}
	return out[0], nil
}

// ListPackages returns one page of package definitions.
func (s *Service) ListPackages(ctx context.Context, orgID int64, f model.PackageFilter) ([]model.Package, int64, error) {
	if _, ok := PackageSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	q := s.repo.Q()
	var services []int64
	if len(f.Services) > 0 {
		rows, err := q.ListPackageServicesByUUID(ctx, db.ListPackageServicesByUUIDParams{OrganizationID: orgID, Uuids: f.Services})
		if err != nil {
			return nil, 0, err
		}
		if len(rows) == 0 {
			return []model.Package{}, 0, nil
		}
		for _, r := range rows {
			services = append(services, r.ID)
		}
	}
	branch := pgtype.Int8{}
	if f.Branch != nil {
		rows, err := q.ListPackageBranchesByUUID(ctx, db.ListPackageBranchesByUUIDParams{OrganizationID: orgID, Uuids: []uuid.UUID{*f.Branch}})
		if err != nil {
			return nil, 0, err
		}
		if len(rows) == 0 {
			return []model.Package{}, 0, nil
		}
		branch = int8Of(rows[0].ID)
	}
	active := pgtype.Bool{}
	if f.IsActive != nil {
		active = pgtype.Bool{Bool: *f.IsActive, Valid: true}
	}
	rows, err := q.ListSessionPackages(ctx, db.ListSessionPackagesParams{
		OrganizationID: orgID, IsActive: active, ServiceIds: services, BranchID: branch, Q: textArg(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountSessionPackages(ctx, db.CountSessionPackagesParams{
		OrganizationID: orgID, IsActive: active, ServiceIds: services, BranchID: branch, Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Package, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapPackage(ctx, db.GetSessionPackageRow(r)))
		ids = append(ids, r.ID)
	}
	if err := s.fillBranches(ctx, q, orgID, out, ids); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (s *Service) fillBranches(ctx context.Context, q *db.Queries, orgID int64, out []model.Package, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.ListSessionPackageBranches(ctx, db.ListSessionPackageBranchesParams{OrganizationID: orgID, PackageIds: ids})
	if err != nil {
		return err
	}
	index := map[int64]int{}
	for i, id := range ids {
		index[id] = i
	}
	for _, r := range rows {
		if i, ok := index[r.PackageID]; ok && !out[i].AllBranches {
			out[i].Branches = append(out[i].Branches, model.Ref{UUID: r.Uuid, Name: r.Name})
		}
	}
	return nil
}

// checked is a validated definition.
type checked struct {
	name, description string
	serviceID         int64
	total             int32
	price             money.Money
	validity          pgtype.Int4
	allBranches       bool
	branchIDs         []int64
	active            bool
}

func (s *Service) checkPackage(ctx context.Context, q *db.Queries, orgID int64, in model.PackageInput) (checked, error) {
	ve := &ValidationError{}
	c := checked{
		name: strings.TrimSpace(in.Name), description: strings.TrimSpace(in.Description),
		total: int32(in.TotalSessions), price: in.Price, allBranches: in.AllBranches, active: true,
	}
	if in.IsActive != nil {
		c.active = *in.IsActive
	}
	switch n := utf8.RuneCountInString(c.name); {
	case n == 0:
		ve.add("name", "required", "name is required")
	case n > model.MaxNameLength:
		ve.add("name", "too_long", "name is too long")
	}
	if utf8.RuneCountInString(c.description) > model.MaxDescriptionLength {
		ve.add("description", "too_long", "description is too long")
	}
	if in.TotalSessions < 1 || in.TotalSessions > model.MaxSessions {
		ve.add("total_sessions", "invalid", "total_sessions must be between 1 and 1000")
	}
	if in.Price.IsNegative() {
		ve.add("price", "invalid", "price must not be negative")
	}
	if in.ValidityDays != nil {
		if *in.ValidityDays < 1 || *in.ValidityDays > model.MaxValidityDays {
			ve.add("validity_days", "invalid", "validity_days must be between 1 and 3650")
		} else {
			c.validity = pgtype.Int4{Int32: int32(*in.ValidityDays), Valid: true}
		}
	}
	if in.ServiceUUID == uuid.Nil {
		ve.add("service_uuid", "required", "service_uuid is required")
	} else {
		svc, err := q.GetPackageServiceByUUID(ctx, db.GetPackageServiceByUUIDParams{OrganizationID: orgID, Uuid: in.ServiceUUID})
		switch {
		case isNotFound(err):
			ve.add("service_uuid", "not_found", "service not found")
		case err != nil:
			return c, err
		default:
			c.serviceID = svc.ID
		}
	}
	if !in.AllBranches {
		if len(in.BranchUUIDs) == 0 {
			ve.add("branch_uuids", "required", "pick at least one branch or every branch")
		} else {
			rows, err := q.ListPackageBranchesByUUID(ctx, db.ListPackageBranchesByUUIDParams{OrganizationID: orgID, Uuids: in.BranchUUIDs})
			if err != nil {
				return c, err
			}
			found := map[uuid.UUID]int64{}
			for _, r := range rows {
				found[r.Uuid] = r.ID
			}
			for i, b := range in.BranchUUIDs {
				id, ok := found[b]
				if !ok {
					ve.add("branch_uuids."+strconv.Itoa(i), "not_found", "branch not found")
					continue
				}
				if !slices.Contains(c.branchIDs, id) {
					c.branchIDs = append(c.branchIDs, id)
				}
			}
		}
	}
	return c, ve.orNil()
}

// CreatePackage adds a package definition (business-wide: every branch
// access is required).
func (s *Service) CreatePackage(ctx context.Context, orgID int64, userID *int64, in model.PackageInput) (model.Package, error) {
	if err := orgctx.RequireAllBranches(ctx); err != nil {
		return model.Package{}, ErrAllBranches
	}
	var id uuid.UUID
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		c, err := s.checkPackage(ctx, q, orgID, in)
		if err != nil {
			return err
		}
		row, err := q.InsertSessionPackage(ctx, db.InsertSessionPackageParams{
			OrganizationID: orgID, Name: c.name, Description: c.description, ServiceID: c.serviceID,
			TotalSessions: c.total, Price: c.price.Numeric(), ValidityDays: c.validity, AllBranches: c.allBranches,
			IsActive: c.active, CreatedBy: int8Ptr(userID),
		})
		if err != nil {
			return mapRepoErr(err)
		}
		id = row.Uuid
		return s.writeBranches(ctx, q, orgID, row.ID, c)
	})
	if err != nil {
		return model.Package{}, err
	}
	return s.GetPackage(ctx, orgID, id)
}

func (s *Service) writeBranches(ctx context.Context, q *db.Queries, orgID, packageID int64, c checked) error {
	if err := q.DeleteSessionPackageBranches(ctx, db.DeleteSessionPackageBranchesParams{OrganizationID: orgID, PackageID: packageID}); err != nil {
		return err
	}
	if c.allBranches {
		return nil
	}
	for _, b := range c.branchIDs {
		if err := q.InsertSessionPackageBranch(ctx, db.InsertSessionPackageBranchParams{
			OrganizationID: orgID, PackageID: packageID, BranchID: b,
		}); err != nil {
			return err
		}
	}
	return nil
}

// UpdatePackage replaces a definition. Packages sold before keep the
// terms they were sold with.
func (s *Service) UpdatePackage(ctx context.Context, orgID int64, id uuid.UUID, in model.PackageInput) (model.Package, model.Package, error) {
	before, err := s.GetPackage(ctx, orgID, id)
	if err != nil {
		return before, before, err
	}
	if err := orgctx.RequireAllBranches(ctx); err != nil {
		return before, before, ErrAllBranches
	}
	err = s.repo.InTx(ctx, func(q *db.Queries) error {
		row, err := q.GetSessionPackage(ctx, db.GetSessionPackageParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			if isNotFound(err) {
				return ErrPackageNotFound
			}
			return err
		}
		c, err := s.checkPackage(ctx, q, orgID, in)
		if err != nil {
			return err
		}
		if _, err := q.UpdateSessionPackage(ctx, db.UpdateSessionPackageParams{
			OrganizationID: orgID, ID: row.ID, Name: c.name, Description: c.description, ServiceID: c.serviceID,
			TotalSessions: c.total, Price: c.price.Numeric(), ValidityDays: c.validity, AllBranches: c.allBranches,
			IsActive: c.active,
		}); err != nil {
			return mapRepoErr(err)
		}
		return s.writeBranches(ctx, q, orgID, row.ID, c)
	})
	if err != nil {
		return before, before, err
	}
	after, err := s.GetPackage(ctx, orgID, id)
	return before, after, err
}

// DeletePackage removes a definition (soft); sold packages stay.
func (s *Service) DeletePackage(ctx context.Context, orgID int64, id uuid.UUID) (model.Package, error) {
	p, err := s.GetPackage(ctx, orgID, id)
	if err != nil {
		return p, err
	}
	if err := orgctx.RequireAllBranches(ctx); err != nil {
		return p, ErrAllBranches
	}
	row, err := s.repo.Q().GetSessionPackage(ctx, db.GetSessionPackageParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return p, mapNotFound(err, ErrPackageNotFound)
	}
	n, err := s.repo.Q().SoftDeleteSessionPackage(ctx, db.SoftDeleteSessionPackageParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return p, err
	}
	if n == 0 {
		return p, ErrPackageNotFound
	}
	return p, nil
}

func mapPackage(ctx context.Context, r db.GetSessionPackageRow) model.Package {
	price := amountOf(r.Price)
	p := model.Package{
		UUID: r.Uuid, Name: r.Name, Description: r.Description, Service: model.Ref{UUID: r.ServiceUuid, Name: r.ServiceName},
		TotalSessions: int(r.TotalSessions), Price: price, Currency: currencyOf(ctx),
		SessionValue: reservationmodel.SessionValue(price, int(r.TotalSessions), 1), AllBranches: r.AllBranches,
		Branches: []model.Ref{}, IsActive: r.IsActive, SoldCount: r.SoldCount,
		CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
	if r.ValidityDays.Valid {
		v := int(r.ValidityDays.Int32)
		p.ValidityDays = &v
	}
	return p
}

// --------------------------------------------------------------- helpers

func currencyOf(ctx context.Context) string {
	if sc, ok := orgctx.From(ctx); ok && sc.Currency != "" {
		return sc.Currency
	}
	return "TRY"
}

// today is the business day in the organization's zone.
func (s *Service) today(ctx context.Context) time.Time {
	loc := time.UTC
	if scope, ok := orgctx.From(ctx); ok && scope.Timezone != "" {
		if l, err := time.LoadLocation(scope.Timezone); err == nil {
			loc = l
		}
	}
	return dayIn(s.now(), loc)
}

func dayIn(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func mapRepoErr(err error) error {
	switch e := repository.MapErr(err); {
	case errors.Is(e, repository.ErrNameTaken):
		return ErrNameTaken
	case errors.Is(e, repository.ErrIdempotencyKey):
		return ErrIdempotencyReused
	default:
		return e
	}
}

func isNotFound(err error) bool {
	return errors.Is(repository.MapErr(err), repository.ErrNotFound)
}

func mapNotFound(err, notFound error) error {
	if isNotFound(err) {
		return notFound
	}
	return err
}

func amountOf(n pgtype.Numeric) money.Money {
	return money.FromNumericOrZero(n)
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
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
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

func datePtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format(dateLayout)
	return &s
}

func uuidPtr(v pgtype.UUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := uuid.UUID(v.Bytes)
	return &id
}

// checkMethod validates the money fields of a sale or refund.
func checkMethod(ve *ValidationError, method string, account *uuid.UUID) {
	if !slices.Contains(salesmodel.Methods, method) {
		ve.add("method", "invalid", "method must be one of cash, card, transfer, online, other")
	}
	if account == nil || *account == uuid.Nil {
		ve.add("cash_account_uuid", "required", "cash_account_uuid is required")
	}
}
