package usecase

import (
	"context"
	"slices"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListRules returns one page of the rules (an own-only viewer sees its own).
func (s *Service) ListRules(ctx context.Context, orgID int64, v Viewer, f model.RuleFilter) ([]model.Rule, int64, error) {
	q := s.repo.Q()
	own, err := s.ownStaff(ctx, q, orgID, v)
	if err != nil {
		if err == ErrNoStaffRecord {
			return []model.Rule{}, 0, nil
		}
		return nil, 0, err
	}
	staff, err := s.staffIDs(ctx, q, orgID, f.Staff, own)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.ListShareRules(ctx, db.ListShareRulesParams{
		OrganizationID: orgID, StaffIds: staff, ItemTypes: nilIfEmpty(f.ItemTypes), LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountShareRules(ctx, db.CountShareRulesParams{OrganizationID: orgID, StaffIds: staff, ItemTypes: nilIfEmpty(f.ItemTypes)})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Rule, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapRule(db.GetShareRuleRow(r)))
	}
	return out, total, nil
}

// GetRule returns one rule (an own-only viewer only its own).
func (s *Service) GetRule(ctx context.Context, orgID int64, v Viewer, id uuid.UUID) (model.Rule, error) {
	q := s.repo.Q()
	row, err := q.GetShareRule(ctx, db.GetShareRuleParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return model.Rule{}, ErrRuleNotFound
		}
		return model.Rule{}, err
	}
	own, err := s.ownStaff(ctx, q, orgID, v)
	if err != nil || (own != 0 && own != row.StaffID) {
		return model.Rule{}, ErrRuleNotFound
	}
	return mapRule(row), nil
}

// ruleFields are the editable values of a rule.
type ruleFields struct {
	basis     string
	rate      *money.Rate
	fixed     *money.Money
	validFrom pgtype.Date
	note      string
}

func (s *Service) applyFields(ctx context.Context, ve *ValidationError, f *ruleFields, in model.RuleInput, create bool) {
	if in.Basis != nil {
		f.basis = *in.Basis
		// A new basis drops the value of the other one.
		f.rate, f.fixed = nil, nil
	}
	if in.Rate != nil {
		r := *in.Rate
		f.rate = &r
	}
	if in.FixedAmount != nil {
		m := *in.FixedAmount
		f.fixed = &m
	}
	switch f.basis {
	case model.BasisPercent:
		if f.rate == nil {
			ve.add("rate", "required", "rate is required for a percentage rule")
		} else if *f.rate < 0 || *f.rate > money.Percent(100) {
			ve.add("rate", "invalid", "rate must be between 0 and 100")
		}
		if in.FixedAmount != nil {
			ve.add("fixed_amount", "invalid", "fixed_amount is for a fixed rule")
		}
		f.fixed = nil
	case model.BasisFixed:
		if f.fixed == nil {
			ve.add("fixed_amount", "required", "fixed_amount is required for a fixed rule")
		} else if f.fixed.IsNegative() || f.fixed.Minor() > money.MaxMinor {
			ve.add("fixed_amount", "invalid", "fixed_amount must not be negative")
		}
		if in.Rate != nil {
			ve.add("rate", "invalid", "rate is for a percentage rule")
		}
		f.rate = nil
	default:
		ve.add("basis", "invalid", "basis must be percent or fixed")
	}
	switch {
	case in.ValidFrom != nil:
		d, err := parseDate(*in.ValidFrom)
		if err != nil {
			ve.add("valid_from", "invalid", "valid_from must be YYYY-MM-DD")
		} else {
			f.validFrom = dateArg(d)
		}
	case create:
		f.validFrom = dateArg(s.today(ctx))
	}
	if in.Note != nil {
		f.note = checkNote(ve, "note", *in.Note)
	}
}

