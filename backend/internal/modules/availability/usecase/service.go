// Package usecase loads the inputs of the availability engine from the
// database and answers availability questions. Every method takes the
// organization id explicitly; the panel and the online channel (booking
// page, widget, public API) go through the same Service, the channel only
// decides which records are visible and which booking window applies.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/engine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	orgsettings "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
)

// Limits of a search.
const (
	MaxLines        = 10
	MaxDays         = 31
	DefaultDays     = 7
	DefaultStep     = 15
	MinStep         = 5
	MaxStep         = 240
	dateLayout      = "2006-01-02"
	defaultTimezone = "Europe/Istanbul"
)

var (
	// ErrNotFound: the branch is missing, deleted or outside the caller's
	// branch scope.
	ErrNotFound = errors.New("branch not found")
	// ErrForbidden: a caller limited to its own calendar asked for someone
	// else (or has no staff record).
	ErrForbidden = errors.New("only your own calendar can be searched")
	// ErrOnlineNotReady: the online channel needs the minimum notice and the
	// maximum advance of the business settings.
	ErrOnlineNotReady = errors.New("online booking rules are not configured")
)

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Message string
	Code    string
}

// ValidationError carries field details; handlers answer 400.
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

// Queries are the reads the service needs (db.Queries implements them).
type Queries interface {
	GetAvailabilityBranch(ctx context.Context, arg db.GetAvailabilityBranchParams) (db.GetAvailabilityBranchRow, error)
	GetAvailabilityService(ctx context.Context, arg db.GetAvailabilityServiceParams) (db.GetAvailabilityServiceRow, error)
	ListAvailabilityServiceStaff(ctx context.Context, arg db.ListAvailabilityServiceStaffParams) ([]db.ListAvailabilityServiceStaffRow, error)
	GetAvailabilityStaffByUser(ctx context.Context, arg db.GetAvailabilityStaffByUserParams) (db.GetAvailabilityStaffByUserRow, error)
	ListAvailabilityTimeOff(ctx context.Context, arg db.ListAvailabilityTimeOffParams) ([]db.ListAvailabilityTimeOffRow, error)
	ListAvailabilityResources(ctx context.Context, arg db.ListAvailabilityResourcesParams) ([]db.ListAvailabilityResourcesRow, error)
	ListAvailabilityResourceClosures(ctx context.Context, arg db.ListAvailabilityResourceClosuresParams) ([]db.ListAvailabilityResourceClosuresRow, error)
}

// SettingsReader returns the current business settings (orgsettings).
type SettingsReader interface {
	Current(ctx context.Context, orgID int64) (orgsettings.Version, error)
}

// BusySource returns the commitments (bookings, holds) of staff members and
// resources overlapping [since, until), by id. Staff commitments come from
// every branch: a person has one calendar (A04). Reservations and holds
// (TEC-898 / TEC-899) plug in here; without a source nothing is booked.
type BusySource interface {
	Busy(ctx context.Context, orgID int64, staffIDs, resourceIDs []int64, since, until time.Time) (staff, resources map[int64][]engine.Interval, err error)
}

// Caller narrows a search to the caller's own calendar: OwnUserID > 0 means
// the member holds only tenant.reservations.own, so every line that takes a
// staff member is planned with the member's own staff record.
// KeepOthers lets lines pinned to another person through (moving a visit
// shared with colleagues: their lines keep their person and are checked
// again like every other line).
type Caller struct {
	OwnUserID  int64
	KeepOthers bool
}

// Service answers availability questions.
type Service struct {
	q        Queries
	settings SettingsReader
	busy     BusySource
	now      func() time.Time
}

// New builds the service. settings may be nil (online channel refused).
func New(q Queries, settings SettingsReader) *Service {
	return &Service{q: q, settings: settings, now: time.Now}
}

// SetBusySource plugs in the commitments of reservations and holds.
func (s *Service) SetBusySource(b BusySource) {
	s.busy = b
}

// SetClock replaces time.Now (tests).
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

// Search answers the days of the request.
func (s *Service) Search(ctx context.Context, orgID int64, in model.SearchInput, caller Caller) (model.Result, error) {
	p, err := s.prepare(ctx, orgID, in, caller, nil)
	if err != nil {
		return model.Result{}, err
	}
	days := engine.Search(p.req)
	out := p.result
	out.Days = make([]model.Day, 0, len(days))
	for _, d := range days {
		day := model.Day{Date: d.Date, Status: d.Status, Slots: make([]model.Slot, 0, len(d.Slots))}
		for _, slot := range d.Slots {
			day.Slots = append(day.Slots, p.slot(slot))
		}
		out.Days = append(out.Days, day)
	}
	return out, nil
}

