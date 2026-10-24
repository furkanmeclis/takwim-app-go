// Package usecase is the customer notification engine (TEC-911,
// docs/design/customer-notifications.md). It works in every business,
// without the confirmation add-on (A28).
//
//   - ReservationChanged runs inside the transaction of a reservation
//     change (the reservations Notifier port) and writes the dispatch: the
//     outbox row commits with the change and never exists for a rolled
//     back one. The idempotency key (event + reservation + discriminator) is
//     unique per business, so a replayed change never writes a second one.
//   - The worker (Run) sends due dispatches every few seconds and plans the
//     reminders before the visit every minute.
//   - Process sends a dispatch on its next channel. Each channel is tried at
//     most once per dispatch (unique dispatch + channel): a retry never
//     sends the same notice twice on one channel. A failed or skipped
//     channel moves on to the next one when the rule allows fallback.
//   - A channel without an adapter or a ready provider is logged as
//     skipped, never as sent.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/templates"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
)

// Dispatch statuses.
const (
	DispatchPending   = "pending"
	DispatchSending   = "sending"
	DispatchDelivered = "delivered"
	DispatchFailed    = "failed"
	DispatchSkipped   = "skipped"
	DispatchCancelled = "cancelled"
)

// Delivery statuses.
const (
	DeliverySending   = "sending"
	DeliverySent      = "sent"
	DeliveryDelivered = "delivered"
	DeliveryFailed    = "failed"
	DeliverySkipped   = "skipped"
)

// Resource is the activity resource of rule / text changes.
const Resource = "tenant.customer_notifications"

var (
	// ErrUnknownEvent: the event code is not in the catalog.
	ErrUnknownEvent = errors.New("unknown notification event")
)

// UsageCounter counts what a business sent (organization_usage, monthly
// per channel).
type UsageCounter func(ctx context.Context, orgID int64, timezone, metric string)

// LinkIssuer gives the personal panel link of a notice at send time (the
// single-use panel link of TEC-910) for a channel and recipient. The stored
// text keeps the public panel address; an empty answer sends the public
// address.
type LinkIssuer func(ctx context.Context, ch string, to channel.Recipient, orgID, reservationID int64) (string, error)

// Engine is the customer notification engine.
type Engine struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	channels map[string]channel.Channel
	log      *slog.Logger
	now      func() time.Time
	panelURL string
	batch    int32
	usage    UsageCounter
	links    LinkIssuer
	// eventLinks replace links for one event (the confirmation request
	// carries a visit link, not a panel link).
	eventLinks map[string]LinkIssuer
	guard      DispatchGuard
}

// DispatchGuard tells, inside the claim transaction, why a dispatch must
// not go out any more ("" = send it); another module owns that decision
// (a confirmation request whose task was answered or planned again).
type DispatchGuard func(ctx context.Context, q *db.Queries, d db.CustomerNotificationDispatch) (string, error)

var _ reservationsusecase.Notifier = (*Engine)(nil)

// New builds the engine. panelURL is the public panel address the texts
// link to (takwim.link/panel).
func New(pool *pgxpool.Pool, q *db.Queries, panelURL string) *Engine {
	return &Engine{
		pool: pool, q: q, channels: map[string]channel.Channel{}, log: slog.Default(), now: time.Now,
		panelURL: strings.TrimSpace(panelURL), batch: 100, eventLinks: map[string]LinkIssuer{},
	}
}

// Register adds channel adapters (a later adapter of the same name
// replaces the earlier one). Reporting channels get the engine as their
// reporter.
func (e *Engine) Register(chs ...channel.Channel) {
	for _, c := range chs {
		if c == nil || !channel.IsKnown(c.Name()) {
			continue
		}
		e.channels[c.Name()] = c
		if rc, ok := c.(channel.ReportingChannel); ok {
			rc.SetReporter(e)
		}
	}
}

// Ready reports whether a channel has an adapter with a ready provider.
func (e *Engine) Ready(name string) bool {
	c, ok := e.channels[name]
	return ok && c.Ready()
}

// SetLogger replaces the logger.
func (e *Engine) SetLogger(l *slog.Logger) {
	if l != nil {
		e.log = l
	}
}

// SetClock replaces the clock (tests).
func (e *Engine) SetClock(now func() time.Time) { e.now = now }

