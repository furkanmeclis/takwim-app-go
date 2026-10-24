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
//   - A channel with delivery reports (WhatsApp, TEC-922; voice calls,
//     TEC-923) keeps its attempt open until the report; delivery and read
//     times are recorded apart from the acceptance. A report that never
//     comes fails the attempt after the receipt timeout so the next channel
//     runs. A call picked up without a valid key is delivered, yet the next
//     channel runs too (it is no answer).
//   - An attempt left "sending" by a process that stopped between the
//     claim and the record is closed by the stuck sweep (CloseStuck) after
//     the stuck timeout: failed with an unknown outcome, never sent again
//     and no next channel (the customer may have got it).
//   - Events set with SetResourceEvents (the confirmation request, TEC-981)
//     tell the panel's live reservation lists about every send result and
//     delivery report of their dispatches, so the confirmation list, its
//     badge and alerts see a failed or skipped request right away.
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
	reservationsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
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
	// receiptTimeout fails an accepted attempt whose delivery report never
	// came.
	receiptTimeout time.Duration
	// stuckTimeout closes an attempt that never got its send result.
	stuckTimeout time.Duration
	// events publishes the outcomes of the dispatches of liveEvents on the
	// reservations live channel.
	events     resourceevents.Publisher
	liveEvents map[string]bool
}

// DefaultReceiptTimeout is how long an accepted message may wait for its
// delivery report before the attempt counts as failed.
const DefaultReceiptTimeout = 30 * time.Minute

// DefaultStuckTimeout is how long a claimed attempt may wait for its send
// result before the stuck sweep closes it. A send takes seconds (provider
// calls have their own timeouts), so an attempt this old belongs to a
// process that stopped.
const DefaultStuckTimeout = 15 * time.Minute

// ErrorOutcomeUnknown is the error of an attempt the stuck sweep closed:
// the provider may or may not have delivered it.
const ErrorOutcomeUnknown = "outcome unknown"

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
		receiptTimeout: DefaultReceiptTimeout, stuckTimeout: DefaultStuckTimeout,
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

// ReadyFor reports whether a channel can deliver for one business: a
// channel on the business's own provider account (WhatsApp) also needs a
// working account.
func (e *Engine) ReadyFor(ctx context.Context, orgID int64, name string) bool {
	c, ok := e.channels[name]
	if !ok || !c.Ready() {
		return false
	}
	if oc, ok := c.(channel.OrganizationChannel); ok {
		return oc.ReadyFor(ctx, orgID)
	}
	return true
}

// SetReceiptTimeout replaces the receipt timeout (0 keeps the default).
func (e *Engine) SetReceiptTimeout(d time.Duration) {
	if d > 0 {
		e.receiptTimeout = d
	}
}

// SetStuckTimeout replaces the stuck timeout (0 keeps the default).
func (e *Engine) SetStuckTimeout(d time.Duration) {
	if d > 0 {
		e.stuckTimeout = d
	}
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

// SetResourceEvents publishes tenant.reservations "updated" for the
// reservation of a dispatch of one of the events whenever its send result
// or a delivery report is recorded (TEC-981: the confirmation list shows
// the last request's outcome). Writing the dispatch is published by the
// change that wrote it.
func (e *Engine) SetResourceEvents(pub resourceevents.Publisher, events ...string) {
	e.events, e.liveEvents = pub, map[string]bool{}
	for _, ev := range events {
		e.liveEvents[ev] = true
	}
}

// changed tells the live lists that a dispatch's outcome moved.
func (e *Engine) changed(ctx context.Context, orgID int64, d db.CustomerNotificationDispatch) {
	if e.events == nil || !e.liveEvents[d.EventCode] || !d.ReservationID.Valid {
		return
	}
	r, err := e.q.GetNotifyReservation(ctx, db.GetNotifyReservationParams{OrganizationID: orgID, ID: d.ReservationID.Int64})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			e.log.WarnContext(ctx, "customer_notice_event_failed", "dispatch", d.Uuid, "error", err)
		}
		return
	}
	e.events.Publish(orgctx.WithOrganizationID(ctx, orgID), reservationsmodel.Slug, resourceevents.ActionUpdated, r.Uuid.String())
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

