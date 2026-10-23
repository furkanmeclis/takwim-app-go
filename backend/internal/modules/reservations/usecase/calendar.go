package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/engine"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// MaxCalendarDays is the longest calendar window (a week).
const MaxCalendarDays = 7

const (
	calendarDateLayout  = "2006-01-02"
	calendarDefaultZone = "Europe/Istanbul"
)

// calendarBranch is a branch of the calendar with its opening instants.
type calendarBranch struct {
	row  db.ListCalendarBranchesRow
	open []engine.Interval
}

// Calendar answers the calendar of local dates From..To: the columns (people
// or resources of one branch, or the branches of the caller's scope), when
// each column works, and the visits placed in their columns. An own
// calendar member sees the visits it created or serves.
func (s *Service) Calendar(ctx context.Context, orgID int64, f model.CalendarFilter, caller availabilityusecase.Caller) (model.Calendar, error) {
	ve := &ValidationError{}
	if f.By == "" {
		f.By = model.CalendarByStaff
	}
	switch f.By {
	case model.CalendarByStaff, model.CalendarByResource:
		if f.BranchUUID == nil {
			ve.add("branch", "required", "a branch is required for this view")
		}
	case model.CalendarByBranch:
	default:
		ve.add("by", "invalid", "by must be staff, resource or branch")
	}
	from, errFrom := time.Parse(calendarDateLayout, f.From)
	if errFrom != nil {
		ve.add("from", "invalid", "from must be a date (YYYY-MM-DD)")
	}
	to := from
	if f.To != "" {
		var err error
		if to, err = time.Parse(calendarDateLayout, f.To); err != nil {
			ve.add("to", "invalid", "to must be a date (YYYY-MM-DD)")
		}
	}
	if errFrom == nil && (to.Before(from) || to.Sub(from) >= MaxCalendarDays*24*time.Hour) {
		ve.add("to", "out_of_range", fmt.Sprintf("to must be on or after from, at most %d days", MaxCalendarDays))
	}
	if err := ve.orNil(); err != nil {
		return model.Calendar{}, err
	}

	params := db.ListCalendarBranchesParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)}
	if f.BranchUUID != nil {
		params.BranchUuid = pgtype.UUID{Bytes: *f.BranchUUID, Valid: true}
	}
	rows, err := s.q.ListCalendarBranches(ctx, params)
	if err != nil {
		return model.Calendar{}, err
	}
	if f.BranchUUID != nil && len(rows) == 0 {
		return model.Calendar{}, availabilityusecase.ErrNotFound
	}

	zone := ""
	if scope, ok := orgctx.From(ctx); ok {
		zone = scope.Timezone
	}
	if f.BranchUUID != nil {
		zone = rows[0].Timezone
	}
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" {
		loc, _ = time.LoadLocation(calendarDefaultZone)
	}
	first := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	last := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, loc)
	since, until := first, last.AddDate(0, 0, 1)

	out := model.Calendar{
		By: f.By, Timezone: loc.String(), From: first.Format(calendarDateLayout), To: last.Format(calendarDateLayout),
		Open: []model.Interval{}, Columns: []model.CalendarColumn{}, Items: []model.CalendarItem{},
	}
	branches := make([]calendarBranch, 0, len(rows))
	branchIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		hours, err := availabilityusecase.ParseWeekly(row.WorkingHours)
		if err != nil {
			return out, fmt.Errorf("branch hours: %w", err)
		}
		exceptions, err := availabilityusecase.ParseExceptions(row.HolidayExceptions)
		if err != nil {
			return out, fmt.Errorf("branch special days: %w", err)
		}
		branches = append(branches, calendarBranch{row: row, open: engine.OpenIntervals(loc, first, last, hours, exceptions)})
		branchIDs = append(branchIDs, row.ID)
	}
	if len(branches) == 0 {
		return out, nil
	}
	if f.BranchUUID != nil {
		out.Branch = &model.Ref{UUID: rows[0].Uuid, Name: rows[0].Name}
		out.Open = intervals(branches[0].open)
	}

	lines := db.ListCalendarLinesParams{
		OrganizationID: orgID, BranchIds: branchIDs, Since: ts(since), Until: ts(until), IncludeCancelled: f.IncludeCancelled,
	}
	ownStaffID := int64(0)
	if caller.OwnUserID > 0 {
		lines.OwnUserID = pgtype.Int8{Int64: caller.OwnUserID, Valid: true}
		member, err := s.q.GetAvailabilityStaffByUser(ctx, db.GetAvailabilityStaffByUserParams{OrganizationID: orgID, UserID: pgtype.Int8{Int64: caller.OwnUserID, Valid: true}})
		switch {
		case err == nil:
			ownStaffID = member.ID
			lines.OwnStaffID = pgtype.Int8{Int64: member.ID, Valid: true}
		case !errors.Is(err, pgx.ErrNoRows):
			return out, err
		}
	}
	lineRows, err := s.q.ListCalendarLines(ctx, lines)
	if err != nil {
		return out, err
	}

	columns := newColumnSet()
	switch f.By {
	case model.CalendarByBranch:
		for _, b := range branches {
			columns.add(model.CalendarColumn{Key: b.row.Uuid.String(), Name: b.row.Name, Kind: model.CalendarByBranch, Work: intervals(b.open)})
			out.Open = append(out.Open, intervals(b.open)...)
		}
	case model.CalendarByStaff:
		if err := s.staffColumns(ctx, orgID, branches[0], loc, first, last, since, until, ownStaffID, caller.OwnUserID > 0, columns); err != nil {
			return out, err
		}
	case model.CalendarByResource:
		if err := s.resourceColumns(ctx, orgID, branches[0], loc, first, last, since, until, columns); err != nil {
			return out, err
		}
	}

	for _, row := range lineRows {
		item := model.CalendarItem{
			ReservationUUID: row.Uuid, Number: row.Number, LineIndex: int(row.LineIndex), BranchUUID: row.BranchUuid,
			ServiceName: row.ServiceName, CustomerName: strings.TrimSpace(row.CustomerFirstName + " " + row.CustomerLastName),
			StartsAt: row.StartsAt.Time.UTC(), EndsAt: row.EndsAt.Time.UTC(),
			BufferBeforeMinutes: int(row.BufferBeforeMinutes), BufferAfterMinutes: int(row.BufferAfterMinutes),
			Status: row.Status, ApprovalStatus: row.ApprovalStatus, ConfirmationStatus: row.ConfirmationStatus,
			PaymentStatus: row.PaymentStatus, Source: row.Source, Version: int(row.Version),
			PartySize: partyOf(row.PartySize),
		}
		if row.StaffName.Valid {
			item.StaffName = &row.StaffName.String
		}
		switch f.By {
		case model.CalendarByBranch:
			item.Columns = []string{row.BranchUuid.String()}
		case model.CalendarByStaff:
			// A person no longer listed in the branch keeps a column of its own.
			if row.StaffUuid.Valid && row.StaffName.Valid {
				key := uuid.UUID(row.StaffUuid.Bytes).String()
				columns.ensure(model.CalendarColumn{Key: key, Name: row.StaffName.String, Kind: model.CalendarByStaff})
				item.Columns = []string{key}
			}
		case model.CalendarByResource:
			var held []struct {
				UUID uuid.UUID `json:"uuid"`
				Name string    `json:"name"`
				Kind string    `json:"kind"`
			}
			if err := json.Unmarshal(row.Resources, &held); err != nil {
				return out, fmt.Errorf("line resources: %w", err)
			}
			for _, r := range held {
				kind := r.Kind
				columns.ensure(model.CalendarColumn{Key: r.UUID.String(), Name: r.Name, Kind: model.CalendarByResource, ResourceKind: &kind})
				item.Columns = append(item.Columns, r.UUID.String())
			}
		}
		if len(item.Columns) == 0 {
			columns.ensure(model.CalendarColumn{Key: model.CalendarUnassigned, Name: "", Kind: model.CalendarUnassigned})
			item.Columns = []string{model.CalendarUnassigned}
		}
		out.Items = append(out.Items, item)
	}
	out.Columns = columns.list
	return out, nil
}

