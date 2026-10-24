package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// Daily builds the daily summary of the tenant home page (TEC-926) for
// one business day (default today): the caller's own visits and earnings
// when it has a staff record, the visits and money of the whole branch
// scope (the header branch picker narrows it to one branch), and what
// waits for the business. Every figure comes from the same reads as the
// reports, so the home page and the reports never disagree. Sections the
// access does not allow stay nil.
func (s *Service) Daily(ctx context.Context, orgID, userID int64, day *time.Time, access model.DailyAccess) (model.Daily, error) {
	// One day: the range cannot be invalid.
	d, _, err := s.Range(ctx, model.Filter{DateFrom: day})
	if err != nil {
		return model.Daily{}, err
	}
	sc, _ := orgctx.From(ctx)
	out := model.Daily{Date: d.Format(dateLayout), Timezone: sc.Timezone, Currency: sc.Currency}
	if out.Timezone == "" {
		out.Timezone = "Europe/Istanbul"
	}
	if out.Currency == "" {
		out.Currency = "TRY"
	}
	dayFilter := model.Filter{DateFrom: &d, DateTo: &d}

	if access.OwnReservations || access.Earnings {
		own, staff, err := s.dailyOwn(ctx, orgID, userID, d, dayFilter, access)
		if err != nil {
			return model.Daily{}, err
		}
		out.Staff, out.Own = staff, own
	}

	if access.BusinessReservations || access.Money {
		rep, err := s.Summary(ctx, orgID, dayFilter)
		if err != nil {
			return model.Daily{}, err
		}
		biz := &model.DailyBusiness{}
		if access.BusinessReservations {
			r, c := rep.Totals.Reservations, rep.Totals.Confirmations
			biz.Reservations, biz.Confirmations = &r, &c
		}
		if access.Money {
			biz.Money = dailyMoney(rep.Totals.Money)
			biz.Branches = make([]model.DailyBranch, 0, len(rep.Branches))
			for _, b := range rep.Branches {
				row := model.DailyBranch{Branch: b.Branch, Money: dailyMoney(b.Money)}
				if access.BusinessReservations {
					r := b.Reservations
					row.Reservations = &r
				}
				biz.Branches = append(biz.Branches, row)
			}
		}
		out.Business = biz

		if access.BusinessReservations {
			approvals, err := s.q.CountPendingApprovals(ctx, db.CountPendingApprovalsParams{
				OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx),
			})
			if err != nil {
				return model.Daily{}, err
			}
			c := rep.Totals.Confirmations
			out.Pending = &model.DailyPending{
				Approvals:              approvals,
				ConfirmationsWaiting:   c.Scheduled + c.Pending,
				ConfirmationsAttention: c.ChangeRequested + c.Flagged + c.Unreachable,
			}
		}
	}
	return out, nil
}

// dailyOwn is the caller's own day; nil (no error) without a staff record.
func (s *Service) dailyOwn(ctx context.Context, orgID, userID int64, d time.Time, f model.Filter, access model.DailyAccess) (*model.DailyOwn, *model.StaffRef, error) {
	st, err := s.q.GetAgendaStaff(ctx, db.GetAgendaStaffParams{OrganizationID: orgID, UserID: pgtype.Int8{Int64: userID, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	f.Staff = []uuid.UUID{st.Uuid}
	rep, err := s.Summary(ctx, orgID, f)
	if err != nil {
		return nil, nil, err
	}
	own := &model.DailyOwn{Visits: []model.DailyVisit{}}
	if access.OwnReservations {
		own.Reservations = rep.Totals.Reservations
		lines, err := s.q.ListDailyStaffLines(ctx, db.ListDailyStaffLinesParams{
			OrganizationID: orgID, StaffID: pgtype.Int8{Int64: st.ID, Valid: true},
			BranchIds: orgctx.BranchScope(ctx), Day: dateArg(d),
		})
		if err != nil {
			return nil, nil, err
		}
		for _, l := range lines {
			own.Visits = append(own.Visits, model.DailyVisit{
				ReservationUUID: l.Uuid, Number: l.Number, LineIndex: l.LineIndex, ServiceName: l.ServiceName,
				StartsAt: l.StartsAt.Time, EndsAt: l.EndsAt.Time, Status: l.Status, ApprovalStatus: l.ApprovalStatus,
				ConfirmationStatus: l.ConfirmationStatus, PaymentStatus: l.PaymentStatus,
				BranchUUID: l.BranchUuid, BranchName: l.BranchName,
				CustomerName: strings.TrimSpace(l.CustomerFirstName + " " + l.CustomerLastName),
			})
		}
	}
	if access.Earnings {
		own.Earnings = make([]model.DailyEarning, 0, len(rep.Totals.Money))
		for _, m := range rep.Totals.Money {
			own.Earnings = append(own.Earnings, model.DailyEarning{
				Currency: m.Currency, Sales: m.Revenue.Net, Share: m.StaffShare, SaleCount: m.Revenue.SaleCount,
			})
		}
	}
	return own, &model.StaffRef{UUID: st.Uuid, Name: st.Name}, nil
}

func dailyMoney(list []model.Money) []model.DailyMoney {
	out := make([]model.DailyMoney, 0, len(list))
	for _, m := range list {
		out = append(out, model.DailyMoney{
			Currency: m.Currency, Revenue: m.Revenue.Net, Collections: m.Collections.Net,
			StaffShare: m.StaffShare, BusinessShare: m.BusinessShare, SaleCount: m.Revenue.SaleCount,
		})
	}
	return out
}
