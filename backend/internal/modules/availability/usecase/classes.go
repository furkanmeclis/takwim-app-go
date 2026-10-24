package usecase

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/engine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/timetable"
)

// The class timetable (TEC-970) in availability: a group class is offered
// only at the sessions of its timetable, and a scheduled session holds its
// trainer and area for every other booking, booked or not.

// ClassQueries are the timetable reads (db.Queries implements them).
type ClassQueries interface {
	ListClassTimetable(ctx context.Context, arg db.ListClassTimetableParams) ([]db.ListClassTimetableRow, error)
	ListClassTimetableBusy(ctx context.Context, arg db.ListClassTimetableBusyParams) ([]db.ListClassTimetableBusyRow, error)
	ListClassSessionChanges(ctx context.Context, arg db.ListClassSessionChangesParams) ([]db.ListClassSessionChangesRow, error)
}

// plannedLine is the timetable of one group line.
type plannedLine struct {
	sessions map[int64]engine.Planned
	// staff and resources the sessions commit (the online channel uses
	// them even when they are not shown online: the class is).
	staff     map[int64]bool
	resources map[int64]bool
	kinds     map[string]bool
}

func pgDate(s string) pgtype.Date {
	t, _ := time.Parse(timetable.DateLayout, s)
	return pgtype.Date{Time: t, Valid: true}
}

func dateString(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format(timetable.DateLayout)
}

type changeKey struct {
	schedule int64
	date     string
}

func (s *Service) sessionChanges(ctx context.Context, orgID int64, ids []int64, from, to string) (map[changeKey]timetable.Change, error) {
	out := map[changeKey]timetable.Change{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.ListClassSessionChanges(ctx, db.ListClassSessionChangesParams{
		OrganizationID: orgID, ScheduleIds: ids, FromDate: pgDate(from), ToDate: pgDate(to),
	})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[changeKey{schedule: r.ScheduleID, date: dateString(r.SessionDate)}] = timetable.Change{
			Cancelled: r.Cancelled, Capacity: int(r.Capacity.Int32),
		}
	}
	return out, nil
}

// planned loads the sessions of a group service in a branch on the local
// dates [first, last]: their seats, trainer and area. Cancelled sessions
// are left out.
func (s *Service) planned(ctx context.Context, orgID, branchID, serviceID int64, seats int, loc *time.Location, first, last time.Time) (plannedLine, error) {
	out := plannedLine{sessions: map[int64]engine.Planned{}, staff: map[int64]bool{}, resources: map[int64]bool{}, kinds: map[string]bool{}}
	from, to := first.Format(timetable.DateLayout), last.Format(timetable.DateLayout)
	rows, err := s.q.ListClassTimetable(ctx, db.ListClassTimetableParams{
		OrganizationID: orgID, FromDate: pgDate(from), ToDate: pgDate(to),
		BranchIds: []int64{branchID}, ServiceIds: []int64{serviceID},
	})
	if err != nil {
		return out, err
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	changes, err := s.sessionChanges(ctx, orgID, ids, from, to)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		entry := timetable.Entry{
			ID: r.ID, Weekday: r.Weekday, Start: r.StartTime, Location: loc,
			ValidFrom: dateString(r.ValidFrom), ValidUntil: dateString(r.ValidUntil),
		}
		for _, occ := range entry.Occurrences(from, to) {
			key := occ.Start.UnixNano()
			if _, taken := out.sessions[key]; taken {
				continue
			}
			var change *timetable.Change
			if c, ok := changes[changeKey{schedule: r.ID, date: occ.Date}]; ok {
				if c.Cancelled {
					continue
				}
				change = &c
			}
			p := engine.Planned{Seats: timetable.Seats(change, int(r.Capacity.Int32), seats)}
			if r.StaffID.Valid {
				p.StaffID = r.StaffID.Int64
				out.staff[p.StaffID] = true
			}
			if r.ResourceID.Valid {
				p.ResourceIDs = []int64{r.ResourceID.Int64}
				out.resources[r.ResourceID.Int64] = true
				if r.ResourceKind.Valid {
					out.kinds[r.ResourceKind.String] = true
				}
			}
			out.sessions[key] = p
		}
	}
	return out, nil
}

// sessionOf names a group session by service and branch.
type sessionOf struct {
	service, branch int64
}

// timetableBusy is the held time of every scheduled session that commits
// one of the staff members or resources on the local dates [first − 1,
// last + 1], by id. Sessions of the lines being planned (own) are left
// out: they are the line's own sessions.
func (s *Service) timetableBusy(ctx context.Context, orgID int64, own map[sessionOf]bool, staffIDs, resourceIDs []int64, first, last time.Time) (map[int64][]engine.Interval, map[int64][]engine.Interval, error) {
	staff, resources := map[int64][]engine.Interval{}, map[int64][]engine.Interval{}
	if len(staffIDs) == 0 && len(resourceIDs) == 0 {
		return staff, resources, nil
	}
	if staffIDs == nil {
		staffIDs = []int64{}
	}
	if resourceIDs == nil {
		resourceIDs = []int64{}
	}
	from := first.AddDate(0, 0, -1).Format(timetable.DateLayout)
	to := last.AddDate(0, 0, 1).Format(timetable.DateLayout)
	rows, err := s.q.ListClassTimetableBusy(ctx, db.ListClassTimetableBusyParams{
		OrganizationID: orgID, FromDate: pgDate(from), ToDate: pgDate(to), StaffIds: staffIDs, ResourceIds: resourceIDs,
	})
	if err != nil {
		return nil, nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		if !own[sessionOf{service: r.ServiceID, branch: r.BranchID}] {
			ids = append(ids, r.ID)
		}
	}
	changes, err := s.sessionChanges(ctx, orgID, ids, from, to)
	if err != nil {
		return nil, nil, err
	}
	for _, r := range rows {
		if own[sessionOf{service: r.ServiceID, branch: r.BranchID}] {
			continue
		}
		loc, err := time.LoadLocation(r.Timezone)
		if err != nil || r.Timezone == "" {
			loc, _ = time.LoadLocation(defaultTimezone)
		}
		entry := timetable.Entry{
			ID: r.ID, Weekday: r.Weekday, Start: r.StartTime, Location: loc,
			ValidFrom: dateString(r.ValidFrom), ValidUntil: dateString(r.ValidUntil),
		}
		for _, occ := range entry.Occurrences(from, to) {
			if c, ok := changes[changeKey{schedule: r.ID, date: occ.Date}]; ok && c.Cancelled {
				continue
			}
			held := engine.Interval{
				Start: occ.Start.Add(-minutes(r.BufferBeforeMinutes)),
				End:   occ.Start.Add(minutes(r.DurationMinutes + r.BufferAfterMinutes)),
			}
			if r.StaffID.Valid {
				staff[r.StaffID.Int64] = append(staff[r.StaffID.Int64], held)
			}
			if r.ResourceID.Valid {
				resources[r.ResourceID.Int64] = append(resources[r.ResourceID.Int64], held)
			}
		}
	}
	return staff, resources, nil
}
