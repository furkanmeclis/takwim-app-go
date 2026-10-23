// Package usecase writes the reservation record (TEC-899): it books the
// capacity of the visit through the availability holds (TEC-898) and keeps
// the record, its lines and its history in the same transaction, so a
// reservation and its capacity always commit or fail together.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	orgsettings "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// RefType names reservations on capacity holds.
const RefType = "reservation"

// MaxNoteLength limits the notes of a booking.
const MaxNoteLength = 2000

var (
	// ErrNotFound: missing, of another business, outside the caller's branch
	// scope or (own calendar) not the caller's.
	ErrNotFound = errors.New("reservation not found")
	// ErrStaleVersion: the record changed since the caller read it.
	ErrStaleVersion = errors.New("the reservation has changed")
	// ErrInvalidTransition: the action does not apply in the current state.
	ErrInvalidTransition = errors.New("the action is not allowed in the current state")
	// ErrOnlineNotReady: online bookings need the approval mode and a
	// payment method in the business settings.
	ErrOnlineNotReady = errors.New("online booking rules are not configured")
	// ErrOtherStaff: an own calendar member acted on the whole visit (or
	// changed a line) while colleagues serve other lines of it (K-003: a
	// member manages only its own line).
	ErrOtherStaff = errors.New("other staff members serve lines of this visit")
)

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Message string
	Code    string
}

// ValidationError carries field details; handlers answer 400. Errors of
// the slot planner (availability ValidationError) pass through unchanged.
type ValidationError struct {
	Details []Detail
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, Detail{Field: field, Code: code, Message: msg})
}

func (e *ValidationError) orNil() error {
	if len(e.Details) == 0 {
		return nil
	}
	return e
}

// Service books and moves reservations.
type Service struct {
	holds    *availabilityusecase.Holds
	q        *db.Queries
	settings availabilityusecase.SettingsReader
	indexer  SearchIndexer
}

// New builds the service. The capacity writer must also run ExpireRequests
// as its expiry hook (see server wiring), so lapsed requests are cancelled.
func New(holds *availabilityusecase.Holds, q *db.Queries, settings availabilityusecase.SettingsReader) *Service {
	return &Service{holds: holds, q: q, settings: settings}
}

// plan is how a new booking proceeds, from its source and the settings.
type plan struct {
	approvalMode   string
	approvalStatus string
	payment        string
	method         string
	status         string
	holdReason     string
	confirm        bool
}

func (s *Service) planFor(ctx context.Context, orgID int64, in model.CreateInput, ve *ValidationError) (plan, error) {
	if in.Source == model.SourcePanel {
		if in.Payment != "" && in.Payment != model.MethodOnSite {
			ve.add("payment", "invalid", "panel bookings are paid at the business")
		}
		// Staff booking a customer accept it themselves; the capacity is
		// booked at once (the checkout hold is confirmed in the same
		// transaction).
		return plan{
			approvalMode: model.ApprovalModePanel, approvalStatus: model.ApprovalAccepted,
			payment: model.PaymentOnSite, status: model.StatusActive,
			holdReason: availabilitymodel.ReasonCheckout, confirm: true,
		}, nil
	}
	if s.settings == nil {
		return plan{}, ErrOnlineNotReady
	}
	v, err := s.settings.Current(ctx, orgID)
	if err != nil {
		return plan{}, err
	}
	r, pay := v.Settings.Reservation, v.Settings.Payment
	if r.ApprovalMode == "" || (!pay.PayAtVenue && !pay.PayOnline) {
		return plan{}, ErrOnlineNotReady
	}
	method := in.Payment
	switch method {
	case "":
		method = model.MethodOnSite
		if !pay.PayAtVenue {
			method = model.MethodOnline
		}
	case model.MethodOnSite:
		if !pay.PayAtVenue {
			ve.add("payment", "not_allowed", "the business does not take payment on site")
		}
	case model.MethodOnline:
		if !pay.PayOnline {
			ve.add("payment", "not_allowed", "the business does not take online payment")
		}
	default:
		ve.add("payment", "invalid", "payment must be on_site or online")
	}
	p := plan{payment: model.PaymentOnSite, method: method}
	if method == model.MethodOnline {
		p.payment = model.PaymentOnlinePending
	}
	if r.ApprovalMode == orgsettings.ApprovalStaff {
		// A request: the capacity is held for the review window; online
		// payment is asked for after the business accepts.
		p.approvalMode, p.approvalStatus = model.ApprovalModeStaff, model.ApprovalPending
		p.status, p.holdReason = model.StatusRequested, availabilitymodel.ReasonApproval
		return p, nil
	}
	p.approvalMode, p.approvalStatus = model.ApprovalModeAuto, model.ApprovalAccepted
	if method == model.MethodOnline {
		// Accepted, waiting for the payment within the payment hold time.
		p.status, p.holdReason = model.StatusRequested, availabilitymodel.ReasonPayment
		return p, nil
	}
	p.status, p.holdReason, p.confirm = model.StatusActive, availabilitymodel.ReasonCheckout, true
	return p, nil
}

