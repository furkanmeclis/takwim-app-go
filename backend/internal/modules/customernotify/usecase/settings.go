package usecase

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/channel"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/templates"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Code    string
	Message string
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
	if e == nil || len(e.Details) == 0 {
		return nil
	}
	return e
}

func ruleView(r Rule) model.Rule {
	ch := r.Channels
	if ch == nil {
		ch = []string{}
	}
	off := r.Offsets
	if off == nil {
		off = []int32{}
	}
	return model.Rule{Enabled: r.Enabled, Channels: ch, Fallback: r.Fallback, ReminderOffsets: off}
}

// Settings answers the channels and every event with its rule and texts.
func (e *Engine) Settings(ctx context.Context, orgID int64) (model.Settings, error) {
	rules, err := e.q.ListCustomerNotificationRules(ctx, orgID)
	if err != nil {
		return model.Settings{}, err
	}
	tpls, err := e.q.ListCustomerNotificationTemplates(ctx, orgID)
	if err != nil {
		return model.Settings{}, err
	}
	byRule := map[string]db.CustomerNotificationRule{}
	for _, r := range rules {
		byRule[r.EventCode] = r
	}
	byTpl := map[string]db.CustomerNotificationTemplate{}
	for _, t := range tpls {
		byTpl[t.EventCode+"|"+t.Channel] = t
	}
	out := model.Settings{Channels: make([]model.ChannelInfo, 0, len(channel.Known))}
	for _, c := range channel.Known {
		out.Channels = append(out.Channels, model.ChannelInfo{Name: c, Ready: e.Ready(c)})
	}
	for _, ev := range catalog.All() {
		def := defaultRule(ev)
		rule := def
		view := model.Event{Code: ev.Code, Reminder: ev.Reminder, Variables: append([]string(nil), catalog.Vars...)}
		if row, ok := byRule[ev.Code]; ok {
			rule = Rule{Enabled: row.Enabled, Channels: row.Channels, Fallback: row.Fallback, Offsets: row.ReminderOffsets}
			t := row.UpdatedAt.Time
			view.UpdatedAt, view.Custom = &t, true
		}
		view.Rule, view.DefaultRule = ruleView(rule), ruleView(def)
		for _, c := range channel.Known {
			d := catalog.DefaultTemplate(ev, c)
			tv := model.Template{Channel: c, Subject: d.Subject, Body: d.Body, DefaultSubject: d.Subject, DefaultBody: d.Body}
			if row, ok := byTpl[ev.Code+"|"+c]; ok {
				tv.Subject, tv.Body, tv.Custom = row.Subject, row.Body, true
				view.Custom = true
				if t := row.UpdatedAt.Time; view.UpdatedAt == nil || t.After(*view.UpdatedAt) {
					view.UpdatedAt = &t
				}
			}
			view.Templates = append(view.Templates, tv)
		}
		out.Events = append(out.Events, view)
	}
	return out, nil
}

