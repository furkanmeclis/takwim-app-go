package usecase

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availabilitymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	classesmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/timetable"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicbooking/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// The class timetable on the customer screens (TEC-990): the sessions of
// the classes module (TEC-970) with what the slot engine offers of them on
// the online channel, so a session shows as bookable exactly when a
// checkout of it would be accepted.

// ClassSessions lists the sessions of a class timetable
// (classes/usecase.Service implements it).
type ClassSessions interface {
	Sessions(ctx context.Context, orgID int64, f classesmodel.SessionFilter) ([]classesmodel.Session, int, error)
}

// SetClasses plugs in the class timetable (nil: no sessions are listed).
func (s *Service) SetClasses(c ClassSessions) {
	s.classes = c
}

// classDefaultDays is the window of a timetable request without To.
const classDefaultDays = 7

type sessionAt struct {
	service uuid.UUID
	start   int64
}

// Classes lists the sessions of the group classes of a public branch on
// the local dates of the request (default: seven days from today), each
// with its trainer, area, seats and whether the customer can book it with
// party_size seats on the channel.
func (s *Service) Classes(ctx context.Context, o Org, in model.ClassesInput) (model.ClassTimetable, error) {
	channel, err := channelOf(in.Channel)
	if err != nil {
		return model.ClassTimetable{}, err
	}
	if err := s.gate(ctx, o, channel); err != nil {
		return model.ClassTimetable{}, err
	}
	if in.BranchUUID == uuid.Nil {
		return model.ClassTimetable{}, invalid("branch_uuid", "required", "branch is required")
	}
	branches, err := s.q.ListPublicBranches(ctx, o.ID)
	if err != nil {
		return model.ClassTimetable{}, err
	}
	var branch *db.ListPublicBranchesRow
	for i := range branches {
		if branches[i].Uuid == in.BranchUUID {
			branch = &branches[i]
		}
	}
	if branch == nil {
		return model.ClassTimetable{}, invalid("branch_uuid", "not_found", "branch not found")
	}
	tz := branch.Timezone
	if tz == "" {
		tz = o.Timezone
	}
	loc, err := time.LoadLocation(tz)
	if err != nil || tz == "" {
		loc, _ = time.LoadLocation("Europe/Istanbul")
	}
	party := 1
	if in.PartySize != nil {
		party = *in.PartySize
	}
	ve := &ValidationError{}
	if party < classesmodel.MinCapacity || party > classesmodel.MaxCapacity {
		ve.add("party_size", "invalid", fmt.Sprintf("party size must be between %d and %d", classesmodel.MinCapacity, classesmodel.MaxCapacity))
	}
	from, to := classWindow(ve, in.From, in.To, s.now().In(loc).Format(timetable.DateLayout))
	if err := ve.orNil(); err != nil {
		return model.ClassTimetable{}, err
	}
	out := model.ClassTimetable{
		Branch: model.Ref{UUID: branch.Uuid, Name: branch.Name}, Timezone: loc.String(),
		From: from, To: to, PartySize: party, Sessions: []model.ClassSession{},
	}

	services, err := s.Services(ctx, o, &in.BranchUUID)
	if err != nil {
		return model.ClassTimetable{}, err
	}
	var group []uuid.UUID
	for _, sv := range services {
		if sv.GroupCapacity != nil && (in.ServiceUUID == nil || *in.ServiceUUID == sv.UUID) {
			group = append(group, sv.UUID)
		}
	}
	if in.ServiceUUID != nil && len(group) == 0 {
		return model.ClassTimetable{}, invalid("service_uuid", "not_found", "group class not found")
	}
	if len(group) == 0 || s.classes == nil || !o.modules.Allows(sector.ModuleClasses) {
		return out, nil
	}
	sessions, _, err := s.classes.Sessions(ctx, o.ID, classesmodel.SessionFilter{
		From: from, To: to, Branches: []uuid.UUID{in.BranchUUID}, Services: group,
	})
	if err != nil {
		return model.ClassTimetable{}, err
	}
	if len(sessions) == 0 {
		return out, nil
	}

	// What the slot engine offers of each class on the channel.
	offered := map[sessionAt]availabilitymodel.SlotLine{}
	searched := map[uuid.UUID]bool{}
	for _, sess := range sessions {
		if searched[sess.Service.UUID] || sess.State == classesmodel.StateCancelled || sess.State == classesmodel.StatePast {
			continue
		}
		searched[sess.Service.UUID] = true
		res, err := s.availability.Search(ctx, o.ID, availabilitymodel.SearchInput{
			BranchUUID: in.BranchUUID, Lines: []availabilitymodel.LineInput{{ServiceUUID: sess.Service.UUID}},
			From: from, To: to, Channel: availabilitymodel.ChannelOnline, PartySize: &party,
		}, availabilityusecase.Caller{})
		if err != nil {
			return model.ClassTimetable{}, err
		}
		if out.EarliestStart == nil {
			earliest := res.EarliestStart
			out.EarliestStart, out.LatestDate = &earliest, res.LatestDate
		}
		for _, d := range res.Days {
			for _, sl := range d.Slots {
				if len(sl.Lines) > 0 {
					offered[sessionAt{service: sess.Service.UUID, start: sl.StartsAt.UnixNano()}] = sl.Lines[0]
				}
			}
		}
	}

	// The trainer of the sessions that already have bookings.
	booked, err := s.bookedTrainers(ctx, o, branch.ID, group, sessions)
	if err != nil {
		return model.ClassTimetable{}, err
	}

	for _, sess := range sessions {
		key := sessionAt{service: sess.Service.UUID, start: sess.StartsAt.UnixNano()}
		item := model.ClassSession{
			Service: model.Ref{UUID: sess.Service.UUID, Name: sess.Service.Name}, Date: sess.Date,
			StartsAt: sess.StartsAt, EndsAt: sess.EndsAt, Capacity: sess.Capacity,
			CapacityChanged: sess.CapacityChanged, SeatsLeft: sess.SeatsLeft,
		}
		if sess.Resource != nil {
			area := sess.Resource.Name
			item.Area = &area
		}
		line, isOffered := offered[key]
		switch t, ok := booked[key]; {
		case ok:
			item.Trainer = &t
			item.TrainerChanged = sess.Staff != nil && sess.Staff.UUID != t.UUID
		case sess.Staff != nil:
			item.Trainer = &model.Ref{UUID: sess.Staff.UUID, Name: sess.Staff.Name}
		case isOffered && line.Staff != nil:
			item.Trainer = &model.Ref{UUID: line.Staff.UUID, Name: line.Staff.Name}
		}
		if isOffered && line.SeatsLeft != nil {
			item.SeatsLeft = *line.SeatsLeft
		}
		item.State = classState(sess, isOffered, item.SeatsLeft, party, out.EarliestStart, out.LatestDate)
		out.Sessions = append(out.Sessions, item)
	}
	sort.SliceStable(out.Sessions, func(i, j int) bool {
		a, b := out.Sessions[i], out.Sessions[j]
		if !a.StartsAt.Equal(b.StartsAt) {
			return a.StartsAt.Before(b.StartsAt)
		}
		return a.Service.Name < b.Service.Name
	})
	return out, nil
}

