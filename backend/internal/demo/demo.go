// Package demo writes the demo businesses of the six sectors and their
// ready-to-use accounts (TEC-928). Every record goes through the module use
// cases (organizations, branches, staff, services, reservations, payments,
// ...), never through raw SQL, so the demo data follows the same rules as
// data entered in the panel and keeps working as the modules change.
//
// Past visits are written with the use cases' clocks moved to the visit
// day: they are booked, started, completed and paid as on that day, so
// revenue, collections, staff shares and reports have a history.
package demo

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	authmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
	authrepo "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/repository"
	authusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/usecase"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	cashrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	classesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/usecase"
	customersrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	customersusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	financerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/repository"
	financeusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	productsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/repository"
	productsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/usecase"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	resourcesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/repository"
	resourcesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/usecase"
	salesmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales"
	salesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/repository"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	servicebundlesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/usecase"
	servicesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/repository"
	servicesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/usecase"
	sessionpackagesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/repository"
	sessionpackagesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/usecase"
	staffrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/repository"
	staffusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/usecase"
	staffsharesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/repository"
	staffsharesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/password"
)

// ErrExists: demo businesses are already there and Reset was not asked.
var ErrExists = errors.New("demo businesses already exist (run with -reset to write them again)")

// Options steer one run.
type Options struct {
	// Password of every demo account; empty generates one.
	Password string
	// Reset deletes the existing demo businesses first (soft delete, the
	// same as the platform's delete); the demo accounts are kept and get
	// the new password.
	Reset bool
	// HistoryDays of completed visits before today and FutureDays of
	// bookings from today on.
	HistoryDays, FutureDays int
	// Only limits the run to these business keys (tests); empty = all.
	Only []string
	// Log receives progress lines (nil: none).
	Log io.Writer
}

// Defaults of Options.
const (
	DefaultHistoryDays = 14
	DefaultFutureDays  = 10
)

// Account is one ready-to-use sign-in.
type Account struct {
	Email    string
	Role     string
	Business string
	Branches string
}

// Business sums up one demo business.
type Business struct {
	Key, Slug, Name, Sector string
	Branches                int
	Customers               int
	Reservations            int
	Completed               int
	Upcoming                int
}

// Result is what a run wrote.
type Result struct {
	Password          string
	GeneratedPassword bool
	Accounts          []Account
	Businesses        []Business
	PanelPhone        string
}

// Seeder holds the use cases, wired as the API wires them (without
// customer notifications and confirmations: demo customers get no
// messages).
type Seeder struct {
	pool *pgxpool.Pool
	q    *db.Queries
	// now is the shared clock of every use case that has one.
	now time.Time
	log io.Writer

	authRepo     *authrepo.Postgres
	auth         *authusecase.AuthUseCase
	orgs         *orgusecase.Service
	branches     *branchusecase.Service
	staff        *staffusecase.Service
	resources    *resourcesusecase.Service
	services     *servicesusecase.Service
	bundles      *servicebundlesusecase.Service
	products     *productsusecase.Service
	customers    *customersusecase.Service
	cash         *cashusecase.Service
	settings     *orgsettingsusecase.Service
	holds        *availabilityusecase.Holds
	search       *availabilityusecase.Service
	reservations *reservationsusecase.Service
	sales        *salesusecase.Service
	shares       *staffsharesusecase.Service
	packages     *sessionpackagesusecase.Service
	finance      *financeusecase.Service
	classes      *classesusecase.Service
	payments     *paymentsusecase.Service
}

// New wires the use cases on pool.
func New(pool *pgxpool.Pool) *Seeder {
	q := db.New(pool)
	s := &Seeder{pool: pool, q: q, now: time.Now(), log: io.Discard}
	clock := func() time.Time { return s.now }

	s.authRepo = authrepo.NewPostgres(pool, q)
	s.auth = authusecase.New(s.authRepo, nil)
	s.orgs = orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	s.branches = branchusecase.New(branchrepository.NewPostgres(q), nil)
	s.staff = staffusecase.New(staffrepository.NewPostgres(pool, q), nil)
	s.resources = resourcesusecase.New(resourcesrepository.NewPostgres(pool, q), nil)
	s.services = servicesusecase.New(servicesrepository.NewPostgres(pool, q), nil)
	s.bundles = servicebundlesusecase.New(servicesrepository.NewPostgres(pool, q))
	s.products = productsusecase.New(productsrepository.NewPostgres(pool, q), nil)
	s.customers = customersusecase.New(customersrepository.NewPostgres(pool, q), nil)
	s.cash = cashusecase.New(cashrepository.NewPostgres(pool, q), nil)
	s.cash.SetClock(clock)
	s.settings = orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))

	s.holds = availabilityusecase.NewHolds(pool, q, s.settings)
	s.holds.SetClock(clock)
	s.search = availabilityusecase.New(q, s.settings)
	s.search.SetClock(clock)
	s.search.SetBusySource(s.holds.Busy())
	s.reservations = reservationsusecase.New(s.holds, q, s.settings)

	s.sales = salesusecase.New(salesrepository.NewPostgres(pool, q), s.cash)
	s.sales.SetClock(clock)
	s.reservations.SetCompletionHook(s.sales.RecordReservationTx)
	s.products.SetSaleRecorder(salesmodule.ProductSaleRecorder(s.sales))
	s.products.SetSaleReverser(salesmodule.ProductSaleReverser(s.sales))
	s.shares = staffsharesusecase.New(staffsharesrepository.NewPostgres(pool, q), s.cash)
	s.shares.SetClock(clock)
	s.sales.SetLineHook(s.shares.RecordLineTx)
	s.packages = sessionpackagesusecase.New(sessionpackagesrepository.NewPostgres(pool, q), s.sales)
	s.packages.SetClock(clock)
	s.finance = financeusecase.New(financerepository.NewPostgres(pool, q), s.cash, s.sales)
	s.finance.SetClock(clock)
	s.classes = classesusecase.New(q, s.holds)
	s.classes.SetClock(clock)
	// Collections at the business only: no online payment provider.
	s.payments = paymentsusecase.New(q, s.reservations, s.sales, nil, nil, paymentsusecase.Config{}, nil)
	s.payments.SetClock(clock)
	return s
}

