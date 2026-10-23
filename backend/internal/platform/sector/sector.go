// Package sector holds the six business sectors of Takwim (TEC-882) and
// what each one changes: the default customer booking flow, the kinds of
// capacity a reservation holds, the words the panel uses (masa, oda,
// koltuk …) and which panel modules are visible or highlighted.
//
// The catalog lives in code, not in the database: a sector is a product
// decision, and every client (panel, booking page, the future mobile app)
// reads the same definition from the API. A business has exactly one
// sector (organizations.sector); its own settings fine-tune the flow later
// (requirements §6).
package sector

// Code identifies a sector. The values are stored in organizations.sector
// (CHECK constraint in migration 000039).
type Code string

// The six beta sectors (requirements K-008).
const (
	Hairdresser Code = "hairdresser"
	Restaurant  Code = "restaurant"
	Gym         Code = "gym"
	CarWash     Code = "car_wash"
	Clinic      Code = "clinic"
	BeautySalon Code = "beauty_salon"
)

// Default is the sector of a business created without one (hairdresser is
// a pilot priority).
const Default = Hairdresser

// Module is a panel area that a sector shows or hides. Core areas
// (dashboard, branches, members, roles, settings, integrations) are not
// modules: every business has them. Plan entitlements (TEC-883) narrow
// the visible set further; they never add a module the sector hides.
type Module string

// Modules.
const (
	ModuleReservations Module = "reservations"
	ModuleCalendar     Module = "calendar"
	ModuleCustomers    Module = "customers"
	ModuleServices     Module = "services"
	ModulePackages     Module = "packages"
	ModuleProducts     Module = "products"
	ModuleStaff        Module = "staff"
	ModuleStaffShares  Module = "staff_shares"
	ModuleResources    Module = "resources"
	ModuleClasses      Module = "classes"
	ModuleVehicles     Module = "vehicles"
	ModuleCash         Module = "cash"
	ModuleFinance      Module = "finance"
	ModuleReports      Module = "reports"
	ModuleConfirmation Module = "confirmation"
	ModuleBookingPage  Module = "booking_page"
)

// Modules lists every module in display order.
var Modules = []Module{
	ModuleReservations, ModuleCalendar, ModuleCustomers, ModuleServices,
	ModulePackages, ModuleProducts, ModuleStaff, ModuleStaffShares,
	ModuleResources, ModuleClasses, ModuleVehicles, ModuleCash,
	ModuleFinance, ModuleReports, ModuleConfirmation, ModuleBookingPage,
}

// Step is one step of the customer booking flow (requirements §5).
type Step string

// Booking flow steps.
const (
	StepBranch      Step = "branch"
	StepService     Step = "service"
	StepStaff       Step = "staff"
	StepPartySize   Step = "party_size"
	StepSessionType Step = "session_type"
	StepClass       Step = "class"
	StepVehicle     Step = "vehicle"
	StepAppointment Step = "appointment_type"
	StepDateTime    Step = "datetime"
	StepArea        Step = "area"
	StepProducts    Step = "products"
	StepContact     Step = "contact"
	StepSummary     Step = "summary"
	StepPayment     Step = "payment"
)

// ResourceKind is a kind of capacity a reservation can hold besides staff
// time.
type ResourceKind string

// Resource kinds.
const (
	ResourceChair  ResourceKind = "chair"
	ResourceTable  ResourceKind = "table"
	ResourceArea   ResourceKind = "area"
	ResourceRoom   ResourceKind = "room"
	ResourceDevice ResourceKind = "device"
	ResourceBay    ResourceKind = "bay"
)

// Capacity is what a reservation of the sector holds (requirements §5).
type Capacity struct {
	// Staff: the reservation holds a staff member's time.
	Staff string `json:"staff"`
	// Resources the sector books, in display order.
	Resources []ResourceKind `json:"resources"`
	// ResourceRequired: a reservation needs a resource (restaurant table,
	// car wash bay); otherwise a service asks for one when it needs it.
	ResourceRequired bool `json:"resource_required"`
	// GroupSessions: sessions with a seat count (gym classes).
	GroupSessions bool `json:"group_sessions"`
	// PartySize: the customer gives a number of people (restaurant).
	PartySize bool `json:"party_size"`
}

// Staff requirement values of Capacity.Staff.
const (
	StaffRequired = "required"
	StaffOptional = "optional"
	StaffNone     = "none"
)

// Terms are the words the panel uses for the sector. Each value is a term
// code the clients translate (`sectors.terms.<code>`).
type Terms struct {
	// Staff: who serves the customer (çalışan, uzman, eğitmen).
	Staff string `json:"staff"`
	// Resource: the bookable place or equipment (koltuk, masa, oda/cihaz,
	// alan, yıkama alanı).
	Resource string `json:"resource"`
	// Booking: the customer's booking (randevu, rezervasyon).
	Booking string `json:"booking"`
}

