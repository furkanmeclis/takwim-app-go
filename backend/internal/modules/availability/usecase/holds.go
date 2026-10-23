package usecase

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/engine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// Hold times used when the business settings leave them empty (minutes).
const (
	DefaultCheckoutMinutes = 10
	DefaultApprovalMinutes = 60
	DefaultPaymentMinutes  = 15
	MaxHoldMinutes         = 1440
)

var (
	// ErrSlotTaken: the start is no longer free (another writer won, or it
	// was never bookable). Nothing was written.
	ErrSlotTaken = errors.New("the time is no longer available")
	// ErrHoldNotFound: the hold is missing, of another business, outside the
	// caller's branch scope or (own calendar) created by someone else.
	ErrHoldNotFound = errors.New("hold not found")
	// ErrHoldExpired: the hold lapsed before it was confirmed or extended;
	// its capacity is free again.
	ErrHoldExpired = errors.New("the hold has expired")
	// ErrNotActive: the commitment was already released or expired.
	ErrNotActive = errors.New("the hold is no longer active")
	// ErrNotHold: the commitment is a confirmed booking, not a timed hold.
	ErrNotHold = errors.New("the commitment is not a timed hold")
)

// AllocationBusy reads the commitments of capacity_allocations: active,
// unexpired holds and bookings of every branch. ExcludeHoldID leaves one
// commitment out (re-planning it).
type AllocationBusy struct {
	Q interface {
		ListBusyCapacityAllocations(ctx context.Context, arg db.ListBusyCapacityAllocationsParams) ([]db.ListBusyCapacityAllocationsRow, error)
	}
	Now           func() time.Time
	ExcludeHoldID int64
}

// Busy implements BusySource.
func (b AllocationBusy) Busy(ctx context.Context, orgID int64, staffIDs, resourceIDs []int64, since, until time.Time) (map[int64][]engine.Interval, map[int64][]engine.Interval, error) {
	now := time.Now
	if b.Now != nil {
		now = b.Now
	}
	if staffIDs == nil {
		staffIDs = []int64{}
	}
	if resourceIDs == nil {
		resourceIDs = []int64{}
	}
	rows, err := b.Q.ListBusyCapacityAllocations(ctx, db.ListBusyCapacityAllocationsParams{
		OrganizationID: orgID, Now: ts(now()), ExcludeHoldID: b.ExcludeHoldID,
		StaffIds: staffIDs, ResourceIds: resourceIDs, Since: ts(since), Until: ts(until),
	})
	if err != nil {
		return nil, nil, err
	}
	staff := map[int64][]engine.Interval{}
	resources := map[int64][]engine.Interval{}
	for _, r := range rows {
		iv := engine.Interval{Start: r.StartsAt.Time, End: r.EndsAt.Time}
		if r.StaffID.Valid {
			staff[r.StaffID.Int64] = append(staff[r.StaffID.Int64], iv)
		}
		if r.ResourceID.Valid {
			resources[r.ResourceID.Int64] = append(resources[r.ResourceID.Int64], iv)
		}
	}
	return staff, resources, nil
}

// Holds writes capacity commitments. Every writer of an organization runs
// in one transaction under the organization's advisory lock: it expires
// lapsed holds, plans with the availability engine on fresh data and writes
// the allocations. The exclusion constraints of capacity_allocations are the
// correctness argument (a concurrent writer that slipped past the lock gets
// 23P01, mapped to ErrSlotTaken); the lock only spares losers the work.
type Holds struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	settings SettingsReader
	now      func() time.Time
	lock     bool
	onExpire ExpiryHook
}

// ExpiryHook runs in the transaction that expires holds (sweep or a
// writer's lazy expiry), with the ids of the holds that just lapsed. The
// reservation record cancels the requests they belonged to.
type ExpiryHook func(ctx context.Context, q *db.Queries, orgID int64, holdIDs []int64, now time.Time) error

// Tx is one capacity transaction of an organization: the queries bound to
// it and the instant it runs at. Run opens it under the lock, after
// expiring lapsed holds.
type Tx struct {
	Q     *db.Queries
	OrgID int64
	Now   time.Time
}

