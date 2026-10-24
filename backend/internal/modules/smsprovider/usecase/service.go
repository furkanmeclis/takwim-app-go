// Package usecase is the SMS channel over the business's own provider
// account (TEC-982, docs/design/confirmations.md "SMS and e-mail"): the
// customer notification channel that sends every notice through the
// business's Netgsm account (Channel) and the delivery report poll
// (Service.PollReports) that turns the provider's job reports into the
// engine's delivery reports.
//
//   - A send the provider took is "accepted" with the provider's job id as
//     the attempt's reference; the attempt stays open until the report.
//     A delivered report records delivered_at (never an answer: the
//     customer answers only through the visit link); a failed report runs
//     the rule's next channel; no report within the engine's receipt
//     timeout fails the attempt the same way (customernotify
//     ExpireReceipts).
//   - Netgsm pushes no SMS delivery report, so the worker asks the report
//     query for the open attempts of each business every minute (one call
//     per business, up to MaxReportJobs job ids). A provider webhook with
//     its own signature check is TEC-987's.
//   - Without the provider enabled in this process the channel falls back
//     to the platform sender handed to it (the development log sender,
//     which is never recorded as sent).
package usecase

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/netgsm"
)

// DefaultPollInterval is how often the worker asks for delivery reports.
const DefaultPollInterval = time.Minute

// DefaultBatch bounds the open attempts one poll looks at.
const DefaultBatch = 500

// Accounts reads integration accounts (the integration accounts service).
type Accounts interface {
	ActiveAccount(ctx context.Context, orgID int64, kind string) (integrations.Account, error)
}

// Recorder records delivery reports (the customer notification engine).
type Recorder interface {
	RecordDelivery(ctx context.Context, r channel.Report) (int64, bool, error)
}

// Client is the provider (netgsm.Provider).
type Client interface {
	Send(ctx context.Context, account integrations.Account, m netgsm.Message) (string, error)
	Reports(ctx context.Context, account integrations.Account, jobIDs []string) ([]netgsm.Report, error)
}

// Service is the SMS channel and its delivery report poll.
type Service struct {
	q        *db.Queries
	client   Client
	accounts Accounts
	reports  Recorder
	log      *slog.Logger
	now      func() time.Time
	batch    int32
}

// New builds the service. client nil = the provider is not enabled in this
// process (the channel falls back to the platform sender, the poll does
// nothing).
func New(q *db.Queries, client Client) *Service {
	if p, ok := client.(*netgsm.Provider); ok && p == nil {
		client = nil
	}
	return &Service{q: q, client: client, log: slog.Default(), now: time.Now, batch: DefaultBatch}
}

// SetAccounts plugs in the integration accounts (the integration kit is
// built after the notification engine).
func (s *Service) SetAccounts(a Accounts) { s.accounts = a }

// SetLogger replaces the logger.
func (s *Service) SetLogger(l *slog.Logger) {
	if l != nil {
		s.log = l
	}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// enabled reports whether texts go through the business's account.
func (s *Service) enabled() bool { return s.client != nil && s.accounts != nil }

// Run polls delivery reports every interval until ctx ends (the worker).
func (s *Service) Run(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = DefaultPollInterval
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := s.PollReports(ctx); err != nil && ctx.Err() == nil {
				s.log.WarnContext(ctx, "sms_reports_failed", "error", err)
			} else if n > 0 {
				s.log.InfoContext(ctx, "sms_reports_recorded", "count", n)
			}
		}
	}
}

// PollReports asks the provider for the delivery reports of the open SMS
// attempts of every business and records the final ones; it answers how
// many it recorded. An attempt still waiting is left for the next poll or
// the receipt timeout; a business whose account changed or fails the query
// is skipped (the timeout covers its attempts).
func (s *Service) PollReports(ctx context.Context) (int, error) {
	if !s.enabled() || s.reports == nil {
		return 0, nil
	}
	rows, err := s.q.ListOpenCustomerNotificationDeliveriesByProvider(ctx, db.ListOpenCustomerNotificationDeliveriesByProviderParams{
		Channel: channel.SMS, Provider: netgsm.ProviderCode, LimitCount: s.batch,
	})
	if err != nil {
		return 0, err
	}
	// Grouped by business (the rows come ordered by organization).
	n := 0
	for i := 0; i < len(rows); {
		j := i
		for j < len(rows) && rows[j].OrganizationID == rows[i].OrganizationID {
			j++
		}
		recorded, err := s.pollOrganization(ctx, rows[i:j])
		n += recorded
		if err != nil {
			if ctx.Err() != nil {
				return n, err
			}
			s.log.WarnContext(ctx, "sms_reports_organization_failed", "organization", rows[i].OrganizationID, "error", err)
		}
		i = j
	}
	return n, nil
}

// pollOrganization records the reports of one business's open attempts.
func (s *Service) pollOrganization(ctx context.Context, rows []db.ListOpenCustomerNotificationDeliveriesByProviderRow) (int, error) {
	orgID := rows[0].OrganizationID
	acc, err := s.accounts.ActiveAccount(ctx, orgID, integrations.KindSMS)
	if err != nil || acc.Provider != netgsm.ProviderCode {
		// No account any more: the receipt timeout closes the attempts.
		return 0, nil
	}
	n := 0
	for start := 0; start < len(rows); start += netgsm.MaxReportJobs {
		end := min(start+netgsm.MaxReportJobs, len(rows))
		chunk := rows[start:end]
		ids := make([]string, 0, len(chunk))
		for _, r := range chunk {
			ids = append(ids, r.ProviderRef)
		}
		reports, err := s.client.Reports(ctx, acc, ids)
		if err != nil {
			return n, err
		}
		byJob := map[string]netgsm.Report{}
		for _, r := range reports {
			// One recipient per job: the first final report wins.
			if prev, ok := byJob[r.JobID]; !ok || prev.Status == netgsm.StatusWaiting {
				byJob[r.JobID] = r
			}
		}
		for _, row := range chunk {
			r, ok := byJob[row.ProviderRef]
			if !ok || r.Status == netgsm.StatusWaiting {
				continue
			}
			id := row.Uuid
			rep := channel.Report{Channel: channel.SMS, OrganizationID: orgID, DeliveryUUID: &id, ProviderRef: r.JobID, At: r.DeliveredAt}
			if r.Failed() {
				rep.Error = r.Error()
			} else {
				rep.Delivered = true
			}
			if _, _, err := s.reports.RecordDelivery(ctx, rep); err != nil {
				if errors.Is(err, context.Canceled) {
					return n, err
				}
				s.log.WarnContext(ctx, "sms_report_record_failed", "organization", orgID, "delivery", row.Uuid, "error", err)
				continue
			}
			n++
		}
	}
	return n, nil
}