// Create books a visit: it holds the capacity (re-planned on fresh data
// under the business's lock), writes the record with its lines at today's
// prices and, when nothing is pending, turns the hold into a booking.
func (s *Service) Create(ctx context.Context, orgID int64, in model.CreateInput, caller availabilityusecase.Caller, actorID int64) (model.Reservation, error) {
	ve := &ValidationError{}
	switch in.Source {
	case "":
		in.Source = model.SourcePanel
	case model.SourcePanel, model.SourceOnline, model.SourceEmbed:
	default:
		ve.add("source", "invalid", "source must be panel, online or embed")
	}
	if in.CustomerUUID == uuid.Nil {
		ve.add("customer_uuid", "required", "customer is required")
	}
	if len(in.CustomerNote) > MaxNoteLength {
		ve.add("customer_note", "too_long", "note is too long")
	}
	if len(in.InternalNote) > MaxNoteLength {
		ve.add("internal_note", "too_long", "note is too long")
	}
	if in.CustomFields == nil {
		in.CustomFields = map[string]any{}
	}
	customFields, err := json.Marshal(in.CustomFields)
	if err != nil {
		ve.add("custom_fields", "invalid", "custom fields must be an object")
	}
	p, err := s.planFor(ctx, orgID, in, ve)
	if err != nil {
		return model.Reservation{}, err
	}
	if err := ve.orNil(); err != nil {
		return model.Reservation{}, err
	}
	channel := availabilitymodel.ChannelOnline
	if in.Source == model.SourcePanel {
		channel = availabilitymodel.ChannelPanel
	}
	var id uuid.UUID
	err = s.holds.Run(ctx, orgID, func(tx *availabilityusecase.Tx) error {
		customer, err := tx.Q.GetReservationCustomer(ctx, db.GetReservationCustomerParams{OrganizationID: orgID, Uuid: in.CustomerUUID})
		if errors.Is(err, pgx.ErrNoRows) {
			ve.add("customer_uuid", "not_found", "customer not found")
			return ve
		}
		if err != nil {
			return err
		}
		created, err := s.holds.CreateIn(ctx, tx, availabilitymodel.HoldInput{
			BranchUUID: in.BranchUUID, Lines: in.Lines, Channel: channel, StartsAt: in.StartsAt, Reason: p.holdReason,
		}, caller, actorID)
		if err != nil {
			return err
		}
		lines, total, err := s.priceLines(ctx, tx, created.Slot, p.method)
		if err != nil {
			return err
		}
		number, err := tx.Q.NextReservationNumber(ctx, orgID)
		if err != nil {
			return err
		}
		currency, err := tx.Q.GetReservationCurrency(ctx, orgID)
		if err != nil {
			return err
		}
		row, err := tx.Q.CreateReservation(ctx, db.CreateReservationParams{
			OrganizationID: orgID, Number: number, BranchID: created.Slot.BranchID, CustomerID: customer.ID,
			HoldID: pgtype.Int8{Int64: created.ID, Valid: true}, Status: p.status, ApprovalMode: p.approvalMode,
			ApprovalStatus: p.approvalStatus, ConfirmationStatus: model.ConfirmationNotRequired,
			PaymentStatus: p.payment, Source: in.Source,
			StartsAt: ts(created.Slot.StartsAt), EndsAt: ts(created.Slot.EndsAt), TotalPrice: total.Numeric(),
			Currency: currency, CustomFields: customFields, CustomerNote: in.CustomerNote, InternalNote: in.InternalNote,
			CreatedBy: pgtype.Int8{Int64: actorID, Valid: actorID > 0},
		})
		if err != nil {
			return err
		}
		for _, l := range lines {
			l.OrganizationID, l.ReservationID = orgID, row.ID
			if err := tx.Q.CreateReservationLine(ctx, l); err != nil {
				return err
			}
		}
		if err := event(ctx, tx.Q, row, "created", "", row.Status, actorID, ""); err != nil {
			return err
		}
		if p.confirm {
			if err := s.holds.ConfirmIn(ctx, tx, created.ID, RefType, &row.Uuid); err != nil {
				return err
			}
		}
		id = row.Uuid
		return nil
	})
	if err != nil {
		return model.Reservation{}, err
	}
	s.index(ctx, orgID, id)
	return s.get(ctx, orgID, id)
}

