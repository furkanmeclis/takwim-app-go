package usecase

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicbooking/model"
	bundlemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Bundles lists the service bundles a customer may book online (TEC-985):
// active, shown online, offered in a public branch and with every service
// bookable online there. With branchUUID only that branch's bundles, with
// the items priced for it; an unknown or hidden branch is branch_uuid /
// not_found. The payment modes are those every service of the bundle
// takes.
func (s *Service) Bundles(ctx context.Context, o Org, branchUUID *uuid.UUID) ([]model.Bundle, error) {
	c, err := s.loadCatalog(ctx, o)
	if err != nil {
		return nil, err
	}
	branchID, err := branchFilter(c, branchUUID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListPublicServiceBundles(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	out := []model.Bundle{}
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	items, err := s.q.ListServiceBundleItems(ctx, db.ListServiceBundleItemsParams{OrganizationID: o.ID, BundleIds: ids})
	if err != nil {
		return nil, err
	}
	branches, err := s.q.ListServiceBundleBranches(ctx, db.ListServiceBundleBranchesParams{OrganizationID: o.ID, BundleIds: ids})
	if err != nil {
		return nil, err
	}
	itemsOf := map[int64][]db.ListServiceBundleItemsRow{}
	for _, it := range items {
		itemsOf[it.BundleID] = append(itemsOf[it.BundleID], it)
	}
	branchesOf := map[int64][]int64{}
	for _, b := range branches {
		if _, ok := c.branches[b.BranchID]; ok {
			branchesOf[b.BundleID] = append(branchesOf[b.BundleID], b.BranchID)
		}
	}
	for _, r := range rows {
		b, ok := s.publicBundle(c, r, itemsOf[r.ID], branchesOf[r.ID], branchID, o.Currency)
		if ok {
			out = append(out, b)
		}
	}
	return out, nil
}

// publicBundle maps one bundle for the branches where every item is
// offered online; false when no public branch offers it (or not the
// filtered one).
func (s *Service) publicBundle(c *catalog, r db.ServiceBundle, items []db.ListServiceBundleItemsRow, branchIDs []int64, branchID int64, currency string) (model.Bundle, bool) {
	if len(items) == 0 {
		return model.Bundle{}, false
	}
	// A branch offers the bundle when it is one of the bundle's branches
	// and every service is a public service with an active offer there.
	offered := []int64{}
	for _, bid := range branchIDs {
		if branchID != 0 && bid != branchID {
			continue
		}
		all := true
		for _, it := range items {
			if it.ServiceDeleted || !it.ServiceIsActive {
				all = false
				break
			}
			if _, public := c.serviceUUID[it.ServiceID]; !public {
				all = false
				break
			}
			if !slices.ContainsFunc(c.offers[it.ServiceID], func(of db.ListPublicServiceOffersRow) bool { return of.BranchID == bid }) {
				all = false
				break
			}
		}
		if all {
			offered = append(offered, bid)
		}
	}
	if len(offered) == 0 {
		return model.Bundle{}, false
	}
	b := model.Bundle{
		UUID: r.Uuid, Name: r.Name, Description: r.Description, Price: money.FromNumericOrZero(r.Price),
		ListPrice: money.Zero(), Currency: currency, Items: make([]model.BundleItem, 0, len(items)),
		BranchUUIDs: make([]uuid.UUID, 0, len(offered)), PaymentModes: []string{"on_site", "online"},
	}
	for _, bid := range offered {
		b.BranchUUIDs = append(b.BranchUUIDs, c.branches[bid])
	}
	lists := make([]money.Money, 0, len(items))
	for _, it := range items {
		price, duration := money.FromNumericOrZero(it.ServicePrice), int(it.DurationMinutes)
		if branchID != 0 {
			for _, of := range c.offers[it.ServiceID] {
				if of.BranchID != branchID {
					continue
				}
				if of.Price.Valid {
					price = money.FromNumericOrZero(of.Price)
				}
				if of.DurationMinutes.Valid {
					duration = int(of.DurationMinutes.Int32)
				}
			}
		}
		lists = append(lists, price)
		b.ListPrice = b.ListPrice.Add(price)
		b.DurationMinutes += duration
		if it.PaymentMode != "both" {
			b.PaymentModes = slices.DeleteFunc(b.PaymentModes, func(m string) bool { return m != it.PaymentMode })
		}
		b.Items = append(b.Items, model.BundleItem{
			ServiceUUID: it.ServiceUuid, ServiceName: it.ServiceName, DurationMinutes: duration, Price: price, StaffMode: it.StaffMode,
		})
	}
	for i, share := range bundlemodel.Distribute(b.Price, lists) {
		b.Items[i].AllocatedPrice = share
	}
	b.Discount = b.ListPrice.Sub(b.Price).ClampZero()
	return b, true
}

// bundleLines checks the bundle a public caller names for the branch and
// returns the lines to search / hold: the bundle's services in order, each
// with the person the caller picked (none = anyone free). A bundle that is
// not active, not shown online or not bookable online in the branch is
// bundle_uuid / not_found (the same answer as an unknown one, so the route
// does not tell which bundles exist); lines naming other services are
// lines / bundle_mismatch.
func (s *Service) bundleLines(ctx context.Context, o Org, branchUUID, bundleUUID uuid.UUID, lines []availabilitymodel.LineInput) (model.Bundle, []availabilitymodel.LineInput, error) {
	bundles, err := s.Bundles(ctx, o, &branchUUID)
	if err != nil {
		return model.Bundle{}, nil, err
	}
	i := slices.IndexFunc(bundles, func(b model.Bundle) bool { return b.UUID == bundleUUID })
	if i < 0 {
		return model.Bundle{}, nil, invalid("bundle_uuid", "not_found", "bundle not found")
	}
	b := bundles[i]
	if len(lines) == 0 {
		lines = make([]availabilitymodel.LineInput, 0, len(b.Items))
		for _, it := range b.Items {
			lines = append(lines, availabilitymodel.LineInput{ServiceUUID: it.ServiceUUID})
		}
		return b, lines, nil
	}
	if len(lines) != len(b.Items) {
		return model.Bundle{}, nil, invalid("lines", "bundle_mismatch", "the lines must be the services of the bundle")
	}
	for i, it := range b.Items {
		if lines[i].ServiceUUID != it.ServiceUUID {
			return model.Bundle{}, nil, invalid("lines", "bundle_mismatch", "the lines must be the services of the bundle")
		}
	}
	return b, lines, nil
}

// checkoutBundle is the bundle of a checkout's lines (nil for services).
func checkoutBundle(lines []model.CheckoutLine) *model.Ref {
	for _, l := range lines {
		if l.Bundle != nil {
			b := *l.Bundle
			return &b
		}
	}
	return nil
}