// NewHolds builds the writer.
func NewHolds(pool *pgxpool.Pool, q *db.Queries, settings SettingsReader) *Holds {
	return &Holds{pool: pool, q: q, settings: settings, now: time.Now, lock: true}
}

// SetClock replaces time.Now (tests).
func (h *Holds) SetClock(now func() time.Time) {
	h.now = now
}

// SetLocking turns the advisory lock off (tests prove the constraints
// alone keep one winner).
func (h *Holds) SetLocking(on bool) {
	h.lock = on
}

// SetExpiryHook registers the reaction to lapsed holds.
func (h *Holds) SetExpiryHook(fn ExpiryHook) {
	h.onExpire = fn
}

// Busy is the BusySource of searches outside a writer's transaction.
func (h *Holds) Busy() BusySource {
	return AllocationBusy{Q: h.q, Now: func() time.Time { return h.now() }}
}

// Run opens a capacity transaction of the organization: it takes the
// organization's advisory lock, expires lapsed holds (running the expiry
// hook), calls fn and commits. fn runs in a savepoint: when it fails, what
// it wrote is undone but the expiry is still committed (a lapsed hold reads
// as expired afterwards). An exclusion violation maps to ErrSlotTaken.
// Callers
// that write their own records (reservations) do so inside fn, so the record
// and its capacity commit together.
func (h *Holds) Run(ctx context.Context, orgID int64, fn func(tx *Tx) error) error {
	return mapWriteErr(h.run(ctx, orgID, true, fn))
}

func (h *Holds) run(ctx context.Context, orgID int64, expire bool, fn func(tx *Tx) error) error {
	pgtx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = pgtx.Rollback(ctx) }()
	q := h.q.WithTx(pgtx)
	now := h.now()
	if h.lock {
		if err := q.LockCapacity(ctx, orgID); err != nil {
			return err
		}
	}
	expired := 0
	if expire {
		if expired, err = h.expireDue(ctx, q, orgID, now); err != nil {
			return err
		}
	}
	sp, err := pgtx.Begin(ctx)
	if err != nil {
		return err
	}
	if fnErr := fn(&Tx{Q: h.q.WithTx(sp), OrgID: orgID, Now: now}); fnErr != nil {
		_ = sp.Rollback(ctx)
		if expired > 0 {
			if err := pgtx.Commit(ctx); err != nil {
				return err
			}
		}
		return fnErr
	}
	if err := sp.Commit(ctx); err != nil {
		return err
	}
	return pgtx.Commit(ctx)
}

