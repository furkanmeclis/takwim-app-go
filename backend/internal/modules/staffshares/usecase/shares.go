package usecase

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/jackc/pgx/v5/pgtype"
)

// Applied is a rule as a share uses it.
type Applied struct {
	RuleID      pgtype.Int8
	Scope       string
	Basis       string
	Rate        money.Rate
	FixedAmount money.Money
}

// ShareOf is the share of a line of quantity units and net amount base: a
// percentage of the base (rounded half up to the kuruş) or a fixed amount
// per unit, never more than the base (the business share cannot go
// negative) and never negative.
func ShareOf(a Applied, base money.Money, quantity int64) money.Money {
	if !base.IsPositive() {
		return money.NewIn(0, base.Currency())
	}
	var share money.Money
	switch a.Basis {
	case model.BasisFixed:
		share = money.NewIn(a.FixedAmount.Minor(), base.Currency()).Mul(max(quantity, 1))
	default:
		share = base.MulRate(a.Rate)
	}
	return share.ClampZero().Min(base)
}

// AllocateBase spreads a package price over its items in proportion to
// their list prices: each part is rounded half up to the kuruş and the
// rounding difference goes to the last part, so the parts add up to the
// package price (requirements §11: 1.000,00 TL over 800 + 400 is 666,67 +
// 333,33). The shares are then calculated per part.
func AllocateBase(total money.Money, listPrices []money.Money) []money.Money {
	out := make([]money.Money, len(listPrices))
	if len(listPrices) == 0 {
		return out
	}
	var sum int64
	for _, p := range listPrices {
		sum += max(p.Minor(), 0)
	}
	if sum == 0 {
		parts, _ := total.Split(len(listPrices))
		return parts
	}
	used := money.NewIn(0, total.Currency())
	for i, p := range listPrices[:len(listPrices)-1] {
		out[i] = total.Ratio(max(p.Minor(), 0), sum)
		used = used.Add(out[i])
	}
	out[len(out)-1] = total.Sub(used)
	return out
}

// RecordLineTx writes the share of a sale line inside the caller's
// transaction (the sales module calls it for every line it writes):
//
//   - a sale line with a staff member gets its share by the rule of its
//     day (a corrected line keeps the rule of the line it replaces); no
//     rule, no staff member or a business owner's own service: no share;
//   - a reversal gets the exact negative of the original's share.
//
// A line is handled once; running it again changes nothing.
func (s *Service) RecordLineTx(ctx context.Context, q *db.Queries, orgID, lineID int64) error {
	line, err := q.GetShareSaleLine(ctx, db.GetShareSaleLineParams{OrganizationID: orgID, ID: lineID})
	if err != nil {
		return err
	}
	if _, err := q.GetShareLineBySaleLine(ctx, db.GetShareLineBySaleLineParams{OrganizationID: orgID, SaleLineID: line.ID}); err == nil {
		return nil
	} else if !isNotFound(err) {
		return err
	}
	if line.Kind != "sale" {
		if !line.ReversesLineID.Valid {
			return nil
		}
		orig, err := q.GetShareLineBySaleLine(ctx, db.GetShareLineBySaleLineParams{OrganizationID: orgID, SaleLineID: line.ReversesLineID.Int64})
		if err != nil {
			if isNotFound(err) {
				return nil
			}
			return err
		}
		_, err = q.InsertShareLine(ctx, db.InsertShareLineParams{
			OrganizationID: orgID, BranchID: line.BranchID, StaffID: orig.StaffID, SaleLineID: line.ID,
			ReservationID: line.ReservationID, Kind: model.LineReversal, ReversesShareID: int8Of(orig.ID),
			ItemType: line.ItemType, RuleID: orig.RuleID, RuleScope: orig.RuleScope, Basis: orig.Basis,
			Rate: orig.Rate, FixedAmount: orig.FixedAmount, BaseAmount: amountOf(orig.BaseAmount).Neg().Numeric(),
			ShareAmount: amountOf(orig.ShareAmount).Neg().Numeric(), Currency: line.Currency, ShareDate: line.SaleDate,
		})
		return mapRepoErr(err)
	}
	if !line.StaffID.Valid {
		return nil
	}
	owner, err := q.IsShareStaffOwner(ctx, db.IsShareStaffOwnerParams{OrganizationID: orgID, StaffID: line.StaffID.Int64})
	if err != nil {
		return err
	}
	if owner {
		return nil
	}
	applied, ok, err := s.ruleFor(ctx, q, orgID, line)
	if err != nil || !ok {
		return err
	}
	base := amountOf(line.NetAmount)
	share := ShareOf(applied, base, int64(line.Quantity))
	in := db.InsertShareLineParams{
		OrganizationID: orgID, BranchID: line.BranchID, StaffID: line.StaffID.Int64, SaleLineID: line.ID,
		ReservationID: line.ReservationID, Kind: model.LineShare, ItemType: line.ItemType, RuleID: applied.RuleID,
		RuleScope: applied.Scope, Basis: applied.Basis, BaseAmount: base.Numeric(), ShareAmount: share.Numeric(),
		Currency: line.Currency, ShareDate: line.SaleDate,
	}
	if applied.Basis == model.BasisFixed {
		in.FixedAmount = applied.FixedAmount.Numeric()
	} else {
		in.Rate = applied.Rate.Numeric()
	}
	_, err = q.InsertShareLine(ctx, in)
	return mapRepoErr(err)
}

// ruleFor finds the rule of a sale line: the snapshot of the replaced
// line for a correction, else the rule of the line's day.
func (s *Service) ruleFor(ctx context.Context, q *db.Queries, orgID int64, line db.GetShareSaleLineRow) (Applied, bool, error) {
	if line.ReplacesLineID.Valid {
		prev, err := q.GetShareLineBySaleLine(ctx, db.GetShareLineBySaleLineParams{OrganizationID: orgID, SaleLineID: line.ReplacesLineID.Int64})
		if err != nil {
			if isNotFound(err) {
				return Applied{}, false, nil
			}
			return Applied{}, false, err
		}
		a := Applied{RuleID: prev.RuleID, Scope: prev.RuleScope, Basis: prev.Basis}
		if r := ratePtr(prev.Rate); r != nil {
			a.Rate = *r
		}
		if f := amountPtr(prev.FixedAmount); f != nil {
			a.FixedAmount = *f
		}
		return a, true, nil
	}
	rule, err := q.ResolveShareRule(ctx, db.ResolveShareRuleParams{
		OrganizationID: orgID, StaffID: line.StaffID.Int64, ItemType: line.ItemType, ServiceID: line.ServiceID,
		OnDate: line.SaleDate,
	})
	if err != nil {
		if isNotFound(err) {
			return Applied{}, false, nil
		}
		return Applied{}, false, err
	}
	a := Applied{RuleID: int8Of(rule.ID), Scope: model.ScopeDefault, Basis: rule.Basis}
	if rule.ServiceID.Valid {
		a.Scope = model.ScopeService
	}
	if r := ratePtr(rule.Rate); r != nil {
		a.Rate = *r
	}
	if f := amountPtr(rule.FixedAmount); f != nil {
		a.FixedAmount = *f
	}
	return a, true, nil
}
