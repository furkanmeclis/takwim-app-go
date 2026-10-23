// Package repository is the Postgres access path of the branches resource.
// Every method takes the organization id explicitly and every query filters
// by it: a uuid from another organization reads as not found.
package repository

import (
	"context"
	"errors"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	// ErrNotFound is returned for rows missing in the organization.
	ErrNotFound = errors.New("not found")
	// ErrCodeTaken is returned when a branch code is already used in the organization.
	ErrCodeTaken = errors.New("branch code taken")
)

// BranchFields are the writable columns of a branch.
type BranchFields struct {
	Code              string
	Name              string
	Address           string
	City              string
	District          string
	Latitude          pgtype.Numeric
	Longitude         pgtype.Numeric
	Phone             string
	Email             string
	WorkingHours      []byte
	HolidayExceptions []byte
	PrepMinutes       int32
	IsActive          bool
}

// ServicePointFields are the writable columns of a service point.
type ServicePointFields struct {
	Type             string
	Name             string
	Description      string
	ExtraFee         pgtype.Numeric
	WorkingHours     []byte
	OnlineSelectable bool
	IsActive         bool
}

// BranchRow is a branch with its service point count.
type BranchRow struct {
	db.Branch
	ServicePointsCount int64
}

// Postgres implements the branches repository with sqlc queries.
type Postgres struct {
	q *db.Queries
}

// NewPostgres builds the repository.
func NewPostgres(q *db.Queries) *Postgres {
	return &Postgres{q: q}
}

// WithQueries returns a repository bound to q (for example a transaction).
func (r *Postgres) WithQueries(q *db.Queries) *Postgres {
	return &Postgres{q: q}
}

func (r *Postgres) ListBranches(ctx context.Context, orgID int64, f model.ListFilter) ([]BranchRow, int64, error) {
	rows, err := r.q.ListBranches(ctx, db.ListBranchesParams{
		OrganizationID: orgID, IsActive: boolArg(f.IsActive), Cities: citiesArg(f.Cities), Q: textArg(f.Q),
		BranchIds: f.BranchIDs, SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountBranches(ctx, db.CountBranchesParams{
		OrganizationID: orgID, IsActive: boolArg(f.IsActive), Cities: citiesArg(f.Cities), Q: textArg(f.Q),
		BranchIds: f.BranchIDs,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]BranchRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, BranchRow{
			Branch: db.Branch{
				ID: row.ID, Uuid: row.Uuid, OrganizationID: row.OrganizationID, Code: row.Code, Name: row.Name,
				Address: row.Address, City: row.City, District: row.District, Latitude: row.Latitude,
				Longitude: row.Longitude, Phone: row.Phone, Email: row.Email, WorkingHours: row.WorkingHours,
				HolidayExceptions: row.HolidayExceptions, PrepMinutes: row.PrepMinutes, IsActive: row.IsActive,
				CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DeletedAt: row.DeletedAt,
			},
			ServicePointsCount: row.ServicePointsCount,
		})
	}
	return out, total, nil
}

func (r *Postgres) CityFacets(ctx context.Context, orgID int64, branchIDs []int64) ([]model.Facet, error) {
	rows, err := r.q.ListBranchCityFacets(ctx, db.ListBranchCityFacetsParams{OrganizationID: orgID, BranchIds: branchIDs})
	if err != nil {
		return nil, err
	}
	out := make([]model.Facet, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.Facet{Value: row.City, Count: row.Count})
	}
	return out, nil
}

func (r *Postgres) ListBranchUUIDs(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]uuid.UUID, error) {
	return r.q.ListBranchUUIDs(ctx, db.ListBranchUUIDsParams{
		OrganizationID: orgID, IsActive: boolArg(f.IsActive), Cities: citiesArg(f.Cities), Q: textArg(f.Q),
		BranchIds: f.BranchIDs, LimitCount: limit,
	})
}

func (r *Postgres) ListBranchesForExport(ctx context.Context, orgID int64, f model.ListFilter) ([]db.Branch, error) {
	return r.q.ListBranchesForExport(ctx, db.ListBranchesForExportParams{
		OrganizationID: orgID, IsActive: boolArg(f.IsActive), Cities: citiesArg(f.Cities), Q: textArg(f.Q),
		BranchIds: f.BranchIDs,
	})
}

func (r *Postgres) GetBranch(ctx context.Context, orgID int64, id uuid.UUID) (db.Branch, error) {
	row, err := r.q.GetBranchByUUID(ctx, db.GetBranchByUUIDParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) GetBranchByCode(ctx context.Context, orgID int64, code string) (db.Branch, error) {
	row, err := r.q.GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: orgID, Code: code})
	return row, mapErr(err)
}

func (r *Postgres) CodeTaken(ctx context.Context, orgID int64, code string, exclude *uuid.UUID) (bool, error) {
	var ex pgtype.UUID
	if exclude != nil {
		ex = pgtype.UUID{Bytes: *exclude, Valid: true}
	}
	return r.q.BranchCodeTaken(ctx, db.BranchCodeTakenParams{OrganizationID: orgID, Code: code, ExcludeUuid: ex})
}

