// Package adapters connects the class timetable to the bulk engine:
// cancelling a session with its bookings (TEC-989) and moving the bookings
// of a session to another session of the class (TEC-994) are bulk runs
// over the session's reservations, so they are reported like any bulk run
// and can be rolled back in the bulk window.
//
// The runs are started by the classes module (POST …/sessions/{date}/cancel
// after it marked the session cancelled; POST …/sessions/{date}/move);
// there is no generic bulk route for them. Every item goes through the
// reservation's own cancel or reschedule; the rollback takes a cancelled
// booking back with the reservation's reinstate (opening the session again
// in the same transaction) and reschedules a moved booking back.
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/timetable"
	classesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/usecase"
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// MaxTargets caps one run (a session never has more seats than this).
const MaxTargets = model.MaxCapacity

var (
	errNoOrganization = errors.New("class sessions: organization scope is required")
	errForbidden      = errors.New("class sessions: missing permission")
)

// SessionBulk cancels or moves the bookings of a class session.
type SessionBulk struct {
	classes      *classesusecase.Service
	reservations *reservationsusecase.Service
}

// NewSessionBulk builds the adapter. Both services may be nil for
// metadata only.
func NewSessionBulk(classes *classesusecase.Service, reservations *reservationsusecase.Service) *SessionBulk {
	return &SessionBulk{classes: classes, reservations: reservations}
}

// Resource is the engine slug of class sessions.
func (a *SessionBulk) Resource() string { return model.SessionsSlug }

// BulkActions are the actions of the resource.
func (a *SessionBulk) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{{
		ID: model.CancelBookingsAction, LabelKey: "bulk.actions.class_sessions.cancel_bookings",
		Permission: rbac.PermTenantClassesWrite, Destructive: true, Reversible: true,
	}, {
		ID: model.MoveBookingsAction, LabelKey: "bulk.actions.class_sessions.move_bookings",
		Permission: rbac.PermTenantClassesWrite, Reversible: true,
	}}
}

// known are the actions of the resource.
var known = map[string]bool{model.CancelBookingsAction: true, model.MoveBookingsAction: true}

// reach needs the timetable write permission and every reservation of
// the business (a session's bookings are anyone's).
func reach(ctx context.Context) (int64, int64, error) {
	scope, ok := orgctx.From(ctx)
	if !ok || scope.ID <= 0 {
		return 0, 0, errNoOrganization
	}
	if !scope.Has(rbac.PermTenantClassesWrite) || scope.Reach(rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsOwn) != orgctx.ReachAll {
		return 0, 0, errForbidden
	}
	var actor int64
	if p, ok := authctx.PrincipalFrom(ctx); ok {
		actor = p.UserInternal
	} else if id, ok := orgctx.JobActor(ctx); ok {
		actor = id
	}
	return scope.ID, actor, nil
}

// sessionParams reads the session of the run (params class, date).
func sessionParams(params map[string]string) (uuid.UUID, string, error) {
	id, err := uuid.Parse(strings.TrimSpace(params["class"]))
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("params.class must be a class uuid")
	}
	date := strings.TrimSpace(params["date"])
	if !timetable.ValidDate(date) {
		return uuid.Nil, "", fmt.Errorf("params.date must be a date (yyyy-MM-dd)")
	}
	return id, date, nil
}

// moveParams reads the sessions of a move (params class, date, to_date).
func moveParams(params map[string]string) (uuid.UUID, string, string, error) {
	id, date, err := sessionParams(params)
	if err != nil {
		return uuid.Nil, "", "", err
	}
	to := strings.TrimSpace(params["to_date"])
	if !timetable.ValidDate(to) || to == date {
		return uuid.Nil, "", "", fmt.Errorf("params.to_date must be another date of the class (yyyy-MM-dd)")
	}
	return id, date, to, nil
}

