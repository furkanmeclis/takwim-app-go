package reservations_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	cashrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	bundlemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/model"
	bundleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/servicebundles/usecase"
	servicesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/repository"
	staffsharesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/repository"
	staffsharesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// TestServiceBundleBooking (TEC-895, A16): a bundle of two services with
// list prices 800 and 400 sold at 1000 is booked as two lines served by two
// people. The discount is spread in proportion (666.67 / 333.33, the
// remainder on the last line); each line keeps its own staff member; on
// completion the shares at 40% and 30% add up to 366.67 and the business
// keeps 633.33.
func TestServiceBundleBooking(t *testing.T) {
	f := newProductFixture(t)
	ctx := context.Background()
	scan := func(dst []any, sql string, args ...any) {
		t.Helper()
		if err := f.pool.QueryRow(ctx, sql, args...).Scan(dst...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	var mehmetID, colorID, blowID int64
	var mehmet, color, blow uuid.UUID
	scan([]any{&mehmetID, &mehmet}, `INSERT INTO staff (organization_id, name) VALUES ($1, 'Mehmet') RETURNING id, uuid`, f.org.ID)
	f.exec(`INSERT INTO staff_branches (organization_id, staff_id, branch_id, shifts) VALUES
		($1, $2, $3, '{"mon":[{"start":"09:00","end":"12:00","breaks":[]}]}')`, f.org.ID, mehmetID, f.branchID)
	var ayse uuid.UUID
	scan([]any{&ayse}, `SELECT uuid FROM staff WHERE id = $1`, f.ayseStaffID)
	scan([]any{&colorID, &color}, `INSERT INTO services (organization_id, name, duration_minutes, price) VALUES ($1, 'Boya', 30, 800) RETURNING id, uuid`, f.org.ID)
	scan([]any{&blowID, &blow}, `INSERT INTO services (organization_id, name, duration_minutes, price) VALUES ($1, 'Fön', 30, 400) RETURNING id, uuid`, f.org.ID)
	f.exec(`INSERT INTO service_branches (organization_id, service_id, branch_id) VALUES ($1, $2, $4), ($1, $3, $4)`, f.org.ID, colorID, blowID, f.branchID)
	f.exec(`INSERT INTO service_staff (organization_id, service_id, staff_id) VALUES ($1, $2, $3), ($1, $4, $5)`,
		f.org.ID, colorID, f.ayseStaffID, blowID, mehmetID)
	f.exec(`INSERT INTO staff_share_rules (organization_id, staff_id, item_type, basis, rate, valid_from) VALUES
		($1, $2, 'service', 'percent', 40, '2000-01-01'), ($1, $3, 'service', 'percent', 30, '2000-01-01')`, f.org.ID, f.ayseStaffID, mehmetID)
	cash := cashusecase.New(cashrepository.NewPostgres(f.pool, f.q), nil)
	shares := staffsharesusecase.New(staffsharesrepository.NewPostgres(f.pool, f.q), cash)
	shares.SetClock(func() time.Time { return f.now })
	f.sales.SetLineHook(shares.RecordLineTx)

	bundles := bundleusecase.New(servicesrepository.NewPostgres(f.pool, f.q))
	name, price := "İlkbahar paketi", json.Number("1000")
	spring, err := bundles.Create(ctx, f.org.ID, bundlemodel.Input{
		Name: &name, Price: &price, ServiceUUIDs: []uuid.UUID{color, blow}, BranchUUIDs: []uuid.UUID{f.branch},
	})
	if err != nil {
		t.Fatal(err)
	}
	if spring.DurationMinutes != 60 || !spring.ListPrice.Equal(money.MustParse("1200")) || !spring.Discount.Equal(money.MustParse("200")) ||
		len(spring.Items) != 2 || !spring.Items[0].AllocatedPrice.Equal(money.MustParse("666.67")) ||
		!spring.Items[1].AllocatedPrice.Equal(money.MustParse("333.33")) || !spring.Bookable || len(spring.Branches) != 1 {
		t.Fatalf("bundle = %+v", spring)
	}
	// Contents and branches are checked.
	if _, err := bundles.Create(ctx, f.org.ID, bundlemodel.Input{Name: ptr("Tek"), Price: &price, ServiceUUIDs: []uuid.UUID{color}}); err == nil {
		t.Fatal("one-service bundle accepted")
	}
	if _, err := bundles.Create(ctx, f.org.ID, bundlemodel.Input{Name: &name, Price: &price, ServiceUUIDs: []uuid.UUID{color, blow}}); err == nil {
		t.Fatal("duplicate name accepted")
	}
	f.exec(`UPDATE service_branches SET is_active = false WHERE organization_id = $1 AND service_id = $2`, f.org.ID, blowID)
	if _, err := bundles.Create(ctx, f.org.ID, bundlemodel.Input{Name: ptr("Kapalı"), Price: &price,
		ServiceUUIDs: []uuid.UUID{color, blow}, BranchUUIDs: []uuid.UUID{f.branch}}); err == nil {
		t.Fatal("branch without an offer accepted")
	} else if bve, ok := err.(*bundleusecase.ValidationError); !ok || bve.Details[0].Code != "services_not_offered" {
		t.Fatalf("not offered = %v", err)
	}
	f.exec(`UPDATE service_branches SET is_active = true WHERE organization_id = $1 AND service_id = $2`, f.org.ID, blowID)

	lines := func(staff ...uuid.UUID) []availabilitymodel.LineInput {
		svc := []uuid.UUID{color, blow}
		out := []availabilitymodel.LineInput{}
		for i, s := range staff {
			s := s
			out = append(out, availabilitymodel.LineInput{ServiceUUID: svc[i], StaffUUID: &s})
		}
		return out
	}
	in := f.input(at(9, 0), "", "")
	in.Lines, in.BundleUUID = lines(ayse, mehmet), &spring.UUID
	r, err := f.svc.Create(ctx, f.org.ID, in, none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !r.TotalPrice.Equal(money.MustParse("1000")) || len(r.Lines) != 2 {
		t.Fatalf("booking = total %s, %d lines", r.TotalPrice, len(r.Lines))
	}
	want := []struct{ staff, unit, list string }{{"Ayşe", "666.67", "800"}, {"Mehmet", "333.33", "400"}}
	for i, w := range want {
		l := r.Lines[i]
		if l.Staff == nil || l.Staff.Name != w.staff || !l.UnitPrice.Equal(money.MustParse(w.unit)) || l.ListPrice == nil ||
			!l.ListPrice.Equal(money.MustParse(w.list)) || l.Bundle == nil || l.Bundle.Name != name || l.Bundle.UUID == nil {
			t.Fatalf("line %d = %+v", i, l)
		}
	}
	// The lines run one after the other, each with its own person.
	if !r.Lines[1].StartsAt.Equal(r.Lines[0].EndsAt) {
		t.Fatalf("lines = %v–%v, %v", r.Lines[0].StartsAt, r.Lines[0].EndsAt, r.Lines[1].StartsAt)
	}

	// A move with the same services keeps the split.
	r, err = f.svc.Reschedule(ctx, f.org.ID, r.UUID, none, f.owner.ID, model.RescheduleInput{StartsAt: ptr(at(10, 0))})
	if err != nil {
		t.Fatal(err)
	}
	if !r.TotalPrice.Equal(money.MustParse("1000")) || r.Lines[0].Bundle == nil || !r.Lines[1].UnitPrice.Equal(money.MustParse("333.33")) {
		t.Fatalf("rescheduled = total %s, lines %+v", r.TotalPrice, r.Lines)
	}

	// Completion: sale lines at list price less the bundle discount, and
	// the shares of A16.
	if _, err := f.svc.Complete(ctx, f.org.ID, r.UUID, none, f.owner.ID, usecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	var gross, discount, net string
	scan([]any{&gross, &discount, &net}, `SELECT gross_amount::text, discount_amount::text, net_amount::text FROM sale_lines
		WHERE organization_id = $1 AND line_index = 0 AND source_uuid = $2`, f.org.ID, r.UUID)
	if gross != "800.00" || discount != "133.33" || net != "666.67" {
		t.Fatalf("sale line = %s - %s = %s", gross, discount, net)
	}
	var total, revenue string
	scan([]any{&total}, `SELECT COALESCE(SUM(share_amount), 0)::text FROM staff_share_lines l
		JOIN reservations r ON r.organization_id = l.organization_id AND r.id = l.reservation_id
		WHERE l.organization_id = $1 AND r.uuid = $2`, f.org.ID, r.UUID)
	scan([]any{&revenue}, `SELECT SUM(net_amount)::text FROM sale_lines WHERE organization_id = $1 AND source_uuid = $2`, f.org.ID, r.UUID)
	if total != "366.67" || revenue != "1000.00" {
		t.Fatalf("shares = %s, revenue = %s (want 366.67 of 1000.00)", total, revenue)
	}

	// A package session on a bundle line (TEC-920): the package pays that
	// service, so the visit total keeps only the other shares and the sale
	// line is the session's value without a bundle discount; the other
	// line still sells at list price less its discount.
	var customerID int64
	scan([]any{&customerID}, `SELECT id FROM customers WHERE organization_id = $1 AND uuid = $2`, f.org.ID, f.customer)
	var pkg uuid.UUID
	scan([]any{&pkg}, `INSERT INTO customer_packages (organization_id, customer_id, branch_id, service_id, name, service_name,
		total_sessions, price, currency, sold_on) VALUES ($1, $2, $3, $4, 'Boya 4', 'Boya', 4, 2000, 'TRY', '2029-12-01') RETURNING uuid`,
		f.org.ID, customerID, f.branchID, colorID)
	mixed := f.input(at(9, 0), "", "")
	mixed.Lines, mixed.BundleUUID = lines(ayse, mehmet), &spring.UUID
	r, err = f.svc.Create(ctx, f.org.ID, mixed, none, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	r, err = f.svc.ApplyPackage(ctx, f.org.ID, r.UUID, none, f.owner.ID, model.ApplyPackageInput{CustomerPackageUUID: pkg, LineIndex: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !r.TotalPrice.Equal(money.MustParse("333.33")) || len(r.PackageSessions) != 1 || r.Lines[0].Bundle == nil {
		t.Fatalf("bundle with a session = total %s, sessions %d", r.TotalPrice, len(r.PackageSessions))
	}
	if _, err := f.svc.Complete(ctx, f.org.ID, r.UUID, none, f.owner.ID, usecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	for i, w := range []struct{ gross, discount, net string }{{"500.00", "0.00", "500.00"}, {"400.00", "66.67", "333.33"}} {
		scan([]any{&gross, &discount, &net}, `SELECT gross_amount::text, discount_amount::text, net_amount::text FROM sale_lines
			WHERE organization_id = $1 AND line_index = $2 AND source_uuid = $3`, f.org.ID, i, r.UUID)
		if gross != w.gross || discount != w.discount || net != w.net {
			t.Fatalf("mixed sale line %d = %s - %s = %s", i, gross, discount, net)
		}
	}

	// The lines must be the bundle's services in order.
	bad := f.input(at(11, 0), "", "")
	bad.Lines = []availabilitymodel.LineInput{{ServiceUUID: blow}, {ServiceUUID: color}}
	bad.BundleUUID = &spring.UUID
	if _, err := f.svc.Create(ctx, f.org.ID, bad, none, f.owner.ID); err == nil {
		t.Fatal("lines in another order accepted")
	} else if field, code := detailCode(err); field != "lines" || code != "bundle_mismatch" {
		t.Fatalf("mismatch = %s %s", field, code)
	}
	// Without lines every service takes anyone free.
	free := f.input(at(11, 0), "", "")
	free.BundleUUID = &spring.UUID
	if r, err := f.svc.Create(ctx, f.org.ID, free, none, f.owner.ID); err != nil || len(r.Lines) != 2 || !r.TotalPrice.Equal(money.MustParse("1000")) {
		t.Fatalf("bundle without lines = %+v, %v", r, err)
	}
	// An inactive bundle, one of another business, one not offered.
	off := false
	if _, err := bundles.Patch(ctx, f.org.ID, spring.UUID, bundlemodel.Input{IsActive: &off}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Create(ctx, f.org.ID, free, none, f.owner.ID); err == nil {
		t.Fatal("inactive bundle booked")
	} else if field, code := detailCode(err); field != "bundle_uuid" || code != "inactive" {
		t.Fatalf("inactive = %s %s", field, code)
	}
	if _, err := bundles.Get(ctx, f.org2.ID, spring.UUID); err != bundleusecase.ErrNotFound {
		t.Fatalf("foreign get = %v", err)
	}
	nowhere, err := bundles.Create(ctx, f.org.ID, bundlemodel.Input{Name: ptr("Şubesiz"), Price: &price, ServiceUUIDs: []uuid.UUID{color, blow}})
	if err != nil {
		t.Fatal(err)
	}
	free.BundleUUID = &nowhere.UUID
	if _, err := f.svc.Create(ctx, f.org.ID, free, none, f.owner.ID); err == nil {
		t.Fatal("bundle not offered in the branch booked")
	} else if field, code := detailCode(err); field != "bundle_uuid" || code != "not_offered" {
		t.Fatalf("not offered = %s %s", field, code)
	}
}