func (s *Seeder) logf(format string, args ...any) {
	_, _ = fmt.Fprintf(s.log, format+"\n", args...)
}

// Run writes the demo data.
func (s *Seeder) Run(ctx context.Context, opts Options) (Result, error) {
	if opts.Log != nil {
		s.log = opts.Log
	}
	if opts.HistoryDays <= 0 {
		opts.HistoryDays = DefaultHistoryDays
	}
	if opts.FutureDays <= 0 {
		opts.FutureDays = DefaultFutureDays
	}
	res := Result{Password: opts.Password, PanelPhone: PanelPhone}
	if res.Password == "" {
		p, err := GeneratePassword()
		if err != nil {
			return res, err
		}
		res.Password, res.GeneratedPassword = p, true
	}
	if err := password.ValidatePassword(res.Password); err != nil {
		return res, err
	}
	specs := selected(opts.Only)
	if err := s.clear(ctx, specs, opts.Reset); err != nil {
		return res, err
	}

	start := time.Now()
	s.now = start
	hash, err := password.Hash(res.Password)
	if err != nil {
		return res, err
	}
	platformEmail := Email("platform")
	if _, _, err := s.authRepo.UpsertSuperAdmin(ctx, platformEmail, "Demo", "Platform", hash); err != nil {
		return res, fmt.Errorf("platform account: %w", err)
	}
	res.Accounts = append(res.Accounts, Account{Email: platformEmail, Role: "Takwim yönetimi (super_admin)", Business: "-", Branches: "-"})

	users := &userCache{seeder: s, password: res.Password, byEmail: map[string]authmodel.User{}}
	for i, spec := range specs {
		s.logf("%s (%s) ...", spec.Name, spec.Sector)
		b, accounts, err := s.seedBusiness(ctx, i, spec, users, start, opts)
		if err != nil {
			return res, fmt.Errorf("%s: %w", spec.Key, err)
		}
		res.Businesses = append(res.Businesses, b)
		res.Accounts = append(res.Accounts, accounts...)
		s.logf("  %d branches, %d customers, %d reservations (%d completed, %d upcoming)",
			b.Branches, b.Customers, b.Reservations, b.Completed, b.Upcoming)
	}
	s.now = time.Now()
	return res, nil
}

// clear refuses (or with reset deletes) the demo businesses that exist.
func (s *Seeder) clear(ctx context.Context, specs []businessSpec, reset bool) error {
	for _, spec := range specs {
		org, err := s.q.GetOrganizationBySlug(ctx, Slug(spec.Key))
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if !reset {
			return fmt.Errorf("%w: %s", ErrExists, org.Slug)
		}
		if err := s.orgs.DeleteOrganization(ctx, org.Uuid); err != nil {
			return fmt.Errorf("delete %s: %w", org.Slug, err)
		}
		s.logf("deleted %s", org.Slug)
	}
	return nil
}

func selected(only []string) []businessSpec {
	all := catalog()
	if len(only) == 0 {
		return all
	}
	out := []businessSpec{}
	for _, spec := range all {
		for _, k := range only {
			if spec.Key == k {
				out = append(out, spec)
			}
		}
	}
	return out
}

// Keys lists the business keys of the catalog.
func Keys() []string {
	out := []string{}
	for _, spec := range catalog() {
		out = append(out, spec.Key)
	}
	return out
}

// Slug is the slug of the demo business with key.
func Slug(key string) string { return SlugPrefix + key }

// Email is the address of a demo account (local part after the prefix).
func Email(local string) string { return EmailPrefix + local + "@" + EmailDomain }

// GeneratePassword returns a random password that meets the policy.
func GeneratePassword() (string, error) {
	const alphabet = "abcdefghijkmnopqrstuvwxyz23456789"
	var b strings.Builder
	b.WriteString("Tk")
	for range 12 {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(alphabet[n.Int64()])
	}
	b.WriteString("7")
	return b.String(), nil
}

// userCache creates each demo account once per run (the shared account
// belongs to two businesses).
type userCache struct {
	seeder   *Seeder
	password string
	hash     string
	byEmail  map[string]authmodel.User
}

// ensure returns the account with email, creating it through the platform
// user use case or resetting the password of the existing one.
func (c *userCache) ensure(ctx context.Context, email, name, surname string) (authmodel.User, error) {
	if u, ok := c.byEmail[email]; ok {
		return u, nil
	}
	s := c.seeder
	u, err := s.authRepo.FindUserByEmail(ctx, email)
	switch {
	case err == nil:
		if c.hash == "" {
			if c.hash, err = password.Hash(c.password); err != nil {
				return u, err
			}
		}
		if err := s.authRepo.UpdatePassword(ctx, u.ID, c.hash); err != nil {
			return u, err
		}
	case errors.Is(err, authrepo.ErrNotFound):
		if _, err := s.auth.CreatePlatformUser(ctx, authmodel.CreatePlatformUserInput{
			Email: email, Password: c.password, Name: name, Surname: surname, Status: "active",
		}); err != nil {
			return u, fmt.Errorf("account %s: %w", email, err)
		}
		if u, err = s.authRepo.FindUserByEmail(ctx, email); err != nil {
			return u, err
		}
	default:
		return u, err
	}
	c.byEmail[email] = u
	return u, nil
}