// Definition is one sector.
type Definition struct {
	Code Code `json:"code"`
	// Flow is the default customer booking flow.
	Flow     []Step   `json:"flow"`
	Capacity Capacity `json:"capacity"`
	Terms    Terms    `json:"terms"`
	// Modules are the visible panel modules, in display order.
	Modules []Module `json:"modules"`
	// Highlights are the modules the panel puts first (subset of Modules).
	Highlights []Module `json:"highlights"`
}

// Has reports whether the sector shows a module.
func (d Definition) Has(m Module) bool {
	for _, v := range d.Modules {
		if v == m {
			return true
		}
	}
	return false
}

// without returns Modules minus the given ones, in order.
func without(drop ...Module) []Module {
	out := make([]Module, 0, len(Modules))
	for _, m := range Modules {
		skip := false
		for _, d := range drop {
			if m == d {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, m)
		}
	}
	return out
}

var catalog = []Definition{
	{
		Code: Hairdresser,
		Flow: []Step{StepBranch, StepService, StepStaff, StepDateTime, StepProducts, StepContact, StepSummary, StepPayment},
		Capacity: Capacity{
			Staff: StaffRequired, Resources: []ResourceKind{ResourceChair},
		},
		Terms:      Terms{Staff: "staff", Resource: "chair", Booking: "appointment"},
		Modules:    without(ModuleClasses, ModuleVehicles),
		Highlights: []Module{ModuleCalendar, ModuleStaffShares, ModuleServices, ModuleProducts},
	},
	{
		Code: Restaurant,
		Flow: []Step{StepBranch, StepPartySize, StepDateTime, StepArea, StepContact, StepSummary},
		Capacity: Capacity{
			Staff: StaffNone, Resources: []ResourceKind{ResourceTable, ResourceArea},
			ResourceRequired: true, PartySize: true,
		},
		Terms:      Terms{Staff: "staff", Resource: "table", Booking: "reservation"},
		Modules:    without(ModulePackages, ModuleProducts, ModuleStaffShares, ModuleClasses, ModuleVehicles),
		Highlights: []Module{ModuleResources, ModuleCalendar, ModuleReservations},
	},
	{
		Code: Gym,
		Flow: []Step{StepBranch, StepSessionType, StepClass, StepDateTime, StepContact, StepSummary, StepPayment},
		Capacity: Capacity{
			Staff: StaffOptional, Resources: []ResourceKind{ResourceArea},
			GroupSessions: true,
		},
		Terms:      Terms{Staff: "trainer", Resource: "area", Booking: "reservation"},
		Modules:    without(ModuleVehicles),
		Highlights: []Module{ModuleClasses, ModulePackages, ModuleCalendar},
	},
	{
		Code: CarWash,
		Flow: []Step{StepBranch, StepVehicle, StepService, StepDateTime, StepProducts, StepContact, StepSummary, StepPayment},
		Capacity: Capacity{
			Staff: StaffOptional, Resources: []ResourceKind{ResourceBay},
			ResourceRequired: true,
		},
		Terms:      Terms{Staff: "staff", Resource: "bay", Booking: "appointment"},
		Modules:    without(ModuleClasses),
		Highlights: []Module{ModuleResources, ModuleVehicles, ModuleServices},
	},
	{
		Code: Clinic,
		Flow: []Step{StepBranch, StepAppointment, StepStaff, StepDateTime, StepContact, StepSummary, StepPayment},
		Capacity: Capacity{
			Staff: StaffRequired, Resources: []ResourceKind{ResourceRoom},
		},
		Terms:      Terms{Staff: "specialist", Resource: "room", Booking: "appointment"},
		Modules:    without(ModuleProducts, ModuleClasses, ModuleVehicles),
		Highlights: []Module{ModuleCalendar, ModuleStaff, ModuleResources},
	},
	{
		Code: BeautySalon,
		Flow: []Step{StepBranch, StepService, StepStaff, StepDateTime, StepProducts, StepContact, StepSummary, StepPayment},
		Capacity: Capacity{
			Staff: StaffRequired, Resources: []ResourceKind{ResourceRoom, ResourceDevice},
		},
		Terms:      Terms{Staff: "specialist", Resource: "room_device", Booking: "appointment"},
		Modules:    without(ModuleClasses, ModuleVehicles),
		Highlights: []Module{ModuleResources, ModulePackages, ModuleStaffShares},
	},
}

// All returns every sector in display order.
func All() []Definition {
	out := make([]Definition, len(catalog))
	copy(out, catalog)
	return out
}

// Get returns the definition of a sector.
func Get(code Code) (Definition, bool) {
	for _, d := range catalog {
		if d.Code == code {
			return d, true
		}
	}
	return Definition{}, false
}

// Valid reports whether code names a sector.
func Valid(code string) bool {
	_, ok := Get(Code(code))
	return ok
}

// Of returns the definition of a stored sector code, falling back to the
// default sector for an unknown value.
func Of(code string) Definition {
	if d, ok := Get(Code(code)); ok {
		return d
	}
	d, _ := Get(Default)
	return d
}