// staffColumns adds the people of the branch: their shifts there (inside
// the opening hours) and their leave. An own calendar member gets only its
// own column (other people appear only with the visits it may see).
func (s *Service) staffColumns(ctx context.Context, orgID int64, b calendarBranch, loc *time.Location, first, last, since, until time.Time, ownStaffID int64, own bool, columns *columnSet) error {
	rows, err := s.q.ListCalendarStaff(ctx, db.ListCalendarStaffParams{OrganizationID: orgID, BranchID: b.row.ID})
	if err != nil {
		return err
	}
	ids := []int64{}
	index := map[int64]string{}
	for _, row := range rows {
		if own && row.ID != ownStaffID {
			continue
		}
		weekly, err := availabilityusecase.ParseShifts(row.Shifts)
		if err != nil {
			return fmt.Errorf("staff shifts: %w", err)
		}
		color := row.Color
		key := row.Uuid.String()
		columns.add(model.CalendarColumn{
			Key: key, Name: row.Name, Kind: model.CalendarByStaff, Color: &color,
			Work: intervals(engine.Intersect(engine.WorkIntervals(loc, first, last, weekly), b.open)),
		})
		ids = append(ids, row.ID)
		index[row.ID] = key
	}
	if len(ids) == 0 {
		return nil
	}
	off, err := s.q.ListAvailabilityTimeOff(ctx, db.ListAvailabilityTimeOffParams{OrganizationID: orgID, StaffIds: ids, Since: ts(since), Until: ts(until)})
	if err != nil {
		return err
	}
	for _, o := range off {
		columns.addOff(index[o.StaffID], model.Interval{StartsAt: o.StartsAt.Time.UTC(), EndsAt: o.EndsAt.Time.UTC()})
	}
	return nil
}

