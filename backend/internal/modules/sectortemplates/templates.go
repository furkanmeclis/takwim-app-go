// Package sectortemplates seeds the starter records of a sector template
// (TEC-903) when a business is created. The booking flow and the capacity
// rules of each sector live in internal/platform/sector; a template only
// adds what that flow cannot run without. Everything seeded is ordinary
// data the business edits or deletes.
package sectortemplates

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// Table is one starter table of the restaurant template.
type Table struct {
	Name        string
	Zone        string
	MinCapacity int32
	Capacity    int32
	Combinable  bool
}

// RestaurantSeatingMinutes is the default sitting of a table booking.
const RestaurantSeatingMinutes = 90

// RestaurantService is the table booking the restaurant flow books: no
// staff, one table that seats the party, free (paid at the business).
const RestaurantService = "Masa rezervasyonu"

// RestaurantTables are the starter tables: two zones, small tables, and
// combinable four-seat tables the business may join for a large party.
var RestaurantTables = []Table{
	{Name: "Masa 1", Zone: "Salon", MinCapacity: 1, Capacity: 2},
	{Name: "Masa 2", Zone: "Salon", MinCapacity: 1, Capacity: 2},
	{Name: "Masa 3", Zone: "Salon", MinCapacity: 2, Capacity: 4, Combinable: true},
	{Name: "Masa 4", Zone: "Salon", MinCapacity: 2, Capacity: 4, Combinable: true},
	{Name: "Masa 5", Zone: "Salon", MinCapacity: 4, Capacity: 6},
	{Name: "Masa 6", Zone: "Bahçe", MinCapacity: 2, Capacity: 4, Combinable: true},
	{Name: "Masa 7", Zone: "Bahçe", MinCapacity: 2, Capacity: 4, Combinable: true},
}

// Search specs of the seeded records (services / resources modules).
const (
	ServicesSpec  = "services"
	ResourcesSpec = "resources"
)

// Seeded is what a template created, for the search index.
type Seeded struct {
	Services  []string
	Resources []string
}

// Seed adds the sector's starter records to the first branch of a new
// business. Sectors without a template (the hairdresser's catalog is the
// salon's own) seed nothing.
func Seed(ctx context.Context, q *db.Queries, orgID int64, code string, branchID int64) (Seeded, error) {
	var out Seeded
	if sector.Code(code) != sector.Restaurant {
		return out, nil
	}
	svc, err := q.CreateTemplateService(ctx, db.CreateTemplateServiceParams{
		OrganizationID: orgID, Name: RestaurantService,
		Description:     "Kişi sayısına uygun masa, oturum süresi boyunca ayrılır.",
		DurationMinutes: RestaurantSeatingMinutes, PaymentMode: "on_site", StaffMode: "none",
		ResourceKinds: []string{string(sector.ResourceTable)},
	})
	if err != nil {
		return out, err
	}
	if err := q.CreateTemplateServiceOffer(ctx, db.CreateTemplateServiceOfferParams{OrganizationID: orgID, ServiceID: svc.ID, BranchID: branchID}); err != nil {
		return out, err
	}
	out.Services = append(out.Services, svc.Uuid.String())
	for i, t := range RestaurantTables {
		row, err := q.CreateTemplateResource(ctx, db.CreateTemplateResourceParams{
			OrganizationID: orgID, BranchID: branchID, Kind: string(sector.ResourceTable), Name: t.Name, Zone: t.Zone,
			MinCapacity: t.MinCapacity, Capacity: t.Capacity, Combinable: t.Combinable, SortOrder: int32(i),
		})
		if err != nil {
			return out, err
		}
		out.Resources = append(out.Resources, row.Uuid.String())
	}
	return out, nil
}
