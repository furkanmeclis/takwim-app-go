// Package sectortemplates seeds the starter records of a sector template
// (TEC-903, TEC-904) when a business is created. The booking flow and the
// capacity rules of each sector live in internal/platform/sector; a
// template only adds what that flow cannot run without, or what every
// business of the sector has. Everything seeded is ordinary data the
// business edits or deletes.
package sectortemplates

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// Resource is one starter resource of a template. Zone and the capacity
// range matter for party seating (restaurant tables).
type Resource struct {
	Kind        sector.ResourceKind
	Name        string
	Zone        string
	MinCapacity int32
	Capacity    int32
	Combinable  bool
}

// Service is one starter service of a template: free (price 0), paid at
// the business, offered in the default branch.
type Service struct {
	Name            string
	Description     string
	DurationMinutes int32
	StaffMode       string
	ResourceKinds   []sector.ResourceKind
	// GroupCapacity > 0 makes it a group class with that many seats.
	GroupCapacity int32
}

// Template is what a sector seeds into the default branch.
type Template struct {
	Services  []Service
	Resources []Resource
}

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

// GymClass is the starter group class of the gym template: one hour,
// twelve seats, a trainer when one is assigned, held in the studio.
const (
	GymClass        = "Grup dersi"
	GymClassSeats   = 12
	GymClassMinutes = 60
	GymStudio       = "Stüdyo"
)

// CarWashBays are the starter wash bays: every car wash booking needs one.
var CarWashBays = []string{"Yıkama alanı 1", "Yıkama alanı 2"}

// ClinicRooms are the starter examination rooms; an appointment type
// takes one when the clinic marks it as needing a room.
var ClinicRooms = []string{"Muayene odası 1", "Muayene odası 2"}

// For returns the template of a sector. Hairdressers and beauty salons get
// nothing: their services, prices, chairs, rooms and devices are each
// salon's own, and the flow runs on what the owner adds.
func For(code sector.Code) Template {
	switch code {
	case sector.Restaurant:
		t := Template{Services: []Service{{
			Name: RestaurantService, Description: "Kişi sayısına uygun masa, oturum süresi boyunca ayrılır.",
			DurationMinutes: RestaurantSeatingMinutes, StaffMode: sector.StaffNone,
			ResourceKinds: []sector.ResourceKind{sector.ResourceTable},
		}}}
		for _, tb := range RestaurantTables {
			t.Resources = append(t.Resources, Resource{
				Kind: sector.ResourceTable, Name: tb.Name, Zone: tb.Zone,
				MinCapacity: tb.MinCapacity, Capacity: tb.Capacity, Combinable: tb.Combinable,
			})
		}
		return t
	case sector.Gym:
		return Template{
			Services: []Service{{
				Name: GymClass, Description: "Kontenjanlı grup dersi; her seans eğitmeni ve stüdyoyu ayırır.",
				DurationMinutes: GymClassMinutes, StaffMode: sector.StaffOptional,
				ResourceKinds: []sector.ResourceKind{sector.ResourceArea}, GroupCapacity: GymClassSeats,
			}},
			Resources: []Resource{{Kind: sector.ResourceArea, Name: GymStudio, MinCapacity: 1, Capacity: GymClassSeats}},
		}
	case sector.CarWash:
		var t Template
		for _, name := range CarWashBays {
			t.Resources = append(t.Resources, Resource{Kind: sector.ResourceBay, Name: name, MinCapacity: 1, Capacity: 1})
		}
		return t
	case sector.Clinic:
		var t Template
		for _, name := range ClinicRooms {
			t.Resources = append(t.Resources, Resource{Kind: sector.ResourceRoom, Name: name, MinCapacity: 1, Capacity: 1})
		}
		return t
	}
	return Template{}
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
// business.
func Seed(ctx context.Context, q *db.Queries, orgID int64, code string, branchID int64) (Seeded, error) {
	var out Seeded
	t := For(sector.Code(code))
	for _, s := range t.Services {
		kinds := make([]string, 0, len(s.ResourceKinds))
		for _, k := range s.ResourceKinds {
			kinds = append(kinds, string(k))
		}
		svc, err := q.CreateTemplateService(ctx, db.CreateTemplateServiceParams{
			OrganizationID: orgID, Name: s.Name, Description: s.Description,
			DurationMinutes: s.DurationMinutes, PaymentMode: "on_site", StaffMode: s.StaffMode, ResourceKinds: kinds,
			GroupCapacity: pgtype.Int4{Int32: s.GroupCapacity, Valid: s.GroupCapacity > 0},
		})
		if err != nil {
			return out, err
		}
		if err := q.CreateTemplateServiceOffer(ctx, db.CreateTemplateServiceOfferParams{OrganizationID: orgID, ServiceID: svc.ID, BranchID: branchID}); err != nil {
			return out, err
		}
		out.Services = append(out.Services, svc.Uuid.String())
	}
	for i, r := range t.Resources {
		row, err := q.CreateTemplateResource(ctx, db.CreateTemplateResourceParams{
			OrganizationID: orgID, BranchID: branchID, Kind: string(r.Kind), Name: r.Name, Zone: r.Zone,
			MinCapacity: r.MinCapacity, Capacity: r.Capacity, Combinable: r.Combinable, SortOrder: int32(i),
		})
		if err != nil {
			return out, err
		}
		out.Resources = append(out.Resources, row.Uuid.String())
	}
	return out, nil
}
