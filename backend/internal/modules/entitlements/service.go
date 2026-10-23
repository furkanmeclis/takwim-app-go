// Package entitlements serves what a business may use (TEC-883): plan,
// add-ons, limits with their current use and the monthly communication
// usage. The catalog and the resolution rules live in
// internal/platform/entitlement; this module reads the counts, enforces the
// limits for the modules that add branches and staff seats and records
// usage. See docs/design/entitlements.md.
package entitlements

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/localtime"
	"github.com/jackc/pgx/v5/pgtype"
)

// Usage metrics (organization_usage.metric).
const (
	MetricSMS       = "sms"
	MetricWhatsApp  = "whatsapp"
	MetricEmail     = "email"
	MetricVoiceCall = "voice_call"
)

// Metrics lists every usage metric in display order.
var Metrics = []string{MetricSMS, MetricWhatsApp, MetricEmail, MetricVoiceCall}

// ErrUnknownMetric: AddUsage got a metric outside Metrics.
var ErrUnknownMetric = errors.New("entitlements: unknown usage metric")

// Counter is one metric's use in a month.
type Counter struct {
	Metric   string `json:"metric"`
	Quantity int64  `json:"quantity"`
}

// Used is the current use of the limited resources.
type Used struct {
	Branches int64 `json:"branches"`
	// Staff: panel members plus open invitations (staff seats).
	Staff int64 `json:"staff"`
}

// Summary is the entitlements page of a business.
type Summary struct {
	entitlement.Effective
	Used Used `json:"used"`
	// Period is the current month in the business time zone ("2026-10").
	Period string    `json:"period"`
	Usage  []Counter `json:"usage"`
}

// Service reads counts and records usage.
type Service struct {
	q   *db.Queries
	now func() time.Time
}

// New builds the service.
func New(q *db.Queries) *Service {
	return &Service{q: q, now: time.Now}
}

func monthStart(now time.Time, zone string) localtime.Date {
	loc, err := localtime.LoadZone(localtime.ZoneOr(zone))
	if err != nil {
		loc = localtime.Default()
	}
	d := localtime.DateIn(now, loc)
	return localtime.NewDate(d.Year, d.Month, 1)
}

func pgDate(d localtime.Date) pgtype.Date {
	return pgtype.Date{Time: d.Time(), Valid: true}
}

// Summary answers the effective entitlements of a scope with their use.
func (s *Service) Summary(ctx context.Context, scope orgctx.Scope) (Summary, error) {
	counts, err := s.q.CountEntitlementUsage(ctx, scope.ID)
	if err != nil {
		return Summary{}, err
	}
	period := monthStart(s.now(), scope.Timezone)
	rows, err := s.q.ListOrganizationUsage(ctx, db.ListOrganizationUsageParams{OrganizationID: scope.ID, Period: pgDate(period)})
	if err != nil {
		return Summary{}, err
	}
	byMetric := map[string]int64{}
	for _, r := range rows {
		byMetric[r.Metric] = r.Quantity
	}
	usage := make([]Counter, 0, len(Metrics))
	for _, m := range Metrics {
		usage = append(usage, Counter{Metric: m, Quantity: byMetric[m]})
	}
	return Summary{
		Effective: entitlement.ForScope(scope),
		Used:      Used{Branches: counts.Branches, Staff: counts.Staff},
		Period:    fmt.Sprintf("%04d-%02d", period.Year, int(period.Month)),
		Usage:     usage,
	}, nil
}

// CheckBranchAdd answers a *entitlement.LimitError when the business cannot
// open one more branch.
func (s *Service) CheckBranchAdd(ctx context.Context, scope orgctx.Scope) error {
	counts, err := s.q.CountEntitlementUsage(ctx, scope.ID)
	if err != nil {
		return err
	}
	return entitlement.CheckAdd(entitlement.LimitBranches, entitlement.ForScope(scope).BranchLimit, counts.Branches)
}

// CheckStaffAdd answers a *entitlement.LimitError when the business cannot
// take one more staff seat (member or invitation). q is the caller's
// transaction (after it locked the organization row) or nil.
func (s *Service) CheckStaffAdd(ctx context.Context, q *db.Queries, scope orgctx.Scope) error {
	if q == nil {
		q = s.q
	}
	counts, err := q.CountEntitlementUsage(ctx, scope.ID)
	if err != nil {
		return err
	}
	return entitlement.CheckAdd(entitlement.LimitStaff, entitlement.ForScope(scope).StaffLimit, counts.Staff)
}

// AddUsage counts n units of a metric in the current month of the business
// (the communication modules call it after a provider accepted the work).
func (s *Service) AddUsage(ctx context.Context, scope orgctx.Scope, metric string, n int64) error {
	known := false
	for _, m := range Metrics {
		if m == metric {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("%w: %q", ErrUnknownMetric, metric)
	}
	if n <= 0 {
		return nil
	}
	_, err := s.q.AddOrganizationUsage(ctx, db.AddOrganizationUsageParams{
		OrganizationID: scope.ID, Metric: metric,
		Period: pgDate(monthStart(s.now(), scope.Timezone)), Quantity: n,
	})
	return err
}