// expireDue expires the organization's lapsed holds, frees their
// allocations and runs the expiry hook; every writer runs it first.
func (h *Holds) expireDue(ctx context.Context, q *db.Queries, orgID int64, now time.Time) (int, error) {
	ids, err := q.ExpireDueCapacityHolds(ctx, db.ExpireDueCapacityHoldsParams{OrganizationID: orgID, Now: ts(now)})
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	if err := q.DeleteCapacityAllocations(ctx, db.DeleteCapacityAllocationsParams{OrganizationID: orgID, HoldIds: ids}); err != nil {
		return 0, err
	}
	if h.onExpire != nil {
		if err := h.onExpire(ctx, q, orgID, ids, now); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

func (h *Holds) holdMinutes(ctx context.Context, orgID int64, reason string, override *int) (int, error) {
	if override != nil {
		return *override, nil
	}
	defaults := map[string]int{
		model.ReasonCheckout: DefaultCheckoutMinutes,
		model.ReasonApproval: DefaultApprovalMinutes,
		model.ReasonPayment:  DefaultPaymentMinutes,
	}
	if h.settings != nil && reason != model.ReasonCheckout {
		v, err := h.settings.Current(ctx, orgID)
		if err != nil {
			return 0, err
		}
		r := v.Settings.Reservation
		if reason == model.ReasonApproval && r.StaffReviewWindowMinutes != nil && *r.StaffReviewWindowMinutes > 0 {
			return *r.StaffReviewWindowMinutes, nil
		}
		if reason == model.ReasonPayment && r.PaymentHoldMinutes != nil && *r.PaymentHoldMinutes > 0 {
			return *r.PaymentHoldMinutes, nil
		}
	}
	return defaults[reason], nil
}

// piece is one allocation row: a holder's merged held time in a visit.
type piece struct {
	line       int
	staffID    int64
	resourceID int64
	start, end time.Time
}

// pieces merges the held times of each holder of a slot where they touch or
// overlap (a visit never conflicts with itself, see the engine).
func pieces(slot model.Slot) []piece {
	type holder struct{ staff, resource int64 }
	byHolder := map[holder][]piece{}
	var order []holder
	add := func(k holder, p piece) {
		if _, ok := byHolder[k]; !ok {
			order = append(order, k)
		}
		byHolder[k] = append(byHolder[k], p)
	}
	for i, l := range slot.Lines {
		if l.StaffID != 0 {
			add(holder{staff: l.StaffID}, piece{line: i, staffID: l.StaffID, start: l.HeldFrom, end: l.HeldUntil})
		}
		for _, r := range l.ResourceIDs {
			add(holder{resource: r}, piece{line: i, resourceID: r, start: l.HeldFrom, end: l.HeldUntil})
		}
	}
	var out []piece
	for _, k := range order {
		list := byHolder[k]
		sort.SliceStable(list, func(a, b int) bool { return list[a].start.Before(list[b].start) })
		cur := list[0]
		for _, p := range list[1:] {
			if !p.start.After(cur.end) {
				if p.end.After(cur.end) {
					cur.end = p.end
				}
				continue
			}
			out = append(out, cur)
			cur = p
		}
		out = append(out, cur)
	}
	return out
}

// Created is a hold written in a capacity transaction.
type Created struct {
	ID   int64
	UUID uuid.UUID
	// Slot is the plan: lines with their staff, resources and held times.
	Slot model.Slot
}

// Create holds the capacity of one visit at in.StartsAt: a timed hold
// ("opsiyon") until the reason's hold time passes. Nothing is written when
// the start is not free (ErrSlotTaken).
func (h *Holds) Create(ctx context.Context, orgID int64, in model.HoldInput, caller Caller, actorID int64) (model.Hold, error) {
	var created Created
	err := h.Run(ctx, orgID, func(tx *Tx) error {
		var err error
		created, err = h.CreateIn(ctx, tx, in, caller, actorID)
		return err
	})
	if err != nil {
		return model.Hold{}, err
	}
	return h.get(ctx, h.q, orgID, created.UUID)
}

// CreateIn writes a hold inside a capacity transaction: it re-plans the
// start with the slot engine on the transaction's data.
func (h *Holds) CreateIn(ctx context.Context, tx *Tx, in model.HoldInput, caller Caller, actorID int64) (Created, error) {
	ve := &ValidationError{}
	if in.StartsAt == nil {
		ve.add("starts_at", "required", "start is required")
	}
	switch in.Reason {
	case model.ReasonApproval, model.ReasonPayment, model.ReasonCheckout:
	default:
		ve.add("reason", "invalid", "reason must be approval, payment or checkout")
	}
	if in.Minutes != nil && (*in.Minutes < 1 || *in.Minutes > MaxHoldMinutes) {
		ve.add("minutes", "invalid", "minutes must be between 1 and 1440")
	}
	if err := ve.orNil(); err != nil {
		return Created{}, err
	}
	minutes, err := h.holdMinutes(ctx, tx.OrgID, in.Reason, in.Minutes)
	if err != nil {
		return Created{}, err
	}
	channel := in.Channel
	if channel == "" {
		channel = model.ChannelPanel
	}
	now := tx.Now
	planner := New(tx.Q, h.settings)
	planner.SetClock(func() time.Time { return now })
	planner.SetBusySource(AllocationBusy{Q: tx.Q, Now: func() time.Time { return now }})
	search := model.SearchInput{BranchUUID: in.BranchUUID, Lines: in.Lines, Channel: in.Channel, PartySize: in.PartySize, Zone: in.Zone}
	slot, ok, err := planner.Check(ctx, tx.OrgID, search, caller, *in.StartsAt)
	if err != nil {
		return Created{}, err
	}
	if !ok {
		return Created{}, ErrSlotTaken
	}
	// A group class is booked on its own: its line names the session.
	var session model.SlotLine
	if len(slot.Lines) == 1 {
		session = slot.Lines[0]
	}
	row, err := tx.Q.CreateCapacityHold(ctx, db.CreateCapacityHoldParams{
		OrganizationID: tx.OrgID, BranchID: slot.BranchID, Kind: model.KindHold,
		Reason: pgtype.Text{String: in.Reason, Valid: true}, Channel: channel,
		StartsAt: ts(slot.StartsAt), EndsAt: ts(slot.EndsAt),
		ExpiresAt:        ts(now.Add(time.Duration(minutes) * time.Minute)),
		CreatedBy:        pgtype.Int8{Int64: actorID, Valid: actorID > 0},
		SessionServiceID: pgtype.Int8{Int64: session.SessionServiceID, Valid: session.SessionServiceID != 0},
		Seats:            pgtype.Int4{Int32: int32(session.Seats), Valid: session.SessionServiceID != 0},
		SessionSeats:     pgtype.Int4{Int32: int32(session.SessionSeats), Valid: session.SessionServiceID != 0},
	})
	if err != nil {
		return Created{}, err
	}
	for _, p := range pieces(slot) {
		if err := tx.Q.CreateCapacityAllocation(ctx, db.CreateCapacityAllocationParams{
			OrganizationID: tx.OrgID, HoldID: row.ID, LineIndex: int32(p.line),
			StaffID:    pgtype.Int8{Int64: p.staffID, Valid: p.staffID != 0},
			ResourceID: pgtype.Int8{Int64: p.resourceID, Valid: p.resourceID != 0},
			StartsAt:   ts(p.start), EndsAt: ts(p.end),
		}); err != nil {
			return Created{}, err
		}
	}
	return Created{ID: row.ID, UUID: row.Uuid, Slot: slot}, nil
}

// mapWriteErr maps an exclusion violation (a concurrent writer took the
// time) to ErrSlotTaken. Two writers that slipped past the lock with
// overlapping allocations can also wait on each other's exclusion check;
// PostgreSQL then aborts one of them with a deadlock. They competed for the
// same capacity, so the aborted one lost it too.
func mapWriteErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23P01" || pgErr.Code == "40P01") {
		return ErrSlotTaken
	}
	return err
}

