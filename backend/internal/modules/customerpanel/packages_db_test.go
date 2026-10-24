package customerpanel_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
)

// TestCustomerPanelPackages ("Paketlerim", TEC-984): the signed-in person
// sees the packages of every business that linked them, with the remaining
// sessions, the last day and the usage history; never another customer's
// package, and released sessions stay out of the history.
func TestCustomerPanelPackages(t *testing.T) {
	f := newFixture(t)
	f.now = at(7, 0)
	sell := func(orgID int64, customer uuid.UUID, name, status string) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := f.pool.QueryRow(context.Background(), `INSERT INTO customer_packages
			(organization_id, customer_id, branch_id, service_id, name, service_name, total_sessions, price, currency, sold_on, expires_on, status, refunded_on, refunded_at, refund_amount)
			SELECT $1, c.id, b.id, s.id, $3, 'Kesim', 3, 300, 'TRY', DATE '2030-01-01', DATE '2099-12-31', $4::text,
				CASE WHEN $4::text = 'refunded' THEN DATE '2030-01-02' END, CASE WHEN $4::text = 'refunded' THEN NOW() END, CASE WHEN $4::text = 'refunded' THEN 0 END
			FROM customers c, branches b, services s
			WHERE c.organization_id = $1 AND c.uuid = $2 AND b.organization_id = $1 AND s.organization_id = $1 AND s.name = 'Kesim'
			ORDER BY b.id LIMIT 1 RETURNING uuid`, orgID, customer, name, status).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	pkgA := sell(f.orgA.ID, f.huseyinA, "Kesim 3", "active")
	pkgB := sell(f.orgB.ID, f.huseyinB, "Klinik 3", "active")
	refunded := sell(f.orgA.ID, f.huseyinA, "Eski paket", "refunded")
	other := sell(f.orgA.ID, f.zeynep, "Zeynep paketi", "active")

	// One visit on the package, one session given and taken back.
	none := availabilityusecase.Caller{}
	ctx := context.Background()
	visit := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, at(10, 0))
	if _, err := f.reservations.ApplyPackage(ctx, f.orgA.ID, visit.UUID, none, f.owner.ID, reservationsmodel.ApplyPackageInput{CustomerPackageUUID: pkgA}); err != nil {
		t.Fatal(err)
	}
	moved := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, at(11, 0))
	moved, err := f.reservations.ApplyPackage(ctx, f.orgA.ID, moved.UUID, none, f.owner.ID, reservationsmodel.ApplyPackageInput{CustomerPackageUUID: pkgA})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.reservations.RemovePackageSession(ctx, f.orgA.ID, moved.UUID, moved.PackageSessions[0].UUID, none, f.owner.ID, reservationsusecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}

	tok := f.signIn(t, national(f.huseyinPhone), f.huseyinPhone).AccessToken
	type page struct {
		Items []model.Package `json:"items"`
		Total int64           `json:"total"`
	}
	r := f.do(t, http.MethodGet, "/v1/customer/packages", tok, nil)
	list := decode[page](t, r)
	if list.Total != 3 || len(list.Items) != 3 {
		t.Fatalf("packages = %d %s", list.Total, r.raw)
	}
	if strings.Contains(r.raw, other.String()) || strings.Contains(r.raw, "Zeynep") {
		t.Fatalf("another customer's package leaks: %s", r.raw)
	}
	states := map[uuid.UUID]string{}
	for _, p := range list.Items {
		states[p.UUID] = p.State
	}
	if states[pkgA] != model.PackageActive || states[pkgB] != model.PackageActive || states[refunded] != model.PackageRefunded {
		t.Fatalf("states = %v", states)
	}
	if list.Items[2].UUID != refunded {
		t.Fatalf("active packages come first: %+v", list.Items)
	}
	active := decode[page](t, f.do(t, http.MethodGet, "/v1/customer/packages?state=active&business="+f.orgA.UUID.String(), tok, nil))
	if active.Total != 1 || active.Items[0].UUID != pkgA || active.Items[0].Held != 1 || active.Items[0].Remaining != 2 ||
		active.Items[0].Business.UUID != f.orgA.UUID || active.Items[0].Sessions != nil {
		t.Fatalf("active in A = %+v", active)
	}
	for _, q := range []string{"?state=gone", "?business=x", "?page=2"} {
		if r := f.do(t, http.MethodGet, "/v1/customer/packages"+q, tok, nil); r.code != http.StatusBadRequest {
			t.Fatalf("%s = %d", q, r.code)
		}
	}

	// Detail: the usage history without the released session.
	r = f.do(t, http.MethodGet, "/v1/customer/packages/"+pkgA.String(), tok, nil)
	detail := decode[model.Package](t, r)
	if r.code != http.StatusOK || len(detail.Sessions) != 1 || detail.Sessions[0].ReservationUUID != visit.UUID ||
		detail.Sessions[0].Status != reservationsmodel.SessionHeld || detail.Sessions[0].SessionNo != 1 {
		t.Fatalf("detail = %d %s", r.code, r.raw)
	}
	if strings.Contains(r.raw, moved.UUID.String()) || strings.Contains(r.raw, "personel notu") {
		t.Fatalf("detail leaks: %s", r.raw)
	}
	for _, id := range []string{other.String(), uuid.NewString(), "x"} {
		if r := f.do(t, http.MethodGet, "/v1/customer/packages/"+id, tok, nil); r.code != http.StatusNotFound {
			t.Fatalf("package %s = %d", id, r.code)
		}
	}
	if r := f.do(t, http.MethodGet, "/v1/customer/packages", "", nil); r.code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", r.code)
	}
}