var placeholderRE = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.]+)\s*\}\}`)

// unknownVars lists the placeholders of text that are not variables.
func unknownVars(texts ...string) []string {
	out := []string{}
	for _, t := range texts {
		for _, m := range placeholderRE.FindAllStringSubmatch(t, -1) {
			if !slices.Contains(catalog.Vars, m[1]) && !slices.Contains(out, m[1]) {
				out = append(out, m[1])
			}
		}
	}
	return out
}

func actor(actorID int64) pgtype.Int8 { return pgtype.Int8{Int64: actorID, Valid: actorID > 0} }

// Save replaces an event's rule and texts. A text that is empty or equal to
// the default goes back to the default; a rule equal to the default is
// still stored (the business chose it).
func (e *Engine) Save(ctx context.Context, orgID, actorID int64, in model.SaveInput) (model.Event, error) {
	ve := &ValidationError{}
	ev, ok := catalog.Get(in.Event)
	if !ok || catalog.Managed(in.Event) {
		ve.add("event", "invalid", "unknown event")
		return model.Event{}, ve
	}
	channels := []string{}
	for i, c := range in.Channels {
		if !channel.IsKnown(c) {
			ve.add("channels."+strconv.Itoa(i), "invalid", "unknown channel")
			continue
		}
		if slices.Contains(channels, c) {
			ve.add("channels."+strconv.Itoa(i), "duplicate", "channel listed twice")
			continue
		}
		channels = append(channels, c)
	}
	if in.Enabled && len(in.Channels) == 0 {
		ve.add("channels", "required", "choose at least one channel")
	}
	offsets := []int32{}
	if ev.Reminder {
		if in.Enabled && len(in.ReminderOffsets) == 0 {
			ve.add("reminder_offsets", "required", "choose at least one reminder time")
		}
		if len(in.ReminderOffsets) > catalog.MaxOffsets {
			ve.add("reminder_offsets", "too_many", "at most 3 reminders")
		}
		for i, o := range in.ReminderOffsets {
			switch {
			case o < catalog.MinOffsetMinutes || o > catalog.MaxOffsetMinutes:
				ve.add("reminder_offsets."+strconv.Itoa(i), "out_of_range", "between 15 minutes and 7 days")
			case slices.Contains(offsets, o):
				ve.add("reminder_offsets."+strconv.Itoa(i), "duplicate", "reminder listed twice")
			default:
				offsets = append(offsets, o)
			}
		}
		slices.Sort(offsets)
		slices.Reverse(offsets)
	} else if len(in.ReminderOffsets) > 0 {
		ve.add("reminder_offsets", "not_allowed", "only reminders have offsets")
	}
	type tplWrite struct {
		channel, subject, body string
		reset                  bool
	}
	writes := []tplWrite{}
	seen := map[string]bool{}
	for i, t := range in.Templates {
		f := "templates." + strconv.Itoa(i)
		if !channel.IsKnown(t.Channel) {
			ve.add(f+".channel", "invalid", "unknown channel")
			continue
		}
		if seen[t.Channel] {
			ve.add(f+".channel", "duplicate", "channel listed twice")
			continue
		}
		seen[t.Channel] = true
		subject, body := strings.TrimSpace(t.Subject), strings.TrimSpace(t.Body)
		if t.Channel != channel.Email {
			subject = ""
		}
		if utf8.RuneCountInString(subject) > catalog.MaxSubject {
			ve.add(f+".subject", "too_long", "subject is at most 200 characters")
		}
		if utf8.RuneCountInString(body) > catalog.MaxBody {
			ve.add(f+".body", "too_long", "text is at most 1000 characters")
		}
		if t.Channel == channel.Email && body != "" && subject == "" {
			ve.add(f+".subject", "required", "subject is required")
		}
		if u := unknownVars(subject, body); len(u) > 0 {
			ve.add(f+".body", "unknown_variable", "unknown variable: "+strings.Join(u, ", "))
		}
		d := catalog.DefaultTemplate(ev, t.Channel)
		reset := body == "" || (body == strings.TrimSpace(d.Body) && subject == strings.TrimSpace(d.Subject))
		writes = append(writes, tplWrite{channel: t.Channel, subject: subject, body: body, reset: reset})
	}
	if err := ve.orNil(); err != nil {
		return model.Event{}, err
	}
	err := e.tx(ctx, func(q *db.Queries) error {
		if _, err := q.UpsertCustomerNotificationRule(ctx, db.UpsertCustomerNotificationRuleParams{
			OrganizationID: orgID, EventCode: ev.Code, Enabled: in.Enabled, Channels: channels, Fallback: in.Fallback,
			ReminderOffsets: offsets, UpdatedBy: actor(actorID),
		}); err != nil {
			return err
		}
		for _, w := range writes {
			if w.reset {
				if err := q.DeleteCustomerNotificationTemplate(ctx, db.DeleteCustomerNotificationTemplateParams{
					OrganizationID: orgID, EventCode: ev.Code, Channel: w.channel,
				}); err != nil {
					return err
				}
				continue
			}
			if err := q.UpsertCustomerNotificationTemplate(ctx, db.UpsertCustomerNotificationTemplateParams{
				OrganizationID: orgID, EventCode: ev.Code, Channel: w.channel, Subject: w.subject, Body: w.body,
				UpdatedBy: actor(actorID),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Event{}, err
	}
	return e.event(ctx, orgID, ev.Code)
}

// Reset sets an event back to the default rule and texts.
func (e *Engine) Reset(ctx context.Context, orgID int64, code string) (model.Event, error) {
	if _, ok := catalog.Get(code); !ok || catalog.Managed(code) {
		ve := &ValidationError{}
		ve.add("event", "invalid", "unknown event")
		return model.Event{}, ve
	}
	err := e.tx(ctx, func(q *db.Queries) error {
		if err := q.DeleteCustomerNotificationRule(ctx, db.DeleteCustomerNotificationRuleParams{OrganizationID: orgID, EventCode: code}); err != nil {
			return err
		}
		return q.DeleteCustomerNotificationTemplates(ctx, db.DeleteCustomerNotificationTemplatesParams{OrganizationID: orgID, EventCode: code})
	})
	if err != nil {
		return model.Event{}, err
	}
	return e.event(ctx, orgID, code)
}

func (e *Engine) event(ctx context.Context, orgID int64, code string) (model.Event, error) {
	s, err := e.Settings(ctx, orgID)
	if err != nil {
		return model.Event{}, err
	}
	for _, ev := range s.Events {
		if ev.Code == code {
			return ev, nil
		}
	}
	return model.Event{}, ErrUnknownEvent
}

// Preview renders a text with sample values (the business's name).
func (e *Engine) Preview(ctx context.Context, orgID int64, in model.PreviewInput) (model.Preview, error) {
	ve := &ValidationError{}
	if _, ok := catalog.Get(in.Event); !ok {
		ve.add("event", "invalid", "unknown event")
	}
	if !channel.IsKnown(in.Channel) {
		ve.add("channel", "invalid", "unknown channel")
	}
	if utf8.RuneCountInString(in.Body) > catalog.MaxBody {
		ve.add("body", "too_long", "text is at most 1000 characters")
	}
	if err := ve.orNil(); err != nil {
		return model.Preview{}, err
	}
	vars := catalog.SampleVars()
	if org, err := e.q.GetNotifyOrganization(ctx, orgID); err == nil {
		vars["business_name"] = org.Name
	}
	if e.panelURL != "" {
		vars["link"] = e.panelURL
	}
	subject := in.Subject
	if in.Channel != channel.Email {
		subject = ""
	}
	return model.Preview{
		Subject: templates.Render(subject, vars), Body: templates.Render(in.Body, vars),
		Unknown: unknownVars(subject, in.Body),
	}, nil
}

// Delivery sort fields.
var deliverySorts = []string{"created_at", "channel", "status", "event"}

// DeliverySortFields lists the sortable history fields.
func DeliverySortFields() []string { return append([]string(nil), deliverySorts...) }

func pgUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

// Deliveries lists the send history inside the caller's branch scope.
func (e *Engine) Deliveries(ctx context.Context, orgID int64, f model.DeliveryFilter) ([]model.Delivery, int64, error) {
	if !slices.Contains(deliverySorts, f.SortField) {
		f.SortField, f.SortDesc = "created_at", true
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 25
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	p := db.ListCustomerNotificationDeliveriesParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Channels: nilIfEmpty(f.Channels),
		Statuses: nilIfEmpty(f.Statuses), Events: nilIfEmpty(f.Events), CustomerUuid: pgUUID(f.CustomerUUID),
		ReservationUuid: pgUUID(f.ReservationUUID), SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if f.CreatedFrom != nil {
		p.CreatedFrom = ts(*f.CreatedFrom)
	}
	if f.CreatedTo != nil {
		p.CreatedTo = ts(*f.CreatedTo)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Q = pgtype.Text{String: strings.TrimPrefix(q, "#"), Valid: true}
	}
	rows, err := e.q.ListCustomerNotificationDeliveries(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	total, err := e.q.CountCustomerNotificationDeliveries(ctx, db.CountCustomerNotificationDeliveriesParams{
		OrganizationID: p.OrganizationID, BranchIds: p.BranchIds, Channels: p.Channels, Statuses: p.Statuses,
		Events: p.Events, CustomerUuid: p.CustomerUuid, ReservationUuid: p.ReservationUuid,
		CreatedFrom: p.CreatedFrom, CreatedTo: p.CreatedTo, Q: p.Q,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Delivery, 0, len(rows))
	for _, r := range rows {
		d := model.Delivery{
			UUID: r.Uuid, DispatchUUID: r.DispatchUuid, Event: r.EventCode, Channel: r.Channel, AttemptNo: r.AttemptNo,
			Status: r.Status, DispatchStatus: r.DispatchStatus, Provider: r.Provider, ProviderRef: r.ProviderRef,
			RecipientMasked: r.RecipientMasked, CustomerUUID: r.CustomerUuid,
			CustomerName: strings.TrimSpace(r.CustomerFirstName + " " + r.CustomerLastName),
			Subject:      r.Subject, Body: r.Body, Error: r.Error, CreatedAt: r.CreatedAt.Time,
		}
		if r.ReservationUuid.Valid {
			id := uuid.UUID(r.ReservationUuid.Bytes)
			d.ReservationUUID = &id
		}
		if r.ReservationNumber.Valid {
			n := r.ReservationNumber.Int64
			d.ReservationNumber = &n
		}
		if r.SentAt.Valid {
			t := r.SentAt.Time
			d.SentAt = &t
		}
		if r.FinishedAt.Valid {
			t := r.FinishedAt.Time
			d.FinishedAt = &t
		}
		out = append(out, d)
	}
	return out, total, nil
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

// IsValidation reports whether err is a validation error.
func IsValidation(err error) (*ValidationError, bool) {
	var ve *ValidationError
	ok := errors.As(err, &ve)
	return ve, ok
}