// Get returns one commitment the caller may see.
func (h *Holds) Get(ctx context.Context, orgID int64, id uuid.UUID, caller Caller) (model.Hold, error) {
	if _, err := h.visible(ctx, h.q, orgID, id, caller); err != nil {
		return model.Hold{}, err
	}
	return h.get(ctx, h.q, orgID, id)
}

// visible loads a commitment and applies the branch scope and, for an own
// calendar caller, the creator check.
func (h *Holds) visible(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID, caller Caller) (db.GetCapacityHoldRow, error) {
	row, err := q.GetCapacityHold(ctx, db.GetCapacityHoldParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrHoldNotFound
	}
	if err != nil {
		return row, err
	}
	if orgctx.CheckBranch(ctx, row.BranchID) != nil {
		return row, ErrHoldNotFound
	}
	if caller.OwnUserID > 0 && (!row.CreatedBy.Valid || row.CreatedBy.Int64 != caller.OwnUserID) {
		return row, ErrHoldNotFound
	}
	return row, nil
}

func (h *Holds) get(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (model.Hold, error) {
	row, err := q.GetCapacityHold(ctx, db.GetCapacityHoldParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Hold{}, ErrHoldNotFound
	}
	if err != nil {
		return model.Hold{}, err
	}
	out := model.Hold{
		UUID: row.Uuid, Kind: row.Kind, Status: row.Status, Channel: row.Channel,
		Branch:   model.Ref{UUID: row.BranchUuid, Name: row.BranchName},
		StartsAt: row.StartsAt.Time.UTC(), EndsAt: row.EndsAt.Time.UTC(), CreatedAt: row.CreatedAt.Time.UTC(),
		Allocations: []model.Allocation{},
	}
	if row.Reason.Valid {
		out.Reason = &row.Reason.String
	}
	if row.ExpiresAt.Valid {
		t := row.ExpiresAt.Time.UTC()
		out.ExpiresAt = &t
		// A lapsed hold no longer counts, even before the sweep marks it.
		if row.Status == model.StatusActive && !t.After(h.now()) {
			out.Status = model.StatusExpired
		}
	}
	if row.RefType.Valid {
		out.RefType = &row.RefType.String
	}
	if row.RefUuid.Valid {
		u := uuid.UUID(row.RefUuid.Bytes)
		out.RefUUID = &u
	}
	if row.ReleasedAt.Valid {
		t := row.ReleasedAt.Time.UTC()
		out.ReleasedAt = &t
	}
	if row.ReleaseReason.Valid {
		out.ReleaseReason = &row.ReleaseReason.String
	}
	allocs, err := q.ListCapacityHoldAllocations(ctx, db.ListCapacityHoldAllocationsParams{OrganizationID: orgID, HoldID: row.ID})
	if err != nil {
		return model.Hold{}, err
	}
	for _, a := range allocs {
		item := model.Allocation{LineIndex: int(a.LineIndex), StartsAt: a.StartsAt.Time.UTC(), EndsAt: a.EndsAt.Time.UTC()}
		if a.StaffUuid.Valid {
			item.Staff = &model.Ref{UUID: uuid.UUID(a.StaffUuid.Bytes), Name: a.StaffName.String}
		}
		if a.ResourceUuid.Valid {
			item.Resource = &model.ResourceRef{UUID: uuid.UUID(a.ResourceUuid.Bytes), Name: a.ResourceName.String, Kind: a.ResourceKind.String}
		}
		out.Allocations = append(out.Allocations, item)
	}
	return out, nil
}

// activeIn locks a commitment in a capacity transaction and checks that it
// is active (and a timed hold when needHold). Run has already expired lapsed
// holds, so a lapsed hold reads as expired here.
func (h *Holds) activeIn(ctx context.Context, tx *Tx, holdID int64, needHold bool) (db.CapacityHold, error) {
	row, err := tx.Q.GetCapacityHoldByIDForUpdate(ctx, db.GetCapacityHoldByIDForUpdateParams{OrganizationID: tx.OrgID, ID: holdID})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrHoldNotFound
	}
	if err != nil {
		return row, err
	}
	switch {
	case row.Status == model.StatusExpired:
		return row, ErrHoldExpired
	case row.Status != model.StatusActive:
		return row, ErrNotActive
	case needHold && row.Kind != model.KindHold:
		return row, ErrNotHold
	}
	return row, nil
}