// SetUsage counts real sends per business and channel.
func (e *Engine) SetUsage(u UsageCounter) { e.usage = u }

// SetLinkIssuer replaces the public panel address with a personal link at
// send time.
func (e *Engine) SetLinkIssuer(fn LinkIssuer) { e.links = fn }

// SetLinkIssuerFor replaces the link of one event at send time (nil
// removes it).
func (e *Engine) SetLinkIssuerFor(event string, fn LinkIssuer) {
	if fn == nil {
		delete(e.eventLinks, event)
		return
	}
	e.eventLinks[event] = fn
}

// SetDispatchGuard installs the guard every claim asks before sending.
func (e *Engine) SetDispatchGuard(fn DispatchGuard) { e.guard = fn }

// linkIssuer is the issuer of an event's personal link.
func (e *Engine) linkIssuer(event string) LinkIssuer {
	if fn, ok := e.eventLinks[event]; ok {
		return fn
	}
	return e.links
}

// ReservationNotice is a notice another module sends for a reservation
// with its own channels (the confirmation request): written in the
// caller's transaction like the reservation notices.
type ReservationNotice struct {
	Event string
	// Key makes the notice unique for the reservation (with Event).
	Key      string
	Channels []string
	Fallback bool
	// DueAt is the visit start the notice is about: a moved, cancelled or
	// started visit drops it at send time.
	DueAt *time.Time
	// Vars are added to the reservation variables.
	Vars map[string]string
}

// EmitReservation writes the dispatch of a notice for the customer of a
// reservation. created is false when the key was already used.
func (e *Engine) EmitReservation(ctx context.Context, q *db.Queries, orgID, reservationID int64, n ReservationNotice) (bool, error) {
	if _, ok := catalog.Get(n.Event); !ok {
		return false, ErrUnknownEvent
	}
	r, err := q.GetNotifyReservation(ctx, db.GetNotifyReservationParams{OrganizationID: orgID, ID: reservationID})
	if err != nil {
		return false, fmt.Errorf("customer notice: reservation: %w", err)
	}
	out, err := e.emit(ctx, q, orgID, emitInput{
		event: n.Event, key: n.Key, reservation: r, dueAt: n.DueAt,
		rule: &Rule{Event: n.Event, Enabled: true, Channels: n.Channels, Fallback: n.Fallback}, vars: n.Vars,
	})
	return out.created, err
}

func (e *Engine) clock() time.Time { return e.now().UTC().Truncate(time.Microsecond) }

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// Rule is an event's effective rule (business override or default).
type Rule struct {
	Event    string
	Enabled  bool
	Channels []string
	Fallback bool
	Offsets  []int32
	Custom   bool
	Updated  *time.Time
}

func defaultRule(ev catalog.Event) Rule {
	return Rule{
		Event: ev.Code, Enabled: true, Channels: append([]string(nil), ev.Channels...), Fallback: true,
		Offsets: append([]int32(nil), ev.Offsets...),
	}
}

// RuleFor returns an event's effective rule.
func (e *Engine) RuleFor(ctx context.Context, q *db.Queries, orgID int64, code string) (Rule, error) {
	ev, ok := catalog.Get(code)
	if !ok {
		return Rule{}, ErrUnknownEvent
	}
	row, err := q.GetCustomerNotificationRule(ctx, db.GetCustomerNotificationRuleParams{OrganizationID: orgID, EventCode: code})
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultRule(ev), nil
	}
	if err != nil {
		return Rule{}, err
	}
	t := row.UpdatedAt.Time
	return Rule{
		Event: code, Enabled: row.Enabled, Channels: row.Channels, Fallback: row.Fallback, Offsets: row.ReminderOffsets,
		Custom: true, Updated: &t,
	}, nil
}