// CreateRule adds a rate of a staff member: the default of an item type
// or, with ServiceUUID / ProductUUID, the rate of one service or product.
// Rules are business-wide, so the caller needs access to every branch.
func (s *Service) CreateRule(ctx context.Context, orgID int64, userID *int64, in model.RuleInput) (model.Rule, error) {
	if err := orgctx.RequireAllBranches(ctx); err != nil {
		return model.Rule{}, ErrAllBranches
	}
	q := s.repo.Q()
	ve := &ValidationError{}
	var staffID int64
	if in.StaffUUID == nil {
		ve.add("staff_uuid", "required", "staff_uuid is required")
	} else if st, err := q.GetShareStaffByUUID(ctx, db.GetShareStaffByUUIDParams{OrganizationID: orgID, Uuid: *in.StaffUUID}); err == nil {
		staffID = st.ID
	} else if isNotFound(err) {
		ve.add("staff_uuid", "not_found", "staff member not found")
	} else {
		return model.Rule{}, err
	}
	itemType := ""
	if in.ItemType != nil {
		itemType = *in.ItemType
	}
	if !slices.Contains(model.ItemTypes, itemType) {
		ve.add("item_type", "invalid", "item_type must be service or product")
	}
	var serviceID pgtype.Int8
	if in.ServiceUUID != nil {
		if itemType != model.ItemService {
			ve.add("service_uuid", "invalid", "a service rate needs item_type service")
		} else if sv, err := q.GetShareServiceByUUID(ctx, db.GetShareServiceByUUIDParams{OrganizationID: orgID, Uuid: *in.ServiceUUID}); err == nil {
			serviceID = int8Of(sv.ID)
		} else if isNotFound(err) {
			ve.add("service_uuid", "not_found", "service not found")
		} else {
			return model.Rule{}, err
		}
	}
	var productID pgtype.Int8
	if in.ProductUUID != nil {
		if itemType != model.ItemProduct {
			ve.add("product_uuid", "invalid", "a product rate needs item_type product")
		} else if p, err := q.GetShareProductByUUID(ctx, db.GetShareProductByUUIDParams{OrganizationID: orgID, Uuid: *in.ProductUUID}); err == nil {
			productID = int8Of(p.ID)
		} else if isNotFound(err) {
			ve.add("product_uuid", "not_found", "product not found")
		} else {
			return model.Rule{}, err
		}
	}
	f := ruleFields{}
	if in.Basis == nil {
		ve.add("basis", "required", "basis is required")
	} else {
		s.applyFields(ctx, ve, &f, in, true)
	}
	if err := ve.orNil(); err != nil {
		return model.Rule{}, err
	}
	row, err := q.InsertShareRule(ctx, db.InsertShareRuleParams{
		OrganizationID: orgID, StaffID: staffID, ItemType: itemType, ServiceID: serviceID, ProductID: productID,
		Basis: f.basis, Rate: rateArg(f.rate), FixedAmount: moneyArg(f.fixed), ValidFrom: f.validFrom, Note: f.note,
		CreatedBy: int8Ptr(userID),
	})
	if err != nil {
		return model.Rule{}, mapRepoErr(err)
	}
	return s.GetRule(ctx, orgID, Viewer{}, row.Uuid)
}

// PatchRule changes the basis, rate, start day or note of a rule. Shares
// calculated before keep their values.
func (s *Service) PatchRule(ctx context.Context, orgID int64, id uuid.UUID, in model.RuleInput) (model.Rule, model.Rule, error) {
	if err := orgctx.RequireAllBranches(ctx); err != nil {
		return model.Rule{}, model.Rule{}, ErrAllBranches
	}
	q := s.repo.Q()
	row, err := q.GetShareRule(ctx, db.GetShareRuleParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return model.Rule{}, model.Rule{}, ErrRuleNotFound
		}
		return model.Rule{}, model.Rule{}, err
	}
	before := mapRule(row)
	ve := &ValidationError{}
	if in.StaffUUID != nil {
		ve.add("staff_uuid", "immutable", "the staff member of a rule does not change")
	}
	if in.ItemType != nil {
		ve.add("item_type", "immutable", "the item type of a rule does not change")
	}
	if in.ServiceUUID != nil {
		ve.add("service_uuid", "immutable", "the service of a rule does not change")
	}
	if in.ProductUUID != nil {
		ve.add("product_uuid", "immutable", "the product of a rule does not change")
	}
	f := ruleFields{basis: row.Basis, rate: ratePtr(row.Rate), fixed: amountPtr(row.FixedAmount), validFrom: row.ValidFrom, note: row.Note}
	s.applyFields(ctx, ve, &f, in, false)
	if err := ve.orNil(); err != nil {
		return model.Rule{}, model.Rule{}, err
	}
	if _, err := q.UpdateShareRule(ctx, db.UpdateShareRuleParams{
		OrganizationID: orgID, ID: row.ID, Basis: f.basis, Rate: rateArg(f.rate), FixedAmount: moneyArg(f.fixed),
		ValidFrom: f.validFrom, Note: f.note,
	}); err != nil {
		return model.Rule{}, model.Rule{}, mapRepoErr(err)
	}
	after, err := s.GetRule(ctx, orgID, Viewer{}, id)
	return before, after, err
}

// DeleteRule removes a rule. Shares calculated with it keep their values.
func (s *Service) DeleteRule(ctx context.Context, orgID int64, id uuid.UUID) (model.Rule, error) {
	if err := orgctx.RequireAllBranches(ctx); err != nil {
		return model.Rule{}, ErrAllBranches
	}
	q := s.repo.Q()
	row, err := q.GetShareRule(ctx, db.GetShareRuleParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		if isNotFound(err) {
			return model.Rule{}, ErrRuleNotFound
		}
		return model.Rule{}, err
	}
	n, err := q.DeleteShareRule(ctx, db.DeleteShareRuleParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return model.Rule{}, err
	}
	if n == 0 {
		return model.Rule{}, ErrRuleNotFound
	}
	return mapRule(row), nil
}

func mapRule(r db.GetShareRuleRow) model.Rule {
	out := model.Rule{
		UUID: r.Uuid, Staff: model.Ref{UUID: r.StaffUuid, Name: r.StaffName}, ItemType: r.ItemType, Basis: r.Basis,
		Rate: ratePtr(r.Rate), FixedAmount: amountPtr(r.FixedAmount), ValidFrom: formatDate(r.ValidFrom.Time),
		Note: r.Note, ShareCount: r.ShareCount, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.ServiceUuid.Valid {
		out.Service = &model.Ref{UUID: uuid.UUID(r.ServiceUuid.Bytes), Name: r.ServiceName.String}
	}
	if r.ProductUuid.Valid {
		out.Product = &model.Ref{UUID: uuid.UUID(r.ProductUuid.Bytes), Name: r.ProductName.String}
	}
	return out
}