// ConfirmIn turns a hold into a booking inside a capacity transaction: the
// same allocations, no expiry, so the capacity is consumed once.
func (h *Holds) ConfirmIn(ctx context.Context, tx *Tx, holdID int64, refType string, refUUID *uuid.UUID) error {
	row, err := h.activeIn(ctx, tx, holdID, true)
	if err != nil {
		return err
	}
	params := db.ConfirmCapacityHoldParams{OrganizationID: tx.OrgID, ID: row.ID}
	if refType != "" {
		params.RefType = pgtype.Text{String: refType, Valid: true}
	}
	if refUUID != nil {
		params.RefUuid = pgtype.UUID{Bytes: *refUUID, Valid: true}
	}
	_, err = tx.Q.ConfirmCapacityHold(ctx, params)
	return err
}

// RenewIn gives an active hold a new reason and expiry (e.g. a request the
// business accepted now waits for online payment). minutes nil = the
// reason's hold time.
func (h *Holds) RenewIn(ctx context.Context, tx *Tx, holdID int64, reason string, minutes *int) error {
	row, err := h.activeIn(ctx, tx, holdID, true)
	if err != nil {
		return err
	}
	m, err := h.holdMinutes(ctx, tx.OrgID, reason, minutes)
	if err != nil {
		return err
	}
	_, err = tx.Q.RenewCapacityHold(ctx, db.RenewCapacityHoldParams{
		OrganizationID: tx.OrgID, ID: row.ID, Reason: pgtype.Text{String: reason, Valid: true},
		ExpiresAt: ts(tx.Now.Add(time.Duration(m) * time.Minute)),
	})
	return err
}

