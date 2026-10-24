package usecase

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// FollowUpSortColumns are the sortable fields of the follow-up list.
var FollowUpSortColumns = map[string]string{"starts_at": "starts_at", "amount": "amount", "number": "number", "updated_at": "updated_at"}

type followArgs struct {
	branches []int64
	q        pgtype.Text
}

func (s *Service) followArgs(ctx context.Context, orgID int64, branches []uuid.UUID, q string) (followArgs, error) {
	var a followArgs
	if len(branches) > 0 {
		rows, err := s.q.ListSalesBranchesByUUID(ctx, db.ListSalesBranchesByUUIDParams{OrganizationID: orgID, Uuids: branches})
		if err != nil {
			return a, err
		}
		a.branches = []int64{}
		for _, r := range rows {
			a.branches = append(a.branches, r.ID)
		}
	}
	if q = strings.TrimSpace(q); q != "" {
		a.q = pgtype.Text{String: q, Valid: true}
	}
	return a, nil
}

// ListFollowUps is the payment follow-up list of the caller's branch
// scope: visits with a refund to make, a refund that failed or whose
// provider result is unknown, and finished visits not paid in full (A14).
func (s *Service) ListFollowUps(ctx context.Context, orgID int64, f model.FollowUpFilter) ([]model.FollowUp, int64, error) {
	if _, ok := FollowUpSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "starts_at", true
	}
	a, err := s.followArgs(ctx, orgID, f.Branches, f.Q)
	if err != nil {
		return nil, 0, err
	}
	var kinds []string
	if len(f.Kinds) > 0 {
		kinds = f.Kinds
	}
	rows, err := s.q.ListPaymentFollowUps(ctx, db.ListPaymentFollowUpsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), FilterBranchIds: a.branches, Q: a.q, Kinds: kinds,
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	counts, err := s.q.CountPaymentFollowUps(ctx, db.CountPaymentFollowUpsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), FilterBranchIds: a.branches, Q: a.q,
	})
	if err != nil {
		return nil, 0, err
	}
	var total int64
	for _, c := range counts {
		if len(kinds) == 0 || slices.Contains(kinds, c.Kind) {
			total += c.Count
		}
	}
	out := make([]model.FollowUp, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapFollowUp(r))
	}
	return out, total, nil
}

func mapFollowUp(r db.ListPaymentFollowUpsRow) model.FollowUp {
	out := model.FollowUp{
		UUID: r.Uuid, Number: r.Number, Kind: r.Kind, Status: r.Status, PaymentStatus: r.PaymentStatus,
		StartsAt: r.StartsAt.Time, Branch: model.Ref{UUID: r.BranchUuid, Name: r.BranchName}, Currency: r.Currency,
		Charged: amountOf(r.Charged), Collected: amountOf(r.Collected), Refunded: amountOf(r.Refunded), Amount: amountOf(r.Amount),
		Late: r.Late, OpenRefunds: r.OpenRefunds, LastRefundStatus: r.LastRefundStatus, LastRefundFailure: r.LastRefundFailure,
		LastRefundAt: timePtr(r.LastRefundAt), UpdatedAt: r.UpdatedAt.Time,
	}
	if r.CancelReason.Valid {
		v := r.CancelReason.String
		out.CancelReason = &v
	}
	if r.CustomerUuid.Valid {
		out.Customer = &model.Ref{
			UUID: uuid.UUID(r.CustomerUuid.Bytes), Name: strings.TrimSpace(r.CustomerFirstName.String + " " + r.CustomerLastName.String),
		}
	}
	return out
}

// FollowUpFacets counts the follow-up list per kind (branch and search
// filters applied, the kind filter not).
func (s *Service) FollowUpFacets(ctx context.Context, orgID int64, branches []uuid.UUID, q string) (model.FollowUpFacets, error) {
	a, err := s.followArgs(ctx, orgID, branches, q)
	if err != nil {
		return model.FollowUpFacets{}, err
	}
	counts, err := s.q.CountPaymentFollowUps(ctx, db.CountPaymentFollowUpsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), FilterBranchIds: a.branches, Q: a.q,
	})
	if err != nil {
		return model.FollowUpFacets{}, err
	}
	out := model.FollowUpFacets{Kind: []model.FollowUpFacet{}}
	for _, c := range counts {
		out.Kind = append(out.Kind, model.FollowUpFacet{Value: c.Kind, Count: c.Count})
	}
	return out, nil
}
