package reports_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

var ist = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

func at(day, hour int) time.Time { return time.Date(2030, 1, day, hour, 0, 0, 0, ist) }

func date(s string) *time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return &t
}

type mapLoader map[uuid.UUID]int64

func (l mapLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: l[id]}, nil
}

// TestReports seeds two branches and the head office with reservations,
// confirmations, sale lines (a correction included), collections and a
// refund, a transfer, an expense, other income, a staff share and a
// payout, and stock; then checks the totals (A20: revenue, collections,
// transfers and staff payouts apart), the days in the branch time zone,
// the branch comparison, the branch / head office / staff filters, the
// caller's branch scope, another business, the CSV and HTTP permissions.
// Runs when TEST_DATABASE_URL points at a migrated database.
func TestReports(t *testing.T) {
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
	owner, desk, other := newUser("owner"), newUser("desk"), newUser("other")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Reports " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	org2, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Reports other " + suffix, OwnerUserID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE organization_id = ANY($1)", []int64{org.ID, org2.ID})
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{org.ID, org2.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{owner.ID, desk.ID, other.ID})
	})
	if _, err := orgSvc.AddMember(ctx, org.UUID, desk.Uuid, "resepsiyon"); err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
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

	xID, xUUID := idOf("SELECT id, uuid FROM branches WHERE organization_id = $1 ORDER BY id LIMIT 1", org.ID)
	exec("UPDATE branches SET timezone = NULL WHERE organization_id = $1", org.ID)
	yID, yUUID := idOf("INSERT INTO branches (organization_id, code, name) VALUES ($1, 'YY', 'Yalı') RETURNING id, uuid", org.ID)
	ayseID, ayseUUID := idOf("INSERT INTO staff (organization_id, name) VALUES ($1, 'Ayşe') RETURNING id, uuid", org.ID)
	mehmetID, _ := idOf("INSERT INTO staff (organization_id, name) VALUES ($1, 'Mehmet') RETURNING id, uuid", org.ID)
	customerID, _ := idOf("INSERT INTO customers (organization_id, first_name, last_name) VALUES ($1, 'Zeynep', 'Kaya') RETURNING id, uuid", org.ID)
	waxID, _ := idOf("INSERT INTO products (organization_id, name, sku, price, cost, low_stock_threshold) VALUES ($1, 'Wax', $2, 400, 250, 5) RETURNING id, uuid",
		org.ID, "WAX-"+suffix)

	number := int64(0)
	reservation := func(branch int64, status, source string, start time.Time, staff *int64) (int64, uuid.UUID) {
		number++
		id, u := idOf(`INSERT INTO reservations (organization_id, number, branch_id, customer_id, status, approval_mode, approval_status,
			payment_status, source, starts_at, ends_at, total_price, currency)
			VALUES ($1, $2, $3, $4, $5, 'panel', 'accepted', 'on_site', $6, $7, $8, 1000, 'TRY') RETURNING id, uuid`,
			org.ID, number, branch, customerID, status, source, start, start.Add(time.Hour))
		exec(`INSERT INTO reservation_lines (organization_id, reservation_id, line_index, service_name, staff_id, starts_at, ends_at,
			duration_minutes, unit_price) VALUES ($1, $2, 0, 'Kesim', $3, $4, $5, 60, 1000)`, org.ID, id, staff, start, start.Add(time.Hour))
		return id, u
	}
	r1, r1UUID := reservation(xID, "completed", "panel", at(14, 10), &ayseID)
	// 01:00 on the 15th in Istanbul is the 14th in UTC: it counts on the 15th.
	r2, _ := reservation(xID, "cancelled", "online", at(15, 1), &mehmetID)
	r3, r3UUID := reservation(yID, "completed", "embed", at(14, 12), nil)
	_ = r3
	confirmation := func(res int64, start time.Time, status string, outcome any) {
		exec(`INSERT INTO reservation_confirmations (organization_id, reservation_id, reservation_version, starts_at, status, channels,
			on_no_answer, deadline_at, planned_at, outcome) VALUES ($1, $2, 1, $3, $4, '{sms}', 'cancel', $5, $5, $6)`,
			org.ID, res, start, status, start.Add(-2*time.Hour), outcome)
	}
	confirmation(r1, at(14, 10), "confirmed", nil)
	confirmation(r2, at(15, 1), "expired", "cancelled")

	saleLine := func(branch int64, reservationID *int64, item, key string, source *uuid.UUID, staff *int64, qty int, unit, discount, day string) int64 {
		var id int64
		sourceType := "reservation"
		var lineIndex any = 0
		var product any
		if item == "product" {
			sourceType, lineIndex, product = "stock_movement", nil, waxID
		}
		if err := pool.QueryRow(ctx, `INSERT INTO sale_lines (organization_id, branch_id, customer_id, kind, item_type, source_type,
			source_uuid, reservation_id, line_index, product_id, staff_id, description, quantity, unit_price, gross_amount,
			discount_amount, net_amount, currency, sale_date, accrual_key)
			VALUES ($1, $2, $3, 'sale', $4, $5, $6, $7, $8, $9, $10, 'x', $11::int, $12::numeric, $12::numeric * $11::int,
			$13::numeric, $12::numeric * $11::int - $13::numeric, 'TRY', $14::date, $15) RETURNING id`,
			org.ID, branch, customerID, item, sourceType, source, reservationID, lineIndex, product, staff, qty, unit, discount, day, key,
		).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	s1 := saleLine(xID, &r1, "service", "reservation:"+r1UUID.String()+":line:0", nil, &ayseID, 1, "1000", "100", "2030-01-14")
	_, smUUID := idOf(`INSERT INTO stock_movements (organization_id, branch_id, product_id, kind, quantity, balance_after, unit_price,
		staff_id, occurred_at) VALUES ($1, $2, $3, 'sale', -2, 0, 400, $4, $5) RETURNING id, uuid`, org.ID, xID, waxID, mehmetID, at(14, 11))
	s2 := saleLine(xID, nil, "product", "stock_movement:"+smUUID.String(), &smUUID, &mehmetID, 2, "400", "0", "2030-01-14")
	// The product sale is reversed the next day (its cost goes with it).
	exec(`INSERT INTO sale_lines (organization_id, branch_id, customer_id, kind, item_type, source_type, source_uuid, product_id,
		staff_id, description, quantity, unit_price, gross_amount, discount_amount, net_amount, currency, sale_date, accrual_key,
		reverses_line_id) SELECT organization_id, branch_id, customer_id, 'reversal', item_type, source_type, source_uuid, product_id,
		staff_id, description, quantity, unit_price, -gross_amount, -discount_amount, -net_amount, currency, '2030-01-15', accrual_key, id
		FROM sale_lines WHERE id = $1`, s2)
	saleLine(yID, &r3, "service", "reservation:"+r3UUID.String()+":line:0", nil, nil, 1, "500", "0", "2030-01-14")
	exec(`INSERT INTO staff_share_lines (organization_id, branch_id, staff_id, sale_line_id, reservation_id, kind, item_type, rule_scope,
		basis, rate, base_amount, share_amount, currency, share_date) VALUES ($1, $2, $3, $4, $5, 'share', 'service', 'default',
		'percent', 40, 900, 360, 'TRY', '2030-01-14')`, org.ID, xID, ayseID, s1, r1)

	tillID, _ := idOf(`INSERT INTO cash_accounts (organization_id, branch_id, kind, name, opening_date) VALUES ($1, $2, 'cash', 'Kasa', '2030-01-01') RETURNING id, uuid`, org.ID, xID)
	bankID, _ := idOf(`INSERT INTO cash_accounts (organization_id, branch_id, kind, name, opening_date) VALUES ($1, NULL, 'bank', 'Banka', '2030-01-01') RETURNING id, uuid`, org.ID)
	movement := func(account int64, branch any, kind, direction, amount, day string, extra ...any) int64 {
		var id int64
		var group, counter any
		if len(extra) == 2 {
			group, counter = extra[0], extra[1]
		}
		if err := pool.QueryRow(ctx, `INSERT INTO cash_movements (organization_id, cash_account_id, branch_id, kind, direction, amount,
			currency, movement_date, transfer_group, counter_account_id) VALUES ($1, $2, $3, $4, $5, $6, 'TRY', $7, $8, $9) RETURNING id`,
			org.ID, account, branch, kind, direction, amount, day, group, counter).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	collected := movement(tillID, xID, "collection", "in", "1000", "2030-01-14")
	var collectionID int64
	if err := pool.QueryRow(ctx, `INSERT INTO ledger_entries (organization_id, branch_id, customer_id, reservation_id, kind, direction,
		amount, currency, entry_date, method, cash_movement_id) VALUES ($1, $2, $3, $4, 'collection', 'credit', 1000, 'TRY', '2030-01-14',
		'cash', $5) RETURNING id`, org.ID, xID, customerID, r1, collected).Scan(&collectionID); err != nil {
		t.Fatal(err)
	}
	refunded := movement(tillID, xID, "refund", "out", "100", "2030-01-15")
	exec(`INSERT INTO ledger_entries (organization_id, branch_id, customer_id, reservation_id, kind, direction, amount, currency,
		entry_date, method, cash_movement_id, refunds_entry_id) VALUES ($1, $2, $3, $4, 'refund', 'debit', 100, 'TRY', '2030-01-15',
		'cash', $5, $6)`, org.ID, xID, customerID, r1, refunded, collectionID)
	group := uuid.New()
	movement(tillID, xID, "transfer_out", "out", "200", "2030-01-14", group, bankID)
	movement(bankID, nil, "transfer_in", "in", "200", "2030-01-14", group, tillID)
	movement(tillID, xID, "expense", "out", "150", "2030-01-14")
	movement(bankID, nil, "income", "in", "50", "2030-01-15")
	paid := movement(bankID, nil, "staff_payment", "out", "300", "2030-01-15")
	exec(`INSERT INTO staff_share_payouts (organization_id, staff_id, branch_id, cash_account_id, cash_movement_id, kind, amount,
		currency, payout_date) VALUES ($1, $2, NULL, $3, $4, 'payout', 300, 'TRY', '2030-01-15')`, org.ID, ayseID, bankID, paid)
	exec(`INSERT INTO stock_levels (organization_id, branch_id, product_id, on_hand, reserved) VALUES ($1, $2, $3, 3, 0)`, org.ID, yID, waxID)

	// Packages (TEC-983): P1 sold at X on the 14th (5 x 500, one session
	// used on the 14th at X, one held at Y); P2 sold at Y before the period,
	// one session used, last valid day the 14th (2 sessions, 600 lost); P3
	// sold and refunded at X on the 15th. None of it is revenue.
	pkg := func(branch int64, sessions int, price, soldOn string, expiresOn any, refunded bool) int64 {
		var id int64
		var refundAmount, refundedOn, refundedAt any
		status := "active"
		if refunded {
			status, refundAmount, refundedOn, refundedAt = "refunded", price, soldOn, at(15, 11)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO customer_packages (organization_id, customer_id, branch_id, name, service_name,
			total_sessions, price, currency, sold_on, expires_on, status, refund_amount, refunded_on, refunded_at)
			VALUES ($1, $2, $3, 'Paket', 'Kesim', $4, $5::numeric, 'TRY', $6::date, $7::date, $8, $9::numeric, $10::date, $11) RETURNING id`,
			org.ID, customerID, branch, sessions, price, soldOn, expiresOn, status, refundAmount, refundedOn, refundedAt).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	session := func(packageID, reservationID int64, no int, amount string, consumedOn any) {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO package_sessions (organization_id, customer_package_id, reservation_id, line_index,
			session_no, amount) VALUES ($1, $2, $3, 0, $4, $5::numeric) RETURNING id`, org.ID, packageID, reservationID, no, amount).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if consumedOn != nil {
			exec(`UPDATE package_sessions SET status = 'consumed', consumed_at = NOW(), consumed_on = $2::date WHERE id = $1`, id, consumedOn)
		}
	}
	p1 := pkg(xID, 5, "2500", "2030-01-14", nil, false)
	session(p1, r1, 1, "500", "2030-01-14")
	session(p1, r3, 2, "500", nil)
	p2 := pkg(yID, 3, "900", "2030-01-10", "2030-01-14", false)
	session(p2, r2, 1, "300", "2030-01-12")
	pkg(xID, 2, "400", "2030-01-15", nil, true)

	svc := usecase.New(q)
	svc.SetClock(func() time.Time { return at(15, 12) })
	all := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, Timezone: "Europe/Istanbul", Currency: "TRY"})
	period := model.Filter{DateFrom: date("2030-01-14"), DateTo: date("2030-01-15")}
	rep, err := svc.Summary(all, org.ID, period)
	if err != nil {
		t.Fatal(err)
	}
	tm := rep.Totals.Money[0]
	for name, pair := range map[string][2]string{
		"revenue.gross":      {tm.Revenue.Gross.String(), "2300.00"},
		"revenue.discount":   {tm.Revenue.Discount.String(), "100.00"},
		"revenue.reversed":   {tm.Revenue.Reversed.String(), "-800.00"},
		"revenue.net":        {tm.Revenue.Net.String(), "1400.00"},
		"revenue.service":    {tm.Revenue.ServiceNet.String(), "1400.00"},
		"revenue.product":    {tm.Revenue.ProductNet.String(), "0.00"},
		"product_cost":       {tm.ProductCost.String(), "0.00"},
		"staff_share":        {tm.StaffShare.String(), "360.00"},
		"business_share":     {tm.BusinessShare.String(), "1040.00"},
		"collections.net":    {tm.Collections.Net.String(), "900.00"},
		"collections.refund": {tm.Collections.Refunded.String(), "100.00"},
		"transfers":          {tm.Transfers.String(), "200.00"},
		"expenses":           {tm.Expenses.String(), "150.00"},
		"income":             {tm.Income.String(), "50.00"},
		"staff_payments":     {tm.StaffPayments.String(), "300.00"},
		"operational_result": {tm.OperationalResult.String(), "890.00"},
	} {
		if pair[0] != pair[1] {
			t.Errorf("totals %s = %s, want %s", name, pair[0], pair[1])
		}
	}
	if r := rep.Totals.Reservations; r.Total != 3 || r.Completed != 2 || r.Cancelled != 1 || r.FromPanel != 1 || r.FromOnline != 1 || r.FromEmbed != 1 {
		t.Errorf("reservations = %+v", r)
	}
	if c := rep.Totals.Confirmations; c.Total != 2 || c.Confirmed != 1 || c.Cancelled != 1 {
		t.Errorf("confirmations = %+v", c)
	}
	if s := rep.Totals.Stock; s.Sold != 2 || s.OnHand != 3 || s.LowItems != 1 || s.Value.String() != "750.00" {
		t.Errorf("stock = %+v", s)
	}
	// Days: the 14th carries the sales, the 15th the reversal, the refund
	// and the visit of 01:00 local time.
	if len(rep.Days) != 2 || rep.Days[0].Date != "2030-01-14" || rep.Days[0].Money[0].Revenue.Net.String() != "2200.00" ||
		rep.Days[0].Money[0].ProductCost.String() != "500.00" || rep.Days[0].Reservations.Total != 2 ||
		rep.Days[1].Money[0].Revenue.Net.String() != "-800.00" || rep.Days[1].Reservations.Total != 1 ||
		rep.Days[1].Confirmations.Cancelled != 1 {
		t.Errorf("days = %+v", rep.Days)
	}
	// Branch comparison: X, Y, then the head office (income, payout).
	if len(rep.Branches) != 3 || rep.Branches[0].Branch == nil || rep.Branches[2].Branch != nil {
		t.Fatalf("branches = %+v", rep.Branches)
	}
	byUUID := map[uuid.UUID]model.Branch{}
	for _, b := range rep.Branches[:2] {
		byUUID[b.Branch.UUID] = b
	}
	if x := byUUID[xUUID]; x.Money[0].Revenue.Net.String() != "900.00" || x.Money[0].Transfers.String() != "200.00" ||
		x.Money[0].Expenses.String() != "150.00" || x.Money[0].OperationalResult.String() != "390.00" || x.Stock.Sold != 2 {
		t.Errorf("branch X = %+v", x)
	}
	if y := byUUID[yUUID]; y.Money[0].Revenue.Net.String() != "500.00" || y.Reservations.Total != 1 || y.Stock.LowItems != 1 {
		t.Errorf("branch Y = %+v", y)
	}
	if ho := rep.Branches[2]; ho.Money[0].Income.String() != "50.00" || ho.Money[0].StaffPayments.String() != "300.00" ||
		!ho.Money[0].Revenue.Net.IsZero() {
		t.Errorf("head office = %+v", ho)
	}

	// Packages: apart from revenue (still 1400 above).
	pk := tm.Packages
	if pk.Sold.Count != 2 || pk.Sold.Sessions != 7 || pk.Sold.Amount.String() != "2900.00" ||
		pk.Used.Sessions != 1 || pk.Used.Amount.String() != "500.00" ||
		pk.Refunded.Count != 1 || pk.Refunded.Sessions != 2 || pk.Refunded.Amount.String() != "400.00" ||
		pk.Expired.Count != 1 || pk.Expired.Sessions != 2 || pk.Expired.Amount.String() != "600.00" {
		t.Errorf("packages = %+v", pk)
	}
	if pb := rep.Totals.PackageBalance; len(pb) != 1 || pb[0].Currency != "TRY" || pb[0].Packages != 1 || pb[0].OpenSessions != 3 ||
		pb[0].OpenAmount.String() != "1500.00" || pb[0].HeldSessions != 1 || pb[0].HeldAmount.String() != "500.00" {
		t.Errorf("package balance = %+v", pb)
	}
	if d0, d1 := rep.Days[0].Money[0].Packages, rep.Days[1].Money[0].Packages; d0.Sold.Amount.String() != "2500.00" ||
		d0.Used.Sessions != 1 || d0.Expired.Sessions != 2 || d1.Sold.Amount.String() != "400.00" || d1.Refunded.Count != 1 {
		t.Errorf("package days = %+v / %+v", d0, d1)
	}
	if x, y := byUUID[xUUID], byUUID[yUUID]; x.Money[0].Packages.Sold.Count != 2 || x.Money[0].Packages.Used.Sessions != 1 ||
		x.PackageBalance[0].OpenSessions != 3 || y.Money[0].Packages.Sold.Count != 0 || y.Money[0].Packages.Expired.Amount.String() != "600.00" ||
		y.PackageBalance[0].Packages != 0 {
		t.Errorf("package branches = %+v / %+v", x.Money[0].Packages, y.Money[0].Packages)
	}

	// Branch filter: Y only (no head office row); head office only.
	onlyY, err := svc.Summary(all, org.ID, model.Filter{DateFrom: period.DateFrom, DateTo: period.DateTo, Branches: []uuid.UUID{yUUID}})
	if err != nil || len(onlyY.Branches) != 1 || onlyY.Totals.Money[0].Revenue.Net.String() != "500.00" ||
		!onlyY.Totals.Money[0].Income.IsZero() || onlyY.Scope.HeadOffice {
		t.Errorf("only Y = %+v %v", onlyY, err)
	}
	onlyHO, err := svc.Summary(all, org.ID, model.Filter{DateFrom: period.DateFrom, DateTo: period.DateTo, HeadOffice: true})
	if err != nil || len(onlyHO.Branches) != 1 || onlyHO.Branches[0].Branch != nil ||
		onlyHO.Totals.Money[0].Income.String() != "50.00" || !onlyHO.Totals.Money[0].Revenue.Net.IsZero() {
		t.Errorf("only head office = %+v %v", onlyHO, err)
	}
	// Staff filter: Ayşe's sale, share and payout; money not tied to a
	// staff member is left out and listed.
	ayse, err := svc.Summary(all, org.ID, model.Filter{DateFrom: period.DateFrom, DateTo: period.DateTo, Staff: []uuid.UUID{ayseUUID}})
	if err != nil || ayse.Totals.Money[0].Revenue.Net.String() != "900.00" || ayse.Totals.Money[0].StaffShare.String() != "360.00" ||
		ayse.Totals.Money[0].StaffPayments.String() != "300.00" || !ayse.Totals.Money[0].Collections.Collected.IsZero() ||
		!ayse.Totals.Money[0].Expenses.IsZero() || ayse.Totals.Money[0].OperationalResult.String() != "540.00" ||
		ayse.Totals.Reservations.Total != 1 || ayse.Totals.Confirmations.Confirmed != 1 || len(ayse.Scope.OmittedForStaff) != 5 ||
		ayse.Totals.Money[0].Packages.Sold.Count != 0 || ayse.Totals.PackageBalance[0].Packages != 0 {
		t.Errorf("staff Ayşe = %+v %v", ayse, err)
	}
	// Unknown branch or staff: an empty report, nothing echoed.
	unknown, err := svc.Summary(all, org.ID, model.Filter{Branches: []uuid.UUID{uuid.New()}, Staff: []uuid.UUID{uuid.New()}})
	if err != nil || !unknown.Totals.Money[0].Revenue.Net.IsZero() || len(unknown.Scope.Branches) != 0 || len(unknown.Scope.Staff) != 0 {
		t.Errorf("unknown filter = %+v %v", unknown, err)
	}
	// A member limited to Y sees Y only, never the head office.
	narrow := orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, Timezone: "Europe/Istanbul", Currency: "TRY", BranchIDs: []int64{yID}})
	ny, err := svc.Summary(narrow, org.ID, period)
	if err != nil || len(ny.Branches) != 1 || ny.Totals.Money[0].Revenue.Net.String() != "500.00" ||
		!ny.Totals.Money[0].StaffPayments.IsZero() || !ny.Totals.Money[0].Transfers.IsZero() ||
		ny.Totals.Money[0].Packages.Sold.Count != 0 || ny.Totals.Money[0].Packages.Expired.Sessions != 2 ||
		ny.Totals.PackageBalance[0].Packages != 0 {
		t.Errorf("narrow = %+v %v", ny, err)
	}
	// Another business sees none of it; the range is checked.
	foreign, err := svc.Summary(orgctx.WithScope(ctx, orgctx.Scope{ID: org2.ID}), org2.ID, period)
	if err != nil || foreign.Totals.Reservations.Total != 0 || !foreign.Totals.Money[0].Revenue.Net.IsZero() {
		t.Errorf("foreign = %+v %v", foreign, err)
	}
	if _, err := svc.Summary(all, org.ID, model.Filter{DateFrom: date("2030-01-15"), DateTo: date("2030-01-14")}); !usecase.IsValidation(err) {
		t.Errorf("reversed range = %v", err)
	}
	if _, err := svc.Summary(all, org.ID, model.Filter{DateFrom: date("2029-01-01"), DateTo: date("2030-01-02")}); !usecase.IsValidation(err) {
		t.Errorf("long range = %v", err)
	}
	if _, err := svc.Summary(all, org.ID, model.Filter{DateFrom: date("2029-01-01"), DateTo: date("2029-12-31")}); err != nil {
		t.Errorf("a year = %v", err)
	}
	if today, err := svc.Summary(all, org.ID, model.Filter{}); err != nil || today.DateFrom != "2030-01-15" || today.DateTo != "2030-01-15" {
		t.Errorf("default range = %+v %v", today, err)
	}

	// CSV: one row per day plus the total, Turkish titles by default.
	ds := usecase.Dataset(rep, usecase.SectionDays, "")
	if len(ds.Rows) != 3 || ds.Rows[2]["revenue_net"] != "1400.00" || ds.Rows[0]["reservations_total"] != int64(2) ||
		ds.Rows[0]["package_sold_amount"] != "2500.00" || ds.Rows[2]["package_expired_sessions"] != int64(2) {
		t.Errorf("csv days = %+v", ds.Rows)
	}
	if bs := usecase.Dataset(rep, usecase.SectionBranches, "en"); len(bs.Rows) != 4 || bs.Rows[2]["branch"] != "Head office (no branch)" ||
		bs.Rows[3]["package_open_sessions"] != int64(3) || bs.Rows[3]["package_open_amount"] != "1500.00" {
		t.Errorf("csv branches = %+v", bs.Rows)
	}

	// HTTP: the owner reads and exports; reception has no finance access.
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	reports.RegisterRoutes(mux, handler.New(svc, nil), tokens, mapLoader{owner.Uuid: owner.ID, desk.Uuid: desk.ID}, orgSvc)
	call := func(user db.User, path string) (int, string, http.Header) {
		t.Helper()
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user.Uuid})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, org.Slug)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr.Code, rr.Body.String(), rr.Header()
	}
	code, raw, _ := call(owner, "/v1/tenant/reports/summary?date_from=2030-01-14&date_to=2030-01-15&branch="+xUUID.String()+",head_office")
	var env struct {
		Data model.Report `json:"data"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(raw), &env) != nil || len(env.Data.Branches) != 2 ||
		env.Data.Totals.Money[0].Revenue.Net.String() != "900.00" || env.Data.Totals.Money[0].Income.String() != "50.00" {
		t.Errorf("owner summary = %d %s", code, raw)
	}
	if code, raw, _ := call(owner, "/v1/tenant/reports/summary?date_from=bad"); code != http.StatusBadRequest {
		t.Errorf("bad date = %d %s", code, raw)
	}
	if code, raw, _ := call(owner, "/v1/tenant/reports/summary?staff=nope"); code != http.StatusBadRequest {
		t.Errorf("bad staff = %d %s", code, raw)
	}
	code, raw, header := call(owner, "/v1/tenant/reports/export?date_from=2030-01-14&date_to=2030-01-15&section=branches")
	if code != http.StatusOK || !strings.HasPrefix(header.Get("Content-Type"), "text/csv") ||
		!strings.Contains(header.Get("Content-Disposition"), "rapor_branches_20300114_20300115.csv") ||
		!strings.Contains(raw, "Ciro (net)") || !strings.Contains(raw, "Yalı") || !strings.Contains(raw, "Paket satış tutarı") {
		t.Errorf("owner export = %d %v %s", code, header, raw)
	}
	if code, raw, _ := call(owner, "/v1/tenant/reports/export?section=bogus"); code != http.StatusBadRequest {
		t.Errorf("bad section = %d %s", code, raw)
	}
	if code, raw, _ := call(desk, "/v1/tenant/reports/summary"); code != http.StatusForbidden {
		t.Errorf("reception summary = %d %s", code, raw)
	}
	if code, raw, _ := call(desk, "/v1/tenant/reports/export"); code != http.StatusForbidden {
		t.Errorf("reception export = %d %s", code, raw)
	}
}