// ReservationChanged implements the reservations Notifier: it runs in the
// change's transaction (q).
func (e *Engine) ReservationChanged(ctx context.Context, q *db.Queries, c reservationsusecase.Change) error {
	var code, key string
	switch c.Type {
	case "created":
		switch c.To {
		case "requested":
			code = catalog.ReservationRequested
		case "active":
			code = catalog.ReservationAccepted
		}
	case "accepted", "paid":
		// Accepted once per visit: a payment after the acceptance (or after
		// an automatic approval's request notice) sends it only when it was
		// not sent yet (same key).
		code = catalog.ReservationAccepted
	case "rejected":
		code = catalog.ReservationRejected
	case "cancelled", "expired":
		code = catalog.ReservationCancelled
	case "rescheduled":
		code, key = catalog.ReservationRescheduled, "v"+strconv.Itoa(int(c.Version))
	}
	if code == "" {
		return nil
	}
	r, err := q.GetNotifyReservation(ctx, db.GetNotifyReservationParams{OrganizationID: c.OrganizationID, ID: c.ReservationID})
	if err != nil {
		return fmt.Errorf("customer notice: reservation: %w", err)
	}
	// A visit entered after its time (a walk-in recorded later) gets no
	// notice; a cancellation of a past visit neither.
	if !r.StartsAt.Time.After(e.clock()) {
		return nil
	}
	_, err = e.emit(ctx, q, c.OrganizationID, emitInput{event: code, key: key, reservation: r})
	return err
}

type emitInput struct {
	event       string
	key         string
	reservation db.GetNotifyReservationRow
	dueAt       *time.Time
	// rule, when set, replaces the business's rule of the event.
	rule *Rule
	vars map[string]string
}

// emitted is the result of an emit.
type emitted struct {
	dispatch db.CustomerNotificationDispatch
	// created is false when the notice already exists (same key) or the
	// business turned the event off.
	created bool
}

func idempotencyKey(event string, reservation uuid.UUID, key string) string {
	k := event + "|reservation:" + reservation.String() + "|" + key
	if len(k) > 200 {
		k = k[:200]
	}
	return k
}

// emit writes the dispatch of one event for the customer of a reservation.
func (e *Engine) emit(ctx context.Context, q *db.Queries, orgID int64, in emitInput) (emitted, error) {
	var rule Rule
	if in.rule != nil {
		rule = *in.rule
	} else {
		var err error
		if rule, err = e.RuleFor(ctx, q, orgID, in.event); err != nil {
			return emitted{}, err
		}
	}
	if !rule.Enabled {
		return emitted{}, nil
	}
	org, err := q.GetNotifyOrganization(ctx, orgID)
	if err != nil {
		return emitted{}, fmt.Errorf("customer notice: organization: %w", err)
	}
	r := in.reservation
	customer, err := q.GetNotifyCustomer(ctx, db.GetNotifyCustomerParams{OrganizationID: orgID, ID: r.CustomerID})
	if err != nil {
		return emitted{}, fmt.Errorf("customer notice: customer: %w", err)
	}
	vars := e.reservationVars(org.Name, customer, r)
	for k, v := range in.vars {
		vars[k] = v
	}
	p := db.InsertCustomerNotificationDispatchParams{
		OrganizationID: orgID, EventCode: in.event, CustomerID: r.CustomerID,
		ReservationID: pgtype.Int8{Int64: r.ID, Valid: true}, ReservationVersion: r.Version,
		IdempotencyKey: idempotencyKey(in.event, r.Uuid, in.key), Channels: filterChannels(rule.Channels),
		Fallback: rule.Fallback, Status: DispatchPending, ScheduledAt: ts(e.clock()),
	}
	if in.dueAt != nil {
		p.DueAt = ts(in.dueAt.UTC())
	}
	if len(p.Channels) == 0 {
		p.Status, p.LastError, p.CompletedAt = DispatchSkipped, "no channel configured", ts(e.clock())
	}
	if p.Vars, err = json.Marshal(vars); err != nil {
		return emitted{}, err
	}
	row, err := q.InsertCustomerNotificationDispatch(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return emitted{}, nil
	}
	if err != nil {
		return emitted{}, err
	}
	return emitted{dispatch: row, created: true}, nil
}

func (e *Engine) reservationVars(business string, c db.GetNotifyCustomerRow, r db.GetNotifyReservationRow) map[string]string {
	loc, err := time.LoadLocation(r.Timezone)
	if err != nil {
		loc, _ = time.LoadLocation("Europe/Istanbul")
	}
	if loc == nil {
		loc = time.UTC
	}
	start := r.StartsAt.Time.In(loc)
	return map[string]string{
		"customer_name": strings.TrimSpace(c.FirstName + " " + c.LastName),
		"business_name": business,
		"branch_name":   r.BranchName,
		"reference":     strconv.FormatInt(r.Number, 10),
		"starts_at":     start.Format("02.01.2006 15:04"),
		"date":          start.Format("02.01.2006"),
		"time":          start.Format("15:04"),
		"services":      r.Services,
		"link":          e.panelURL,
	}
}

