package demo

import (
	"context"
	"encoding/json"
	"fmt"
	mrand "math/rand/v2"
	"strings"
	"time"

	"github.com/google/uuid"

	authmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
	branchmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	cashmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/model"
	classesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/model"
	customersmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	financemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	productsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/model"
	resourcesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/model"
	bundlesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/model"
	servicesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/model"
	packagesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/model"
	staffmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/model"
	sharesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/slug"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Demo account roles (local part of the address after the business key).
const (
	roleOwner     = "sahip"
	roleManager   = "yonetici"
	roleReception = "resepsiyon"
	roleStaff     = "calisan"
	roleShared    = "coklu"
)

// accountRoles maps a demo role to its template role key and label.
var accountRoles = map[string]struct{ Key, Label string }{
	roleOwner:     {orgctx.RoleOwner, "İşletme sahibi"},
	roleManager:   {"sube_yoneticisi", "Şube yöneticisi"},
	roleReception: {"resepsiyon", "Resepsiyon"},
	roleStaff:     {orgctx.RoleMember, "Çalışan"},
}

type customerPackage struct {
	UUID    uuid.UUID
	Service string
}

// biz is one business being written.
type biz struct {
	spec  businessSpec
	index int
	org   orgmodel.Organization
	ctx   context.Context
	loc   *time.Location
	rng   *mrand.Rand

	owner, reception authmodel.User
	branches         []branchmodel.Branch
	tills            []uuid.UUID
	bank             uuid.UUID
	categories       map[string]uuid.UUID
	staff            map[string]staffmodel.Staff
	staffNames       []string
	staffBranches    map[string][]int
	services         map[string]uuid.UUID
	products         map[string]uuid.UUID
	productPrice     map[string]string
	bundles          map[string]uuid.UUID
	bundleServices   map[string][]string
	packages         map[string]uuid.UUID
	zones            [][]string
	customers        []uuid.UUID
	panel            uuid.UUID
	owned            map[uuid.UUID][]customerPackage
	stats            Business
}

func (s *Seeder) seedBusiness(ctx context.Context, index int, spec businessSpec, users *userCache, start time.Time, opts Options) (Business, []Account, error) {
	b := &biz{
		spec: spec, index: index,
		rng:        mrand.New(mrand.NewPCG(uint64(index+1), 928)),
		categories: map[string]uuid.UUID{}, staff: map[string]staffmodel.Staff{}, staffBranches: map[string][]int{},
		services: map[string]uuid.UUID{}, products: map[string]uuid.UUID{}, productPrice: map[string]string{},
		bundles: map[string]uuid.UUID{}, bundleServices: map[string][]string{}, packages: map[string]uuid.UUID{},
		owned: map[uuid.UUID][]customerPackage{},
	}
	var accounts []Account
	owner, err := users.ensure(ctx, Email(spec.Key+"."+roleOwner), accountRoles[roleOwner].Label, spec.Name)
	if err != nil {
		return b.stats, nil, err
	}
	b.owner = owner
	b.org, err = s.orgs.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{
		Name: spec.Name, Slug: Slug(spec.Key), Sector: string(spec.Sector), OwnerUserID: owner.ID,
		Plan: spec.Plan, Addons: spec.Addons,
	})
	if err != nil {
		return b.stats, nil, fmt.Errorf("organization: %w", err)
	}
	b.stats = Business{Key: spec.Key, Slug: b.org.Slug, Name: b.org.Name, Sector: b.org.Sector}
	b.loc, err = time.LoadLocation(b.org.Timezone)
	if err != nil || b.org.Timezone == "" {
		b.loc = time.UTC
	}
	b.ctx = orgctx.WithScope(ctx, orgusecase.OperatorScope(b.org))

	today := dateOf(start, b.loc)
	historyStart := today.AddDate(0, 0, -opts.HistoryDays)
	// The set-up happens the day before the first visit.
	s.now = historyStart.AddDate(0, 0, -1).Add(9 * time.Hour)

	steps := []struct {
		name string
		fn   func() error
	}{
		{"branches", func() error { return s.seedBranches(b) }},
		{"settings", func() error { return s.seedSettings(b) }},
		{"accounts", func() error {
			var err error
			accounts, err = s.seedMembers(b, users)
			return err
		}},
		{"cash accounts", func() error { return s.seedCash(b, historyStart.AddDate(0, 0, -1)) }},
		{"staff", func() error { return s.seedStaff(b, users) }},
		{"resources", func() error { return s.seedResources(b) }},
		{"services", func() error { return s.seedServices(b) }},
		{"products", func() error { return s.seedProducts(b, historyStart.AddDate(0, 0, -1)) }},
		{"bundles", func() error { return s.seedBundles(b) }},
		{"packages", func() error { return s.seedPackages(b) }},
		{"staff shares", func() error { return s.seedShareRules(b, historyStart.AddDate(0, 0, -1)) }},
		{"classes", func() error { return s.seedClasses(b, historyStart.AddDate(0, 0, -1)) }},
		{"customers", func() error { return s.seedCustomers(b) }},
		{"package sales", func() error { return s.sellPackages(b) }},
		{"past visits", func() error { return s.seedHistory(b, historyStart, today) }},
		{"bookings", func() error { return s.seedUpcoming(b, start, today, opts.FutureDays) }},
		{"finance", func() error { s.now = start; return s.seedFinance(b, historyStart, today) }},
	}
	for _, step := range steps {
		if err := step.fn(); err != nil {
			return b.stats, nil, fmt.Errorf("%s: %w", step.name, err)
		}
	}
	b.stats.Branches = len(b.branches)
	b.stats.Customers = len(b.customers)
	accounts = append([]Account{{Email: owner.Email, Role: accountRoles[roleOwner].Label, Business: spec.Name, Branches: "Tümü"}}, accounts...)
	return b.stats, accounts, nil
}

