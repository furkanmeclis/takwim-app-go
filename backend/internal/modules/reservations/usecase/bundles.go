package usecase

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	bundlemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// bundlePlan is the service bundle a booking takes (TEC-895).
type bundlePlan struct {
	id    int64
	name  string
	price money.Money
}

// planBundle checks the bundle a booking names and returns its lines: the
// bundle's services in order. Given lines must be exactly those services
// (each with the staff member the caller picked); without lines every
// service takes any free staff member. The bundle must be active, offered
// in the branch and, for online bookings, shown online.
func planBundle(ctx context.Context, q *db.Queries, orgID int64, branchUUID, bundleUUID uuid.UUID, lines []availabilitymodel.LineInput, online bool) (*bundlePlan, []availabilitymodel.LineInput, error) {
	ve := &ValidationError{}
	var branchID int64
	if branchUUID != uuid.Nil {
		rows, err := q.ListBranchIDsByUUIDs(ctx, db.ListBranchIDsByUUIDsParams{OrganizationID: orgID, Uuids: []uuid.UUID{branchUUID}})
		if err != nil {
			return nil, nil, err
		}
		if len(rows) == 0 {
			// The planner answers for an unknown branch.
			return nil, lines, nil
		}
		branchID = rows[0].ID
	}
	b, err := q.GetReservationBundle(ctx, db.GetReservationBundleParams{OrganizationID: orgID, Uuid: bundleUUID, BranchID: branchID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && online && (!b.ShowOnline || !b.IsActive)) {
		ve.add("bundle_uuid", "not_found", "bundle not found")
		return nil, nil, ve
	}
	if err != nil {
		return nil, nil, err
	}
	switch {
	case !b.IsActive:
		ve.add("bundle_uuid", "inactive", "the bundle is not active")
	case !b.Offered:
		ve.add("bundle_uuid", "not_offered", "the branch does not offer the bundle")
	}
	items, err := q.ListServiceBundleItems(ctx, db.ListServiceBundleItemsParams{OrganizationID: orgID, BundleIds: []int64{b.ID}})
	if err != nil {
		return nil, nil, err
	}
	for _, it := range items {
		if it.ServiceDeleted || !it.ServiceIsActive {
			ve.add("bundle_uuid", "not_bookable", "a service of the bundle is not bookable")
			break
		}
	}
	if len(items) == 0 {
		ve.add("bundle_uuid", "not_bookable", "the bundle has no services")
	}
	if err := ve.orNil(); err != nil {
		return nil, nil, err
	}
	if len(lines) == 0 {
		lines = make([]availabilitymodel.LineInput, 0, len(items))
		for _, it := range items {
			lines = append(lines, availabilitymodel.LineInput{ServiceUUID: it.ServiceUuid})
		}
	} else {
		if len(lines) != len(items) {
			ve.add("lines", "bundle_mismatch", "the lines must be the services of the bundle")
			return nil, nil, ve
		}
		for i, it := range items {
			if lines[i].ServiceUUID != it.ServiceUuid {
				ve.add("lines", "bundle_mismatch", "the lines must be the services of the bundle")
				return nil, nil, ve
			}
		}
	}
	return &bundlePlan{id: b.ID, name: b.Name, price: money.FromNumericOrZero(b.Price)}, lines, nil
}

// applyBundle spreads the bundle price over the priced lines in proportion
// to their prices (remainder to the last line) and stamps the bundle on
// them. It returns the new total, the bundle price.
func applyBundle(lines []db.CreateReservationLineParams, b *bundlePlan) money.Money {
	lists := make([]money.Money, len(lines))
	for i, l := range lines {
		lists[i] = money.FromNumericOrZero(l.UnitPrice)
	}
	for i, share := range bundlemodel.Distribute(b.price, lists) {
		lines[i].ListPrice = lines[i].UnitPrice
		lines[i].UnitPrice = share.Numeric()
		lines[i].BundleID = pgtype.Int8{Int64: b.id, Valid: true}
		lines[i].BundleName = pgtype.Text{String: b.name, Valid: true}
	}
	return b.price
}

// keepBundle carries the bundle split of a booking over a move that keeps
// its services in the same order (staff members may change): the lines keep
// their shares and list prices. It reports false when the booking was not
// a bundle or its services changed (the lines then take today's prices).
func keepBundle(priced []db.CreateReservationLineParams, before []db.ListReservationLinePricesRow) bool {
	if len(before) == 0 || len(priced) != len(before) || !before[0].BundleName.Valid {
		return false
	}
	for i := range priced {
		if !before[i].BundleName.Valid || before[i].ServiceID != priced[i].ServiceID {
			return false
		}
	}
	for i := range priced {
		priced[i].UnitPrice = before[i].UnitPrice
		priced[i].ListPrice = before[i].ListPrice
		priced[i].BundleID = before[i].BundleID
		priced[i].BundleName = before[i].BundleName
	}
	return true
}