func (r *Postgres) CreateBranch(ctx context.Context, orgID int64, f BranchFields) (db.Branch, error) {
	row, err := r.q.CreateBranch(ctx, db.CreateBranchParams{
		OrganizationID: orgID, Code: f.Code, Name: f.Name, Address: f.Address, City: f.City,
		District: f.District, Latitude: f.Latitude, Longitude: f.Longitude, Phone: f.Phone, Email: f.Email,
		WorkingHours: f.WorkingHours, HolidayExceptions: f.HolidayExceptions, PrepMinutes: f.PrepMinutes,
		IsActive: f.IsActive,
	})
	return row, mapErr(err)
}

func (r *Postgres) UpdateBranch(ctx context.Context, orgID int64, id uuid.UUID, f BranchFields) (db.Branch, error) {
	row, err := r.q.UpdateBranch(ctx, db.UpdateBranchParams{
		OrganizationID: orgID, Uuid: id, Code: f.Code, Name: f.Name, Address: f.Address, City: f.City,
		District: f.District, Latitude: f.Latitude, Longitude: f.Longitude, Phone: f.Phone, Email: f.Email,
		WorkingHours: f.WorkingHours, HolidayExceptions: f.HolidayExceptions, PrepMinutes: f.PrepMinutes,
		IsActive: f.IsActive,
	})
	return row, mapErr(err)
}

func (r *Postgres) SetBranchActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (db.Branch, error) {
	row, err := r.q.SetBranchActive(ctx, db.SetBranchActiveParams{OrganizationID: orgID, Uuid: id, IsActive: active})
	return row, mapErr(err)
}

func (r *Postgres) DeleteBranch(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := r.q.SoftDeleteBranch(ctx, db.SoftDeleteBranchParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Postgres) ListServicePoints(ctx context.Context, orgID, branchID int64) ([]db.ServicePoint, error) {
	return r.q.ListServicePointsByBranch(ctx, db.ListServicePointsByBranchParams{OrganizationID: orgID, BranchID: branchID})
}

func (r *Postgres) GetServicePoint(ctx context.Context, orgID, branchID int64, id uuid.UUID) (db.ServicePoint, error) {
	row, err := r.q.GetServicePoint(ctx, db.GetServicePointParams{OrganizationID: orgID, BranchID: branchID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) CreateServicePoint(ctx context.Context, orgID, branchID int64, f ServicePointFields) (db.ServicePoint, error) {
	f.ExtraFee = feeOrZero(f.ExtraFee)
	row, err := r.q.CreateServicePoint(ctx, db.CreateServicePointParams{
		OrganizationID: orgID, BranchID: branchID, Type: f.Type, Name: f.Name, Description: f.Description,
		ExtraFee: f.ExtraFee, WorkingHours: f.WorkingHours, OnlineSelectable: f.OnlineSelectable, IsActive: f.IsActive,
	})
	return row, mapErr(err)
}

func (r *Postgres) UpdateServicePoint(ctx context.Context, orgID, branchID int64, id uuid.UUID, f ServicePointFields) (db.ServicePoint, error) {
	f.ExtraFee = feeOrZero(f.ExtraFee)
	row, err := r.q.UpdateServicePoint(ctx, db.UpdateServicePointParams{
		OrganizationID: orgID, BranchID: branchID, Uuid: id, Type: f.Type, Name: f.Name, Description: f.Description,
		ExtraFee: f.ExtraFee, WorkingHours: f.WorkingHours, OnlineSelectable: f.OnlineSelectable, IsActive: f.IsActive,
	})
	return row, mapErr(err)
}

func (r *Postgres) DeleteServicePoint(ctx context.Context, orgID, branchID int64, id uuid.UUID) error {
	n, err := r.q.DeleteServicePoint(ctx, db.DeleteServicePointParams{OrganizationID: orgID, BranchID: branchID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Postgres) ListActivity(ctx context.Context, orgID int64, branchUUID uuid.UUID, limit, offset int32) ([]db.ListOrganizationResourceActivityRow, int64, error) {
	org := pgtype.Int8{Int64: orgID, Valid: true}
	ru := pgtype.UUID{Bytes: branchUUID, Valid: true}
	rows, err := r.q.ListOrganizationResourceActivity(ctx, db.ListOrganizationResourceActivityParams{
		OrganizationID: org, Resource: model.Resource, ResourceUuid: ru, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountOrganizationResourceActivity(ctx, db.CountOrganizationResourceActivityParams{
		OrganizationID: org, Resource: model.Resource, ResourceUuid: ru,
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_branches_organization_code" {
		return ErrCodeTaken
	}
	return err
}

func textArg(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func boolArg(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

// feeOrZero treats an unset fee as zero (the column is NOT NULL DEFAULT 0).
func feeOrZero(n pgtype.Numeric) pgtype.Numeric {
	if n.Valid {
		return n
	}
	var zero pgtype.Numeric
	_ = zero.Scan("0")
	return zero
}

func citiesArg(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}
