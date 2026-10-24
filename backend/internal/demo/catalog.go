package demo

import (
	branchmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// The demo catalog: one business per sector, each with two branches. The
// records are ordinary data written through the use cases, so they follow
// the same rules as records entered in the panel.

// SlugPrefix starts the slug of every demo business; EmailPrefix and
// EmailDomain build the demo account addresses (example.com is reserved
// for documentation, RFC 2606).
const (
	SlugPrefix  = "demo-"
	EmailPrefix = "demo."
	EmailDomain = "example.com"
)

// PanelPhone is the customer known to the hairdresser and the clinic: the
// customer panel signs in with it and shows both businesses' visits.
const PanelPhone = "0555 000 00 01"

type branchSpec struct {
	Code, Name, City, District, Address, Phone string
}

type serviceSpec struct {
	Name     string
	Category string
	Minutes  int32
	Price    string
	// StaffMode is required, optional or none.
	StaffMode string
	Kinds     []string
	// Group is the seats of a group class session (0 = individual).
	Group int32
	// Template marks a service the sector template already seeded in the
	// first branch: it is updated and offered in every branch.
	Template bool
	// Staff are the staff names who give the service (nil = every staff
	// member of the business).
	Staff []string
}

type resourceSpec struct {
	Branch     int
	Kind, Name string
	Zone       string
	Min, Cap   int32
	Combinable bool
}

type staffSpec struct {
	Name, Title, Color string
	Branches           []int
	// Account links the staff record to a demo account (role key of
	// accountSpec), "" for none.
	Account string
}

type productSpec struct {
	Name, SKU, Unit string
	Price, Cost     string
	Stock           int32
}

type bundleSpec struct {
	Name, Price string
	Services    []string
}

type packageSpec struct {
	Name, Service string
	Sessions      int
	Price         string
	ValidityDays  int
}

type classSpec struct {
	Service  string
	Branch   int
	Weekdays []string
	Start    string
	Staff    string
	Resource string
}

// bookingSpec is one kind of visit the generator draws from.
type bookingSpec struct {
	Weight   int
	Services []string
	Bundle   string
	// PartyMin..PartyMax and Zones: restaurant tables.
	PartyMin, PartyMax int
	Zones              []string
	Vehicle            bool
	Product            string
}

type financeSpec struct {
	Kind, Code, Description, Amount string
	// DaysAgo dates the record; Bank posts it on the business bank account
	// (otherwise the first branch's till).
	DaysAgo int
	Bank    bool
}

type businessSpec struct {
	Key      string
	Sector   sector.Code
	Name     string
	Plan     string
	Addons   []string
	Approval string
	Hours    branchmodel.WeeklyHours
	Branches []branchSpec
	Services []serviceSpec
	// Resources are added on top of the template's resources.
	Resources []resourceSpec
	Staff     []staffSpec
	Products  []productSpec
	Bundles   []bundleSpec
	Packages  []packageSpec
	Classes   []classSpec
	Bookings  []bookingSpec
	Finance   []financeSpec
	// ServiceShare / ProductShare are the default staff share percents
	// (0 = no rule).
	ServiceShare, ProductShare int64
	// PanelCustomer: the business knows PanelPhone.
	PanelCustomer bool
	// SharedRole and SharedBranches add the shared account (one person in
	// two businesses) with this template role key; "" = not a member.
	SharedRole     string
	SharedBranches []int
	// VisitsPerDay is the number of visits per branch and open day.
	VisitsPerDay int
}

func day(open, close string) branchmodel.DayHours {
	return branchmodel.DayHours{Open: open, Close: close}
}

var closed = branchmodel.DayHours{Closed: true}

func week(weekday, sat, sun branchmodel.DayHours) branchmodel.WeeklyHours {
	return branchmodel.WeeklyHours{Mon: weekday, Tue: weekday, Wed: weekday, Thu: weekday, Fri: weekday, Sat: sat, Sun: sun}
}

var allWeekdays = []string{"mon", "tue", "wed", "thu", "fri"}

func catalog() []businessSpec {
	return []businessSpec{
		{
			Key: "kuafor", Sector: sector.Hairdresser, Name: "Lale Kuaför",
			Plan: string(entitlement.PlanGrowth), Addons: []string{string(entitlement.AddonConfirmation)}, Approval: orgsettingsmodel.ApprovalAuto,
			Hours: week(day("09:00", "20:00"), day("09:00", "20:00"), closed),
			Branches: []branchSpec{
				{Code: "KDK", Name: "Kadıköy", City: "İstanbul", District: "Kadıköy", Address: "Moda Cad. No: 12", Phone: "0216 000 00 11"},
				{Code: "BSK", Name: "Beşiktaş", City: "İstanbul", District: "Beşiktaş", Address: "Çarşı Sok. No: 4", Phone: "0212 000 00 12"},
			},
			Services: []serviceSpec{
				{Name: "Kadın saç kesimi", Category: "Kesim", Minutes: 45, Price: "650", StaffMode: "required", Kinds: []string{"chair"}},
				{Name: "Erkek saç kesimi", Category: "Kesim", Minutes: 30, Price: "400", StaffMode: "required", Kinds: []string{"chair"}},
				{Name: "Fön", Category: "Şekillendirme", Minutes: 30, Price: "300", StaffMode: "required", Kinds: []string{"chair"}},
				{Name: "Saç boyama", Category: "Renk", Minutes: 90, Price: "1500", StaffMode: "required", Kinds: []string{"chair"}, Staff: []string{"Elif Kara", "Zeynep Aksoy"}},
				{Name: "Sakal tıraşı", Category: "Kesim", Minutes: 20, Price: "250", StaffMode: "required", Kinds: []string{"chair"}, Staff: []string{"Mert Şahin", "Can Öztürk"}},
			},
			Resources: []resourceSpec{
				{Branch: 0, Kind: "chair", Name: "Koltuk 1", Min: 1, Cap: 1},
				{Branch: 0, Kind: "chair", Name: "Koltuk 2", Min: 1, Cap: 1},
				{Branch: 0, Kind: "chair", Name: "Koltuk 3", Min: 1, Cap: 1},
				{Branch: 1, Kind: "chair", Name: "Koltuk 1", Min: 1, Cap: 1},
				{Branch: 1, Kind: "chair", Name: "Koltuk 2", Min: 1, Cap: 1},
			},
			Staff: []staffSpec{
				{Name: "Elif Kara", Title: "Kuaför", Color: "#e11d48", Branches: []int{0}, Account: roleStaff},
				{Name: "Mert Şahin", Title: "Berber", Color: "#2563eb", Branches: []int{0}},
				{Name: "Zeynep Aksoy", Title: "Renk uzmanı", Color: "#9333ea", Branches: []int{0, 1}},
				{Name: "Can Öztürk", Title: "Berber", Color: "#059669", Branches: []int{1}},
			},
			Products: []productSpec{
				{Name: "Onarıcı şampuan 250 ml", SKU: "KF-SMP-250", Unit: "bottle", Price: "320", Cost: "180", Stock: 24},
				{Name: "Saç maskesi 200 ml", SKU: "KF-MSK-200", Unit: "tube", Price: "450", Cost: "240", Stock: 16},
				{Name: "Şekillendirici wax", SKU: "KF-WAX-100", Unit: "piece", Price: "280", Cost: "130", Stock: 20},
			},
			Bundles: []bundleSpec{{Name: "Kesim + fön", Price: "850", Services: []string{"Kadın saç kesimi", "Fön"}}},
			Bookings: []bookingSpec{
				{Weight: 4, Services: []string{"Kadın saç kesimi"}},
				{Weight: 4, Services: []string{"Erkek saç kesimi"}},
				{Weight: 2, Services: []string{"Fön"}, Product: "Onarıcı şampuan 250 ml"},
				{Weight: 2, Services: []string{"Saç boyama"}},
				{Weight: 2, Services: []string{"Sakal tıraşı"}},
				{Weight: 2, Bundle: "Kesim + fön"},
			},
			Finance: []financeSpec{
				{Kind: "expense", Code: "rent", Description: "Kadıköy dükkan kirası", Amount: "35000", DaysAgo: 12, Bank: true},
				{Kind: "expense", Code: "utilities", Description: "Elektrik ve su faturası", Amount: "2400", DaysAgo: 8, Bank: true},
				{Kind: "expense", Code: "supplies", Description: "Boya ve oksidan alımı", Amount: "650", DaysAgo: 5},
			},
			ServiceShare: 40, ProductShare: 10, PanelCustomer: true, VisitsPerDay: 3,
			SharedRole: "resepsiyon", SharedBranches: []int{1},
		},
		{
			Key: "restoran", Sector: sector.Restaurant, Name: "Kordon Balık Restoran",
			Plan: string(entitlement.PlanGrowth), Approval: orgsettingsmodel.ApprovalAuto,
			Hours: week(day("12:00", "23:00"), day("12:00", "23:00"), day("12:00", "22:00")),
			Branches: []branchSpec{
				{Code: "ALS", Name: "Alsancak", City: "İzmir", District: "Konak", Address: "Kordon Boyu No: 21", Phone: "0232 000 00 21"},
				{Code: "KSK", Name: "Karşıyaka", City: "İzmir", District: "Karşıyaka", Address: "Sahil Yolu No: 8", Phone: "0232 000 00 22"},
			},
			Services: []serviceSpec{
				{Name: "Masa rezervasyonu", Minutes: 90, Price: "0", StaffMode: "none", Kinds: []string{"table"}, Template: true},
			},
			Resources: []resourceSpec{
				{Branch: 1, Kind: "table", Name: "Masa 1", Zone: "Salon", Min: 1, Cap: 2},
				{Branch: 1, Kind: "table", Name: "Masa 2", Zone: "Salon", Min: 2, Cap: 4, Combinable: true},
				{Branch: 1, Kind: "table", Name: "Masa 3", Zone: "Salon", Min: 2, Cap: 4, Combinable: true},
				{Branch: 1, Kind: "table", Name: "Masa 4", Zone: "Teras", Min: 2, Cap: 6},
				{Branch: 1, Kind: "table", Name: "Masa 5", Zone: "Teras", Min: 4, Cap: 8},
			},
			Staff: []staffSpec{
				{Name: "Burak Yıldız", Title: "Salon şefi", Color: "#0ea5e9", Branches: []int{0}, Account: roleStaff},
				{Name: "Seda Arslan", Title: "Garson", Color: "#f97316", Branches: []int{1}},
			},
			Bookings: []bookingSpec{
				{Weight: 3, Services: []string{"Masa rezervasyonu"}, PartyMin: 2, PartyMax: 4},
				{Weight: 1, Services: []string{"Masa rezervasyonu"}, PartyMin: 1, PartyMax: 2},
				{Weight: 1, Services: []string{"Masa rezervasyonu"}, PartyMin: 4, PartyMax: 6},
			},
			Finance: []financeSpec{
				{Kind: "expense", Code: "rent", Description: "Alsancak kira", Amount: "60000", DaysAgo: 12, Bank: true},
				{Kind: "expense", Code: "supplies", Description: "Balık ve sebze alımı", Amount: "1800", DaysAgo: 3},
				{Kind: "income", Code: "other_income", Description: "Özel gün organizasyonu kaporası", Amount: "5000", DaysAgo: 6, Bank: true},
			},
			VisitsPerDay: 4,
		},
		{
			Key: "spor", Sector: sector.Gym, Name: "Form Spor Salonu",
			Plan: string(entitlement.PlanGrowth), Approval: orgsettingsmodel.ApprovalAuto,
			Hours: week(day("07:00", "22:00"), day("09:00", "18:00"), day("09:00", "18:00")),
			Branches: []branchSpec{
				{Code: "CNK", Name: "Çankaya", City: "Ankara", District: "Çankaya", Address: "Tunalı Hilmi Cad. No: 40", Phone: "0312 000 00 31"},
				{Code: "BTK", Name: "Bilkent", City: "Ankara", District: "Çankaya", Address: "Bilkent Plaza No: 3", Phone: "0312 000 00 32"},
			},
			Services: []serviceSpec{
				{Name: "Grup dersi", Category: "Grup dersleri", Minutes: 60, Price: "250", StaffMode: "optional", Kinds: []string{"area"}, Group: 12, Template: true},
				{Name: "Spinning", Category: "Grup dersleri", Minutes: 45, Price: "200", StaffMode: "optional", Kinds: []string{"area"}, Group: 10},
				{Name: "Kişisel antrenman", Category: "Bireysel", Minutes: 60, Price: "900", StaffMode: "required", Kinds: []string{}},
			},
			Resources: []resourceSpec{
				{Branch: 0, Kind: "area", Name: "Spinning salonu", Min: 1, Cap: 10},
				{Branch: 1, Kind: "area", Name: "Stüdyo", Min: 1, Cap: 12},
			},
			Staff: []staffSpec{
				{Name: "Emre Koç", Title: "Eğitmen", Color: "#16a34a", Branches: []int{0}, Account: roleStaff},
				{Name: "Gizem Polat", Title: "Pilates eğitmeni", Color: "#db2777", Branches: []int{0}},
				{Name: "Onur Demirtaş", Title: "Eğitmen", Color: "#ca8a04", Branches: []int{1}},
			},
			Products: []productSpec{
				{Name: "Protein bar", SKU: "FS-BAR-01", Unit: "piece", Price: "90", Cost: "45", Stock: 60},
				{Name: "Su 500 ml", SKU: "FS-SU-500", Unit: "bottle", Price: "20", Cost: "6", Stock: 120},
			},
			Packages: []packageSpec{{Name: "10 grup dersi", Service: "Grup dersi", Sessions: 10, Price: "2000", ValidityDays: 90}},
			Classes: []classSpec{
				{Service: "Grup dersi", Branch: 0, Weekdays: allWeekdays, Start: "18:30", Staff: "Gizem Polat", Resource: "Stüdyo"},
				{Service: "Grup dersi", Branch: 0, Weekdays: []string{"sat"}, Start: "10:00", Staff: "Emre Koç", Resource: "Stüdyo"},
				{Service: "Spinning", Branch: 0, Weekdays: []string{"mon", "wed", "fri"}, Start: "07:30", Staff: "Emre Koç", Resource: "Spinning salonu"},
				{Service: "Grup dersi", Branch: 1, Weekdays: []string{"tue", "thu"}, Start: "19:00", Staff: "Onur Demirtaş", Resource: "Stüdyo"},
			},
			Bookings: []bookingSpec{
				{Weight: 4, Services: []string{"Grup dersi"}},
				{Weight: 2, Services: []string{"Spinning"}},
				{Weight: 2, Services: []string{"Kişisel antrenman"}},
			},
			Finance: []financeSpec{
				{Kind: "expense", Code: "rent", Description: "Çankaya salon kirası", Amount: "55000", DaysAgo: 12, Bank: true},
				{Kind: "expense", Code: "maintenance", Description: "Koşu bandı bakımı", Amount: "3200", DaysAgo: 7, Bank: true},
			},
			ServiceShare: 30, ProductShare: 0, VisitsPerDay: 4,
		},
		{
			Key: "otoyikama", Sector: sector.CarWash, Name: "Parlak Oto Yıkama",
			Plan: string(entitlement.PlanGrowth), Approval: orgsettingsmodel.ApprovalAuto,
			Hours: week(day("08:00", "19:00"), day("08:00", "19:00"), day("10:00", "17:00")),
			Branches: []branchSpec{
				{Code: "NLF", Name: "Nilüfer", City: "Bursa", District: "Nilüfer", Address: "FSM Bulvarı No: 101", Phone: "0224 000 00 41"},
				{Code: "OSM", Name: "Osmangazi", City: "Bursa", District: "Osmangazi", Address: "Ankara Yolu No: 15", Phone: "0224 000 00 42"},
			},
			Services: []serviceSpec{
				{Name: "Dış yıkama", Category: "Yıkama", Minutes: 30, Price: "350", StaffMode: "optional", Kinds: []string{"bay"}},
				{Name: "İç dış yıkama", Category: "Yıkama", Minutes: 60, Price: "600", StaffMode: "optional", Kinds: []string{"bay"}},
				{Name: "Detaylı temizlik", Category: "Bakım", Minutes: 120, Price: "1800", StaffMode: "required", Kinds: []string{"bay"}},
			},
			Resources: []resourceSpec{
				{Branch: 1, Kind: "bay", Name: "Yıkama alanı 1", Min: 1, Cap: 1},
				{Branch: 1, Kind: "bay", Name: "Yıkama alanı 2", Min: 1, Cap: 1},
			},
			Staff: []staffSpec{
				{Name: "Hakan Çelik", Title: "Yıkama ustası", Color: "#0284c7", Branches: []int{0}, Account: roleStaff},
				{Name: "Serkan Aydın", Title: "Detay uzmanı", Color: "#7c3aed", Branches: []int{0, 1}},
				{Name: "Murat Kılıç", Title: "Yıkama ustası", Color: "#ea580c", Branches: []int{1}},
			},
			Products: []productSpec{
				{Name: "Oto kokusu", SKU: "PO-KOK-01", Unit: "piece", Price: "120", Cost: "50", Stock: 40},
				{Name: "Cam suyu 4 L", SKU: "PO-CAM-4L", Unit: "bottle", Price: "150", Cost: "70", Stock: 30},
			},
			Bookings: []bookingSpec{
				{Weight: 4, Services: []string{"Dış yıkama"}, Vehicle: true},
				{Weight: 3, Services: []string{"İç dış yıkama"}, Vehicle: true, Product: "Oto kokusu"},
				{Weight: 1, Services: []string{"Detaylı temizlik"}, Vehicle: true},
			},
			Finance: []financeSpec{
				{Kind: "expense", Code: "rent", Description: "Nilüfer alan kirası", Amount: "30000", DaysAgo: 12, Bank: true},
				{Kind: "expense", Code: "supplies", Description: "Köpük ve cila alımı", Amount: "900", DaysAgo: 4},
			},
			ServiceShare: 25, ProductShare: 5, VisitsPerDay: 4,
		},
		{
			Key: "klinik", Sector: sector.Clinic, Name: "Şifa Fizik Tedavi Kliniği",
			Plan: string(entitlement.PlanGrowth), Addons: []string{string(entitlement.AddonConfirmation)}, Approval: orgsettingsmodel.ApprovalStaff,
			Hours: week(day("09:00", "18:00"), day("09:00", "13:00"), closed),
			Branches: []branchSpec{
				{Code: "ATS", Name: "Ataşehir", City: "İstanbul", District: "Ataşehir", Address: "Barbaros Mah. No: 7", Phone: "0216 000 00 51"},
				{Code: "SSL", Name: "Şişli", City: "İstanbul", District: "Şişli", Address: "Halaskargazi Cad. No: 90", Phone: "0212 000 00 52"},
			},
			Services: []serviceSpec{
				{Name: "İlk değerlendirme", Category: "Muayene", Minutes: 45, Price: "1500", StaffMode: "required", Kinds: []string{"room"}},
				{Name: "Kontrol", Category: "Muayene", Minutes: 20, Price: "750", StaffMode: "required", Kinds: []string{"room"}},
				{Name: "Fizik tedavi seansı", Category: "Tedavi", Minutes: 50, Price: "1100", StaffMode: "required", Kinds: []string{"room"}},
			},
			Resources: []resourceSpec{
				{Branch: 1, Kind: "room", Name: "Tedavi odası 1", Min: 1, Cap: 1},
				{Branch: 1, Kind: "room", Name: "Tedavi odası 2", Min: 1, Cap: 1},
			},
			Staff: []staffSpec{
				{Name: "Dr. Selin Yurt", Title: "Fizik tedavi uzmanı", Color: "#0d9488", Branches: []int{0}},
				{Name: "Fzt. Okan Eren", Title: "Fizyoterapist", Color: "#4f46e5", Branches: []int{0}, Account: roleStaff},
				{Name: "Fzt. Derya Uçar", Title: "Fizyoterapist", Color: "#be123c", Branches: []int{1}},
			},
			Packages: []packageSpec{{Name: "10 seans fizik tedavi", Service: "Fizik tedavi seansı", Sessions: 10, Price: "9500", ValidityDays: 120}},
			Bookings: []bookingSpec{
				{Weight: 2, Services: []string{"İlk değerlendirme"}},
				{Weight: 2, Services: []string{"Kontrol"}},
				{Weight: 4, Services: []string{"Fizik tedavi seansı"}},
			},
			Finance: []financeSpec{
				{Kind: "expense", Code: "rent", Description: "Ataşehir klinik kirası", Amount: "48000", DaysAgo: 12, Bank: true},
				{Kind: "expense", Code: "supplies", Description: "Sarf malzeme", Amount: "1200", DaysAgo: 6, Bank: true},
			},
			ServiceShare: 35, PanelCustomer: true, VisitsPerDay: 3,
		},
		{
			Key: "guzellik", Sector: sector.BeautySalon, Name: "Nar Güzellik Salonu",
			Plan: string(entitlement.PlanGrowth), Approval: orgsettingsmodel.ApprovalAuto,
			Hours: week(day("10:00", "20:00"), day("10:00", "20:00"), closed),
			Branches: []branchSpec{
				{Code: "NSN", Name: "Nişantaşı", City: "İstanbul", District: "Şişli", Address: "Teşvikiye Cad. No: 33", Phone: "0212 000 00 61"},
				{Code: "BGD", Name: "Bağdat Caddesi", City: "İstanbul", District: "Kadıköy", Address: "Bağdat Cad. No: 250", Phone: "0216 000 00 62"},
			},
			Services: []serviceSpec{
				{Name: "Cilt bakımı", Category: "Cilt", Minutes: 60, Price: "1200", StaffMode: "required", Kinds: []string{"room"}},
				{Name: "Lazer epilasyon", Category: "Epilasyon", Minutes: 30, Price: "900", StaffMode: "required", Kinds: []string{"device"}},
				{Name: "Manikür", Category: "El ve ayak", Minutes: 40, Price: "450", StaffMode: "required", Kinds: []string{}},
				{Name: "Kaş tasarımı", Category: "Yüz", Minutes: 30, Price: "350", StaffMode: "required", Kinds: []string{}},
			},
			Resources: []resourceSpec{
				{Branch: 0, Kind: "room", Name: "Bakım odası 1", Min: 1, Cap: 1},
				{Branch: 0, Kind: "room", Name: "Bakım odası 2", Min: 1, Cap: 1},
				{Branch: 0, Kind: "device", Name: "Lazer cihazı", Min: 1, Cap: 1},
				{Branch: 1, Kind: "room", Name: "Bakım odası", Min: 1, Cap: 1},
				{Branch: 1, Kind: "device", Name: "Lazer cihazı", Min: 1, Cap: 1},
			},
			Staff: []staffSpec{
				{Name: "Buse Tan", Title: "Estetisyen", Color: "#c026d3", Branches: []int{0}, Account: roleStaff},
				{Name: "Ece Güler", Title: "Manikür uzmanı", Color: "#f43f5e", Branches: []int{0, 1}, Account: roleShared},
				{Name: "Pınar Doğan", Title: "Estetisyen", Color: "#14b8a6", Branches: []int{1}},
			},
			Products: []productSpec{
				{Name: "Nemlendirici krem 50 ml", SKU: "NG-KRM-50", Unit: "tube", Price: "650", Cost: "300", Stock: 18},
				{Name: "Güneş kremi SPF50", SKU: "NG-GNS-50", Unit: "tube", Price: "520", Cost: "250", Stock: 18},
			},
			Bundles:  []bundleSpec{{Name: "Bakım günü", Price: "1450", Services: []string{"Cilt bakımı", "Manikür"}}},
			Packages: []packageSpec{{Name: "6 seans lazer epilasyon", Service: "Lazer epilasyon", Sessions: 6, Price: "4500", ValidityDays: 180}},
			Bookings: []bookingSpec{
				{Weight: 3, Services: []string{"Cilt bakımı"}, Product: "Nemlendirici krem 50 ml"},
				{Weight: 3, Services: []string{"Lazer epilasyon"}},
				{Weight: 3, Services: []string{"Manikür"}},
				{Weight: 2, Services: []string{"Kaş tasarımı"}},
				{Weight: 1, Bundle: "Bakım günü"},
			},
			Finance: []financeSpec{
				{Kind: "expense", Code: "rent", Description: "Nişantaşı salon kirası", Amount: "52000", DaysAgo: 12, Bank: true},
				{Kind: "expense", Code: "marketing", Description: "Sosyal medya reklamı", Amount: "4000", DaysAgo: 9, Bank: true},
			},
			ServiceShare: 35, ProductShare: 10, VisitsPerDay: 3,
			SharedRole: "member", SharedBranches: []int{0, 1},
		},
	}
}

// Customer names; each business gets its own people (the panel customer
// is added to the businesses that know her).
var (
	firstNames = []string{"Ahmet", "Ayşe", "Mehmet", "Fatma", "Mustafa", "Emine", "Ali", "Hatice", "Hüseyin", "Zehra", "İbrahim", "Elif", "Hasan", "Merve", "Yusuf", "Esra"}
	lastNames  = []string{"Yılmaz", "Kaya", "Demir", "Şahin", "Çelik", "Yıldız", "Aydın", "Öztürk", "Arslan", "Doğan", "Kılıç", "Aslan", "Çetin", "Kurt", "Koç", "Özdemir"}
	plates     = []string{"16 ABC 101", "16 DF 2024", "34 KLM 77", "16 PR 450", "06 TT 318", "16 YZ 909"}
	vehicles   = []string{"car", "car", "suv", "car", "van"}
)

// panelCustomer is the person behind PanelPhone.
var panelCustomer = struct{ First, Last string }{"Ayşe", "Demir"}

// customersPerBusiness is the size of each business's customer list.
const customersPerBusiness = 14