// Check reports whether a booking of in can start at start, with its plan.
// The reservation flow calls it again under its lock before writing.
func (s *Service) Check(ctx context.Context, orgID int64, in model.SearchInput, caller Caller, start time.Time) (model.Slot, bool, error) {
	p, err := s.prepare(ctx, orgID, in, caller, &start)
	if err != nil {
		return model.Slot{}, false, err
	}
	slot, ok := engine.Check(p.req, start)
	if !ok {
		return model.Slot{}, false, nil
	}
	out := p.slot(slot)
	out.BranchID = p.branchID
	return out, true, nil
}

// prepared is an engine request with the lookups to render its answer.
type prepared struct {
	branchID  int64
	req       engine.Request
	result    model.Result
	services  []uuid.UUID
	staff     map[int64]model.Ref
	resources map[int64]model.ResourceRef
}

func (p *prepared) slot(s engine.Slot) model.Slot {
	out := model.Slot{StartsAt: s.Start.UTC(), EndsAt: s.End.UTC(), Lines: make([]model.SlotLine, 0, len(s.Lines))}
	for i, a := range s.Lines {
		line := model.SlotLine{
			ServiceUUID: p.services[i], StartsAt: a.Start.UTC(), EndsAt: a.End.UTC(),
			Resources: make([]model.ResourceRef, 0, len(a.ResourceIDs)),
			StaffID:   a.StaffID, ResourceIDs: a.ResourceIDs, HeldFrom: a.Held.Start.UTC(), HeldUntil: a.Held.End.UTC(),
		}
		if a.StaffID != 0 {
			ref := p.staff[a.StaffID]
			line.Staff = &ref
		}
		for _, id := range a.ResourceIDs {
			line.Resources = append(line.Resources, p.resources[id])
		}
		out.Lines = append(out.Lines, line)
	}
	return out
}

// dayHours is the stored shape of one day of branch / resource hours.
type dayHours struct {
	Closed bool   `json:"closed"`
	Open   string `json:"open"`
	Close  string `json:"close"`
}

type holiday struct {
	Date string `json:"date"`
	dayHours
}

