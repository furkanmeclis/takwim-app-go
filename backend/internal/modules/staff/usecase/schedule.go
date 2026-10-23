package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Limits of a weekly schedule and of a time off.
const (
	maxShiftsPerDay  = 6
	maxBreaksPer     = 4
	maxTimeOffSpan   = 366 * 24 * time.Hour
	maxTimeOffNote   = 1000
	minutesPerDay    = 24 * 60
	timeOffPageLimit = 200
)

// SetShifts replaces the weekly shifts of a person in one branch. The shifts
// of one person never overlap across branches on the same weekday (A04):
// the check runs under a lock on the staff row, so two concurrent edits in
// two branches cannot both pass. Times are wall-clock times of the branch.
func (s *Service) SetShifts(ctx context.Context, orgID int64, id, branchUUID uuid.UUID, shifts model.WeeklyShifts) (model.Staff, error) {
	ve := &ValidationError{}
	validateShifts(ve, &shifts)
	err := s.repo.InTx(ctx, func(q *db.Queries) error {
		row, assignments, err := s.load(ctx, q, orgID, id)
		if err != nil {
			return err
		}
		if _, err := q.LockStaff(ctx, db.LockStaffParams{OrganizationID: orgID, ID: row.ID}); err != nil {
			return mapNotFound(repository.MapErr(err), ErrNotFound)
		}
		idx := slices.IndexFunc(assignments, func(a db.ListStaffAssignmentsRow) bool { return a.BranchUuid == branchUUID })
		if idx < 0 || !allowsBranch(ctx, assignments[idx].BranchID) {
			return ErrAssignmentNotFound
		}
		if err := ve.orNil(); err != nil {
			return err
		}
		for i, other := range assignments {
			if i == idx {
				continue
			}
			checkCrossBranch(ve, &shifts, decodeShifts(other.Shifts), other.BranchName)
		}
		if err := ve.orNil(); err != nil {
			return err
		}
		raw, err := json.Marshal(shifts)
		if err != nil {
			return err
		}
		n, err := q.UpdateStaffShifts(ctx, db.UpdateStaffShiftsParams{
			OrganizationID: orgID, StaffID: row.ID, BranchID: assignments[idx].BranchID, Shifts: raw,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrAssignmentNotFound
		}
		return nil
	})
	if err != nil {
		return model.Staff{}, err
	}
	return s.Get(ctx, orgID, id)
}

// validateShifts checks the format of every shift and break, the order
// inside a day (no overlap) and sorts each day by start time.
func validateShifts(ve *ValidationError, w *model.WeeklyShifts) {
	w.Normalize()
	for _, d := range w.Days() {
		day := *d.Shifts
		if len(day) > maxShiftsPerDay {
			ve.add("shifts."+d.Key, "too_many", fmt.Sprintf("at most %d shifts per day", maxShiftsPerDay))
			continue
		}
		valid := true
		for i, sh := range day {
			prefix := fmt.Sprintf("shifts.%s.%d", d.Key, i)
			start, end, ok := interval(ve, prefix, sh.Start, sh.End)
			if !ok {
				valid = false
				continue
			}
			for j, other := range day[:i] {
				os, oe, ok := parseInterval(other.Start, other.End)
				if ok && start < oe && os < end {
					ve.add(prefix+".start", "overlap", fmt.Sprintf("overlaps the shift %s–%s", day[j].Start, day[j].End))
					valid = false
				}
			}
			if len(sh.Breaks) > maxBreaksPer {
				ve.add(prefix+".breaks", "too_many", fmt.Sprintf("at most %d breaks per shift", maxBreaksPer))
				valid = false
				continue
			}
			for k, b := range sh.Breaks {
				bp := fmt.Sprintf("%s.breaks.%d", prefix, k)
				bs, be, ok := interval(ve, bp, b.Start, b.End)
				if !ok {
					valid = false
					continue
				}
				if bs < start || be > end {
					ve.add(bp+".start", "outside", "the break must be inside the shift")
					valid = false
				}
				for _, ob := range sh.Breaks[:k] {
					os, oe, ok := parseInterval(ob.Start, ob.End)
					if ok && bs < oe && os < be {
						ve.add(bp+".start", "overlap", "breaks must not overlap")
						valid = false
					}
				}
			}
		}
		if valid {
			slices.SortFunc(day, func(a, b model.Shift) int { return strings.Compare(a.Start, b.Start) })
			for i := range day {
				slices.SortFunc(day[i].Breaks, func(a, b model.Break) int { return strings.Compare(a.Start, b.Start) })
			}
		}
	}
}

// checkCrossBranch reports the shifts of w that overlap a shift of other
// (another branch of the same person) on the same weekday.
func checkCrossBranch(ve *ValidationError, w *model.WeeklyShifts, other model.WeeklyShifts, branchName string) {
	others := other.Days()
	for di, d := range w.Days() {
		for i, sh := range *d.Shifts {
			start, end, ok := parseInterval(sh.Start, sh.End)
			if !ok {
				continue
			}
			for _, o := range *others[di].Shifts {
				os, oe, ok := parseInterval(o.Start, o.End)
				if ok && start < oe && os < end {
					ve.add(fmt.Sprintf("shifts.%s.%d.start", d.Key, i), "branch_overlap",
						fmt.Sprintf("overlaps the shift %s–%s in %s", o.Start, o.End, branchName))
					break
				}
			}
		}
	}
}

// interval parses a start / end pair and reports format problems.
func interval(ve *ValidationError, prefix, start, end string) (int, int, bool) {
	s, okS := parseHM(start)
	e, okE := parseHM(end)
	if !okS {
		ve.add(prefix+".start", "invalid", "use HH:MM")
	}
	if !okE {
		ve.add(prefix+".end", "invalid", "use HH:MM")
	}
	if !okS || !okE {
		return 0, 0, false
	}
	if e <= s {
		ve.add(prefix+".end", "invalid", "end must be after start")
		return 0, 0, false
	}
	return s, e, true
}

func parseInterval(start, end string) (int, int, bool) {
	s, okS := parseHM(start)
	e, okE := parseHM(end)
	return s, e, okS && okE && e > s
}

// parseHM parses "HH:MM" into minutes after midnight; "24:00" is the end
// of the day.
func parseHM(v string) (int, bool) {
	if len(v) != 5 || v[2] != ':' {
		return 0, false
	}
	h, m := int(v[0]-'0')*10+int(v[1]-'0'), int(v[3]-'0')*10+int(v[4]-'0')
	for _, c := range []byte{v[0], v[1], v[3], v[4]} {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	if m > 59 || h > 24 || (h == 24 && m != 0) {
		return 0, false
	}
	return h*60 + m, h*60+m <= minutesPerDay
}

// ListTimeOff pages the time off of a person, newest first.
func (s *Service) ListTimeOff(ctx context.Context, orgID int64, id uuid.UUID, f model.TimeOffFilter) ([]model.TimeOff, int64, error) {
	row, _, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 || f.Limit > timeOffPageLimit {
		f.Limit = timeOffPageLimit
	}
	from, to := tsArg(f.From), tsArg(f.To)
	rows, err := s.repo.Q().ListStaffTimeOff(ctx, db.ListStaffTimeOffParams{
		OrganizationID: orgID, StaffID: row.ID, RangeFrom: from, RangeTo: to, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.Q().CountStaffTimeOff(ctx, db.CountStaffTimeOffParams{
		OrganizationID: orgID, StaffID: row.ID, RangeFrom: from, RangeTo: to,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.TimeOff, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapTimeOff(r))
	}
	return out, total, nil
}

// CreateTimeOff records a time off of a person (every branch).
func (s *Service) CreateTimeOff(ctx context.Context, orgID int64, id uuid.UUID, actorID *int64, in model.TimeOffInput) (model.TimeOff, error) {
	row, _, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.TimeOff{}, err
	}
	f := timeOffFields{Kind: model.TimeOffLeave}
	if err := f.apply(in, true); err != nil {
		return model.TimeOff{}, err
	}
	created := pgtype.Int8{}
	if actorID != nil {
		created = pgtype.Int8{Int64: *actorID, Valid: true}
	}
	r, err := s.repo.Q().CreateStaffTimeOff(ctx, db.CreateStaffTimeOffParams{
		OrganizationID: orgID, StaffID: row.ID, Kind: f.Kind, StartsAt: ts(f.StartsAt), EndsAt: ts(f.EndsAt),
		Note: f.Note, CreatedBy: created,
	})
	if err != nil {
		return model.TimeOff{}, err
	}
	return mapTimeOff(r), nil
}

// PatchTimeOff updates a time off. It returns the previous and the new state.
func (s *Service) PatchTimeOff(ctx context.Context, orgID int64, id, timeOffID uuid.UUID, in model.TimeOffInput) (model.TimeOff, model.TimeOff, error) {
	row, _, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.TimeOff{}, model.TimeOff{}, err
	}
	current, err := s.repo.Q().GetStaffTimeOff(ctx, db.GetStaffTimeOffParams{OrganizationID: orgID, StaffID: row.ID, Uuid: timeOffID})
	if err != nil {
		return model.TimeOff{}, model.TimeOff{}, mapNotFound(repository.MapErr(err), ErrTimeOffNotFound)
	}
	f := timeOffFields{Kind: current.Kind, StartsAt: current.StartsAt.Time, EndsAt: current.EndsAt.Time, Note: current.Note}
	if err := f.apply(in, false); err != nil {
		return model.TimeOff{}, model.TimeOff{}, err
	}
	r, err := s.repo.Q().UpdateStaffTimeOff(ctx, db.UpdateStaffTimeOffParams{
		OrganizationID: orgID, StaffID: row.ID, Uuid: timeOffID, Kind: f.Kind,
		StartsAt: ts(f.StartsAt), EndsAt: ts(f.EndsAt), Note: f.Note,
	})
	if err != nil {
		return model.TimeOff{}, model.TimeOff{}, mapNotFound(repository.MapErr(err), ErrTimeOffNotFound)
	}
	return mapTimeOff(current), mapTimeOff(r), nil
}

// DeleteTimeOff removes a time off and returns it.
func (s *Service) DeleteTimeOff(ctx context.Context, orgID int64, id, timeOffID uuid.UUID) (model.TimeOff, error) {
	row, _, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.TimeOff{}, err
	}
	current, err := s.repo.Q().GetStaffTimeOff(ctx, db.GetStaffTimeOffParams{OrganizationID: orgID, StaffID: row.ID, Uuid: timeOffID})
	if err != nil {
		return model.TimeOff{}, mapNotFound(repository.MapErr(err), ErrTimeOffNotFound)
	}
	if _, err := s.repo.Q().DeleteStaffTimeOff(ctx, db.DeleteStaffTimeOffParams{OrganizationID: orgID, StaffID: row.ID, Uuid: timeOffID}); err != nil {
		return model.TimeOff{}, err
	}
	return mapTimeOff(current), nil
}

type timeOffFields struct {
	Kind             string
	StartsAt, EndsAt time.Time
	Note             string
}

func (f *timeOffFields) apply(in model.TimeOffInput, create bool) error {
	ve := &ValidationError{}
	if in.Kind != nil {
		if !slices.Contains(model.TimeOffKinds, *in.Kind) {
			ve.add("kind", "invalid", "kind must be one of "+strings.Join(model.TimeOffKinds, ", "))
		} else {
			f.Kind = *in.Kind
		}
	}
	if in.StartsAt != nil {
		f.StartsAt = in.StartsAt.UTC()
	} else if create {
		ve.add("starts_at", "required", "starts_at is required")
	}
	if in.EndsAt != nil {
		f.EndsAt = in.EndsAt.UTC()
	} else if create {
		ve.add("ends_at", "required", "ends_at is required")
	}
	if in.Note != nil {
		f.Note = strings.TrimSpace(*in.Note)
		if utf8.RuneCountInString(f.Note) > maxTimeOffNote {
			ve.add("note", "too_long", fmt.Sprintf("note must be at most %d characters", maxTimeOffNote))
		}
	}
	if len(ve.Details) == 0 {
		switch {
		case !f.EndsAt.After(f.StartsAt):
			ve.add("ends_at", "invalid", "ends_at must be after starts_at")
		case f.EndsAt.Sub(f.StartsAt) > maxTimeOffSpan:
			ve.add("ends_at", "too_long", "a time off spans at most 366 days")
		}
	}
	if err := ve.orNil(); err != nil {
		return err
	}
	return nil
}

func mapTimeOff(r db.StaffTimeOff) model.TimeOff {
	return model.TimeOff{
		UUID: r.Uuid, Kind: r.Kind, StartsAt: r.StartsAt.Time, EndsAt: r.EndsAt.Time,
		Note: r.Note, CreatedAt: r.CreatedAt.Time,
	}
}

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func tsArg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return ts(*t)
}

// IsValidation reports whether err is a field validation error.
func IsValidation(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
