package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/engine"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// Agenda answers the programme of the member userID (its own staff record,
// whatever its permissions): its shifts in every branch of the caller's
// scope, its leave and the lines it serves, for local dates From..To (at
// most a week) in the business time zone.
func (s *Service) Agenda(ctx context.Context, orgID, userID int64, f model.AgendaFilter) (model.Agenda, error) {
	zone := ""
	if scope, ok := orgctx.From(ctx); ok {
		zone = scope.Timezone
	}
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" {
		loc, _ = time.LoadLocation(calendarDefaultZone)
	}
	ve := &ValidationError{}
	now := time.Now().In(loc)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if f.From != "" {
		if from, err = time.Parse(calendarDateLayout, f.From); err != nil {
			ve.add("from", "invalid", "from must be a date (YYYY-MM-DD)")
		}
	}
	to := from
	if f.To != "" {
		if to, err = time.Parse(calendarDateLayout, f.To); err != nil {
			ve.add("to", "invalid", "to must be a date (YYYY-MM-DD)")
		}
	}
	if len(ve.Details) == 0 && (to.Before(from) || to.Sub(from) >= MaxCalendarDays*24*time.Hour) {
		ve.add("to", "out_of_range", fmt.Sprintf("to must be on or after from, at most %d days", MaxCalendarDays))
	}
	if err := ve.orNil(); err != nil {
		return model.Agenda{}, err
	}
	first := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	last := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, loc)
	since, until := first, last.AddDate(0, 0, 1)
	out := model.Agenda{
		Timezone: loc.String(), From: first.Format(calendarDateLayout), To: last.Format(calendarDateLayout),
		Work: []model.AgendaWork{}, Off: []model.Interval{}, Items: []model.AgendaItem{},
	}

	member, err := s.q.GetAgendaStaff(ctx, db.GetAgendaStaffParams{OrganizationID: orgID, UserID: pgtype.Int8{Int64: userID, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Staff = &model.AgendaStaff{UUID: member.Uuid, Name: member.Name, Color: member.Color}
	scope := orgctx.BranchScope(ctx)

	shifts, err := s.q.ListAgendaShifts(ctx, db.ListAgendaShiftsParams{OrganizationID: orgID, StaffID: member.ID, BranchIds: scope})
	if err != nil {
		return out, err
	}
	for _, row := range shifts {
		bloc, err := time.LoadLocation(row.Timezone)
		if err != nil || row.Timezone == "" {
			bloc = loc
		}
		bFirst := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, bloc)
		bLast := time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, bloc)
		hours, err := availabilityusecase.ParseWeekly(row.WorkingHours)
		if err != nil {
			return out, fmt.Errorf("branch hours: %w", err)
		}
		exceptions, err := availabilityusecase.ParseExceptions(row.HolidayExceptions)
		if err != nil {
			return out, fmt.Errorf("branch special days: %w", err)
		}
		weekly, err := availabilityusecase.ParseShifts(row.Shifts)
		if err != nil {
			return out, fmt.Errorf("staff shifts: %w", err)
		}
		open := engine.OpenIntervals(bloc, bFirst, bLast, hours, exceptions)
		for _, iv := range engine.Intersect(engine.WorkIntervals(bloc, bFirst, bLast, weekly), open) {
			out.Work = append(out.Work, model.AgendaWork{
				Branch: model.Ref{UUID: row.Uuid, Name: row.Name}, StartsAt: iv.Start.UTC(), EndsAt: iv.End.UTC(),
			})
		}
	}

	off, err := s.q.ListAvailabilityTimeOff(ctx, db.ListAvailabilityTimeOffParams{OrganizationID: orgID, StaffIds: []int64{member.ID}, Since: ts(since), Until: ts(until)})
	if err != nil {
		return out, err
	}
	for _, o := range off {
		out.Off = append(out.Off, model.Interval{StartsAt: o.StartsAt.Time.UTC(), EndsAt: o.EndsAt.Time.UTC()})
	}

	lines, err := s.q.ListAgendaLines(ctx, db.ListAgendaLinesParams{
		OrganizationID: orgID, StaffID: pgtype.Int8{Int64: member.ID, Valid: true}, BranchIds: scope, Since: ts(since), Until: ts(until), IncludeCancelled: f.IncludeCancelled,
	})
	if err != nil {
		return out, err
	}
	for _, row := range lines {
		item := model.AgendaItem{
			CalendarItem: model.CalendarItem{
				ReservationUUID: row.Uuid, Number: row.Number, LineIndex: int(row.LineIndex),
				Columns: []string{member.Uuid.String()}, BranchUUID: row.BranchUuid, ServiceName: row.ServiceName,
				CustomerName: strings.TrimSpace(row.CustomerFirstName + " " + row.CustomerLastName),
				StartsAt:     row.StartsAt.Time.UTC(), EndsAt: row.EndsAt.Time.UTC(),
				BufferBeforeMinutes: int(row.BufferBeforeMinutes), BufferAfterMinutes: int(row.BufferAfterMinutes),
				Status: row.Status, ApprovalStatus: row.ApprovalStatus, ConfirmationStatus: row.ConfirmationStatus,
				PaymentStatus: row.PaymentStatus, Source: row.Source, Version: int(row.Version),
			},
			BranchName: row.BranchName, CustomerNote: row.CustomerNote,
		}
		if row.StaffName.Valid {
			item.StaffName = &row.StaffName.String
		}
		if row.CustomerPhone.Valid {
			item.CustomerPhone = &row.CustomerPhone.String
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}
