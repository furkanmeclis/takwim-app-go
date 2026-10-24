package customerpanel_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	cashrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/model"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	salesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/model"
	salesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/repository"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// ledgerKit posts money for the fixture's visits through the sales ledger,
// the way the payments and refunds modules do: a till per business, the
// completion hook that posts sale lines.
type ledgerKit struct {
	f     *fixture
	sales *salesusecase.Service
	tills map[int64]uuid.UUID
}

func (f *fixture) withLedger(t *testing.T) *ledgerKit {
	t.Helper()
	clock := func() time.Time { return f.now }
	cash := cashusecase.New(cashrepository.NewPostgres(f.pool, f.q), nil)
	cash.SetClock(clock)
	sales := salesusecase.New(salesrepository.NewPostgres(f.pool, f.q), cash)
	sales.SetClock(clock)
	f.reservations.SetCompletionHook(sales.RecordReservationTx)
	k := &ledgerKit{f: f, sales: sales, tills: map[int64]uuid.UUID{}}
	for _, orgID := range []int64{f.orgA.ID, f.orgB.ID} {
		var id uuid.UUID
		if err := f.pool.QueryRow(context.Background(), `INSERT INTO cash_accounts (organization_id, branch_id, kind, name, currency, opening_date)
			VALUES ($1, NULL, 'cash', 'Kasa', 'TRY', '2026-01-01') RETURNING uuid`, orgID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		k.tills[orgID] = id
	}
	return k
}

// inTx runs fn with queries bound to one transaction, as the payments
// module does.
func (k *ledgerKit) inTx(t *testing.T, fn func(q *db.Queries) error) {
	t.Helper()
	ctx := context.Background()
	tx, err := k.f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(k.f.q.WithTx(tx)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func (k *ledgerKit) collect(t *testing.T, orgID int64, res uuid.UUID, amount, method string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	k.inTx(t, func(q *db.Queries) error {
		var err error
		id, err = k.sales.RecordCollectionTx(context.Background(), q, orgID, salesmodel.CollectionInput{
			ReservationUUID: &res, CashAccountUUID: k.tills[orgID], Amount: money.MustParse(amount), Method: method,
			Description: "kasa notu", SourceType: "reservation", SourceUUID: &res,
		})
		return err
	})
	return id
}

func (k *ledgerKit) refund(t *testing.T, orgID int64, collection uuid.UUID, amount string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	k.inTx(t, func(q *db.Queries) error {
		var err error
		id, err = k.sales.RecordRefundTx(context.Background(), q, orgID, salesmodel.RefundInput{
			CollectionUUID: collection, CashAccountUUID: k.tills[orgID], Amount: money.MustParse(amount), Method: "cash",
			Description: "iade notu", SourceType: "refund",
		})
		return err
	})
	return id
}

// TestCustomerPanelLedgerMoney (TEC-967): the panel's amounts come from the
// business's payment ledger: partial payments one by one, refunds and
// reversals netted, what is still owed and what the business holds to pay
// back; the summary adds the same figures per currency.
func TestCustomerPanelLedgerMoney(t *testing.T) {
	f := newFixture(t)
	f.now = at(7, 0)
	k := f.withLedger(t)
	ctx := context.Background()
	none := availabilityusecase.Caller{}

	// v1: paid in two parts, completed in between; the first part is
	// reversed by the business (posted by mistake) and taken again.
	v1 := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, at(9, 0))
	first := k.collect(t, f.orgA.ID, v1.UUID, "40.00", "cash")

	tok := f.signIn(t, national(f.huseyinPhone), f.huseyinPhone).AccessToken
	type page struct {
		Items []model.ReservationItem `json:"items"`
		Total int64                   `json:"total"`
	}
	moneyOf := func(id uuid.UUID) model.Money {
		t.Helper()
		pays := decode[page](t, f.do(t, http.MethodGet, "/v1/customer/payments", tok, nil))
		for _, it := range pays.Items {
			if it.UUID == id {
				return it.Money
			}
		}
		t.Fatalf("%s not in payments", id)
		return model.Money{}
	}
	want := func(name string, m model.Money, expected, paid, refunded, pending, balance string) {
		t.Helper()
		if m.Expected.String() != expected || m.Paid.String() != paid || m.Refunded.String() != refunded ||
			m.RefundPending.String() != pending || m.Balance.String() != balance {
			t.Fatalf("%s money = %+v", name, m)
		}
	}
	want("v1 part paid", moneyOf(v1.UUID), "100.00", "40.00", "0.00", "0.00", "60.00")

	if _, err := f.reservations.Complete(ctx, f.orgA.ID, v1.UUID, none, f.owner.ID, reservationsusecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	k.collect(t, f.orgA.ID, v1.UUID, "60.00", "card")
	want("v1 paid", moneyOf(v1.UUID), "100.00", "100.00", "0.00", "0.00", "0.00")
	k.inTx(t, func(q *db.Queries) error {
		_, err := k.sales.ReverseEntryTx(ctx, q, f.orgA.ID, first, "yanlış kayıt", &f.owner.ID)
		return err
	})
	want("v1 reversed", moneyOf(v1.UUID), "100.00", "60.00", "0.00", "0.00", "40.00")

	// v2: paid in full, cancelled, partly refunded: the rest is owed back.
	v2 := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, at(10, 0))
	paid := k.collect(t, f.orgA.ID, v2.UUID, "100.00", "card")
	if _, err := f.reservations.Cancel(ctx, f.orgA.ID, v2.UUID, none, f.owner.ID, reservationsusecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	k.refund(t, f.orgA.ID, paid, "30.00")
	m2 := moneyOf(v2.UUID)
	want("v2", m2, "0.00", "100.00", "30.00", "70.00", "0.00")
	if m2.PaymentStatus != reservationsmodel.PaymentRefundPending {
		t.Fatalf("v2 payment status = %s", m2.PaymentStatus)
	}

	// v3: cancelled before any money: not a payments row.
	v3 := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.huseyinA, at(11, 0))
	if _, err := f.reservations.Cancel(ctx, f.orgA.ID, v3.UUID, none, f.owner.ID, reservationsusecase.ChangeInput{}); err != nil {
		t.Fatal(err)
	}
	// Zeynep's paid visit never counts for Hüseyin.
	z := f.book(t, f.orgA.ID, f.branchA, f.cutA, f.ayseA, f.zeynep, at(9, 30))
	k.collect(t, f.orgA.ID, z.UUID, "100.00", "cash")

	pays := decode[page](t, f.do(t, http.MethodGet, "/v1/customer/payments", tok, nil))
	if pays.Total != 2 {
		t.Fatalf("payments = %+v", pays)
	}
	for _, it := range pays.Items {
		if it.UUID == v3.UUID || it.UUID == z.UUID {
			t.Fatalf("%s in payments", it.UUID)
		}
	}

	// Detail: the live payments one by one, no notes or accounts.
	r := f.do(t, http.MethodGet, "/v1/customer/reservations/"+v1.UUID.String(), tok, nil)
	d1 := decode[model.Reservation](t, r)
	if len(d1.Payments) != 1 || d1.Payments[0].Kind != model.PaymentKindPayment || d1.Payments[0].Amount.String() != "60.00" ||
		d1.Payments[0].Method != "card" || d1.Payments[0].Date != "2030-01-07" {
		t.Fatalf("v1 payments = %+v", d1.Payments)
	}
	want("v1 detail", d1.Money, "100.00", "60.00", "0.00", "0.00", "40.00")
	for _, leak := range []string{"kasa notu", "yanlış kayıt", k.tills[f.orgA.ID].String(), first.String()} {
		if strings.Contains(r.raw, leak) {
			t.Fatalf("detail leaks %q: %s", leak, r.raw)
		}
	}
	d2 := decode[model.Reservation](t, f.do(t, http.MethodGet, "/v1/customer/reservations/"+v2.UUID.String(), tok, nil))
	if len(d2.Payments) != 2 || d2.Payments[0].Kind != model.PaymentKindPayment || d2.Payments[1].Kind != model.PaymentKindRefund ||
		d2.Payments[1].Amount.String() != "30.00" {
		t.Fatalf("v2 payments = %+v", d2.Payments)
	}
	want("v2 detail", d2.Money, "0.00", "100.00", "30.00", "70.00", "0.00")

	summary := decode[model.PaymentSummary](t, f.do(t, http.MethodGet, "/v1/customer/payments/summary", tok, nil))
	if len(summary.Totals) != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	s := summary.Totals[0]
	if s.Currency != "TRY" || s.Booked.String() != "100.00" || s.ReservationCount != 1 || s.Paid.String() != "160.00" ||
		s.Refunded.String() != "30.00" || s.RefundPending.String() != "70.00" || s.Balance.String() != "40.00" {
		t.Fatalf("summary = %+v", s)
	}

	// Closed without a refund (kept, TEC-978): the money stays with the
	// business as the customer's credit, not a refund on its way.
	f.exec("UPDATE reservations SET payment_status = 'kept' WHERE organization_id = $1 AND uuid = $2", f.orgA.ID, v2.UUID)
	want("v2 kept", moneyOf(v2.UUID), "0.00", "100.00", "30.00", "0.00", "0.00")
}
