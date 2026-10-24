package staffshares_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	cashrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	salesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	salesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/repository"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

var ist = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

func on(day, hour int) time.Time {
	return time.Date(2030, 1, day, hour, 0, 0, 0, ist)
}

func date(s string) *time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return &t
}

func ptr[T any](v T) *T { return &v }

func m(s string) money.Money { return money.MustParse(s) }

type mapLoader map[uuid.UUID]int64

func (l mapLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: l[id]}, nil
}

func TestAllocateAndShareOf(t *testing.T) {
	// A16: 1.000,00 over list prices 800 + 400.
	parts := usecase.AllocateBase(m("1000"), []money.Money{m("800"), m("400")})
	if parts[0].String() != "666.67" || parts[1].String() != "333.33" {
		t.Fatalf("parts = %v", parts)
	}
	a := usecase.ShareOf(usecase.Applied{Basis: model.BasisPercent, Rate: money.Percent(40)}, parts[0], 1)
	b := usecase.ShareOf(usecase.Applied{Basis: model.BasisPercent, Rate: money.Percent(30)}, parts[1], 1)
	if a.String() != "266.67" || b.String() != "100.00" || a.Add(b).String() != "366.67" {
		t.Fatalf("shares = %s %s", a, b)
	}
	// Fixed per unit, capped at the line.
	if s := usecase.ShareOf(usecase.Applied{Basis: model.BasisFixed, FixedAmount: m("30")}, m("100"), 2); s.String() != "60.00" {
		t.Fatalf("fixed = %s", s)
	}
	if s := usecase.ShareOf(usecase.Applied{Basis: model.BasisFixed, FixedAmount: m("80")}, m("100"), 2); s.String() != "100.00" {
		t.Fatalf("capped = %s", s)
	}
	if s := usecase.ShareOf(usecase.Applied{Basis: model.BasisPercent, Rate: money.Percent(40)}, m("0"), 1); !s.IsZero() {
		t.Fatalf("zero line = %s", s)
	}
}