// priceLines snapshots the planned lines: service name, staff, resources,
// times and the price (personal › branch offer › list price). An online
// booking's payment method must be one every service takes (method empty =
// panel, which collects at the counter whatever the service's mode).
func (s *Service) priceLines(ctx context.Context, tx *availabilityusecase.Tx, slot availabilitymodel.Slot, method string) ([]db.CreateReservationLineParams, money.Money, error) {
	ve := &ValidationError{}
	total := money.Zero()
	out := make([]db.CreateReservationLineParams, 0, len(slot.Lines))
	for i, l := range slot.Lines {
		svc, err := tx.Q.GetReservationLineService(ctx, db.GetReservationLineServiceParams{
			OrganizationID: tx.OrgID, BranchID: slot.BranchID, StaffID: l.StaffID, Uuid: l.ServiceUUID,
		})
		if err != nil {
			return nil, total, err
		}
		if method != "" && svc.PaymentMode != "both" && svc.PaymentMode != method {
			ve.add("payment", "not_allowed", "a service of the booking does not take this payment method")
		}
		price := money.FromNumericOrZero(svc.ServicePrice)
		if svc.BranchPrice.Valid {
			price = money.FromNumericOrZero(svc.BranchPrice)
		}
		if svc.StaffPrice.Valid {
			price = money.FromNumericOrZero(svc.StaffPrice)
		}
		total = total.Add(price)
		resources := l.Resources
		if resources == nil {
			resources = []availabilitymodel.ResourceRef{}
		}
		raw, err := json.Marshal(resources)
		if err != nil {
			return nil, total, err
		}
		line := db.CreateReservationLineParams{
			LineIndex: int32(i), ServiceID: pgtype.Int8{Int64: svc.ID, Valid: true}, ServiceName: svc.Name,
			Resources: raw, StartsAt: ts(l.StartsAt), EndsAt: ts(l.EndsAt),
			DurationMinutes:     int32(l.EndsAt.Sub(l.StartsAt) / time.Minute),
			BufferBeforeMinutes: int32(l.StartsAt.Sub(l.HeldFrom) / time.Minute),
			BufferAfterMinutes:  int32(l.HeldUntil.Sub(l.EndsAt) / time.Minute),
			UnitPrice:           price.Numeric(),
		}
		if l.HeldFrom.IsZero() {
			line.BufferBeforeMinutes, line.BufferAfterMinutes = 0, 0
		}
		if l.StaffID != 0 && l.Staff != nil {
			line.StaffID = pgtype.Int8{Int64: l.StaffID, Valid: true}
			line.StaffName = pgtype.Text{String: l.Staff.Name, Valid: true}
		}
		out = append(out, line)
	}
	return out, total, ve.orNil()
}