func (a *SessionBulk) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	orgID, _, err := reach(ctx)
	if err != nil {
		return nil, err
	}
	if !known[action] {
		return nil, fmt.Errorf("unknown action")
	}
	classID, date, err := sessionParams(target.Params)
	if err != nil {
		return nil, err
	}
	if _, _, err := a.classes.SessionStart(ctx, orgID, classID, date); err != nil {
		return nil, err
	}
	if action == model.MoveBookingsAction {
		_, _, to, err := moveParams(target.Params)
		if err != nil {
			return nil, err
		}
		if _, _, err := a.classes.SessionStart(ctx, orgID, classID, to); err != nil {
			return nil, err
		}
	}
	if target.Scope != "ids" {
		return nil, fmt.Errorf("invalid target scope")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(target.IDs))
	for _, raw := range target.IDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil || seen[id.String()] {
			continue
		}
		seen[id.String()] = true
		out = append(out, id.String())
	}
	if len(out) > MaxTargets {
		return nil, fmt.Errorf("at most %d targets are allowed", MaxTargets)
	}
	return out, nil
}

// previous is what a cancelled booking needs to be taken back.
type previous struct {
	Class          string                                  `json:"class"`
	Date           string                                  `json:"date"`
	Status         string                                  `json:"status"`
	ApprovalStatus string                                  `json:"approval_status"`
	PaymentStatus  string                                  `json:"payment_status"`
	Version        int                                     `json:"version"`
	Packages       []reservationsmodel.PackageSessionInput `json:"packages,omitempty"`
	Products       []uuid.UUID                             `json:"products,omitempty"`
}

func (p previous) asMap() map[string]any {
	raw, _ := json.Marshal(p)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func (a *SessionBulk) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	orgID, actor, err := reach(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	res := bulkengine.BulkItemResult{EntityUUID: entityUUID, EntityType: "reservation", Op: "update"}
	if action == model.MoveBookingsAction {
		return a.moveItem(ctx, orgID, actor, res)
	}
	if action != model.CancelBookingsAction {
		res.Error = "unknown_action"
		return res, nil
	}
	classID, date, err := sessionParams(bulkengine.Params(ctx))
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	start, branch, err := a.classes.SessionStart(ctx, orgID, classID, date)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		res.Error = "not_found"
		return res, nil
	}
	caller := availabilityusecase.Caller{}
	current, err := a.reservations.Get(ctx, orgID, id, caller)
	if errors.Is(err, reservationsusecase.ErrNotFound) {
		res.Error = "not_found"
		return res, nil
	}
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	res.Label = "#" + strconv.FormatInt(current.Number, 10)
	// Only a booking of this session is cancelled by the run.
	if !current.StartsAt.Equal(start) || current.Branch.UUID != branch {
		res.Error = "not_in_session"
		return res, nil
	}
	prev := previous{
		Class: classID.String(), Date: date, Status: current.Status,
		ApprovalStatus: current.ApprovalStatus, PaymentStatus: current.PaymentStatus,
	}
	for _, ps := range current.PackageSessions {
		if ps.Status == reservationsmodel.SessionHeld {
			prev.Packages = append(prev.Packages, reservationsmodel.PackageSessionInput{
				CustomerPackageUUID: ps.CustomerPackage.UUID, LineIndex: ps.LineIndex,
			})
		}
	}
	for _, p := range current.Products {
		if p.Status == "held" {
			prev.Products = append(prev.Products, p.UUID)
		}
	}
	version := current.Version
	cancelled, err := a.reservations.Cancel(ctx, orgID, id, caller, actor, reservationsusecase.ChangeInput{
		ExpectedVersion: &version, Reason: model.CancelReason,
	})
	var ve *reservationsusecase.ValidationError
	switch {
	case err == nil:
		res.OK = true
		prev.Version = cancelled.Version
		res.Previous = prev.asMap()
	case errors.Is(err, reservationsusecase.ErrInvalidTransition):
		res.Skipped, res.Error = true, "invalid_transition"
	case errors.Is(err, reservationsusecase.ErrStaleVersion):
		res.Skipped, res.Error = true, "stale"
	case errors.Is(err, reservationsusecase.ErrNotFound):
		res.Error = "not_found"
	case errors.As(err, &ve):
		res.Error = ve.Error()
	default:
		return bulkengine.BulkItemResult{}, err
	}
	return res, nil
}

// moved is what a moved booking needs to come back.
type moved struct {
	Class   string `json:"class"`
	Date    string `json:"date"`
	ToDate  string `json:"to_date"`
	Version int    `json:"version"`
}

