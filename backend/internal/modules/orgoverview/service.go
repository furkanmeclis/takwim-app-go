// Package orgoverview serves the 360 page of a business in Takwim yönetimi
// (TEC-927): plan use (branches, staff), catalogue size, reservation volume,
// communication use with the Konfirmasyon right kept apart from the channel
// counters, and the set-up state of every channel and of online payment.
// A channel that is not ready is listed as such; nothing here reports a
// send that did not happen. See docs/design/entitlements.md ("Takwim
// yönetimi 360").
package orgoverview

import (
	"context"
	"fmt"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/entitlements"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/localtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Channel set-up states.
const (
	// ChannelReady: a provider delivers now; sends are recorded as sent.
	ChannelReady = "ready"
	// ChannelNotReady: the channel exists but no provider can deliver;
	// its sends are recorded as skipped.
	ChannelNotReady = "not_ready"
	// ChannelUnavailable: the channel has no adapter in this version yet
	// (voice calls until their issue lands).
	ChannelUnavailable = "unavailable"
)

// Payment set-up states.
const (
	PaymentReady = "ready"
	// PaymentNotConnected: no active payment provider account.
	PaymentNotConnected = "not_connected"
	// PaymentNoCashAccount: the provider is connected but no online cash
	// account takes its money.
	PaymentNoCashAccount = "no_cash_account"
	// PaymentProviderUnavailable: the connected provider has no working
	// adapter in this server.
	PaymentProviderUnavailable = "provider_unavailable"
)

// Channels lists the communication channels in display order; each one is
// also a usage metric.
var Channels = []string{entitlements.MetricSMS, entitlements.MetricWhatsApp, entitlements.MetricEmail, entitlements.MetricVoiceCall}

// volumeMonths is how many months the reservation volume series covers
// (the current month included).
const volumeMonths = 6

// Readiness tells whether a notification channel can deliver now (the
// customer notification engine).
type Readiness interface {
	Ready(channel string) bool
}

// PaymentReadiness tells whether a business can take online payments (the
// payments service).
type PaymentReadiness interface {
	OnlineReady(ctx context.Context, orgID int64) bool
}

// Account is a connected provider account (never its credentials).
type Account struct {
	UUID            uuid.UUID  `json:"uuid"`
	Provider        string     `json:"provider"`
	DisplayName     string     `json:"display_name"`
	Mode            string     `json:"mode"`
	Status          string     `json:"status"`
	LastCheckStatus string     `json:"last_check_status"`
	LastCheckedAt   *time.Time `json:"last_checked_at"`
}

// ChannelState is one channel's set-up and this month's use.
type ChannelState struct {
	Channel string `json:"channel"`
	Status  string `json:"status"`
	// Account is the business's own account of the channel's kind, if one
	// is connected (the active one first).
	Account *Account `json:"account"`
	// Usage is the counted use this month (entitlements usage metric).
	Usage int64 `json:"usage"`
	// Sent / Failed / Skipped are this month's customer notification
	// attempts on the channel (sent includes delivered).
	Sent    int64 `json:"sent"`
	Failed  int64 `json:"failed"`
	Skipped int64 `json:"skipped"`
}

// PaymentState is the online payment set-up.
type PaymentState struct {
	Status             string   `json:"status"`
	Account            *Account `json:"account"`
	OnlineCashAccounts int64    `json:"online_cash_accounts"`
}

// ConfirmationState is the Konfirmasyon right, kept apart from the channel
// use it causes.
type ConfirmationState struct {
	// SectorOffers: the business's sector has the module.
	SectorOffers bool `json:"sector_offers"`
	// Granted: the add-on is assigned to the business.
	Granted bool `json:"granted"`
	// Active: the business may use the module (sector and add-on).
	Active bool `json:"active"`
	// This month's confirmation tasks by outcome.
	Total     int64 `json:"total"`
	Confirmed int64 `json:"confirmed"`
	Declined  int64 `json:"declined"`
	Expired   int64 `json:"expired"`
	Open      int64 `json:"open"`
}

// MonthVolume is one month's bookings.
type MonthVolume struct {
	Period       string `json:"period"`
	Reservations int64  `json:"reservations"`
}

// Reservations is the reservation volume (bookings by the time they were
// taken).
type Reservations struct {
	Total              int64         `json:"total"`
	ThisMonth          int64         `json:"this_month"`
	PreviousMonth      int64         `json:"previous_month"`
	CancelledThisMonth int64         `json:"cancelled_this_month"`
	Upcoming           int64         `json:"upcoming"`
	Monthly            []MonthVolume `json:"monthly"`
}

// Catalogue counts what the business set up.
type Catalogue struct {
	Customers int64 `json:"customers"`
	Services  int64 `json:"services"`
}

// Overview is the platform 360 view of a business.
type Overview struct {
	// Period / PreviousPeriod are months in the business time zone.
	Period         string `json:"period"`
	PreviousPeriod string `json:"previous_period"`
	// Entitlements: plan, add-ons, limits with use (branches, staff seats)
	// and this month's usage counters.
	Entitlements  entitlements.Summary   `json:"entitlements"`
	PreviousUsage []entitlements.Counter `json:"previous_usage"`
	Catalogue     Catalogue              `json:"catalogue"`
	Reservations  Reservations           `json:"reservations"`
	Confirmation  ConfirmationState      `json:"confirmation"`
	Channels      []ChannelState         `json:"channels"`
	Payment       PaymentState           `json:"payment"`
}

// Service builds overviews.
type Service struct {
	q        *db.Queries
	ent      *entitlements.Service
	channels Readiness
	payments PaymentReadiness
	now      func() time.Time
}

// New builds the service; channels and payments may be nil (every channel
// reads not ready, payments not connected / unavailable).
func New(q *db.Queries, ent *entitlements.Service, channels Readiness, payments PaymentReadiness) *Service {
	return &Service{q: q, ent: ent, channels: channels, payments: payments, now: time.Now}
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func period(d localtime.Date) string { return fmt.Sprintf("%04d-%02d", d.Year, int(d.Month)) }

func addMonths(d localtime.Date, n int) localtime.Date {
	t := time.Date(d.Year, d.Month+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	return localtime.NewDate(t.Year(), t.Month(), 1)
}

func accountOf(r db.ListOrganizationOverviewAccountsRow) *Account {
	a := &Account{
		UUID: r.Uuid, Provider: r.Provider, DisplayName: r.DisplayName, Mode: r.Mode,
		Status: r.Status, LastCheckStatus: r.LastCheckStatus,
	}
	if r.LastCheckedAt.Valid {
		v := r.LastCheckedAt.Time.UTC()
		a.LastCheckedAt = &v
	}
	return a
}

func channelKnown(name string) bool {
	return channel.IsKnown(name)
}

// Get builds the overview of a business (scope from PlatformScope).
func (s *Service) Get(ctx context.Context, scope orgctx.Scope) (Overview, error) {
	loc, err := localtime.LoadZone(localtime.ZoneOr(scope.Timezone))
	if err != nil {
		loc = localtime.Default()
	}
	now := s.now()
	today := localtime.DateIn(now, loc)
	month := localtime.NewDate(today.Year, today.Month, 1)
	prev, next := addMonths(month, -1), addMonths(month, 1)
	first := addMonths(month, -(volumeMonths - 1))
	monthStart, monthEnd, prevStart := month.StartIn(loc), next.StartIn(loc), prev.StartIn(loc)

	sum, err := s.ent.Summary(ctx, scope)
	if err != nil {
		return Overview{}, err
	}
	prevRows, err := s.q.ListOrganizationUsage(ctx, db.ListOrganizationUsageParams{
		OrganizationID: scope.ID, Period: pgtype.Date{Time: prev.Time(), Valid: true},
	})
	if err != nil {
		return Overview{}, err
	}
	prevBy := map[string]int64{}
	for _, r := range prevRows {
		prevBy[r.Metric] = r.Quantity
	}
	prevUsage := make([]entitlements.Counter, 0, len(entitlements.Metrics))
	for _, m := range entitlements.Metrics {
		prevUsage = append(prevUsage, entitlements.Counter{Metric: m, Quantity: prevBy[m]})
	}

	counts, err := s.q.CountOrganizationOverview(ctx, db.CountOrganizationOverviewParams{
		OrganizationID: scope.ID, MonthStart: ts(monthStart), MonthEnd: ts(monthEnd),
		PreviousMonthStart: ts(prevStart), Now: ts(now),
	})
	if err != nil {
		return Overview{}, err
	}
	volume, err := s.q.ListOrganizationReservationVolume(ctx, db.ListOrganizationReservationVolumeParams{
		Timezone: loc.String(), OrganizationID: scope.ID, FromTime: ts(first.StartIn(loc)),
	})
	if err != nil {
		return Overview{}, err
	}
	byMonth := map[string]int64{}
	for _, v := range volume {
		byMonth[v.Period] = v.Reservations
	}
	monthly := make([]MonthVolume, 0, volumeMonths)
	for i := 0; i < volumeMonths; i++ {
		p := period(addMonths(first, i))
		monthly = append(monthly, MonthVolume{Period: p, Reservations: byMonth[p]})
	}

	conf, err := s.q.CountOrganizationConfirmations(ctx, db.CountOrganizationConfirmationsParams{
		OrganizationID: scope.ID, FromTime: ts(monthStart), ToTime: ts(monthEnd),
	})
	if err != nil {
		return Overview{}, err
	}
	deliveries, err := s.q.CountOrganizationDeliveries(ctx, db.CountOrganizationDeliveriesParams{
		OrganizationID: scope.ID, FromTime: ts(monthStart), ToTime: ts(monthEnd),
	})
	if err != nil {
		return Overview{}, err
	}
	accounts, err := s.q.ListOrganizationOverviewAccounts(ctx, scope.ID)
	if err != nil {
		return Overview{}, err
	}
	// The query lists the active account of a kind first.
	byKind := map[string]db.ListOrganizationOverviewAccountsRow{}
	for _, a := range accounts {
		if _, ok := byKind[a.Kind]; !ok {
			byKind[a.Kind] = a
		}
	}

	usageBy := map[string]int64{}
	for _, c := range sum.Usage {
		usageBy[c.Metric] = c.Quantity
	}
	channels := make([]ChannelState, 0, len(Channels))
	for _, name := range Channels {
		st := ChannelState{Channel: name, Status: ChannelUnavailable, Usage: usageBy[name]}
		if channelKnown(name) {
			st.Status = ChannelNotReady
			if s.channels != nil && s.channels.Ready(name) {
				st.Status = ChannelReady
			}
		}
		if a, ok := byKind[name]; ok {
			st.Account = accountOf(a)
		}
		for _, d := range deliveries {
			if d.Channel != name {
				continue
			}
			switch d.Status {
			case "sent", "delivered":
				st.Sent += d.Deliveries
			case "failed":
				st.Failed += d.Deliveries
			case "skipped":
				st.Skipped += d.Deliveries
			}
		}
		channels = append(channels, st)
	}

	pay := PaymentState{Status: PaymentNotConnected}
	if a, ok := byKind[integrations.KindPayment]; ok {
		pay.Account = accountOf(a)
		if a.Status == "active" {
			n, err := s.q.CountOnlineCashAccounts(ctx, db.CountOnlineCashAccountsParams{OrganizationID: scope.ID, Provider: a.Provider})
			if err != nil {
				return Overview{}, err
			}
			pay.OnlineCashAccounts = n
			switch {
			case s.payments != nil && s.payments.OnlineReady(ctx, scope.ID):
				pay.Status = PaymentReady
			case n == 0:
				pay.Status = PaymentNoCashAccount
			default:
				pay.Status = PaymentProviderUnavailable
			}
		}
	}

	def := sector.Of(scope.Sector)
	return Overview{
		Period:         period(month),
		PreviousPeriod: period(prev),
		Entitlements:   sum,
		PreviousUsage:  prevUsage,
		Catalogue:      Catalogue{Customers: counts.Customers, Services: counts.Services},
		Reservations: Reservations{
			Total: counts.ReservationsTotal, ThisMonth: counts.ReservationsThisMonth,
			PreviousMonth: counts.ReservationsPreviousMonth, CancelledThisMonth: counts.ReservationsCancelledThisMonth,
			Upcoming: counts.ReservationsUpcoming, Monthly: monthly,
		},
		Confirmation: ConfirmationState{
			SectorOffers: def.Has(sector.ModuleConfirmation),
			Granted:      sum.HasAddon(entitlement.AddonConfirmation),
			Active:       sum.Allows(sector.ModuleConfirmation),
			Total:        conf.Total, Confirmed: conf.Confirmed, Declined: conf.Declined,
			Expired: conf.Expired, Open: conf.Open,
		},
		Channels: channels,
		Payment:  pay,
	}, nil
}