// --- set-up ------------------------------------------------------------------

func dateOf(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func ptr[T any](v T) *T { return &v }

func num(v string) *json.Number { n := json.Number(v); return &n }

func amount(v string) money.Money { return money.MustParse(v) }

func (s *Seeder) seedBranches(b *biz) error {
	list, _, err := s.branches.List(b.ctx, b.org.ID, branchmodel.ListFilter{Limit: 10})
	if err != nil {
		return err
	}
	if len(list) != 1 {
		return fmt.Errorf("expected the default branch, found %d branches", len(list))
	}
	input := func(sp branchSpec) branchmodel.BranchInput {
		hours := b.spec.Hours
		return branchmodel.BranchInput{
			Code: ptr(sp.Code), Name: ptr(sp.Name), Slug: ptr(slug.FromName(sp.Name)),
			Address: ptr(sp.Address), City: ptr(sp.City), District: ptr(sp.District), Phone: ptr(sp.Phone),
			WorkingHours: &hours, HolidayExceptions: &[]branchmodel.HolidayException{},
			IsActive: ptr(true), ShowOnline: ptr(true),
			Description: ptr(b.spec.Name + " " + sp.Name + " şubesi"),
		}
	}
	first, err := s.branches.Patch(b.ctx, b.org.ID, list[0].UUID, input(b.spec.Branches[0]))
	if err != nil {
		return err
	}
	b.branches = append(b.branches, first)
	for _, sp := range b.spec.Branches[1:] {
		br, err := s.branches.Create(b.ctx, b.org.ID, input(sp))
		if err != nil {
			return err
		}
		b.branches = append(b.branches, br)
	}
	return nil
}

// seedSettings publishes the online booking rules, so the business page
// and the customer panel work for the demo businesses.
func (s *Seeder) seedSettings(b *biz) error {
	set := orgsettingsmodel.Defaults()
	set.Company.LegalName = b.spec.Name + " Ltd. Şti."
	set.Reservation.ApprovalMode = b.spec.Approval
	if b.spec.Approval == orgsettingsmodel.ApprovalStaff {
		set.Reservation.StaffReviewWindowMinutes = ptr(120)
	}
	set.Reservation.MinLeadMinutes = ptr(60)
	set.Reservation.MaxAdvanceDays = ptr(60)
	set.Reservation.CancelCutoffMinutes = ptr(120)
	set.Reservation.ChangeCutoffMinutes = ptr(120)
	set.Policies.Cancellation = "Randevunuzu başlangıçtan 2 saat öncesine kadar ücretsiz iptal edebilirsiniz."
	set.Policies.Change = "Randevu saatini başlangıçtan 2 saat öncesine kadar değiştirebilirsiniz."
	set.Policies.NoShow = "Gelmediğiniz randevular kayıtlarınızda görünür."
	for i := range set.SalesChannels {
		set.SalesChannels[i].IsActive = set.SalesChannels[i].Code == "web"
	}
	_, err := s.settings.Save(b.ctx, b.org.ID, b.owner.ID, orgsettingsmodel.SaveInput{Settings: set})
	return err
}

// seedMembers adds the branch manager, the reception and the staff
// account (first branch) and the shared account.
func (s *Seeder) seedMembers(b *biz, users *userCache) ([]Account, error) {
	var out []Account
	add := func(u authmodel.User, roleKey, label string, branches []int) error {
		scope := orgctx.BranchScopeSelected
		ids := make([]uuid.UUID, 0, len(branches))
		names := make([]string, 0, len(branches))
		for _, i := range branches {
			ids = append(ids, b.branches[i].UUID)
			names = append(names, b.branches[i].Name)
		}
		if _, err := s.orgs.PlatformAddMember(b.ctx, b.org.UUID, orgmodel.PlatformMemberInput{
			UserUUID: u.UUID, RoleKey: roleKey, BranchScope: &scope, BranchUUIDs: &ids,
		}); err != nil {
			return fmt.Errorf("member %s: %w", u.Email, err)
		}
		out = append(out, Account{Email: u.Email, Role: label, Business: b.spec.Name, Branches: strings.Join(names, ", ")})
		return nil
	}
	for _, role := range []string{roleManager, roleReception} {
		r := accountRoles[role]
		u, err := users.ensure(b.ctx, Email(b.spec.Key+"."+role), r.Label, b.spec.Name)
		if err != nil {
			return nil, err
		}
		if role == roleReception {
			b.reception = u
		}
		if err := add(u, r.Key, r.Label, []int{0}); err != nil {
			return nil, err
		}
	}
	for _, st := range b.spec.Staff {
		if st.Account == roleStaff {
			first, last := splitName(st.Name)
			u, err := users.ensure(b.ctx, Email(b.spec.Key+"."+roleStaff), first, last)
			if err != nil {
				return nil, err
			}
			if err := add(u, accountRoles[roleStaff].Key, accountRoles[roleStaff].Label, st.Branches); err != nil {
				return nil, err
			}
		}
	}
	if b.spec.SharedRole != "" {
		u, err := users.ensure(b.ctx, Email(roleShared), "Deniz", "Ortak")
		if err != nil {
			return nil, err
		}
		label := "Resepsiyon (iki işletmede)"
		if b.spec.SharedRole == orgctx.RoleMember {
			label = "Çalışan (iki işletmede)"
		}
		if err := add(u, b.spec.SharedRole, label, b.spec.SharedBranches); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func splitName(full string) (string, string) {
	full = strings.TrimSpace(full)
	if i := strings.LastIndex(full, " "); i > 0 {
		return full[:i], full[i+1:]
	}
	return full, "-"
}

// seedCash opens a till per branch and a business-wide bank account (card
// payments and rent go there).
func (s *Seeder) seedCash(b *biz, opening time.Time) error {
	date := opening.Format(time.DateOnly)
	for _, br := range b.branches {
		acc, err := s.cash.CreateAccount(b.ctx, b.org.ID, &b.owner.ID, cashmodel.AccountInput{
			BranchUUID: ptr(br.UUID), Kind: ptr(cashmodel.KindCash), Name: ptr(br.Name + " kasası"),
			OpeningBalance: ptr(amount("2000")), OpeningDate: ptr(date),
		})
		if err != nil {
			return err
		}
		b.tills = append(b.tills, acc.UUID)
	}
	acc, err := s.cash.CreateAccount(b.ctx, b.org.ID, &b.owner.ID, cashmodel.AccountInput{
		Kind: ptr(cashmodel.KindBank), Name: ptr("İşletme banka hesabı"), BankName: ptr("Demo Bank"),
		OpeningBalance: ptr(amount("250000")), OpeningDate: ptr(date),
	})
	if err != nil {
		return err
	}
	b.bank = acc.UUID
	return nil
}

// shifts turns the branch hours into a staff member's week, with a short
// break on long days. A person of several branches works the days in turn
// (pos of n): shifts of one person never overlap across branches.
func shifts(h branchmodel.WeeklyHours, pos, n int) staffmodel.WeeklyShifts {
	k := -1
	day := func(d branchmodel.DayHours) []staffmodel.Shift {
		k++
		if d.Closed || k%n != pos {
			return []staffmodel.Shift{}
		}
		sh := staffmodel.Shift{Start: d.Open, End: d.Close, Breaks: []staffmodel.Break{}}
		open, _ := time.Parse("15:04", d.Open)
		end, _ := time.Parse("15:04", d.Close)
		if end.Sub(open) >= 9*time.Hour && d.Open < "13:00" && d.Close > "14:00" {
			sh.Breaks = []staffmodel.Break{{Start: "13:00", End: "13:30"}}
		}
		return []staffmodel.Shift{sh}
	}
	return staffmodel.WeeklyShifts{
		Mon: day(h.Mon), Tue: day(h.Tue), Wed: day(h.Wed), Thu: day(h.Thu), Fri: day(h.Fri), Sat: day(h.Sat), Sun: day(h.Sun),
	}
}

func (s *Seeder) seedStaff(b *biz, users *userCache) error {
	for _, sp := range b.spec.Staff {
		ids := make([]uuid.UUID, 0, len(sp.Branches))
		for _, i := range sp.Branches {
			ids = append(ids, b.branches[i].UUID)
		}
		in := staffmodel.StaffInput{
			Name: ptr(sp.Name), Title: ptr(sp.Title), Color: ptr(sp.Color),
			ShowOnline: ptr(true), IsActive: ptr(true), BranchUUIDs: &ids,
		}
		switch sp.Account {
		case roleStaff:
			u := users.byEmail[Email(b.spec.Key+"."+roleStaff)]
			in.AccountUUID = staffmodel.OptionalUUID{Set: true, Value: &u.UUID}
		case roleShared:
			u := users.byEmail[Email(roleShared)]
			in.AccountUUID = staffmodel.OptionalUUID{Set: true, Value: &u.UUID}
		}
		st, err := s.staff.Create(b.ctx, b.org.ID, in)
		if err != nil {
			return fmt.Errorf("%s: %w", sp.Name, err)
		}
		for pos, id := range ids {
			if _, err := s.staff.SetShifts(b.ctx, b.org.ID, st.UUID, id, shifts(b.spec.Hours, pos, len(ids))); err != nil {
				return fmt.Errorf("%s shifts: %w", sp.Name, err)
			}
		}
		b.staff[sp.Name] = st
		b.staffNames = append(b.staffNames, sp.Name)
		b.staffBranches[sp.Name] = sp.Branches
	}
	return nil
}

func (s *Seeder) seedResources(b *biz) error {
	for _, r := range b.spec.Resources {
		if _, err := s.resources.Create(b.ctx, b.org.ID, resourcesmodel.Input{
			BranchUUID: ptr(b.branches[r.Branch].UUID), Kind: ptr(r.Kind), Name: ptr(r.Name), Zone: ptr(r.Zone),
			MinCapacity: ptr(r.Min), Capacity: ptr(r.Cap), Combinable: ptr(r.Combinable),
			ShowOnline: ptr(true), IsActive: ptr(true),
		}); err != nil {
			return fmt.Errorf("%s: %w", r.Name, err)
		}
	}
	// The zones of each branch (restaurant: the area asked for).
	b.zones = make([][]string, len(b.branches))
	for i, br := range b.branches {
		list, _, err := s.resources.List(b.ctx, b.org.ID, resourcesmodel.ListFilter{Branches: []uuid.UUID{br.UUID}, Limit: 200})
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, r := range list {
			if r.Zone != "" && !seen[r.Zone] {
				seen[r.Zone] = true
				b.zones[i] = append(b.zones[i], r.Zone)
			}
		}
	}
	return nil
}

func (s *Seeder) resource(b *biz, branch int, name string) (uuid.UUID, error) {
	list, _, err := s.resources.List(b.ctx, b.org.ID, resourcesmodel.ListFilter{Branches: []uuid.UUID{b.branches[branch].UUID}, Q: name, Limit: 50})
	if err != nil {
		return uuid.Nil, err
	}
	for _, r := range list {
		if r.Name == name {
			return r.UUID, nil
		}
	}
	return uuid.Nil, fmt.Errorf("resource %q not found in %s", name, b.branches[branch].Name)
}

func (s *Seeder) allBranchUUIDs(b *biz) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(b.branches))
	for _, br := range b.branches {
		out = append(out, br.UUID)
	}
	return out
}

func (s *Seeder) seedServices(b *biz) error {
	for i, sp := range b.spec.Services {
		if sp.Category != "" {
			if _, ok := b.categories[sp.Category]; !ok {
				c, err := s.services.CreateCategory(b.ctx, b.org.ID, servicesmodel.CategoryInput{Name: ptr(sp.Category), SortOrder: ptr(int32(len(b.categories)))})
				if err != nil {
					return err
				}
				b.categories[sp.Category] = c.UUID
			}
		}
		in := servicesmodel.Input{
			Name: ptr(sp.Name), DurationMinutes: ptr(sp.Minutes), Price: num(sp.Price),
			PaymentMode: ptr("on_site"), StaffMode: ptr(sp.StaffMode), ResourceKinds: &sp.Kinds,
			ShowOnline: ptr(true), IsActive: ptr(true), SortOrder: ptr(int32(i)),
		}
		if sp.Category != "" {
			id := b.categories[sp.Category]
			in.CategoryUUID = servicesmodel.OptionalUUID{Set: true, Value: &id}
		}
		if sp.Group > 0 {
			in.GroupCapacity = servicesmodel.OptionalInt{Set: true, Value: ptr(sp.Group)}
		}
		var id uuid.UUID
		if sp.Template {
			list, _, err := s.services.List(b.ctx, b.org.ID, servicesmodel.ListFilter{Q: sp.Name, Limit: 20})
			if err != nil {
				return err
			}
			for _, sv := range list {
				if sv.Name == sp.Name {
					id = sv.UUID
				}
			}
			if id == uuid.Nil {
				return fmt.Errorf("template service %q not found", sp.Name)
			}
			if _, err := s.services.Patch(b.ctx, b.org.ID, id, in); err != nil {
				return fmt.Errorf("%s: %w", sp.Name, err)
			}
			offers := make([]servicesmodel.OfferInput, 0, len(b.branches))
			for _, br := range b.branches {
				offers = append(offers, servicesmodel.OfferInput{BranchUUID: br.UUID, IsActive: ptr(true)})
			}
			if _, _, err := s.services.SetOffers(b.ctx, b.org.ID, id, offers); err != nil {
				return fmt.Errorf("%s offers: %w", sp.Name, err)
			}
		} else {
			in.BranchUUIDs = s.allBranchUUIDs(b)
			sv, err := s.services.Create(b.ctx, b.org.ID, in)
			if err != nil {
				return fmt.Errorf("%s: %w", sp.Name, err)
			}
			id = sv.UUID
		}
		b.services[sp.Name] = id
		if sp.StaffMode == "none" {
			continue
		}
		names := sp.Staff
		if names == nil {
			names = b.staffNames
		}
		links := make([]servicesmodel.StaffInput, 0, len(names))
		for _, n := range names {
			st, ok := b.staff[n]
			if !ok {
				return fmt.Errorf("%s: staff %q not found", sp.Name, n)
			}
			links = append(links, servicesmodel.StaffInput{StaffUUID: st.UUID})
		}
		if _, _, err := s.services.SetStaff(b.ctx, b.org.ID, id, links); err != nil {
			return fmt.Errorf("%s staff: %w", sp.Name, err)
		}
	}
	return nil
}

// seedProducts adds the products and their opening stock in every branch.
func (s *Seeder) seedProducts(b *biz, at time.Time) error {
	received := at.Add(10 * time.Hour)
	for _, sp := range b.spec.Products {
		p, err := s.products.Create(b.ctx, b.org.ID, productsmodel.Input{
			Name: ptr(sp.Name), SKU: ptr(sp.SKU), Unit: ptr(sp.Unit), Price: num(sp.Price), Cost: num(sp.Cost),
			LowStockThreshold: ptr(int32(5)), ShowOnline: ptr(true), IsActive: ptr(true), BranchUUIDs: s.allBranchUUIDs(b),
		})
		if err != nil {
			return fmt.Errorf("%s: %w", sp.Name, err)
		}
		b.products[sp.Name] = p.UUID
		b.productPrice[sp.Name] = sp.Price
		for _, br := range b.branches {
			if _, err := s.products.RecordMovement(b.ctx, b.org.ID, b.owner.ID, productsmodel.MovementInput{
				Kind: productsmodel.KindReceipt, BranchUUID: br.UUID, ProductUUID: p.UUID,
				Quantity: ptr(sp.Stock), UnitCost: num(sp.Cost), Note: ptr("Açılış stoğu"), OccurredAt: &received,
			}); err != nil {
				return fmt.Errorf("%s stock: %w", sp.Name, err)
			}
		}
	}
	return nil
}

func (s *Seeder) seedBundles(b *biz) error {
	for _, sp := range b.spec.Bundles {
		ids := make([]uuid.UUID, 0, len(sp.Services))
		for _, n := range sp.Services {
			ids = append(ids, b.services[n])
		}
		bd, err := s.bundles.Create(b.ctx, b.org.ID, bundlesmodel.Input{
			Name: ptr(sp.Name), Price: num(sp.Price), ShowOnline: ptr(true), IsActive: ptr(true),
			ServiceUUIDs: ids, BranchUUIDs: s.allBranchUUIDs(b),
		})
		if err != nil {
			return fmt.Errorf("%s: %w", sp.Name, err)
		}
		b.bundles[sp.Name] = bd.UUID
		b.bundleServices[sp.Name] = sp.Services
	}
	return nil
}

func (s *Seeder) seedPackages(b *biz) error {
	for _, sp := range b.spec.Packages {
		p, err := s.packages.CreatePackage(b.ctx, b.org.ID, &b.owner.ID, packagesmodel.PackageInput{
			Name: sp.Name, ServiceUUID: b.services[sp.Service], TotalSessions: sp.Sessions,
			Price: amount(sp.Price), ValidityDays: ptr(sp.ValidityDays), AllBranches: true, IsActive: ptr(true),
		})
		if err != nil {
			return fmt.Errorf("%s: %w", sp.Name, err)
		}
		b.packages[sp.Name] = p.UUID
	}
	return nil
}

// seedShareRules gives every staff member the default service and product
// rates of the business.
func (s *Seeder) seedShareRules(b *biz, from time.Time) error {
	validFrom := from.Format(time.DateOnly)
	for _, n := range b.staffNames {
		st := b.staff[n]
		for _, r := range []struct {
			item    string
			percent int64
		}{{sharesmodel.ItemService, b.spec.ServiceShare}, {sharesmodel.ItemProduct, b.spec.ProductShare}} {
			if r.percent <= 0 {
				continue
			}
			if _, err := s.shares.CreateRule(b.ctx, b.org.ID, &b.owner.ID, sharesmodel.RuleInput{
				StaffUUID: ptr(st.UUID), ItemType: ptr(r.item), Basis: ptr(sharesmodel.BasisPercent),
				Rate: ptr(money.Percent(r.percent)), ValidFrom: ptr(validFrom),
			}); err != nil {
				return fmt.Errorf("%s: %w", n, err)
			}
		}
	}
	return nil
}

func (s *Seeder) seedClasses(b *biz, from time.Time) error {
	for _, c := range b.spec.Classes {
		res, err := s.resource(b, c.Branch, c.Resource)
		if err != nil {
			return err
		}
		st, ok := b.staff[c.Staff]
		if !ok {
			return fmt.Errorf("trainer %q not found", c.Staff)
		}
		for _, wd := range c.Weekdays {
			if _, err := s.classes.Create(b.ctx, b.org.ID, &b.owner.ID, classesmodel.ScheduleInput{
				BranchUUID: b.branches[c.Branch].UUID, ServiceUUID: b.services[c.Service], Weekday: wd, StartTime: c.Start,
				StaffUUID: ptr(st.UUID), ResourceUUID: ptr(res), ValidFrom: from.Format(time.DateOnly),
			}); err != nil {
				return fmt.Errorf("%s %s %s: %w", c.Service, wd, c.Start, err)
			}
		}
	}
	return nil
}

// customerPhone is a mobile number of the demo range (0555 1BB NNNN).
func customerPhone(business, n int) string {
	return fmt.Sprintf("0555 1%02d %04d", business, n)
}

func (s *Seeder) seedCustomers(b *biz) error {
	create := func(first, last, phone string, tags []string) (uuid.UUID, error) {
		c, err := s.customers.Create(b.ctx, b.org.ID, customersmodel.Input{
			FirstName: ptr(first), LastName: ptr(last),
			Phone: customersmodel.OptionalString{Set: true, Value: ptr(phone)},
			Tags:  &tags, SMSConsent: ptr(false), EmailConsent: ptr(false),
		}, "panel")
		if err != nil {
			return uuid.Nil, fmt.Errorf("%s %s: %w", first, last, err)
		}
		return c.UUID, nil
	}
	if b.spec.PanelCustomer {
		id, err := create(panelCustomer.First, panelCustomer.Last, PanelPhone, []string{"panel-demo"})
		if err != nil {
			return err
		}
		b.panel = id
		b.customers = append(b.customers, id)
	}
	for i := range customersPerBusiness {
		first := firstNames[(i+b.index*3)%len(firstNames)]
		last := lastNames[(i*5+b.index)%len(lastNames)]
		tags := []string{}
		if i%5 == 0 {
			tags = append(tags, "sadık müşteri")
		}
		id, err := create(first, last, customerPhone(b.index, i+1), tags)
		if err != nil {
			return err
		}
		b.customers = append(b.customers, id)
	}
	return nil
}

// sellPackages sells each session package to two customers on the day
// before the first visit (paid by card into the bank account).
func (s *Seeder) sellPackages(b *biz) error {
	for _, sp := range b.spec.Packages {
		for i, c := range b.customers[len(b.customers)-2:] {
			cp, _, err := s.packages.Sell(b.ctx, b.org.ID, &b.owner.ID, packagesmodel.SellInput{
				PackageUUID: b.packages[sp.Name], CustomerUUID: c, BranchUUID: b.branches[0].UUID,
				Method: "card", CashAccountUUID: ptr(b.bank),
				IdempotencyKey: fmt.Sprintf("demo-%s-%s-%d", b.spec.Key, slug.FromName(sp.Name), i),
			})
			if err != nil {
				return fmt.Errorf("%s: %w", sp.Name, err)
			}
			b.owned[c] = append(b.owned[c], customerPackage{UUID: cp.UUID, Service: sp.Service})
		}
	}
	return nil
}

// seedFinance writes the business's income and expense records (rent,
// bills, supplies) inside the history window.
func (s *Seeder) seedFinance(b *biz, historyStart, today time.Time) error {
	cats, _, err := s.finance.ListCategories(b.ctx, b.org.ID, financemodel.CategoryFilter{Limit: 200})
	if err != nil {
		return err
	}
	byCode := map[string]uuid.UUID{}
	for _, c := range cats {
		if c.Code != nil {
			byCode[*c.Code] = c.UUID
		}
	}
	for i, f := range b.spec.Finance {
		cat, ok := byCode[f.Code]
		if !ok {
			return fmt.Errorf("finance category %q not found", f.Code)
		}
		date := today.AddDate(0, 0, -f.DaysAgo)
		if date.Before(historyStart) {
			date = historyStart
		}
		account, branch := b.tills[0], ptr(b.branches[0].UUID)
		if f.Bank {
			account, branch = b.bank, nil
		}
		if _, err := s.finance.CreateEntry(b.ctx, b.org.ID, &b.owner.ID, financemodel.EntryInput{
			Kind: f.Kind, CategoryUUID: cat, CashAccountUUID: account, BranchUUID: branch,
			Amount: amount(f.Amount), EntryDate: ptr(date.Format(time.DateOnly)), Description: f.Description,
			IdempotencyKey: fmt.Sprintf("demo-%s-finance-%d", b.spec.Key, i),
		}); err != nil {
			return fmt.Errorf("%s: %w", f.Description, err)
		}
	}
	return nil
}