type storedShift struct {
	Start  string `json:"start"`
	End    string `json:"end"`
	Breaks []struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"breaks"`
}

func weekly(raw []byte) (engine.Weekly, error) {
	var days map[string]dayHours
	if err := json.Unmarshal(raw, &days); err != nil {
		return nil, err
	}
	out := engine.Weekly{}
	for key, h := range days {
		if wd, ok := engine.WeekdayKeys[key]; ok {
			out[wd] = engine.Hours{Closed: h.Closed, Open: h.Open, Close: h.Close}
		}
	}
	return out, nil
}

func shifts(raw []byte) (engine.WeeklyShifts, error) {
	var days map[string][]storedShift
	if err := json.Unmarshal(raw, &days); err != nil {
		return nil, err
	}
	out := engine.WeeklyShifts{}
	for key, list := range days {
		wd, ok := engine.WeekdayKeys[key]
		if !ok {
			continue
		}
		for _, sh := range list {
			shift := engine.Shift{Span: engine.Span{Start: sh.Start, End: sh.End}}
			for _, b := range sh.Breaks {
				shift.Breaks = append(shift.Breaks, engine.Span{Start: b.Start, End: b.End})
			}
			out[wd] = append(out[wd], shift)
		}
	}
	return out, nil
}

// ParseWeekly reads stored weekly hours (branch or resource).
func ParseWeekly(raw []byte) (engine.Weekly, error) {
	return weekly(raw)
}

// ParseShifts reads the stored weekly shifts of a person in a branch.
func ParseShifts(raw []byte) (engine.WeeklyShifts, error) {
	return shifts(raw)
}

// ParseExceptions reads the stored special days of a branch, by date.
func ParseExceptions(raw []byte) (map[string]engine.Hours, error) {
	var holidays []holiday
	if err := json.Unmarshal(raw, &holidays); err != nil {
		return nil, err
	}
	out := make(map[string]engine.Hours, len(holidays))
	for _, h := range holidays {
		out[h.Date] = engine.Hours{Closed: h.Closed, Open: h.Open, Close: h.Close}
	}
	return out, nil
}

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func minutes(n int32) time.Duration {
	return time.Duration(n) * time.Minute
}

// prepare validates the input and loads everything the engine needs. When
// at is set the window is the local date of that instant.
func (s *Service) prepare(ctx context.Context, orgID int64, in model.SearchInput, caller Caller, at *time.Time) (*prepared, error) {
	ve := &ValidationError{}
	channel := in.Channel
	if channel == "" {
		channel = model.ChannelPanel
	}
	if channel != model.ChannelPanel && channel != model.ChannelOnline {
		ve.add("channel", "invalid", "channel must be panel or online")
	}
	if in.BranchUUID == uuid.Nil {
		ve.add("branch_uuid", "required", "branch is required")
	}
	switch {
	case len(in.Lines) == 0:
		ve.add("lines", "required", "at least one service is required")
	case len(in.Lines) > MaxLines:
		ve.add("lines", "too_many", fmt.Sprintf("at most %d services", MaxLines))
	}
	step := DefaultStep
	if in.StepMinutes != nil {
		step = *in.StepMinutes
		if step < MinStep || step > MaxStep || step%MinStep != 0 {
			ve.add("step_minutes", "invalid", fmt.Sprintf("step must be a multiple of %d between %d and %d", MinStep, MinStep, MaxStep))
		}
	}
	party := 0
	if in.PartySize != nil {
		party = *in.PartySize
		if party < model.MinPartySize || party > model.MaxPartySize {
			ve.add("party_size", "invalid", fmt.Sprintf("party size must be between %d and %d", model.MinPartySize, model.MaxPartySize))
		}
	}
	zone := strings.TrimSpace(in.Zone)
	if len(zone) > model.MaxZone {
		ve.add("zone", "too_long", "zone is too long")
	}
	if err := ve.orNil(); err != nil {
		return nil, err
	}

	branch, err := s.q.GetAvailabilityBranch(ctx, db.GetAvailabilityBranchParams{OrganizationID: orgID, Uuid: in.BranchUUID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if orgctx.CheckBranch(ctx, branch.ID) != nil {
		return nil, ErrNotFound
	}
	if !branch.IsActive {
		ve.add("branch_uuid", "inactive", "the branch is not active")
		return nil, ve
	}
	loc, err := time.LoadLocation(branch.Timezone)
	if err != nil || branch.Timezone == "" {
		loc, _ = time.LoadLocation(defaultTimezone)
	}

	now := s.now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	first, last, ok := s.window(ve, in, today, loc, at)
	if !ok {
		return nil, ve
	}

	result := model.Result{
		Branch: model.Ref{UUID: branch.Uuid, Name: branch.Name}, Timezone: loc.String(), Channel: channel,
		StepMinutes: step, EarliestStart: now.UTC(),
	}
	req := engine.Request{
		Location: loc, FirstDay: first, LastDay: last, Step: time.Duration(step) * time.Minute, NotBefore: now,
		PartySize: party, Zone: zone, Combine: channel == model.ChannelPanel,
	}
	if channel == model.ChannelOnline {
		if s.settings == nil {
			return nil, ErrOnlineNotReady
		}
		v, err := s.settings.Current(ctx, orgID)
		if err != nil {
			return nil, err
		}
		r := v.Settings.Reservation
		if r.MinLeadMinutes == nil || r.MaxAdvanceDays == nil {
			return nil, ErrOnlineNotReady
		}
		req.NotBefore = now.Add(time.Duration(*r.MinLeadMinutes) * time.Minute)
		lastDay := today.AddDate(0, 0, *r.MaxAdvanceDays)
		req.NotAfter = lastDay.AddDate(0, 0, 1)
		latest := lastDay.Format(dateLayout)
		result.EarliestStart = req.NotBefore.UTC()
		result.LatestDate = &latest
	}

	hours, err := weekly(branch.WorkingHours)
	if err != nil {
		return nil, fmt.Errorf("branch hours: %w", err)
	}
	exceptions, err := ParseExceptions(branch.HolidayExceptions)
	if err != nil {
		return nil, fmt.Errorf("branch special days: %w", err)
	}
	req.Open = engine.OpenIntervals(loc, first, last, hours, exceptions)

	ownStaffID := int64(0)
	if caller.OwnUserID > 0 {
		own, err := s.q.GetAvailabilityStaffByUser(ctx, db.GetAvailabilityStaffByUserParams{OrganizationID: orgID, UserID: pgtype.Int8{Int64: caller.OwnUserID, Valid: true}})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrForbidden
		}
		if err != nil {
			return nil, err
		}
		ownStaffID = own.ID
		for _, l := range in.Lines {
			if l.StaffUUID != nil && *l.StaffUUID != own.Uuid && !caller.KeepOthers {
				return nil, ErrForbidden
			}
		}
	}

	p := &prepared{branchID: branch.ID, staff: map[int64]model.Ref{}, resources: map[int64]model.ResourceRef{}}
	staffShifts := map[int64]engine.WeeklyShifts{}
	kinds := map[string]bool{}
	online := channel == model.ChannelOnline
	for i, l := range in.Lines {
		field := "lines." + strconv.Itoa(i)
		svc, err := s.q.GetAvailabilityService(ctx, db.GetAvailabilityServiceParams{OrganizationID: orgID, Uuid: l.ServiceUUID, BranchID: branch.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			ve.add(field+".service_uuid", "not_found", "service not found")
			continue
		}
		if err != nil {
			return nil, err
		}
		switch {
		case !svc.IsActive || (online && !svc.ShowOnline):
			ve.add(field+".service_uuid", "inactive", "the service cannot be booked")
			continue
		case !svc.OfferID.Valid || !svc.OfferIsActive.Bool:
			ve.add(field+".service_uuid", "not_offered", "the branch does not offer the service")
			continue
		}
		duration := svc.DurationMinutes
		if svc.OfferDurationMinutes.Valid {
			duration = svc.OfferDurationMinutes.Int32
		}
		line := engine.Line{
			Key: svc.Uuid.String(), Duration: minutes(duration),
			BufferBefore: minutes(svc.BufferBeforeMinutes), BufferAfter: minutes(svc.BufferAfterMinutes),
			StaffMode: svc.StaffMode, ResourceKinds: svc.ResourceKinds, StaffDuration: map[int64]time.Duration{},
		}
		info := model.LineInfo{
			ServiceUUID: svc.Uuid, ServiceName: svc.Name, DurationMinutes: int(duration),
			BufferBeforeMinutes: int(svc.BufferBeforeMinutes), BufferAfterMinutes: int(svc.BufferAfterMinutes),
			StaffMode: svc.StaffMode, ResourceKinds: append([]string{}, svc.ResourceKinds...), Staff: []model.Ref{},
		}
		for _, k := range svc.ResourceKinds {
			kinds[k] = true
		}
		if svc.StaffMode != engine.StaffNone {
			rows, err := s.q.ListAvailabilityServiceStaff(ctx, db.ListAvailabilityServiceStaffParams{OrganizationID: orgID, ServiceID: svc.ID, BranchID: branch.ID})
			if err != nil {
				return nil, err
			}
			pinned := l.StaffUUID != nil
			matched := false
			for _, row := range rows {
				if online && !row.ShowOnline {
					continue
				}
				if pinned && row.Uuid != *l.StaffUUID {
					continue
				}
				if ownStaffID > 0 && row.ID != ownStaffID && !pinned {
					continue
				}
				matched = true
				line.Staff = append(line.Staff, row.ID)
				if row.DurationMinutes.Valid {
					line.StaffDuration[row.ID] = minutes(row.DurationMinutes.Int32)
				}
				ref := model.Ref{UUID: row.Uuid, Name: row.Name}
				p.staff[row.ID] = ref
				info.Staff = append(info.Staff, ref)
				if _, ok := staffShifts[row.ID]; !ok {
					sh, err := shifts(row.Shifts)
					if err != nil {
						return nil, fmt.Errorf("staff shifts: %w", err)
					}
					staffShifts[row.ID] = sh
				}
			}
			if pinned && !matched {
				ve.add(field+".staff_uuid", "not_found", "this person cannot perform the service in the branch")
				continue
			}
			// The own calendar of a member: the line is the member's or nobody's.
			if ownStaffID > 0 && svc.StaffMode == engine.StaffOptional && !matched {
				line.StaffMode = engine.StaffNone
			}
		}
		req.Lines = append(req.Lines, line)
		p.services = append(p.services, svc.Uuid)
		result.Lines = append(result.Lines, info)
	}
	// A sector that books by party (restaurant) needs the party size for
	// lines that take a resource: the resource must seat it.
	if party == 0 && len(kinds) > 0 && partySector(ctx) {
		ve.add("party_size", "required", "the number of people is required")
	}
	if err := ve.orNil(); err != nil {
		return nil, err
	}

	since := first.AddDate(0, 0, -1)
	until := last.AddDate(0, 0, 2)
	staffIDs := make([]int64, 0, len(staffShifts))
	for id, sh := range staffShifts {
		staffIDs = append(staffIDs, id)
		req.Staff = append(req.Staff, engine.Staff{ID: id, Work: engine.WorkIntervals(loc, first, last, sh)})
	}
	staffIndex := map[int64]int{}
	for i, st := range req.Staff {
		staffIndex[st.ID] = i
	}
	if len(staffIDs) > 0 {
		off, err := s.q.ListAvailabilityTimeOff(ctx, db.ListAvailabilityTimeOffParams{OrganizationID: orgID, StaffIds: staffIDs, Since: ts(since), Until: ts(until)})
		if err != nil {
			return nil, err
		}
		for _, o := range off {
			st := &req.Staff[staffIndex[o.StaffID]]
			st.Busy = append(st.Busy, engine.Interval{Start: o.StartsAt.Time, End: o.EndsAt.Time})
		}
	}

	var resourceIDs []int64
	if len(kinds) > 0 {
		list := make([]string, 0, len(kinds))
		for k := range kinds {
			list = append(list, k)
		}
		rows, err := s.q.ListAvailabilityResources(ctx, db.ListAvailabilityResourcesParams{OrganizationID: orgID, BranchID: branch.ID, Kinds: list})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if online && !row.ShowOnline {
				continue
			}
			open := req.Open
			if len(row.WorkingHours) > 0 && string(row.WorkingHours) != "null" {
				own, err := weekly(row.WorkingHours)
				if err != nil {
					return nil, fmt.Errorf("resource hours: %w", err)
				}
				open = engine.OpenIntervals(loc, first, last, own, nil)
			}
			req.Resources = append(req.Resources, engine.Resource{
				ID: row.ID, Kind: row.Kind, Open: open,
				MinCapacity: int(row.MinCapacity), Capacity: int(row.Capacity), Zone: row.Zone, Combinable: row.Combinable,
			})
			p.resources[row.ID] = model.ResourceRef{UUID: row.Uuid, Name: row.Name, Kind: row.Kind}
			resourceIDs = append(resourceIDs, row.ID)
		}
	}
	resourceIndex := map[int64]int{}
	for i, r := range req.Resources {
		resourceIndex[r.ID] = i
	}
	if len(resourceIDs) > 0 {
		closures, err := s.q.ListAvailabilityResourceClosures(ctx, db.ListAvailabilityResourceClosuresParams{OrganizationID: orgID, ResourceIds: resourceIDs, Since: ts(since), Until: ts(until)})
		if err != nil {
			return nil, err
		}
		for _, c := range closures {
			r := &req.Resources[resourceIndex[c.ResourceID]]
			r.Busy = append(r.Busy, engine.Interval{Start: c.StartsAt.Time, End: c.EndsAt.Time})
		}
	}

	if s.busy != nil && (len(staffIDs) > 0 || len(resourceIDs) > 0) {
		staffBusy, resourceBusy, err := s.busy.Busy(ctx, orgID, staffIDs, resourceIDs, since, until)
		if err != nil {
			return nil, err
		}
		for id, list := range staffBusy {
			if i, ok := staffIndex[id]; ok {
				req.Staff[i].Busy = append(req.Staff[i].Busy, list...)
			}
		}
		for id, list := range resourceBusy {
			if i, ok := resourceIndex[id]; ok {
				req.Resources[i].Busy = append(req.Resources[i].Busy, list...)
			}
		}
	}

	p.req = req
	p.result = result
	return p, nil
}

// partySector reports whether the business's sector books by party size.
func partySector(ctx context.Context) bool {
	scope, ok := orgctx.From(ctx)
	if !ok {
		return false
	}
	def, ok := sector.Get(sector.Code(scope.Sector))
	return ok && def.Capacity.PartySize
}

// window resolves the local dates of the search.
func (s *Service) window(ve *ValidationError, in model.SearchInput, today time.Time, loc *time.Location, at *time.Time) (time.Time, time.Time, bool) {
	if at != nil {
		t := at.In(loc)
		d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		return d, d, true
	}
	first := today
	if in.From != "" {
		d, err := time.ParseInLocation(dateLayout, in.From, loc)
		if err != nil {
			ve.add("from", "invalid", "from must be a date (yyyy-MM-dd)")
			return first, first, false
		}
		first = d
	}
	last := first.AddDate(0, 0, DefaultDays-1)
	if in.To != "" {
		d, err := time.ParseInLocation(dateLayout, in.To, loc)
		if err != nil {
			ve.add("to", "invalid", "to must be a date (yyyy-MM-dd)")
			return first, first, false
		}
		last = d
	}
	if last.Before(first) {
		ve.add("to", "before_from", "to must not be before from")
		return first, last, false
	}
	if last.After(first.AddDate(0, 0, MaxDays-1)) {
		ve.add("to", "too_long", fmt.Sprintf("at most %d days per search", MaxDays))
		return first, last, false
	}
	return first, last, true
}