// resourceColumns adds the resources of the branch: their own hours (or
// the branch hours) and their closures.
func (s *Service) resourceColumns(ctx context.Context, orgID int64, b calendarBranch, loc *time.Location, first, last, since, until time.Time, columns *columnSet) error {
	rows, err := s.q.ListCalendarResources(ctx, db.ListCalendarResourcesParams{OrganizationID: orgID, BranchID: b.row.ID})
	if err != nil {
		return err
	}
	ids := []int64{}
	index := map[int64]string{}
	for _, row := range rows {
		open := b.open
		if len(row.WorkingHours) > 0 && string(row.WorkingHours) != "null" {
			own, err := availabilityusecase.ParseWeekly(row.WorkingHours)
			if err != nil {
				return fmt.Errorf("resource hours: %w", err)
			}
			open = engine.OpenIntervals(loc, first, last, own, nil)
		}
		kind := row.Kind
		key := row.Uuid.String()
		columns.add(model.CalendarColumn{Key: key, Name: row.Name, Kind: model.CalendarByResource, ResourceKind: &kind, Work: intervals(open)})
		ids = append(ids, row.ID)
		index[row.ID] = key
	}
	if len(ids) == 0 {
		return nil
	}
	closures, err := s.q.ListAvailabilityResourceClosures(ctx, db.ListAvailabilityResourceClosuresParams{OrganizationID: orgID, ResourceIds: ids, Since: ts(since), Until: ts(until)})
	if err != nil {
		return err
	}
	for _, c := range closures {
		columns.addOff(index[c.ResourceID], model.Interval{StartsAt: c.StartsAt.Time.UTC(), EndsAt: c.EndsAt.Time.UTC()})
	}
	return nil
}

// columnSet keeps the columns in order, each key once.
type columnSet struct {
	list  []model.CalendarColumn
	index map[string]int
}

func newColumnSet() *columnSet {
	return &columnSet{list: []model.CalendarColumn{}, index: map[string]int{}}
}

func (c *columnSet) add(col model.CalendarColumn) {
	if col.Work == nil {
		col.Work = []model.Interval{}
	}
	if col.Off == nil {
		col.Off = []model.Interval{}
	}
	c.index[col.Key] = len(c.list)
	c.list = append(c.list, col)
}

// ensure adds col unless its key is already there.
func (c *columnSet) ensure(col model.CalendarColumn) {
	if _, ok := c.index[col.Key]; !ok {
		c.add(col)
	}
}

func (c *columnSet) addOff(key string, iv model.Interval) {
	if i, ok := c.index[key]; ok {
		c.list[i].Off = append(c.list[i].Off, iv)
	}
}

func intervals(list []engine.Interval) []model.Interval {
	out := make([]model.Interval, 0, len(list))
	for _, iv := range list {
		out = append(out, model.Interval{StartsAt: iv.Start.UTC(), EndsAt: iv.End.UTC()})
	}
	return out
}
