package usecase

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// CSV sections of GET /v1/tenant/reports/export.
const (
	SectionDays     = "days"
	SectionBranches = "branches"
)

// Sections lists the CSV sections.
var Sections = []string{SectionDays, SectionBranches}

// countColumns are the reservation and confirmation counts.
var countColumns = []string{
	"reservations_total", "reservations_requested", "reservations_active", "reservations_completed",
	"reservations_cancelled", "reservations_rejected", "reservations_no_show",
	"reservations_from_panel", "reservations_from_online", "reservations_from_embed",
	"confirmations_total", "confirmations_open", "confirmations_confirmed", "confirmations_declined",
	"confirmations_change_requested", "confirmations_cancelled", "confirmations_flagged", "confirmations_unreachable",
}

// moneyColumns are the figures of one currency.
var moneyColumns = []string{
	"currency", "revenue_gross", "revenue_discount", "revenue_reversed", "revenue_net",
	"revenue_service", "revenue_product", "collections_collected", "collections_refunded",
	"collections_net", "transfers", "staff_share", "staff_payments", "business_share",
	"product_cost", "income", "expenses", "operational_result",
}

// packageColumns are the package sales of the period (TEC-983), per
// currency like the money; packageBalanceColumns the rights open today
// (branch comparison only, like the stock).
var packageColumns = []string{
	"package_sold_count", "package_sold_sessions", "package_sold_amount", "package_used_sessions", "package_used_amount",
	"package_refunded_count", "package_refunded_amount", "package_expired_sessions", "package_expired_amount",
}

var packageBalanceColumns = []string{
	"package_open_packages", "package_open_sessions", "package_open_amount", "package_held_sessions",
}

var stockColumns = []string{
	"stock_received", "stock_sold", "stock_returned", "stock_adjusted", "stock_transferred_in",
	"stock_transferred_out", "stock_on_hand", "stock_reserved", "stock_low_items", "stock_out_items", "stock_value",
}

func columns(keys ...[]string) []ioengine.Column {
	var out []ioengine.Column
	for _, list := range keys {
		for _, k := range list {
			out = append(out, ioengine.Column{Key: k, LabelKey: "report." + k, Type: ioengine.ColumnTypeString})
		}
	}
	return out
}

func countCells(row map[string]any, f model.Figures) {
	r, c := f.Reservations, f.Confirmations
	for k, v := range map[string]int64{
		"reservations_total": r.Total, "reservations_requested": r.Requested, "reservations_active": r.Active + r.InService,
		"reservations_completed": r.Completed, "reservations_cancelled": r.Cancelled, "reservations_rejected": r.Rejected,
		"reservations_no_show": r.NoShow, "reservations_from_panel": r.FromPanel, "reservations_from_online": r.FromOnline,
		"reservations_from_embed": r.FromEmbed,
		"confirmations_total":     c.Total, "confirmations_open": c.Scheduled + c.Pending, "confirmations_confirmed": c.Confirmed,
		"confirmations_declined": c.Declined, "confirmations_change_requested": c.ChangeRequested,
		"confirmations_cancelled": c.Cancelled, "confirmations_flagged": c.Flagged, "confirmations_unreachable": c.Unreachable,
	} {
		row[k] = v
	}
}

func moneyCells(row map[string]any, m model.Money) {
	for k, v := range map[string]string{
		"currency": m.Currency, "revenue_gross": m.Revenue.Gross.String(), "revenue_discount": m.Revenue.Discount.String(),
		"revenue_reversed": m.Revenue.Reversed.String(), "revenue_net": m.Revenue.Net.String(),
		"revenue_service": m.Revenue.ServiceNet.String(), "revenue_product": m.Revenue.ProductNet.String(),
		"collections_collected": m.Collections.Collected.String(), "collections_refunded": m.Collections.Refunded.String(),
		"collections_net": m.Collections.Net.String(), "transfers": m.Transfers.String(),
		"staff_share": m.StaffShare.String(), "staff_payments": m.StaffPayments.String(),
		"business_share": m.BusinessShare.String(), "product_cost": m.ProductCost.String(),
		"income": m.Income.String(), "expenses": m.Expenses.String(), "operational_result": m.OperationalResult.String(),
	} {
		row[k] = v
	}
	p := m.Packages
	for k, v := range map[string]any{
		"package_sold_count": p.Sold.Count, "package_sold_sessions": p.Sold.Sessions, "package_sold_amount": p.Sold.Amount.String(),
		"package_used_sessions": p.Used.Sessions, "package_used_amount": p.Used.Amount.String(),
		"package_refunded_count": p.Refunded.Count, "package_refunded_amount": p.Refunded.Amount.String(),
		"package_expired_sessions": p.Expired.Sessions, "package_expired_amount": p.Expired.Amount.String(),
	} {
		row[k] = v
	}
}