// EmitEvent writes the notice of a catalog event for the customer of a
// reservation with the business's rule for the event (the refund notices,
// TEC-978), in the caller's transaction. key makes it unique for the
// reservation (one notice per refund). created is false when the key was
// used already or the business turned the event off.
func (e *Engine) EmitEvent(ctx context.Context, q *db.Queries, orgID, reservationID int64, event, key string) (bool, error) {
	if _, ok := catalog.Get(event); !ok {
		return false, ErrUnknownEvent
	}
	r, err := q.GetNotifyReservation(ctx, db.GetNotifyReservationParams{OrganizationID: orgID, ID: reservationID})
	if err != nil {
		return false, fmt.Errorf("customer notice: reservation: %w", err)
	}
	out, err := e.emit(ctx, q, orgID, emitInput{event: event, key: key, reservation: r})
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
	// The outcome is published once, after the last step.
	var moved *db.CustomerNotificationDispatch
	defer func() {
		if moved != nil {
			e.changed(ctx, orgID, *moved)
		}
	}()
	for i := 0; i < len(channel.Known)+1; i++ {
		st, ended, err := e.claim(ctx, orgID, dispatchID)
		if ended != nil {
			moved = ended
		}
		if err != nil || st == nil {
			return err
		}
		moved = &st.dispatch
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
// (nil when there is nothing to send). ended is the dispatch it closed
// instead (cancelled, or failed with no channel left).
func (e *Engine) claim(ctx context.Context, orgID, dispatchID int64) (*step, *db.CustomerNotificationDispatch, error) {
	var (
		out    *step
		closed *db.CustomerNotificationDispatch
	)
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
			closed = &d
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
		closed = &d
		return q.UpdateCustomerNotificationDispatchState(ctx, db.UpdateCustomerNotificationDispatchStateParams{
			OrganizationID: orgID, ID: d.ID, Status: DispatchFailed, NextChannelIndex: idx, LastError: d.LastError,
			CompletedAt: ts(e.clock()),
		})
	})
	if err != nil {
		return nil, nil, err
	}
	return out, closed, nil
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
		ReservationID: st.dispatch.ReservationID.Int64, Vars: map[string]string{},
	}
	_ = json.Unmarshal(st.dispatch.Vars, &msg.Vars)
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
// should run now. The attempt and its dispatch change in one transaction
// (a process that stops in between leaves neither half written), locking
// the dispatch first like the delivery reports and the stuck sweep.
func (e *Engine) record(ctx context.Context, orgID int64, st *step, res channel.Result) (bool, error) {
	next := false
	err := e.tx(ctx, func(q *db.Queries) error {
		_, err := q.LockCustomerNotificationDispatchWait(ctx, db.LockCustomerNotificationDispatchWaitParams{OrganizationID: orgID, ID: st.dispatch.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // the booking or the customer is gone
		}
		if err != nil {
			return err
		}
		next, err = e.recordIn(ctx, q, orgID, st, res)
		return err
	})
	if err != nil {
		return false, err
	}
	return next, nil
}

func (e *Engine) recordIn(ctx context.Context, q *db.Queries, orgID int64, st *step, res channel.Result) (bool, error) {
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
		_, err := q.UpdateCustomerNotificationDelivery(ctx, p)
		return false, err
	case channel.OutcomeDelivered, channel.OutcomeSent:
		p.Status, p.SentAt, p.FinishedAt = DeliverySent, ts(now), ts(now)
		if res.Outcome == channel.OutcomeDelivered {
			p.Status = DeliveryDelivered
		}
		if n, err := q.UpdateCustomerNotificationDelivery(ctx, p); err != nil || n == 0 {
			return false, err
		}
		if res.Outcome == channel.OutcomeDelivered {
			if err := q.MarkCustomerNotificationDeliveryReceipt(ctx, db.MarkCustomerNotificationDeliveryReceiptParams{
				OrganizationID: orgID, ID: st.delivery.ID, At: ts(now),
			}); err != nil {
				return false, err
			}
		}
		return false, q.UpdateCustomerNotificationDispatchState(ctx, db.UpdateCustomerNotificationDispatchStateParams{
			OrganizationID: orgID, ID: st.dispatch.ID, Status: DispatchDelivered, NextChannelIndex: st.dispatch.NextChannelIndex,
			CompletedAt: ts(now),
		})
	default:
		p.Status, p.FinishedAt = DeliveryFailed, ts(now)
		if res.Outcome == channel.OutcomeSkipped {
			p.Status = DeliverySkipped
		}
		if n, err := q.UpdateCustomerNotificationDelivery(ctx, p); err != nil || n == 0 {
			return false, err
		}
		return e.advance(ctx, q, orgID, st.dispatch, st.channel, res.Error)
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
// accepted message. A failure runs the next channel (fallback) right away.
func (e *Engine) ReportDelivery(ctx context.Context, r channel.Report) {
	d, found, next, err := e.recordDelivery(ctx, r)
	if err != nil {
		e.log.WarnContext(ctx, "customer_notice_report_failed", "organization", r.OrganizationID, "error", err)
		return
	}
	if !next {
		if found {
			e.changed(ctx, r.OrganizationID, d)
		}
		return
	}
	// Process publishes the next channel's result.
	if err := e.Process(ctx, r.OrganizationID, d.ID); err != nil {
		e.log.WarnContext(ctx, "customer_notice_fallback_failed", "dispatch", d.ID, "error", err)
	}
}

// RecordDelivery records a delivery report without sending anything: next
// tells that the dispatch went back to pending for its next channel, which
// the worker's scan sends (a webhook in the API process only records).
// Delivery and read times are recorded even when the attempt ended before
// (a late report); they never answer anything. An unanswered report (a
// call picked up without a valid key) records the receipt and still moves
// the dispatch to its next channel. An unknown delivery is no error.
func (e *Engine) RecordDelivery(ctx context.Context, r channel.Report) (int64, bool, error) {
	d, found, next, err := e.recordDelivery(ctx, r)
	if err != nil || !found {
		return 0, false, err
	}
	e.changed(ctx, r.OrganizationID, d)
	return d.ID, next, nil
}

// recordDelivery records a report; found tells that the delivery is ours,
// d is its dispatch.
func (e *Engine) recordDelivery(ctx context.Context, r channel.Report) (db.CustomerNotificationDispatch, bool, bool, error) {
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
	var disp db.CustomerNotificationDispatch
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && dl.Channel != r.Channel) {
		return disp, false, false, nil
	}
	if err != nil {
		return disp, false, false, err
	}
	if r.Read || r.Unanswered {
		r.Delivered = true
	}
	status := DeliveryDelivered
	if !r.Delivered {
		status = DeliveryFailed
	}
	now := e.clock()
	at := now
	if !r.At.IsZero() {
		at = r.At.UTC().Truncate(time.Microsecond)
	}
	if r.Delivered {
		if err := e.q.MarkCustomerNotificationDeliveryReceipt(ctx, db.MarkCustomerNotificationDeliveryReceiptParams{
			OrganizationID: orgID, ID: dl.ID, At: ts(at), IsRead: r.Read,
		}); err != nil {
			return disp, false, false, err
		}
	}
	next := false
	err = e.tx(ctx, func(q *db.Queries) error {
		d, err := q.LockCustomerNotificationDispatchWait(ctx, db.LockCustomerNotificationDispatchWaitParams{OrganizationID: orgID, ID: dl.DispatchID})
		if err != nil {
			return err
		}
		disp = d
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
		if r.Delivered && !r.Unanswered {
			return q.UpdateCustomerNotificationDispatchState(ctx, db.UpdateCustomerNotificationDispatchStateParams{
				OrganizationID: orgID, ID: d.ID, Status: DispatchDelivered, NextChannelIndex: d.NextChannelIndex,
				LastError: d.LastError, CompletedAt: ts(now),
			})
		}
		reason := r.Error
		switch {
		case reason != "":
		case r.Unanswered:
			reason = "no answer"
		default:
			reason = "not delivered"
		}
		next, err = e.advance(ctx, q, orgID, d, dl.Channel, reason)
		return err
	})
	if err != nil {
		return disp, false, false, err
	}
	// A receipt recorded on an attempt that already ended is shown too
	// (delivery and read times).
	return disp, true, next, nil
}

// ExpireReceipts fails the accepted attempts whose delivery report did not
// come within the receipt timeout (the rule's next channel runs) and
// returns how many it failed. Only channels with reports are looked at.
func (e *Engine) ExpireReceipts(ctx context.Context) (int, error) {
	var names []string
	for name, c := range e.channels {
		if _, ok := c.(channel.ReportingChannel); ok {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return 0, nil
	}
	rows, err := e.q.ListUnreportedCustomerNotificationDeliveries(ctx, db.ListUnreportedCustomerNotificationDeliveriesParams{
		Before: ts(e.clock().Add(-e.receiptTimeout)), Channels: names, LimitCount: e.batch,
	})
	if err != nil {
		return 0, err
	}
	for _, r := range rows {
		id := r.Uuid
		e.ReportDelivery(ctx, channel.Report{
			Channel: r.Channel, OrganizationID: r.OrganizationID, DeliveryUUID: &id, Error: "no delivery report",
		})
	}
	return len(rows), nil
}

// CloseStuck closes the attempts that never got their send result within
// the stuck timeout (the process stopped between the claim and the
// record) and returns how many it closed. Such an attempt may have reached
// the customer, so it is never sent again and no next channel runs: the
// attempt is failed with ErrorOutcomeUnknown and its dispatch failed. A
// confirmation request closed so shows on the confirmation list as a
// request that did not go out (a person follows it up).
func (e *Engine) CloseStuck(ctx context.Context) (int, error) {
	rows, err := e.q.ListStuckCustomerNotificationDeliveries(ctx, db.ListStuckCustomerNotificationDeliveriesParams{
		Before: ts(e.clock().Add(-e.stuckTimeout)), LimitCount: e.batch,
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		d, closed, err := e.closeStuck(ctx, r)
		if err != nil {
			e.log.WarnContext(ctx, "customer_notice_stuck_failed", "organization", r.OrganizationID, "delivery", r.ID, "error", err)
			continue
		}
		if closed {
			n++
			e.log.WarnContext(ctx, "customer_notice_stuck_closed", "organization", r.OrganizationID, "dispatch", d.Uuid, "channel", r.Channel)
			e.changed(ctx, r.OrganizationID, d)
		}
	}
	return n, nil
}

func (e *Engine) closeStuck(ctx context.Context, r db.ListStuckCustomerNotificationDeliveriesRow) (db.CustomerNotificationDispatch, bool, error) {
	var (
		disp   db.CustomerNotificationDispatch
		closed bool
	)
	err := e.tx(ctx, func(q *db.Queries) error {
		d, err := q.LockCustomerNotificationDispatchWait(ctx, db.LockCustomerNotificationDispatchWaitParams{OrganizationID: r.OrganizationID, ID: r.DispatchID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		disp = d
		now := e.clock()
		n, err := q.FailStuckCustomerNotificationDelivery(ctx, db.FailStuckCustomerNotificationDeliveryParams{
			OrganizationID: r.OrganizationID, ID: r.ID, Error: ErrorOutcomeUnknown, FinishedAt: ts(now),
		})
		if err != nil || n == 0 {
			return err // the send result came meanwhile
		}
		closed = true
		// Only the attempt the dispatch waits for ends it.
		if d.Status != DispatchSending || int(d.NextChannelIndex) >= len(d.Channels) || d.Channels[d.NextChannelIndex] != r.Channel {
			return nil
		}
		disp.Status = DispatchFailed
		return q.UpdateCustomerNotificationDispatchState(ctx, db.UpdateCustomerNotificationDispatchStateParams{
			OrganizationID: r.OrganizationID, ID: d.ID, Status: DispatchFailed, NextChannelIndex: d.NextChannelIndex,
			LastError: r.Channel + ": " + ErrorOutcomeUnknown, CompletedAt: ts(now),
		})
	})
	if err != nil {
		return disp, false, err
	}
	return disp, closed, nil
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
		if n, err := e.ExpireReceipts(ctx); err != nil && ctx.Err() == nil {
			e.log.WarnContext(ctx, "customer_notice_receipts_failed", "error", err)
		} else if n > 0 {
			e.log.InfoContext(ctx, "customer_notice_receipts_expired", "count", n)
		}
		if _, err := e.CloseStuck(ctx); err != nil && ctx.Err() == nil {
			e.log.WarnContext(ctx, "customer_notice_stuck_sweep_failed", "error", err)
		}
	}
	if _, err := e.ProcessDue(ctx); err != nil && ctx.Err() == nil {
		e.log.WarnContext(ctx, "customer_notice_send_failed", "error", err)
	}
}
