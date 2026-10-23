// Package overview serves the central panel of an organization (TEC-892):
// the branches the caller sees, one by one and in total. The branch picker
// in the panel header (X-Branch) narrows it to one branch like every other
// list. Later modules (reservations, revenue) add their figures here.
package overview

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// defaultTimezone matches the organizations.timezone column default.
const defaultTimezone = "Europe/Istanbul"

// Overview is the central panel payload.
type Overview struct {
	Totals   Totals          `json:"totals"`
	Branches []BranchSummary `json:"branches"`
}

// Totals sums the listed branches. Members counts each member once, even
// when they work in several of the branches.
type Totals struct {
	Branches       int64 `json:"branches"`
	ActiveBranches int64 `json:"active_branches"`
	OnlineBranches int64 `json:"online_branches"`
	OpenNow        int64 `json:"open_now"`
	Members        int64 `json:"members"`
}

// BranchSummary is one branch of the central panel.
type BranchSummary struct {
	UUID       uuid.UUID `json:"uuid"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	City       string    `json:"city"`
	IsActive   bool      `json:"is_active"`
	ShowOnline bool      `json:"show_online"`
	// Timezone is the effective zone (branch, else business).
	Timezone string   `json:"timezone"`
	Today    DayHours `json:"today"`
	// OpenNow: active and inside today's opening hours, in the branch zone.
	OpenNow     bool  `json:"open_now"`
	MemberCount int64 `json:"member_count"`
}

// DayHours is the schedule of the branch's current local day: the dated
// exception when there is one, the weekly hours otherwise. A day without
// both times is closed.
type DayHours struct {
	Date      string `json:"date"`
	Closed    bool   `json:"closed"`
	Open      string `json:"open"`
	Close     string `json:"close"`
	Exception bool   `json:"exception"`
	Note      string `json:"note"`
}

// BranchLister returns every branch of the caller's (narrowed) branch scope.
type BranchLister interface {
	ExportRows(ctx context.Context, orgID int64, f branchmodel.ListFilter) ([]branchmodel.Branch, error)
}

// Queries are the member counts.
type Queries interface {
	CountMembersByBranch(ctx context.Context, arg db.CountMembersByBranchParams) ([]db.CountMembersByBranchRow, error)
	CountMembersInBranches(ctx context.Context, arg db.CountMembersInBranchesParams) (int64, error)
}

// Service builds the overview.
type Service struct {
	branches BranchLister
	q        Queries
	now      func() time.Time
}

// New returns a Service.
func New(branches BranchLister, q Queries) *Service {
	return &Service{branches: branches, q: q, now: time.Now}
}

// Get returns the overview of the organization on ctx.
func (s *Service) Get(ctx context.Context, scope orgctx.Scope) (Overview, error) {
	rows, err := s.branches.ExportRows(ctx, scope.ID, branchmodel.ListFilter{SortField: "name"})
	if err != nil {
		return Overview{}, err
	}
	branchIDs := orgctx.BranchScope(ctx)
	counts, err := s.q.CountMembersByBranch(ctx, db.CountMembersByBranchParams{OrganizationID: scope.ID, BranchIds: branchIDs})
	if err != nil {
		return Overview{}, err
	}
	members := make(map[uuid.UUID]int64, len(counts))
	for _, c := range counts {
		members[c.BranchUuid] = c.MemberCount
	}
	total, err := s.q.CountMembersInBranches(ctx, db.CountMembersInBranchesParams{OrganizationID: scope.ID, BranchIds: branchIDs})
	if err != nil {
		return Overview{}, err
	}

	now := s.now()
	out := Overview{Totals: Totals{Members: total}, Branches: make([]BranchSummary, 0, len(rows))}
	for _, b := range rows {
		zone := effectiveZone(b.Timezone, scope.Timezone)
		today, open := daySchedule(b, zone, now)
		sum := BranchSummary{
			UUID: b.UUID, Code: b.Code, Name: b.Name, City: b.City,
			IsActive: b.IsActive, ShowOnline: b.ShowOnline, Timezone: zone,
			Today: today, OpenNow: b.IsActive && open, MemberCount: members[b.UUID],
		}
		out.Branches = append(out.Branches, sum)
		out.Totals.Branches++
		if sum.IsActive {
			out.Totals.ActiveBranches++
			if sum.ShowOnline {
				out.Totals.OnlineBranches++
			}
		}
		if sum.OpenNow {
			out.Totals.OpenNow++
		}
	}
	return out, nil
}

func effectiveZone(branchZone *string, orgZone string) string {
	if branchZone != nil && *branchZone != "" {
		return *branchZone
	}
	if orgZone != "" {
		return orgZone
	}
	return defaultTimezone
}

// daySchedule returns the schedule of the local day of now in zone and
// whether now is inside it. Hours that close at or before they open run
// past midnight (open until the close time of the next day is not tracked:
// only the opening day counts).
func daySchedule(b branchmodel.Branch, zone string, now time.Time) (DayHours, bool) {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		loc, _ = time.LoadLocation(defaultTimezone)
	}
	local := now.In(loc)
	day := DayHours{Date: local.Format(time.DateOnly)}
	weekly := weekday(b.WorkingHours, local.Weekday())
	day.Closed, day.Open, day.Close = weekly.Closed, weekly.Open, weekly.Close
	for _, ex := range b.HolidayExceptions {
		if ex.Date == day.Date {
			day.Closed, day.Open, day.Close = ex.Closed, ex.Open, ex.Close
			day.Exception, day.Note = true, ex.Note
			break
		}
	}
	day.Open, day.Close = strings.TrimSpace(day.Open), strings.TrimSpace(day.Close)
	if day.Open == "" || day.Close == "" {
		day.Closed = true
	}
	if day.Closed {
		day.Open, day.Close = "", ""
		return day, false
	}
	clock := local.Format("15:04")
	if day.Close <= day.Open {
		return day, clock >= day.Open
	}
	return day, clock >= day.Open && clock < day.Close
}

func weekday(w branchmodel.WeeklyHours, d time.Weekday) branchmodel.DayHours {
	switch d {
	case time.Monday:
		return w.Mon
	case time.Tuesday:
		return w.Tue
	case time.Wednesday:
		return w.Wed
	case time.Thursday:
		return w.Thu
	case time.Friday:
		return w.Fri
	case time.Saturday:
		return w.Sat
	default:
		return w.Sun
	}
}