// balanceCells writes the open package rights of the row's currency.
func balanceCells(row map[string]any, currency string, list []model.PackageBalance) {
	pb := model.PackageBalance{OpenAmount: money.Zero()}
	for _, b := range list {
		if b.Currency == currency {
			pb = b
		}
	}
	for k, v := range map[string]any{
		"package_open_packages": pb.Packages, "package_open_sessions": pb.OpenSessions,
		"package_open_amount": pb.OpenAmount.String(), "package_held_sessions": pb.HeldSessions,
	} {
		row[k] = v
	}
}

func stockCells(row map[string]any, s model.Stock) {
	for k, v := range map[string]any{
		"stock_received": s.Received, "stock_sold": s.Sold, "stock_returned": s.Returned, "stock_adjusted": s.Adjusted,
		"stock_transferred_in": s.TransferredIn, "stock_transferred_out": s.TransferredOut, "stock_on_hand": s.OnHand,
		"stock_reserved": s.Reserved, "stock_low_items": s.LowItems, "stock_out_items": s.OutItems, "stock_value": s.Value.String(),
	} {
		row[k] = v
	}
}

// figureRows writes one row per currency; the counts (and stock) only on
// the first one, so a column sum in a spreadsheet stays right.
func figureRows(lead map[string]any, f model.Figures, stock *model.Stock, balance []model.PackageBalance) []map[string]any {
	var out []map[string]any
	for i, m := range f.Money {
		row := map[string]any{}
		for k, v := range lead {
			row[k] = v
		}
		if i == 0 {
			countCells(row, f)
			if stock != nil {
				stockCells(row, *stock)
			}
		}
		moneyCells(row, m)
		if stock != nil {
			balanceCells(row, m.Currency, balance)
		}
		out = append(out, row)
	}
	return out
}

// Dataset is the CSV table of one section of a report: the days (with a
// total row) or the branch comparison.
func Dataset(rep model.Report, section, locale string) ioengine.Dataset {
	loc := i18n.Normalize(locale)
	if section == SectionBranches {
		ds := ioengine.Dataset{Resource: "tenant.reports", Columns: columns([]string{"branch_code", "branch"}, countColumns, stockColumns, moneyColumns, packageColumns, packageBalanceColumns)}
		for _, b := range rep.Branches {
			lead := map[string]any{"branch": i18n.Translate(loc, "report.head_office"), "branch_code": ""}
			if b.Branch != nil {
				lead = map[string]any{"branch": b.Branch.Name, "branch_code": b.Branch.Code}
			}
			stock := b.Stock
			ds.Rows = append(ds.Rows, figureRows(lead, b.Figures, &stock, b.PackageBalance)...)
		}
		stock := rep.Totals.Stock
		ds.Rows = append(ds.Rows, figureRows(map[string]any{"branch": i18n.Translate(loc, "report.total"), "branch_code": ""},
			rep.Totals.Figures, &stock, rep.Totals.PackageBalance)...)
		return ds
	}
	ds := ioengine.Dataset{Resource: "tenant.reports", Columns: columns([]string{"date"}, countColumns, moneyColumns, packageColumns)}
	for _, d := range rep.Days {
		ds.Rows = append(ds.Rows, figureRows(map[string]any{"date": d.Date}, d.Figures, nil, nil)...)
	}
	ds.Rows = append(ds.Rows, figureRows(map[string]any{"date": i18n.Translate(loc, "report.total")}, rep.Totals.Figures, nil, nil)...)
	return ds
}