// TestStaffShares walks A15 (1.400 TL → 440 TL share, 960 TL business),
// A16 (366,67 / 633,33) and A17 (a rate change keeps the past, a
// reversal reverses the share, a payout is no second expense), the
// collection gate (payable once the visit is paid), payouts through the
// cash book, the owner's own sales, the member's own view and HTTP
// permissions. Runs when TEST_DATABASE_URL points at a migrated database.
func TestStaffShares(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	suffix := uuid.NewString()[:8]

	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	owner, mel, dave, other := newUser("owner"), newUser("mel"), newUser("dave"), newUser("other")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Shares " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	org2, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Shares other " + suffix, OwnerUserID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE organization_id = ANY($1)", []int64{org.ID, org2.ID})
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{org.ID, org2.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{owner.ID, mel.ID, dave.ID, other.ID})
	})
	if _, err := orgSvc.AddMember(ctx, org.UUID, mel.Uuid, "member"); err != nil {
		t.Fatal(err)
	}
	if _, err := orgSvc.AddMember(ctx, org.UUID, dave.Uuid, "resepsiyon"); err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	count := func(sql string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}
	idOf := func(sql string, args ...any) (int64, uuid.UUID) {
		t.Helper()
		var id int64
		var u uuid.UUID
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id, &u); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id, u
	}

	branchID, branchUUID := idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1", org.ID)
	exec(`UPDATE branches SET working_hours = $3, holiday_exceptions = '[]', timezone = NULL WHERE organization_id = $1 AND id = $2`,
		org.ID, branchID, `{"mon":{"closed":false,"open":"09:00","close":"18:00"}}`)
	newStaff := func(name string, userID *int64) (int64, uuid.UUID) {
		id, u := idOf(`INSERT INTO staff (organization_id, name, user_id) VALUES ($1, $2, $3) RETURNING id, uuid`, org.ID, name, userID)
		exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES
			($1, $2, $3, '{"mon":[{"start":"09:00","end":"18:00","breaks":[]}]}')`, org.ID, id, branchID)
		return id, u
	}
	ayseID, ayseUUID := newStaff("Ayşe", nil)
	mehmetID, mehmetUUID := newStaff("Mehmet", &mel.ID)
	bossID, bossUUID := newStaff("Sahip", &owner.ID)
	newService := func(name, price string, staff ...int64) (int64, uuid.UUID) {
		id, u := idOf(`INSERT INTO services (organization_id, name, duration_minutes, price, payment_mode)
			VALUES ($1, $2, 30, $3, 'both') RETURNING id, uuid`, org.ID, name, price)
		exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $3)`, org.ID, id, branchID)
		for _, s := range staff {
			exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3)`, org.ID, id, s)
		}
		return id, u
	}
	_, springUUID := newService("İlkbahar paketi", "1000", ayseID, bossID)
	_, aUUID := newService("Hizmet A", "666.67", ayseID)
	_, bUUID := newService("Hizmet B", "333.33", mehmetID)
	waxID, _ := idOf(`INSERT INTO products (organization_id, name, sku, price, cost) VALUES ($1, 'Wax', $2, 400, 250) RETURNING id, uuid`,
		org.ID, "WAX-"+suffix)
	_, customerUUID := idOf(`INSERT INTO customers (organization_id, first_name, last_name) VALUES ($1, 'Zeynep', 'Kaya') RETURNING id, uuid`, org.ID)
	_, tillUUID := idOf(`INSERT INTO cash_accounts (organization_id, branch_id, kind, name, currency, opening_date)
		VALUES ($1, $2, 'cash', 'Kasa', 'TRY', '2026-01-01') RETURNING id, uuid`, org.ID, branchID)

	now := on(7, 12)
	clock := func() time.Time { return now }
	cashSvc := cashusecase.New(cashrepository.NewPostgres(pool, q), nil)
	cashSvc.SetClock(clock)
	sales := salesusecase.New(salesrepository.NewPostgres(pool, q), cashSvc)
	sales.SetClock(clock)
	svc := usecase.New(repository.NewPostgres(pool, q), cashSvc)
	svc.SetClock(clock)
	sales.SetLineHook(svc.RecordLineTx)
	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	holds := availabilityusecase.NewHolds(pool, q, settings)
	holds.SetClock(func() time.Time { return on(7, 8) })
	holds.SetExpiryHook(reservationsusecase.ExpireRequests)
	resSvc := reservationsusecase.New(holds, q, settings)
	resSvc.SetCompletionHook(sales.RecordReservationTx)
	all := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, Timezone: "Europe/Istanbul"})

	book := func(start time.Time, service, staff uuid.UUID) uuid.UUID {
		t.Helper()
		res, err := resSvc.Create(ctx, org.ID, reservationsmodel.CreateInput{
			BranchUUID: branchUUID, CustomerUUID: customerUUID, StartsAt: &start, Source: reservationsmodel.SourcePanel,
			Lines: []availabilitymodel.LineInput{{ServiceUUID: service, StaffUUID: &staff}},
		}, availabilityusecase.Caller{}, owner.ID)
		if err != nil {
			t.Fatal(err)
		}
		return res.UUID
	}
	complete := func(id uuid.UUID) {
		t.Helper()
		if _, err := resSvc.Complete(ctx, org.ID, id, availabilityusecase.Caller{}, owner.ID, reservationsusecase.ChangeInput{}); err != nil {
			t.Fatal(err)
		}
	}
	sellWax := func(staff int64) uuid.UUID {
		t.Helper()
		line, err := sales.RecordProductSaleTx(ctx, q, org.ID, salesusecase.ProductSale{
			MovementUUID: uuid.New(), BranchID: branchID, ProductID: waxID, StaffID: &staff, Description: "Wax",
			Quantity: 1, UnitPrice: m("400"), OccurredAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		return line
	}
	rule := func(staff uuid.UUID, item string, service *uuid.UUID, rate string, from string) model.Rule {
		t.Helper()
		r, err := money.ParseRate(rate)
		if err != nil {
			t.Fatal(err)
		}
		out, err := svc.CreateRule(all, org.ID, &owner.ID, model.RuleInput{
			StaffUUID: &staff, ItemType: &item, ServiceUUID: service, Basis: ptr(model.BasisPercent), Rate: &r, ValidFrom: &from,
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	row := func(step string, v usecase.Viewer, from, to string, staff uuid.UUID) model.StaffSummary {
		t.Helper()
		s, err := svc.Summary(all, org.ID, v, model.SummaryFilter{DateFrom: date(from), DateTo: date(to)})
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		for _, r := range s.Staff {
			if r.Staff.UUID == staff {
				return r
			}
		}
		t.Fatalf("%s: no row for %s in %+v", step, staff, s.Staff)
		return model.StaffSummary{}
	}
	eq := func(step string, got money.Money, want string) {
		t.Helper()
		if got.String() != want {
			t.Fatalf("%s = %s, want %s", step, got, want)
		}
	}

	// Rules: Ayşe 40 % on services, 10 % product commission; Mehmet 30 %;
	// the owner has a rate too, but its own work stays with the business.
	ayseService := rule(ayseUUID, model.ItemService, nil, "40", "2030-01-01")
	rule(ayseUUID, model.ItemProduct, nil, "10", "2030-01-01")
	rule(mehmetUUID, model.ItemService, nil, "30", "2030-01-01")
	rule(bossUUID, model.ItemService, nil, "50", "2030-01-01")
	rule(bossUUID, model.ItemProduct, nil, "50", "2030-01-01")
	if _, err := svc.CreateRule(all, org.ID, &owner.ID, model.RuleInput{
		StaffUUID: &ayseUUID, ItemType: ptr(model.ItemService), Basis: ptr(model.BasisPercent), Rate: ptr(money.Percent(45)),
		ValidFrom: ptr("2030-01-01"),
	}); !errors.Is(err, usecase.ErrRuleTaken) {
		t.Fatalf("duplicate rule = %v", err)
	}
	if _, err := svc.CreateRule(all, org.ID, &owner.ID, model.RuleInput{
		StaffUUID: &ayseUUID, ItemType: ptr(model.ItemProduct), ServiceUUID: &aUUID, Basis: ptr(model.BasisPercent), Rate: ptr(money.Percent(1)),
	}); !usecase.IsValidation(err) {
		t.Fatalf("service rate on a product rule = %v", err)
	}
	if _, err := svc.CreateRule(all, org.ID, &owner.ID, model.RuleInput{
		StaffUUID: &ayseUUID, ItemType: ptr(model.ItemService), Basis: ptr(model.BasisPercent), Rate: ptr(money.Percent(120)),
	}); !usecase.IsValidation(err) {
		t.Fatalf("rate over 100 = %v", err)
	}
	narrow := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, BranchIDs: []int64{}})
	if _, err := svc.CreateRule(narrow, org.ID, &owner.ID, model.RuleInput{}); !errors.Is(err, usecase.ErrAllBranches) {
		t.Fatalf("branch-limited rule = %v", err)
	}

	// A15: the spring package (1.000) by Ayşe, a wax (400) sold by Ayşe.
	visit := book(on(7, 10), springUUID, ayseUUID)
	complete(visit)
	waxLine := sellWax(ayseID)
	// The owner's own sale: no share.
	sellWax(bossID)
	if n := count(`SELECT count(*) FROM staff_share_lines WHERE organization_id = $1 AND staff_id = $2`, org.ID, bossID); n != 0 {
		t.Fatalf("owner shares = %d", n)
	}
	ayse := row("A15", usecase.Viewer{}, "2030-01-07", "2030-01-07", ayseUUID)
	eq("A15 sales", ayse.Sales, "1400.00")
	eq("A15 share", ayse.Share, "440.00")
	eq("A15 business share", ayse.BusinessShare, "960.00")
	// The visit is not paid yet: its 400 waits, the counter sale is payable.
	eq("pending", ayse.Pending, "400.00")
	eq("payable", ayse.Payable, "40.00")
	eq("payable now", ayse.Balance.PayableNow, "40.00")
	boss := row("owner", usecase.Viewer{}, "2030-01-07", "2030-01-07", bossUUID)
	eq("owner sales", boss.Sales, "400.00")
	eq("owner share", boss.Share, "0.00")
	eq("owner business share", boss.BusinessShare, "400.00")
	lines, n, err := svc.ListLines(all, org.ID, usecase.Viewer{}, model.LineFilter{Staff: []uuid.UUID{ayseUUID}, Status: model.StatusPending, Limit: 10})
	if err != nil || n != 1 || lines[0].ItemType != model.ItemService || lines[0].Rate == nil || lines[0].Rate.String() != "40.00" {
		t.Fatalf("pending lines = %d %v %+v", n, err, lines)
	}

	// A payout above the payable share is refused.
	pay := func(amount, key string) (model.Payout, error) {
		return svc.Pay(all, org.ID, &owner.ID, model.PayoutInput{
			StaffUUID: ayseUUID, CashAccountUUID: tillUUID, Amount: m(amount), Note: "haftalık", IdempotencyKey: key,
		})
	}
	if _, err := pay("300", ""); !errors.Is(err, usecase.ErrPayoutTooLarge) {
		t.Fatalf("payout before collection = %v", err)
	}
	// The visit is paid: the share becomes payable.
	if _, err := sales.RecordCollection(all, org.ID, salesmodel.CollectionInput{
		ReservationUUID: &visit, CashAccountUUID: tillUUID, Amount: m("1000"), Method: salesmodel.MethodCash,
	}); err != nil {
		t.Fatal(err)
	}
	ayse = row("collected", usecase.Viewer{}, "2030-01-07", "2030-01-07", ayseUUID)
	eq("pending after collection", ayse.Pending, "0.00")
	eq("payable now after collection", ayse.Balance.PayableNow, "440.00")

	// 300 paid: 140 left (A15). The payout is a staff_payment of the cash
	// book, no expense (A17); a retried request is the same payout.
	p, err := pay("300", "pay-"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != model.PayoutPayout || p.Staff.UUID != ayseUUID || p.PayoutDate != "2030-01-07" || p.CashAccount.UUID != tillUUID {
		t.Fatalf("payout = %+v", p)
	}
	if again, err := pay("300", "pay-"+suffix); err != nil || again.UUID != p.UUID {
		t.Fatalf("replay = %v %v", again.UUID, err)
	}
	if _, err := pay("200", "pay-"+suffix); !errors.Is(err, usecase.ErrIdempotencyReused) {
		t.Fatalf("reused key = %v", err)
	}
	if n := count(`SELECT count(*) FROM cash_movements WHERE organization_id = $1 AND kind = 'staff_payment'`, org.ID); n != 1 {
		t.Fatalf("staff payments = %d", n)
	}
	if n := count(`SELECT count(*) FROM cash_movements WHERE organization_id = $1 AND kind = 'expense'`, org.ID); n != 0 {
		t.Fatalf("expenses = %d", n)
	}
	ayse = row("paid", usecase.Viewer{}, "2030-01-07", "2030-01-07", ayseUUID)
	eq("period paid", ayse.PeriodPaid, "300.00")
	eq("remaining", ayse.Balance.Remaining, "140.00")
	eq("payable now after payout", ayse.Balance.PayableNow, "140.00")
	if _, err := pay("150", ""); !errors.Is(err, usecase.ErrPayoutTooLarge) {
		t.Fatalf("payout above the rest = %v", err)
	}

	// A17: a rate change does not rewrite the past.
	if _, _, err := svc.PatchRule(all, org.ID, ayseService.UUID, model.RuleInput{Rate: ptr(money.Percent(60))}); err != nil {
		t.Fatal(err)
	}
	eq("share after rate change", row("rate change", usecase.Viewer{}, "2030-01-07", "2030-01-07", ayseUUID).Share, "440.00")
	// A correction keeps the rate of the line it replaces: 40 % of 900.
	svcLines, _, err := sales.ListLines(all, org.ID, salesmodel.LineFilter{ReservationUUID: &visit, Limit: 10})
	if err != nil || len(svcLines) != 1 {
		t.Fatalf("visit lines = %v %v", svcLines, err)
	}
	if _, err := sales.CorrectLine(all, org.ID, svcLines[0].UUID, &owner.ID, salesmodel.CorrectInput{
		DiscountAmount: ptr(m("100")), Reason: "sadakat indirimi",
	}); err != nil {
		t.Fatal(err)
	}
	// A returned wax reverses its share.
	if _, err := sales.ReverseLineTx(ctx, q, org.ID, waxLine, "iade", &owner.ID); err != nil {
		t.Fatal(err)
	}
	ayse = row("corrected", usecase.Viewer{}, "2030-01-07", "2030-01-07", ayseUUID)
	eq("sales after correction and return", ayse.Sales, "900.00")
	eq("share after correction and return", ayse.Share, "360.00")
	eq("remaining after correction", ayse.Balance.Remaining, "60.00")
	if n := count(`SELECT count(*) FROM staff_share_lines WHERE organization_id = $1 AND kind = 'reversal'`, org.ID); n != 2 {
		t.Fatalf("share reversals = %d", n)
	}
	// Shares are append-only.
	for _, sql := range []string{
		`UPDATE staff_share_lines SET share_amount = 1 WHERE organization_id = $1`,
		`DELETE FROM staff_share_lines WHERE organization_id = $1`,
		`UPDATE staff_share_payouts SET amount = 1 WHERE organization_id = $1`,
	} {
		if _, err := pool.Exec(ctx, sql, org.ID); err == nil {
			t.Fatalf("%s accepted", sql)
		}
	}

	// A wrong payout is reversed with its cash movement.
	rev, err := svc.ReversePayout(all, org.ID, p.UUID, &owner.ID, model.ReverseInput{Reason: "yanlış kasa"})
	if err != nil {
		t.Fatal(err)
	}
	if rev.Kind != model.PayoutReversal || rev.ReversesUUID == nil || *rev.ReversesUUID != p.UUID {
		t.Fatalf("reversal = %+v", rev)
	}
	if _, err := svc.ReversePayout(all, org.ID, p.UUID, &owner.ID, model.ReverseInput{Reason: "tekrar"}); !errors.Is(err, usecase.ErrAlreadyReversed) {
		t.Fatalf("second reversal = %v", err)
	}
	eq("remaining after reversal", row("reversed", usecase.Viewer{}, "2030-01-07", "2030-01-07", ayseUUID).Balance.Remaining, "360.00")

	// Deleting a rule keeps its shares.
	if _, err := svc.DeleteRule(all, org.ID, ayseService.UUID); err != nil {
		t.Fatal(err)
	}
	eq("share after rule delete", row("rule delete", usecase.Viewer{}, "2030-01-07", "2030-01-07", ayseUUID).Share, "360.00")
	rule(ayseUUID, model.ItemService, nil, "40", "2030-01-01")

	// A16: Ayşe (service A, an override of 40 %) and Mehmet (service B,
	// 30 %) share a package of 1.000: 266,67 + 100,00.
	now = on(14, 12)
	rule(ayseUUID, model.ItemService, &aUUID, "40", "2030-01-10")
	va := book(on(14, 10), aUUID, ayseUUID)
	vb := book(on(14, 10), bUUID, mehmetUUID)
	complete(va)
	complete(vb)
	s, err := svc.Summary(all, org.ID, usecase.Viewer{}, model.SummaryFilter{DateFrom: date("2030-01-14"), DateTo: date("2030-01-14")})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Totals) != 1 || len(s.Branches) != 1 {
		t.Fatalf("A16 summary = %+v", s)
	}
	eq("A16 revenue", s.Totals[0].Revenue, "1000.00")
	eq("A16 shares", s.Totals[0].Share, "366.67")
	eq("A16 business share", s.Totals[0].BusinessShare, "633.33")
	aLines, _, _ := svc.ListLines(all, org.ID, usecase.Viewer{}, model.LineFilter{Staff: []uuid.UUID{ayseUUID}, DateFrom: date("2030-01-14"), Limit: 10})
	if len(aLines) != 1 || aLines[0].RuleScope != model.ScopeService {
		t.Fatalf("A16 Ayşe lines = %+v", aLines)
	}

	// The member sees only its own shares.
	melView := usecase.Viewer{OwnUserID: mel.ID}
	own, n, err := svc.ListLines(all, org.ID, melView, model.LineFilter{Limit: 50})
	if err != nil || n != 1 || own[0].Staff.UUID != mehmetUUID {
		t.Fatalf("own lines = %d %v", n, err)
	}
	if own, n, _ := svc.ListLines(all, org.ID, melView, model.LineFilter{Staff: []uuid.UUID{ayseUUID}, Limit: 50}); n != 0 || len(own) != 0 {
		t.Fatalf("own lines naming Ayşe = %d", n)
	}
	ms, err := svc.Summary(all, org.ID, melView, model.SummaryFilter{DateFrom: date("2030-01-01"), DateTo: date("2030-01-31")})
	if err != nil || len(ms.Staff) != 1 || ms.Staff[0].Staff.UUID != mehmetUUID || len(ms.Branches) != 0 {
		t.Fatalf("own summary = %+v %v", ms, err)
	}
	mine, err := svc.Mine(all, org.ID, mel.ID, ptrDate(""), ptrDate(""))
	if err != nil || mine.Staff.UUID != mehmetUUID || mine.Today != "2030-01-14" || mine.DateFrom != "2030-01-01" ||
		len(mine.Daily) != 1 || mine.Daily[0].Share.String() != "100.00" {
		t.Fatalf("mine = %+v %v", mine, err)
	}
	if _, err := svc.Mine(all, org.ID, dave.ID, ptrDate(""), ptrDate("")); !errors.Is(err, usecase.ErrNoStaffRecord) {
		t.Fatalf("mine without staff = %v", err)
	}

	// Another business and an out-of-scope member see none of it.
	if s, err := svc.Summary(ctx, org2.ID, usecase.Viewer{}, model.SummaryFilter{DateFrom: date("2030-01-01"), DateTo: date("2030-01-31")}); err != nil || len(s.Staff) != 0 {
		t.Fatalf("foreign summary = %+v %v", s, err)
	}
	if _, err := svc.GetPayout(ctx, org2.ID, usecase.Viewer{}, p.UUID); !errors.Is(err, usecase.ErrPayoutNotFound) {
		t.Fatalf("foreign payout = %v", err)
	}
	if _, err := svc.GetRule(ctx, org2.ID, usecase.Viewer{}, ayseService.UUID); !errors.Is(err, usecase.ErrRuleNotFound) {
		t.Fatalf("foreign rule = %v", err)
	}
	if _, n, err := svc.ListLines(narrow, org.ID, usecase.Viewer{}, model.LineFilter{Limit: 10}); err != nil || n != 0 {
		t.Fatalf("out of scope lines = %d %v", n, err)
	}
	if _, err := svc.GetPayout(narrow, org.ID, usecase.Viewer{}, p.UUID); !errors.Is(err, usecase.ErrPayoutNotFound) {
		t.Fatalf("out of scope payout = %v", err)
	}

	// HTTP: the owner manages rates and pays; the member reads its own and
	// may not pay or change rates; reception sees nothing.
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	loader := mapLoader{owner.Uuid: owner.ID, mel.Uuid: mel.ID, dave.Uuid: dave.ID}
	staffshares.RegisterRoutes(mux, handler.New(svc, nil), tokens, loader, orgSvc)
	call := func(user db.User, method, path string, body any) (int, map[string]any, string) {
		t.Helper()
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user.Uuid})
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, org.Slug)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return rr.Code, env.Data, rr.Body.String()
	}
	if code, data, raw := call(owner, http.MethodGet, "/v1/tenant/staff-shares/summary?date_from=2030-01-14&date_to=2030-01-14", nil); code != http.StatusOK ||
		data["totals"].([]any)[0].(map[string]any)["business_share"] != "633.33" {
		t.Fatalf("owner summary = %d %s", code, raw)
	}
	if code, data, raw := call(mel, http.MethodGet, "/v1/tenant/staff-shares/summary?date_from=2030-01-01&date_to=2030-01-31", nil); code != http.StatusOK ||
		len(data["staff"].([]any)) != 1 || len(data["branches"].([]any)) != 0 {
		t.Fatalf("member summary = %d %s", code, raw)
	}
	if code, _, raw := call(mel, http.MethodGet, "/v1/tenant/staff-shares/me", nil); code != http.StatusOK {
		t.Fatalf("member me = %d %s", code, raw)
	}
	if code, _, raw := call(mel, http.MethodPost, "/v1/tenant/staff-shares/rules", map[string]any{
		"staff_uuid": mehmetUUID, "item_type": "service", "basis": "percent", "rate": "90",
	}); code != http.StatusForbidden {
		t.Fatalf("member rule = %d %s", code, raw)
	}
	if code, _, raw := call(mel, http.MethodPost, "/v1/tenant/staff-shares/payouts", map[string]any{
		"staff_uuid": mehmetUUID, "cash_account_uuid": tillUUID, "amount": "1",
	}); code != http.StatusForbidden {
		t.Fatalf("member payout = %d %s", code, raw)
	}
	if code, _, raw := call(dave, http.MethodGet, "/v1/tenant/staff-shares/lines", nil); code != http.StatusForbidden {
		t.Fatalf("reception lines = %d %s", code, raw)
	}
	if code, data, raw := call(owner, http.MethodPost, "/v1/tenant/staff-shares/rules", map[string]any{
		"staff_uuid": mehmetUUID, "item_type": "product", "basis": "fixed", "fixed_amount": "15", "valid_from": "2030-01-01",
	}); code != http.StatusCreated || data["fixed_amount"] != "15.00" {
		t.Fatalf("owner rule = %d %s", code, raw)
	}
	// The visits of 01-14 are not paid: Mehmet's 100 is not payable yet.
	if code, _, raw := call(owner, http.MethodPost, "/v1/tenant/staff-shares/payouts", map[string]any{
		"staff_uuid": mehmetUUID, "cash_account_uuid": tillUUID, "amount": "50",
	}); code != http.StatusConflict {
		t.Fatalf("payout of a pending share = %d %s", code, raw)
	}
	if code, data, raw := call(owner, http.MethodPost, "/v1/tenant/staff-shares/payouts", map[string]any{
		"staff_uuid": ayseUUID, "cash_account_uuid": tillUUID, "amount": "100", "note": "avans",
	}); code != http.StatusCreated || data["amount"] != "100.00" {
		t.Fatalf("owner payout = %d %s", code, raw)
	}
	if code, data, raw := call(mel, http.MethodGet, "/v1/tenant/staff-shares/payouts", nil); code != http.StatusOK || data["total"].(float64) != 0 {
		t.Fatalf("member payouts = %d %s", code, raw)
	}
	if code, _, raw := call(owner, http.MethodGet, "/v1/tenant/staff-shares/lines?status=bogus", nil); code != http.StatusBadRequest {
		t.Fatalf("bad status = %d %s", code, raw)
	}
	_ = vb
}

func ptrDate(string) *pgtype.Date { return &pgtype.Date{} }