// ReleaseIn frees a hold or booking exactly once inside a capacity
// transaction (ErrNotActive after).
func (h *Holds) ReleaseIn(ctx context.Context, tx *Tx, holdID int64, reason string) error {
	if reason == "" {
		reason = "released"
	}
	row, err := h.activeIn(ctx, tx, holdID, false)
	if err != nil {
		return err
	}
	if _, err := tx.Q.ReleaseCapacityHold(ctx, db.ReleaseCapacityHoldParams{
		OrganizationID: tx.OrgID, ID: row.ID, Now: ts(tx.Now), ReleaseReason: pgtype.Text{String: reason, Valid: true},
	}); err != nil {
		return err
	}
	return tx.Q.DeleteCapacityAllocations(ctx, db.DeleteCapacityAllocationsParams{OrganizationID: tx.OrgID, HoldIds: []int64{row.ID}})
}

// change runs fn on a commitment the caller may see, in a capacity
// transaction, and returns it afterwards.
func (h *Holds) change(ctx context.Context, orgID int64, id uuid.UUID, caller Caller, fn func(tx *Tx, holdID int64) error) (model.Hold, error) {
	err := h.Run(ctx, orgID, func(tx *Tx) error {
		row, err := h.visible(ctx, tx.Q, orgID, id, caller)
		if err != nil {
			return err
		}
		return fn(tx, row.ID)
	})
	if err != nil {
		return model.Hold{}, err
	}
	return h.get(ctx, h.q, orgID, id)
}

// Confirm turns a hold into a booking. refType / refUUID name the record
// (reservation) when given.
func (h *Holds) Confirm(ctx context.Context, orgID int64, id uuid.UUID, caller Caller, refType string, refUUID *uuid.UUID) (model.Hold, error) {
	return h.change(ctx, orgID, id, caller, func(tx *Tx, holdID int64) error {
		return h.ConfirmIn(ctx, tx, holdID, refType, refUUID)
	})
}

// Extend moves the expiry of an active hold to now + minutes.
func (h *Holds) Extend(ctx context.Context, orgID int64, id uuid.UUID, caller Caller, minutes int) (model.Hold, error) {
	// Another business's uuid is 404 before the body is judged.
	if _, err := h.visible(ctx, h.q, orgID, id, caller); err != nil {
		return model.Hold{}, err
	}
	if minutes < 1 || minutes > MaxHoldMinutes {
		ve := &ValidationError{}
		ve.add("minutes", "invalid", "minutes must be between 1 and 1440")
		return model.Hold{}, ve
	}
	return h.change(ctx, orgID, id, caller, func(tx *Tx, holdID int64) error {
		row, err := h.activeIn(ctx, tx, holdID, true)
		if err != nil {
			return err
		}
		_, err = tx.Q.ExtendCapacityHold(ctx, db.ExtendCapacityHoldParams{
			OrganizationID: tx.OrgID, ID: row.ID, ExpiresAt: ts(tx.Now.Add(time.Duration(minutes) * time.Minute)),
		})
		return err
	})
}

// Release frees a hold or a booking exactly once (ErrNotActive after).
func (h *Holds) Release(ctx context.Context, orgID int64, id uuid.UUID, caller Caller, reason string) (model.Hold, error) {
	return h.change(ctx, orgID, id, caller, func(tx *Tx, holdID int64) error {
		return h.ReleaseIn(ctx, tx, holdID, reason)
	})
}

// ExpireDue is the minute sweep: it expires lapsed holds of every
// organization (each under its lock) and returns how many it expired.
// Lapsed holds stop counting at their expiry even before the sweep.
func (h *Holds) ExpireDue(ctx context.Context) (int, error) {
	orgs, err := h.q.ListDueCapacityHoldOrganizations(ctx, ts(h.now()))
	if err != nil {
		return 0, err
	}
	total := 0
	for _, orgID := range orgs {
		err := h.run(ctx, orgID, false, func(tx *Tx) error {
			n, err := h.expireDue(ctx, tx.Q, orgID, tx.Now)
			total += n
			return err
		})
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