func (m moved) asMap() map[string]any {
	raw, _ := json.Marshal(m)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

// moveItem moves one open booking of the source session to the target
// session through the reservation's own reschedule: the seat is held in
// the target under the capacity lock, the lines keep their trainer and
// price, package sessions follow where the validity allows and the
// customer gets the rescheduled notice. No seat left, or the trainer or
// area busy, is slot_taken: the booking stays where it was.
func (a *SessionBulk) moveItem(ctx context.Context, orgID, actor int64, res bulkengine.BulkItemResult) (bulkengine.BulkItemResult, error) {
	classID, date, toDate, err := moveParams(bulkengine.Params(ctx))
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	start, branch, err := a.classes.SessionStart(ctx, orgID, classID, date)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	target, _, err := a.classes.SessionStart(ctx, orgID, classID, toDate)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(res.EntityUUID)
	if err != nil {
		res.Error = "not_found"
		return res, nil
	}
	caller := availabilityusecase.Caller{}
	current, err := a.reservations.Get(ctx, orgID, id, caller)
	if errors.Is(err, reservationsusecase.ErrNotFound) {
		res.Error = "not_found"
		return res, nil
	}
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	res.Label = "#" + strconv.FormatInt(current.Number, 10)
	// Only a booking of the source session is moved by the run.
	if !current.StartsAt.Equal(start) || current.Branch.UUID != branch {
		res.Error = "not_in_session"
		return res, nil
	}
	version := current.Version
	updated, err := a.reservations.Reschedule(ctx, orgID, id, caller, actor, reservationsmodel.RescheduleInput{
		StartsAt: &target, ExpectedVersion: &version,
	})
	var ve *reservationsusecase.ValidationError
	switch {
	case err == nil:
		res.OK = true
		res.Previous = moved{Class: classID.String(), Date: date, ToDate: toDate, Version: updated.Version}.asMap()
	case errors.Is(err, availabilityusecase.ErrSlotTaken):
		res.Error = "slot_taken"
	case errors.Is(err, reservationsusecase.ErrInvalidTransition):
		res.Skipped, res.Error = true, "invalid_transition"
	case errors.Is(err, reservationsusecase.ErrStaleVersion):
		res.Skipped, res.Error = true, "stale"
	case errors.Is(err, reservationsusecase.ErrNotFound):
		res.Error = "not_found"
	case errors.As(err, &ve):
		res.Error = ve.Error()
	default:
		return bulkengine.BulkItemResult{}, err
	}
	return res, nil
}

// RevertItem takes one cancelled booking back and opens its session again
// in the same transaction, or moves a moved booking back to its session.
// A booking changed since the run, a time no longer free (a source session
// cancelled meanwhile included), a package without a session left or a
// product out of stock stays as it is (the rollback is then partial).
func (a *SessionBulk) RevertItem(ctx context.Context, action, entityUUID string, raw map[string]any) error {
	orgID, actor, err := reach(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(raw)
	if action == model.MoveBookingsAction {
		var m moved
		if err := json.Unmarshal(b, &m); err != nil {
			return err
		}
		classID, date, _, err := moveParams(map[string]string{"class": m.Class, "date": m.Date, "to_date": m.ToDate})
		if err != nil {
			return err
		}
		back, _, err := a.classes.SessionStart(ctx, orgID, classID, date)
		if err != nil {
			return err
		}
		_, err = a.reservations.Reschedule(ctx, orgID, id, availabilityusecase.Caller{}, actor, reservationsmodel.RescheduleInput{
			StartsAt: &back, ExpectedVersion: &m.Version,
		})
		return err
	}
	if action != model.CancelBookingsAction {
		return fmt.Errorf("unknown action")
	}
	var prev previous
	if err := json.Unmarshal(b, &prev); err != nil {
		return err
	}
	classID, date, err := sessionParams(map[string]string{"class": prev.Class, "date": prev.Date})
	if err != nil {
		return err
	}
	_, err = a.reservations.Reinstate(ctx, orgID, id, availabilityusecase.Caller{}, actor, reservationsusecase.ReinstateInput{
		ExpectedVersion: prev.Version, Status: prev.Status, ApprovalStatus: prev.ApprovalStatus,
		PaymentStatus: prev.PaymentStatus, Packages: prev.Packages, Products: prev.Products,
		Prepare: func(ctx context.Context, q *db.Queries) error {
			return a.classes.ReopenSessionIn(ctx, q, orgID, classID, date, time.Now())
		},
	})
	return err
}

var _ bulkengine.BulkAdapter = (*SessionBulk)(nil)
