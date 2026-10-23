package reservations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// TestRestaurantTemplate (TEC-903, A02 restaurant): a new restaurant gets
// the table booking service and starter tables; a booking needs the party
// size, seats the party at a table that fits in the chosen area, joins
// combinable tables only from the panel, keeps the party on a move and is
// never paid online without a price.
func TestRestaurantTemplate(t *testing.T) {
	f := newFixture(t)
	bg := context.Background()
	orgSvc := orgusecase.New(orgrepository.NewPostgres(f.pool, f.q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(bg, orgmodel.CreateOrganizationInput{
		Name: "Restoran " + uuid.NewString()[:8], OwnerUserID: f.owner.ID, Sector: string(sector.Restaurant),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f.pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID) })
	ctx := orgctx.WithScope(bg, orgctx.Scope{ID: org.ID, Sector: org.Sector, Timezone: "Europe/Istanbul"})

	if n := f.count(`SELECT count(*) FROM resources WHERE organization_id = $1 AND kind = 'table'`, org.ID); n != 7 {
		t.Fatalf("starter tables = %d, want 7", n)
	}
	var branch, service, customer uuid.UUID
	if err := f.pool.QueryRow(bg, `SELECT b.uuid, s.uuid FROM branches b
		JOIN service_branches sb ON sb.organization_id = b.organization_id AND sb.branch_id = b.id
		JOIN services s ON s.organization_id = sb.organization_id AND s.id = sb.service_id
		WHERE b.organization_id = $1 AND s.staff_mode = 'none' AND s.resource_kinds = '{table}'`, org.ID).Scan(&branch, &service); err != nil {
		t.Fatalf("table booking service: %v", err)
	}
	f.exec(`UPDATE branches SET working_hours = '{"mon":{"closed":false,"open":"12:00","close":"23:00"}}', holiday_exceptions = '[]'
		WHERE organization_id = $1`, org.ID)
	if err := f.pool.QueryRow(bg, `INSERT INTO customers (organization_id, first_name, last_name) VALUES ($1, 'Deniz', 'Ak') RETURNING uuid`, org.ID).Scan(&customer); err != nil {
		t.Fatal(err)
	}
	booking := func(party int, zone string) model.CreateInput {
		start := at(19, 0)
		in := model.CreateInput{BranchUUID: branch, CustomerUUID: customer, StartsAt: &start, Zone: zone,
			Lines: []availabilitymodel.LineInput{{ServiceUUID: service}}}
		if party > 0 {
			in.PartySize = &party
		}
		return in
	}
	tablesOf := func(r model.Reservation) []string {
		var names []string
		for _, res := range r.Lines[0].Resources {
			names = append(names, res.Name)
		}
		return names
	}

	var ave *availabilityusecase.ValidationError
	if _, err := f.svc.Create(ctx, org.ID, booking(0, ""), none, f.owner.ID); !errors.As(err, &ave) || ave.Details[0].Field != "party_size" {
		t.Fatalf("booking without a party size err = %v", err)
	}
	two, err := f.svc.Create(ctx, org.ID, booking(2, ""), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := tablesOf(two); len(got) != 1 || got[0] != "Masa 1" || two.PartySize == nil || *two.PartySize != 2 {
		t.Fatalf("party of 2 = %v / %+v", got, two.PartySize)
	}
	if !two.EndsAt.Equal(at(20, 30)) {
		t.Fatalf("sitting ends %v, want 20:30", two.EndsAt)
	}
	garden, err := f.svc.Create(ctx, org.ID, booking(4, "Bahçe"), none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := tablesOf(garden); len(got) != 1 || got[0] != "Masa 6" || garden.Zone != "Bahçe" {
		t.Fatalf("party of 4 in the garden = %v (%q)", got, garden.Zone)
	}
	// Ten people: no single table, and the joined combinable tables seat
	// eight in the salon and four in the garden (one is taken).
	if _, err := f.svc.Create(ctx, org.ID, booking(10, ""), none, f.owner.ID); !errors.Is(err, availabilityusecase.ErrSlotTaken) {
		t.Fatalf("party of 10 err = %v, want taken", err)
	}
	eight, err := f.svc.Create(ctx, org.ID, booking(8, "Salon"), none, f.owner.ID)
	if err != nil {
		t.Fatalf("party of 8 joined in the salon: %v", err)
	}
	if got := tablesOf(eight); len(got) != 2 {
		t.Fatalf("party of 8 = %v, want two joined tables", got)
	}

	// A move keeps the party; a new party size is planned again.
	later := at(21, 0)
	moved, err := f.svc.Reschedule(ctx, org.ID, two.UUID, none, f.owner.ID, model.RescheduleInput{StartsAt: &later})
	if err != nil {
		t.Fatal(err)
	}
	if moved.PartySize == nil || *moved.PartySize != 2 || tablesOf(moved)[0] != "Masa 1" {
		t.Fatalf("moved = %+v %v", moved.PartySize, tablesOf(moved))
	}
	five := 5
	grown, err := f.svc.Reschedule(ctx, org.ID, two.UUID, none, f.owner.ID, model.RescheduleInput{StartsAt: &later, PartySize: &five})
	if err != nil {
		t.Fatal(err)
	}
	if *grown.PartySize != 5 || tablesOf(grown)[0] != "Masa 5" {
		t.Fatalf("grown = %+v %v", grown.PartySize, tablesOf(grown))
	}

	// Online: a free table booking is paid at the business, never online.
	lead, advance, window, payment := 0, 30, 30, 15
	s := orgsettingsmodel.Defaults()
	s.Reservation = orgsettingsmodel.Reservation{
		ApprovalMode: "auto", StaffReviewWindowMinutes: &window, PaymentHoldMinutes: &payment,
		MinLeadMinutes: &lead, MaxAdvanceDays: &advance,
	}
	s.Payment = orgsettingsmodel.Payment{PayAtVenue: true, PayOnline: true}
	if _, err := f.settings.Save(bg, org.ID, f.owner.ID, orgsettingsmodel.SaveInput{Settings: s}); err != nil {
		t.Fatal(err)
	}
	f.exec(`UPDATE services SET payment_mode = 'both' WHERE organization_id = $1`, org.ID)
	var ve *usecase.ValidationError
	online := booking(2, "")
	online.Source, online.Payment = model.SourceOnline, model.MethodOnline
	if _, err := f.svc.Create(ctx, org.ID, online, none, 0); !errors.As(err, &ve) || ve.Details[0].Field != "payment" {
		t.Fatalf("online payment of a free booking err = %v", err)
	}
}