func filterChannels(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, c := range in {
		if channel.IsKnown(c) && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// step is one claimed channel attempt.
type step struct {
	dispatch db.CustomerNotificationDispatch
	delivery db.CustomerNotificationDelivery
	channel  string
	business string
	timezone string
}

// Process sends a dispatch on its next channel, moving on while channels
// fail and fallback is on. It returns when the dispatch is delivered,
// waits for a delivery report, or has no channel left.
func (e *Engine) Process(ctx context.Context, orgID, dispatchID int64) error {
	for i := 0; i < len(channel.Known)+1; i++ {
		st, err := e.claim(ctx, orgID, dispatchID)
		if err != nil || st == nil {
			return err
		}
		res := e.send(ctx, orgID, st)
		next, err := e.record(ctx, orgID, st, res)
		if err != nil || !next {
			return err
		}
	}
	return nil
}

func (e *Engine) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	if e.pool == nil {
		return fn(e.q)
	}
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(e.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// stale tells why a dispatch must not go out any more ("" = send it): the
// business is suspended or gone, or a reminder's visit moved or ended.
func (e *Engine) stale(ctx context.Context, q *db.Queries, orgID int64, d db.CustomerNotificationDispatch) (string, db.GetNotifyOrganizationRow, error) {
	org, err := q.GetNotifyOrganization(ctx, orgID)
	if err != nil {
		return "", org, err
	}
	if org.Deleted || org.Status != "active" {
		return "business not active", org, nil
	}
	if e.guard != nil {
		reason, err := e.guard(ctx, q, d)
		if err != nil || reason != "" {
			return reason, org, err
		}
	}
	if d.DueAt.Valid && d.ReservationID.Valid {
		r, err := q.GetNotifyReservation(ctx, db.GetNotifyReservationParams{OrganizationID: orgID, ID: d.ReservationID.Int64})
		if errors.Is(err, pgx.ErrNoRows) {
			return "reservation not found", org, nil
		}
		if err != nil {
			return "", org, err
		}
		if r.Status != "active" || !r.StartsAt.Time.Equal(d.DueAt.Time) {
			return "reservation changed", org, nil
		}
		if !r.StartsAt.Time.After(e.clock()) {
			return "visit started", org, nil
		}
	}
	return "", org, nil
}

// claim locks the dispatch and inserts the attempt row of its next channel
// (nil when there is nothing to send).
func (e *Engine) claim(ctx context.Context, orgID, dispatchID int64) (*step, error) {
	var out *step
	err := e.tx(ctx, func(q *db.Queries) error {
		d, err := q.LockCustomerNotificationDispatch(ctx, db.LockCustomerNotificationDispatchParams{OrganizationID: orgID, ID: dispatchID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // another worker sends it, or it is gone
		}
		if err != nil {
			return err
		}
		if d.Status != DispatchPending {
			return nil
		}
		reason, org, err := e.stale(ctx, q, orgID, d)
		if err != nil {
			return err
		}
		if reason != "" {
			return q.UpdateCustomerNotificationDispatchState(ctx, db.UpdateCustomerNotificationDispatchStateParams{
				OrganizationID: orgID, ID: d.ID, Status: DispatchCancelled, NextChannelIndex: d.NextChannelIndex,
				LastError: reason, CompletedAt: ts(e.clock()),
			})
		}
		idx := d.NextChannelIndex
		for int(idx) < len(d.Channels) {
			ch := d.Channels[idx]
			subject, body := e.render(ctx, q, orgID, d, ch, "")
			dl, err := q.InsertCustomerNotificationDelivery(ctx, db.InsertCustomerNotificationDeliveryParams{
				OrganizationID: orgID, DispatchID: d.ID, Channel: ch, AttemptNo: idx + 1,
				Subject: truncate(subject, catalog.MaxSubject), Body: body,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				idx++ // this channel was already tried for this dispatch
				continue
			}
			if err != nil {
				return err
			}
			if err := q.UpdateCustomerNotificationDispatchState(ctx, db.UpdateCustomerNotificationDispatchStateParams{
				OrganizationID: orgID, ID: d.ID, Status: DispatchSending, NextChannelIndex: idx, LastError: d.LastError,
			}); err != nil {
				return err
			}
			d.Status, d.NextChannelIndex = DispatchSending, idx
			out = &step{dispatch: d, delivery: dl, channel: ch, business: org.Name, timezone: org.Timezone}
			return nil
		}
		if d.LastError == "" {
			d.LastError = "no channel left"
		}
		return q.UpdateCustomerNotificationDispatchState(ctx, db.UpdateCustomerNotificationDispatchStateParams{
			OrganizationID: orgID, ID: d.ID, Status: DispatchFailed, NextChannelIndex: idx, LastError: d.LastError,
			CompletedAt: ts(e.clock()),
		})
	})
	return out, err
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// render fills the business's text of the channel (else the catalog
// default) with the dispatch variables; link, when set, replaces the
// stored panel address.
func (e *Engine) render(ctx context.Context, q *db.Queries, orgID int64, d db.CustomerNotificationDispatch, ch, link string) (string, string) {
	vars := map[string]string{}
	_ = json.Unmarshal(d.Vars, &vars)
	if link != "" {
		vars["link"] = link
	}
	subject, body := e.templateFor(ctx, q, orgID, d.EventCode, ch)
	return templates.Render(subject, vars), templates.Render(body, vars)
}

func (e *Engine) templateFor(ctx context.Context, q *db.Queries, orgID int64, code, ch string) (string, string) {
	row, err := q.GetCustomerNotificationTemplate(ctx, db.GetCustomerNotificationTemplateParams{
		OrganizationID: orgID, EventCode: code, Channel: ch,
	})
	if err == nil {
		return row.Subject, row.Body
	}
	ev, _ := catalog.Get(code)
	t := catalog.DefaultTemplate(ev, ch)
	return t.Subject, t.Body
}

func (e *Engine) send(ctx context.Context, orgID int64, st *step) channel.Result {
	c, ok := e.channels[st.channel]
	if !ok {
		return channel.Skipped("", "channel not available")
	}
	cust, err := e.q.GetNotifyCustomer(ctx, db.GetNotifyCustomerParams{OrganizationID: orgID, ID: st.dispatch.CustomerID})
	if err != nil {
		return channel.Skipped("", "customer not found")
	}
	if cust.Deleted {
		return channel.Skipped("", "customer deleted")
	}
	msg := channel.Message{
		DeliveryUUID: st.delivery.Uuid, Event: st.dispatch.EventCode,
		Recipient: channel.Recipient{
			CustomerID: cust.ID, Name: strings.TrimSpace(cust.FirstName + " " + cust.LastName),
			Phone: cust.PhoneE164.String, Email: strings.TrimSpace(cust.Email),
		},
		Subject: st.delivery.Subject, Body: st.delivery.Body, BusinessName: st.business, OrganizationID: orgID,
		ReservationID: st.dispatch.ReservationID.Int64,
	}
	// A personal link is put in at send time only, and only when the channel
	// really delivers (a log sender never uses up a link); the stored text keeps
	// the public panel address.
	if issue := e.linkIssuer(st.dispatch.EventCode); issue != nil && c.Ready() {
		link, err := issue(ctx, st.channel, msg.Recipient, orgID, st.dispatch.ReservationID.Int64)
		if err != nil {
			e.log.WarnContext(ctx, "customer_notice_link_failed", "dispatch", st.dispatch.Uuid, "error", err)
		} else if link != "" {
			msg.Subject, msg.Body = e.render(ctx, e.q, orgID, st.dispatch, st.channel, link)
			msg.Subject = truncate(msg.Subject, catalog.MaxSubject)
		}
	}
	res, err := c.Send(ctx, msg)
	if err != nil {
		res.Outcome = channel.OutcomeFailed
		if res.Error == "" {
			res.Error = err.Error()
		}
	}
	if res.Outcome == "" {
		res.Outcome = channel.OutcomeFailed
	}
	// A channel without a ready provider may log the message (development)
	// but never counts as a send.
	if !c.Ready() && (res.Outcome == channel.OutcomeSent || res.Outcome == channel.OutcomeDelivered || res.Outcome == channel.OutcomeAccepted) {
		res.Outcome, res.ExternalUUID = channel.OutcomeSkipped, nil
		if res.Error == "" {
			res.Error = "provider not configured"
		}
	}
	switch res.Outcome {
	case channel.OutcomeSent, channel.OutcomeDelivered, channel.OutcomeAccepted:
		if e.usage != nil {
			e.usage(ctx, orgID, st.timezone, st.channel)
		}
	}
	return res
}

// record writes the attempt's result and reports whether the next channel
// should run now.
func (e *Engine) record(ctx context.Context, orgID int64, st *step, res channel.Result) (bool, error) {
	now := e.clock()
	p := db.UpdateCustomerNotificationDeliveryParams{
		OrganizationID: orgID, ID: st.delivery.ID, Provider: truncate(res.Provider, 32),
		ProviderRef: truncate(res.ProviderRef, 128), RecipientMasked: truncate(res.RecipientMasked, 255), Error: res.Error,
	}
	if res.ExternalUUID != nil {
		p.ExternalUuid = pgtype.UUID{Bytes: *res.ExternalUUID, Valid: true}
	}
	switch res.Outcome {
	case channel.OutcomeAccepted:
		p.Status, p.SentAt = DeliverySent, ts(now)
		// The dispatch stays "sending" until the report.
		_, err := e.q.UpdateCustomerNotificationDelivery(ctx, p)
		return false, err
	case channel.OutcomeDelivered, channel.OutcomeSent:
		p.Status, p.SentAt, p.FinishedAt = DeliverySent, ts(now), ts(now)
		if res.Outcome == channel.OutcomeDelivered {
			p.Status = DeliveryDelivered
		}
		if n, err := e.q.UpdateCustomerNotificationDelivery(ctx, p); err != nil || n == 0 {
			return false, err
		}
		return false, e.q.UpdateCustomerNotificationDispatchState(ctx, db.UpdateCustomerNotificationDispatchStateParams{
			OrganizationID: orgID, ID: st.dispatch.ID, Status: DispatchDelivered, NextChannelIndex: st.dispatch.NextChannelIndex,
			CompletedAt: ts(now),
		})
	default:
		p.Status, p.FinishedAt = DeliveryFailed, ts(now)
		if res.Outcome == channel.OutcomeSkipped {
			p.Status = DeliverySkipped
		}
		if n, err := e.q.UpdateCustomerNotificationDelivery(ctx, p); err != nil || n == 0 {
			return false, err
		}
		return e.advance(ctx, e.q, orgID, st.dispatch, st.channel, res.Error)
	}
}

// advance moves a dispatch past a failed channel: to the next channel
// (pending) when fallback allows, else failed (skipped when every attempt
// was skipped).
func (e *Engine) advance(ctx context.Context, q *db.Queries, orgID int64, d db.CustomerNotificationDispatch, ch, reason string) (bool, error) {
	lastErr := ch + ": " + reason
	next := d.NextChannelIndex + 1
	if d.Fallback && int(next) < len(d.Channels) {
		return true, q.UpdateCustomerNotificationDispatchState(ctx, db.UpdateCustomerNotificationDispatchStateParams{
			OrganizationID: orgID, ID: d.ID, Status: DispatchPending, NextChannelIndex: next, LastError: lastErr,
		})
	}
	status := DispatchFailed
	if rows, err := q.ListCustomerNotificationDeliveriesForDispatch(ctx, db.ListCustomerNotificationDeliveriesForDispatchParams{
		OrganizationID: orgID, DispatchID: d.ID,
	}); err == nil {
		all := len(rows) > 0
		for _, r := range rows {
			if r.Status != DeliverySkipped {
				all = false
			}
		}
		if all {
			status = DispatchSkipped
		}
	}
	return false, q.UpdateCustomerNotificationDispatchState(ctx, db.UpdateCustomerNotificationDispatchStateParams{
		OrganizationID: orgID, ID: d.ID, Status: status, NextChannelIndex: d.NextChannelIndex, LastError: lastErr,
		CompletedAt: ts(e.clock()),
	})
}

// ReportDelivery implements channel.Reporter: the final result of an
// accepted message. A failure runs the next channel (fallback).
func (e *Engine) ReportDelivery(ctx context.Context, r channel.Report) {
	orgID := r.OrganizationID
	var (
		dl  db.CustomerNotificationDelivery
		err error
	)
	if r.DeliveryUUID != nil {
		dl, err = e.q.GetCustomerNotificationDeliveryByUUID(ctx, db.GetCustomerNotificationDeliveryByUUIDParams{OrganizationID: orgID, Uuid: *r.DeliveryUUID})
	} else {
		dl, err = e.q.GetCustomerNotificationDeliveryByExternal(ctx, db.GetCustomerNotificationDeliveryByExternalParams{
			OrganizationID: orgID, Channel: r.Channel, ExternalUuid: pgtype.UUID{Bytes: r.ExternalUUID, Valid: true},
		})
	}
	if err != nil || dl.Channel != r.Channel {
		return
	}
	status := DeliveryDelivered
	if !r.Delivered {
		status = DeliveryFailed
	}
	now := e.clock()
	next := false
	err = e.tx(ctx, func(q *db.Queries) error {
		d, err := q.LockCustomerNotificationDispatchWait(ctx, db.LockCustomerNotificationDispatchWaitParams{OrganizationID: orgID, ID: dl.DispatchID})
		if err != nil {
			return err
		}
		n, err := q.FinishCustomerNotificationDelivery(ctx, db.FinishCustomerNotificationDeliveryParams{
			OrganizationID: orgID, ID: dl.ID, Status: status, Error: r.Error, ProviderRef: truncate(r.ProviderRef, 128),
			FinishedAt: ts(now),
		})
		if err != nil || n == 0 {
			return err
		}
		// Only the attempt the dispatch waits for moves it.
		if d.Status != DispatchSending || int(d.NextChannelIndex) >= len(d.Channels) || d.Channels[d.NextChannelIndex] != dl.Channel {
			return nil
		}
		if r.Delivered {
			return q.UpdateCustomerNotificationDispatchState(ctx, db.UpdateCustomerNotificationDispatchStateParams{
				OrganizationID: orgID, ID: d.ID, Status: DispatchDelivered, NextChannelIndex: d.NextChannelIndex,
				LastError: d.LastError, CompletedAt: ts(now),
			})
		}
		reason := r.Error
		if reason == "" {
			reason = "not delivered"
		}
		next, err = e.advance(ctx, q, orgID, d, dl.Channel, reason)
		return err
	})
	if err != nil {
		e.log.WarnContext(ctx, "customer_notice_report_failed", "delivery", dl.Uuid, "error", err)
		return
	}
	if next {
		if err := e.Process(ctx, orgID, dl.DispatchID); err != nil {
			e.log.WarnContext(ctx, "customer_notice_fallback_failed", "delivery", dl.Uuid, "error", err)
		}
	}
}

// ProcessDue sends every due dispatch and returns how many were processed.
func (e *Engine) ProcessDue(ctx context.Context) (int, error) {
	orgs, err := e.q.ListDueCustomerNotificationOrganizations(ctx, ts(e.clock()))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, orgID := range orgs {
		m, err := e.ProcessDueOrganization(ctx, orgID)
		if err != nil {
			e.log.WarnContext(ctx, "customer_notice_due_failed", "organization", orgID, "error", err)
		}
		n += m
	}
	return n, nil
}

// ProcessDueOrganization sends one business's due dispatches.
func (e *Engine) ProcessDueOrganization(ctx context.Context, orgID int64) (int, error) {
	ids, err := e.q.ListDueCustomerNotificationDispatches(ctx, db.ListDueCustomerNotificationDispatchesParams{
		OrganizationID: orgID, NowAt: ts(e.clock()), LimitCount: e.batch,
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if err := e.Process(ctx, orgID, id); err != nil {
			e.log.WarnContext(ctx, "customer_notice_process_failed", "organization", orgID, "dispatch", id, "error", err)
			continue
		}
		n++
	}
	return n, nil
}

// offsetDue picks the reminder to send for a visit: the smallest offset
// whose moment has passed, among those planned after the booking was made
// (a booking made 2 hours before its time gets no 24 hour reminder). A
// later reminder supersedes the missed earlier ones, so a customer never
// gets two at once.
func offsetDue(offsets []int32, start, created, now time.Time) (int32, bool) {
	best, ok := int32(0), false
	for _, o := range offsets {
		if o < catalog.MinOffsetMinutes || o > catalog.MaxOffsetMinutes {
			continue
		}
		at := start.Add(-time.Duration(o) * time.Minute)
		if now.Before(at) || !created.Before(at) {
			continue
		}
		if !ok || o < best {
			best, ok = o, true
		}
	}
	return best, ok
}

func maxOffset(offsets []int32) time.Duration {
	m := int32(0)
	for _, o := range offsets {
		if o > m && o <= catalog.MaxOffsetMinutes {
			m = o
		}
	}
	return time.Duration(m) * time.Minute
}

// Reminders writes the due reminder dispatches of every business. Keys
// (offset + visit start) make every run safe to repeat; a moved visit gets
// its own reminders.
func (e *Engine) Reminders(ctx context.Context) (int, error) {
	now := e.clock()
	orgs, err := e.q.ListNotifyReminderOrganizations(ctx, db.ListNotifyReminderOrganizationsParams{
		NowAt: ts(now), UntilAt: ts(now.Add(time.Duration(catalog.MaxOffsetMinutes) * time.Minute)),
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, orgID := range orgs {
		m, err := e.RemindOrganization(ctx, orgID)
		if err != nil {
			e.log.WarnContext(ctx, "customer_notice_reminders_failed", "organization", orgID, "error", err)
		}
		n += m
	}
	return n, nil
}

// RemindOrganization writes one business's due reminder dispatches.
func (e *Engine) RemindOrganization(ctx context.Context, orgID int64) (int, error) {
	now := e.clock()
	rule, err := e.RuleFor(ctx, e.q, orgID, catalog.ReservationReminder)
	if err != nil || !rule.Enabled || len(rule.Offsets) == 0 {
		return 0, err
	}
	rows, err := e.q.ListNotifyReminderCandidates(ctx, db.ListNotifyReminderCandidatesParams{
		OrganizationID: orgID, NowAt: ts(now), UntilAt: ts(now.Add(maxOffset(rule.Offsets))),
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, c := range rows {
		o, ok := offsetDue(rule.Offsets, c.StartsAt.Time, c.CreatedAt.Time, now)
		if !ok {
			continue
		}
		r, err := e.q.GetNotifyReservation(ctx, db.GetNotifyReservationParams{OrganizationID: orgID, ID: c.ID})
		if err != nil {
			return n, err
		}
		due := r.StartsAt.Time
		out, err := e.emit(ctx, e.q, orgID, emitInput{
			event: catalog.ReservationReminder, reservation: r, dueAt: &due,
			key: "remind:" + strconv.Itoa(int(o)) + ":" + strconv.FormatInt(due.Unix(), 10),
		})
		if err != nil {
			return n, err
		}
		if out.created {
			n++
		}
	}
	return n, nil
}

// Intervals of Run.
const (
	DefaultSendInterval     = 5 * time.Second
	DefaultReminderInterval = time.Minute
)

// Run sends due notices every few seconds and plans reminders every minute
// until ctx ends (the worker).
func (e *Engine) Run(ctx context.Context, sendEvery, remindEvery time.Duration) {
	if sendEvery <= 0 {
		sendEvery = DefaultSendInterval
	}
	if remindEvery <= 0 {
		remindEvery = DefaultReminderInterval
	}
	send := time.NewTicker(sendEvery)
	defer send.Stop()
	remind := time.NewTicker(remindEvery)
	defer remind.Stop()
	e.sweep(ctx, true)
	for {
		select {
		case <-ctx.Done():
			return
		case <-remind.C:
			e.sweep(ctx, true)
		case <-send.C:
			e.sweep(ctx, false)
		}
	}
}

func (e *Engine) sweep(ctx context.Context, reminders bool) {
	if reminders {
		if n, err := e.Reminders(ctx); err != nil && ctx.Err() == nil {
			e.log.WarnContext(ctx, "customer_notice_reminders_failed", "error", err)
		} else if n > 0 {
			e.log.InfoContext(ctx, "customer_notice_reminders_planned", "count", n)
		}
	}
	if _, err := e.ProcessDue(ctx); err != nil && ctx.Err() == nil {
		e.log.WarnContext(ctx, "customer_notice_send_failed", "error", err)
	}
}