func event(ctx context.Context, q *db.Queries, row db.Reservation, typ, from, to string, actorID int64, note string) error {
	return q.CreateReservationEvent(ctx, db.CreateReservationEventParams{
		OrganizationID: row.OrganizationID, ReservationID: row.ID, Version: row.Version, Type: typ,
		FromStatus: pgtype.Text{String: from, Valid: from != ""}, ToStatus: pgtype.Text{String: to, Valid: to != ""},
		ActorID: pgtype.Int8{Int64: actorID, Valid: actorID > 0}, Note: note,
	})
}

// Get returns a reservation the caller may see.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID, caller availabilityusecase.Caller) (model.Reservation, error) {
	if _, err := s.visible(ctx, s.q, orgID, id, caller, false); err != nil {
		return model.Reservation{}, err
	}
	return s.get(ctx, orgID, id)
}

// visible applies the branch scope and, for an own calendar caller, keeps
// the reservations the member created or serves.
func (s *Service) visible(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID, caller availabilityusecase.Caller, forUpdate bool) (db.Reservation, error) {
	var row db.Reservation
	var err error
	if forUpdate {
		row, err = q.GetReservationForUpdate(ctx, db.GetReservationForUpdateParams{OrganizationID: orgID, Uuid: id})
	} else {
		row, err = q.GetReservationRecord(ctx, db.GetReservationRecordParams{OrganizationID: orgID, Uuid: id})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, err
	}
	if orgctx.CheckBranch(ctx, row.BranchID) != nil {
		return row, ErrNotFound
	}
	if caller.OwnUserID > 0 && (!row.CreatedBy.Valid || row.CreatedBy.Int64 != caller.OwnUserID) {
		member, err := q.GetAvailabilityStaffByUser(ctx, db.GetAvailabilityStaffByUserParams{OrganizationID: orgID, UserID: pgtype.Int8{Int64: caller.OwnUserID, Valid: true}})
		if errors.Is(err, pgx.ErrNoRows) {
			return row, ErrNotFound
		}
		if err != nil {
			return row, err
		}
		ids, err := q.ListReservationStaffIDs(ctx, db.ListReservationStaffIDsParams{OrganizationID: orgID, ReservationID: row.ID})
		if err != nil {
			return row, err
		}
		for _, sid := range ids {
			if sid == member.ID {
				return row, nil
			}
		}
		return row, ErrNotFound
	}
	return row, nil
}

