package sector

import "testing"

func TestCatalogIsConsistent(t *testing.T) {
	all := All()
	if len(all) != 6 {
		t.Fatalf("want 6 sectors, got %d", len(all))
	}
	known := map[Module]bool{}
	for _, m := range Modules {
		known[m] = true
	}
	seen := map[Code]bool{}
	for _, d := range all {
		if seen[d.Code] {
			t.Fatalf("duplicate sector %s", d.Code)
		}
		seen[d.Code] = true
		if !Valid(string(d.Code)) {
			t.Fatalf("%s not valid", d.Code)
		}
		if len(d.Flow) == 0 || d.Flow[0] != StepBranch {
			t.Fatalf("%s: flow must start with the branch", d.Code)
		}
		if d.Terms.Staff == "" || d.Terms.Resource == "" || d.Terms.Booking == "" {
			t.Fatalf("%s: missing terms", d.Code)
		}
		for _, m := range d.Modules {
			if !known[m] {
				t.Fatalf("%s: unknown module %s", d.Code, m)
			}
		}
		for _, h := range d.Highlights {
			if !d.Has(h) {
				t.Fatalf("%s: highlight %s is not visible", d.Code, h)
			}
		}
		if !d.Has(ModuleReservations) || !d.Has(ModuleCalendar) {
			t.Fatalf("%s: reservations and calendar are in every sector", d.Code)
		}
	}
}

func TestSectorSpecifics(t *testing.T) {
	if d := Of(string(Restaurant)); !d.Capacity.PartySize || !d.Capacity.ResourceRequired || d.Has(ModuleStaffShares) {
		t.Fatalf("restaurant: party size and a table, no staff shares: %+v", d)
	}
	if d := Of(string(Gym)); !d.Capacity.GroupSessions || !d.Has(ModuleClasses) {
		t.Fatalf("gym: group sessions: %+v", d)
	}
	if d := Of(string(CarWash)); !d.Has(ModuleVehicles) || d.Has(ModuleClasses) {
		t.Fatalf("car wash: vehicles, no classes: %+v", d)
	}
	if d := Of(string(Hairdresser)); !d.Has(ModuleStaffShares) || d.Highlights[0] != ModuleCalendar {
		t.Fatalf("hairdresser: staff shares: %+v", d)
	}
	if Of("unknown").Code != Default {
		t.Fatal("unknown code falls back to the default sector")
	}
	if Valid("bakery") {
		t.Fatal("bakery is not a sector")
	}
}
