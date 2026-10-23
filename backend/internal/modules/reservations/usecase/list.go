package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// SortColumns are the sortable list fields.
var SortColumns = map[string]string{
	"starts_at": "starts_at", "number": "number", "customer": "customer", "status": "status",
	"total_price": "total_price", "created_at": "created_at",
}

// List answers the panel list inside the caller's branch scope; an own
// calendar member sees what it created or serves.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter, caller availabilityusecase.Caller) ([]model.ListItem, int64, error) {
	if _, ok := SortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "starts_at", true
	}
	p := db.ListReservationsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Branches: f.Branches, Staff: f.Staff,
		Statuses: f.Statuses, ApprovalStatuses: f.ApprovalStatuses, PaymentStatuses: f.PaymentStatuses, Sources: f.Sources,
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if f.CustomerUUID != nil {
		p.CustomerUuid = pgtype.UUID{Bytes: *f.CustomerUUID, Valid: true}
	}
	if f.StartsFrom != nil {
		p.StartsFrom = ts(*f.StartsFrom)
	}
	if f.StartsTo != nil {
		p.StartsTo = ts(*f.StartsTo)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Q = pgtype.Text{String: strings.TrimPrefix(q, "#"), Valid: true}
		if digits := onlyDigits(q); len(digits) >= 3 {
			p.QDigits = pgtype.Text{String: digits, Valid: true}
		}
	}
	if caller.OwnUserID > 0 {
		p.OwnUserID = pgtype.Int8{Int64: caller.OwnUserID, Valid: true}
		member, err := s.q.GetAvailabilityStaffByUser(ctx, db.GetAvailabilityStaffByUserParams{OrganizationID: orgID, UserID: pgtype.Int8{Int64: caller.OwnUserID, Valid: true}})
		switch {
		case err == nil:
			p.OwnStaffID = pgtype.Int8{Int64: member.ID, Valid: true}
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, 0, err
		}
	}
	rows, err := s.q.ListReservations(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountReservations(ctx, db.CountReservationsParams{
		OrganizationID: p.OrganizationID, BranchIds: p.BranchIds, OwnUserID: p.OwnUserID, OwnStaffID: p.OwnStaffID,
		Branches: p.Branches, CustomerUuid: p.CustomerUuid, Staff: p.Staff, Statuses: p.Statuses,
		ApprovalStatuses: p.ApprovalStatuses, PaymentStatuses: p.PaymentStatuses, Sources: p.Sources,
		StartsFrom: p.StartsFrom, StartsTo: p.StartsTo, Q: p.Q, QDigits: p.QDigits,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ListItem, 0, len(rows))
	for _, r := range rows {
		item := model.ListItem{
			UUID: r.Uuid, Number: r.Number, Status: r.Status, ApprovalStatus: r.ApprovalStatus,
			ConfirmationStatus: r.ConfirmationStatus, PaymentStatus: r.PaymentStatus, Source: r.Source,
			Branch:   model.Ref{UUID: r.BranchUuid, Name: r.BranchName},
			Customer: model.Customer{UUID: r.CustomerUuid, Name: strings.TrimSpace(r.CustomerFirstName + " " + r.CustomerLastName)},
			Services: r.Services, Staff: r.StaffNames, StartsAt: r.StartsAt.Time.UTC(), EndsAt: r.EndsAt.Time.UTC(),
			TotalPrice: money.FromNumericOrZero(r.TotalPrice), Currency: r.Currency, Version: int(r.Version),
			CreatedAt: r.CreatedAt.Time.UTC(), PartySize: partyOf(r.PartySize), Zone: r.Zone,
		}
		if r.CustomerPhone.Valid {
			item.Customer.Phone = &r.CustomerPhone.String
		}
		out = append(out, item)
	}
	return out, total, nil
}

// Facets returns the branches and staff members of the list filters.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	out := model.Facets{Branch: []model.Facet{}, Staff: []model.Facet{}}
	scope := orgctx.BranchScope(ctx)
	branches, err := s.q.ListReservationBranchFacets(ctx, db.ListReservationBranchFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, b := range branches {
		out.Branch = append(out.Branch, model.Facet{Value: b.Value.String(), Label: b.Label, Count: b.Count})
	}
	staff, err := s.q.ListReservationStaffFacets(ctx, db.ListReservationStaffFacetsParams{OrganizationID: orgID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, st := range staff {
		out.Staff = append(out.Staff, model.Facet{Value: st.Value.String(), Label: st.Label, Count: st.Count})
	}
	return out, nil
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