// ownStaff is the staff record of an own calendar caller (0 = none, or the
// caller is not limited to its own calendar).
func (s *Service) ownStaff(ctx context.Context, q *db.Queries, orgID int64, caller availabilityusecase.Caller) (int64, error) {
	if caller.OwnUserID == 0 {
		return 0, nil
	}
	member, err := q.GetAvailabilityStaffByUser(ctx, db.GetAvailabilityStaffByUserParams{OrganizationID: orgID, UserID: pgtype.Int8{Int64: caller.OwnUserID, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return member.ID, err
}

// sharedWithOthers reports whether an own calendar caller shares the visit
// with colleagues: another person serves one of its lines.
func (s *Service) sharedWithOthers(ctx context.Context, q *db.Queries, orgID int64, row db.Reservation, caller availabilityusecase.Caller) (bool, error) {
	if caller.OwnUserID == 0 {
		return false, nil
	}
	own, err := s.ownStaff(ctx, q, orgID, caller)
	if err != nil {
		return false, err
	}
	ids, err := q.ListReservationStaffIDs(ctx, db.ListReservationStaffIDsParams{OrganizationID: orgID, ReservationID: row.ID})
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if id != own {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) get(ctx context.Context, orgID int64, id uuid.UUID) (model.Reservation, error) {
	row, err := s.q.GetReservation(ctx, db.GetReservationParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Reservation{}, ErrNotFound
	}
	if err != nil {
		return model.Reservation{}, err
	}
	out := model.Reservation{
		UUID: row.Uuid, Number: row.Number, Status: row.Status, ApprovalMode: row.ApprovalMode,
		ApprovalStatus: row.ApprovalStatus, ConfirmationStatus: row.ConfirmationStatus, PaymentStatus: row.PaymentStatus,
		Source: row.Source, Branch: model.Ref{UUID: row.BranchUuid, Name: row.BranchName},
		Customer: model.Ref{UUID: row.CustomerUuid, Name: strings.TrimSpace(row.CustomerFirstName + " " + row.CustomerLastName)},
		StartsAt: row.StartsAt.Time.UTC(), EndsAt: row.EndsAt.Time.UTC(),
		TotalPrice: money.FromNumericOrZero(row.TotalPrice), Currency: row.Currency, CustomFields: map[string]any{},
		CustomerNote: row.CustomerNote, InternalNote: row.InternalNote, Version: int(row.Version),
		Lines: []model.Line{}, Events: []model.Event{},
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if len(row.CustomFields) > 0 {
		if err := json.Unmarshal(row.CustomFields, &out.CustomFields); err != nil {
			return model.Reservation{}, err
		}
	}
	if row.CancelReason.Valid {
		out.CancelReason = &row.CancelReason.String
	}
	if row.HoldExpiresAt.Valid {
		t := row.HoldExpiresAt.Time.UTC()
		out.HoldExpiresAt = &t
	}
	lines, err := s.q.ListReservationLines(ctx, db.ListReservationLinesParams{OrganizationID: orgID, ReservationID: row.ID})
	if err != nil {
		return model.Reservation{}, err
	}
	for _, l := range lines {
		item := model.Line{
			Index: int(l.LineIndex), ServiceName: l.ServiceName, Resources: []availabilitymodel.ResourceRef{},
			StartsAt: l.StartsAt.Time.UTC(), EndsAt: l.EndsAt.Time.UTC(), DurationMinutes: int(l.DurationMinutes),
			BufferBeforeMinutes: int(l.BufferBeforeMinutes), BufferAfterMinutes: int(l.BufferAfterMinutes),
			UnitPrice: money.FromNumericOrZero(l.UnitPrice),
		}
		if l.ServiceUuid.Valid {
			u := uuid.UUID(l.ServiceUuid.Bytes)
			item.ServiceUUID = &u
		}
		if l.StaffName.Valid {
			item.Staff = &model.LineStaff{Name: l.StaffName.String}
			if l.StaffUuid.Valid {
				u := uuid.UUID(l.StaffUuid.Bytes)
				item.Staff.UUID = &u
			}
		}
		if err := json.Unmarshal(l.Resources, &item.Resources); err != nil {
			return model.Reservation{}, err
		}
		out.Lines = append(out.Lines, item)
	}
	events, err := s.q.ListReservationEvents(ctx, db.ListReservationEventsParams{OrganizationID: orgID, ReservationID: row.ID})
	if err != nil {
		return model.Reservation{}, err
	}
	for _, e := range events {
		item := model.Event{Version: int(e.Version), Type: e.Type, Note: e.Note, CreatedAt: e.CreatedAt.Time.UTC()}
		if e.FromStatus.Valid {
			item.FromStatus = &e.FromStatus.String
		}
		if e.ToStatus.Valid {
			item.ToStatus = &e.ToStatus.String
		}
		if e.ActorID.Valid {
			name := e.ActorName
			item.Actor = &name
		}
		out.Events = append(out.Events, item)
	}
	return out, nil
}

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