// classWindow resolves the local dates of a timetable request.
func classWindow(ve *ValidationError, from, to, today string) (string, string) {
	if from == "" {
		from = today
	} else if !timetable.ValidDate(from) {
		ve.add("from", "invalid", "from must be a date (yyyy-MM-dd)")
		return from, to
	}
	first, _ := time.Parse(timetable.DateLayout, from)
	if to == "" {
		to = first.AddDate(0, 0, classDefaultDays-1).Format(timetable.DateLayout)
	} else if !timetable.ValidDate(to) {
		ve.add("to", "invalid", "to must be a date (yyyy-MM-dd)")
		return from, to
	}
	last, _ := time.Parse(timetable.DateLayout, to)
	switch {
	case last.Before(first):
		ve.add("to", "before_from", "to must not be before from")
	case last.After(first.AddDate(0, 0, availabilityusecase.MaxDays-1)):
		ve.add("to", "too_long", fmt.Sprintf("at most %d days per request", availabilityusecase.MaxDays))
	}
	return from, to
}

// bookedTrainers reads who serves the bookings of the listed sessions.
func (s *Service) bookedTrainers(ctx context.Context, o Org, branchID int64, group []uuid.UUID, sessions []classesmodel.Session) (map[sessionAt]model.Ref, error) {
	out := map[sessionAt]model.Ref{}
	c, err := s.loadCatalog(ctx, o)
	if err != nil {
		return nil, err
	}
	var serviceIDs []int64
	byID := map[int64]uuid.UUID{}
	for id, u := range c.serviceUUID {
		for _, g := range group {
			if g == u {
				serviceIDs = append(serviceIDs, id)
				byID[id] = u
			}
		}
	}
	var since, until time.Time
	for _, sess := range sessions {
		if sess.Booked == 0 {
			continue
		}
		if since.IsZero() || sess.StartsAt.Before(since) {
			since = sess.StartsAt
		}
		if sess.StartsAt.After(until) {
			until = sess.StartsAt
		}
	}
	if len(serviceIDs) == 0 || since.IsZero() {
		return out, nil
	}
	rows, err := s.q.ListPublicClassSessionStaff(ctx, db.ListPublicClassSessionStaffParams{
		OrganizationID: o.ID, BranchID: branchID, ServiceIds: serviceIDs,
		Since: pgtype.Timestamptz{Time: since, Valid: true}, Until: pgtype.Timestamptz{Time: until.Add(time.Second), Valid: true},
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		ref := model.Ref{Name: r.StaffName}
		if r.StaffUuid.Valid {
			ref.UUID = uuid.UUID(r.StaffUuid.Bytes)
		}
		out[sessionAt{service: byID[r.ServiceID], start: r.StartsAt.Time.UnixNano()}] = ref
	}
	return out, nil
}

// classState says why a session can or cannot be booked.
func classState(sess classesmodel.Session, offered bool, seatsLeft, party int, earliest *time.Time, latest *string) string {
	switch {
	case sess.State == classesmodel.StateCancelled:
		return model.ClassCancelled
	case sess.State == classesmodel.StatePast:
		return model.ClassPast
	case offered:
		return model.ClassOpen
	case seatsLeft < party:
		return model.ClassFull
	case earliest != nil && sess.StartsAt.Before(*earliest):
		return model.ClassTooSoon
	case latest != nil && sess.Date > *latest:
		return model.ClassTooFar
	default:
		return model.ClassUnavailable
	}
}
