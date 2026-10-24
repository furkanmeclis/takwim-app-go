package usecase

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// "Paketlerim" (TEC-984): the person's session packages across businesses,
// what is left of them, until when, and the visits that used them. The
// panel only reads; a package is sold, used and refunded at the business.

const dateLayout = "2006-01-02"

// Packages returns one page of the person's packages: valid ones first,
// soonest to expire first.
func (s *Service) Packages(ctx context.Context, p Principal, f model.PackageFilter) ([]model.Package, int64, error) {
	s.linkRecords(ctx, p.Phone, p.VerifiedAt, p.source())
	now := ts(s.now())
	var business pgtype.UUID
	if f.BusinessUUID != nil {
		business = pgtype.UUID{Bytes: *f.BusinessUUID, Valid: true}
	}
	var states []string
	if len(f.States) > 0 {
		states = f.States
	}
	rows, err := s.q.ListCustomerPanelPackages(ctx, db.ListCustomerPanelPackagesParams{
		IdentityID: p.IdentityID, Now: now, BusinessUuid: business, States: states,
		RowLimit: f.Limit, RowOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountCustomerPanelPackages(ctx, db.CountCustomerPanelPackagesParams{
		IdentityID: p.IdentityID, Now: now, BusinessUuid: business, States: states,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Package, 0, len(rows))
	for _, r := range rows {
		out = append(out, packageOf(r))
	}
	return out, total, nil
}

// Package returns one package of the person with its usage history.
// Another person's package and an unknown uuid answer ErrNotFound.
func (s *Service) Package(ctx context.Context, p Principal, id uuid.UUID) (model.Package, error) {
	rows, err := s.q.ListCustomerPanelPackages(ctx, db.ListCustomerPanelPackagesParams{
		IdentityID: p.IdentityID, Now: ts(s.now()), PackageUuid: pgtype.UUID{Bytes: id, Valid: true},
		RowLimit: 1, RowOffset: 0,
	})
	if err != nil {
		return model.Package{}, err
	}
	if len(rows) == 0 {
		return model.Package{}, ErrNotFound
	}
	out := packageOf(rows[0])
	sessions, err := s.q.ListCustomerPanelPackageSessions(ctx, db.ListCustomerPanelPackageSessionsParams{
		OrganizationID: rows[0].OrganizationID, CustomerPackageID: rows[0].ID,
	})
	if err != nil {
		return out, err
	}
	out.Sessions = make([]model.PackageSession, 0, len(sessions))
	for _, r := range sessions {
		ps := model.PackageSession{
			UUID: r.Uuid, SessionNo: int(r.SessionNo), Status: r.Status, ReservationUUID: r.ReservationUuid,
			ReservationNumber: r.ReservationNumber, StartsAt: r.ReservationStartsAt.Time.UTC(),
			ReservationStatus: r.ReservationStatus, BranchName: r.BranchName, Timezone: r.Timezone,
		}
		if r.ConsumedOn.Valid {
			d := r.ConsumedOn.Time.Format(dateLayout)
			ps.ConsumedOn = &d
		}
		if r.ServiceName.Valid {
			v := r.ServiceName.String
			ps.ServiceName = &v
		}
		if r.StaffName.Valid {
			v := r.StaffName.String
			ps.StaffName = &v
		}
		out.Sessions = append(out.Sessions, ps)
	}
	return out, nil
}

func packageOf(r db.ListCustomerPanelPackagesRow) model.Package {
	used, held := int(r.ConsumedCount), int(r.HeldCount)
	out := model.Package{
		UUID:     r.Uuid,
		Business: model.PackageBusiness{UUID: r.BusinessUuid, Slug: r.BusinessSlug, Name: r.BusinessName},
		Name:     r.Name, ServiceName: r.ServiceName, TotalSessions: int(r.TotalSessions), Used: used, Held: held,
		Remaining: max(int(r.TotalSessions)-used-held, 0), Price: money.FromNumericOrZero(r.Price), Currency: r.Currency,
		SoldOn: r.SoldOn.Time.Format(dateLayout), State: r.State,
		Branch:      model.PackageRef{UUID: r.BranchUuid, Name: r.BranchName},
		AllBranches: r.AllBranches, Branches: []model.PackageRef{}, CreatedAt: r.CreatedAt.Time.UTC(),
	}
	if r.State == model.PackageRefunded {
		out.Remaining = 0
	}
	if r.ServiceUuid.Valid {
		u := uuid.UUID(r.ServiceUuid.Bytes)
		out.ServiceUUID = &u
	}
	if r.ExpiresOn.Valid {
		d := r.ExpiresOn.Time.Format(dateLayout)
		out.ExpiresOn = &d
	}
	if r.RefundedOn.Valid {
		d := r.RefundedOn.Time.Format(dateLayout)
		out.RefundedOn = &d
	}
	for i, id := range r.BranchUuids {
		name := ""
		if i < len(r.BranchNames) {
			name = r.BranchNames[i]
		}
		out.Branches = append(out.Branches, model.PackageRef{UUID: id, Name: name})
	}
	return out
}

// LinkedCustomer returns the business's customer record the person books
// as (TEC-984): the record the person's proven phone is linked to now.
// ErrNotFound when the business does not know the person. It runs inside
// the booking's transaction, so it only reads (the panel reads link new
// records before).
func (s *Service) LinkedCustomer(ctx context.Context, p Principal, orgID int64) (uuid.UUID, error) {
	row, err := s.q.GetCustomerPanelLinkedCustomer(ctx, db.GetCustomerPanelLinkedCustomerParams{
		OrganizationID: orgID, IdentityID: p.IdentityID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	return row.Uuid, nil
}
