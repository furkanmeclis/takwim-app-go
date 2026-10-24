package usecase

import (
	"context"
	"slices"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/model"
)

// Texts of managed events (TEC-979). A managed event (the confirmation
// request) takes its rule from its own module, so the notification
// settings refuse it; its texts are still the business's, on the channels
// the catalog lists as editable (SMS and e-mail). WhatsApp sends the
// provider's approved template and the voice call reads the catalog text,
// so neither is stored here.

func managedEvent(code string) (catalog.Event, error) {
	ev, ok := catalog.Get(code)
	if !ok || !catalog.Managed(code) || len(ev.Editable) == 0 {
		return catalog.Event{}, ErrUnknownEvent
	}
	return ev, nil
}

// ManagedTexts answers the texts of a managed event with their defaults.
func (e *Engine) ManagedTexts(ctx context.Context, orgID int64, code string) (model.ManagedTexts, error) {
	ev, err := managedEvent(code)
	if err != nil {
		return model.ManagedTexts{}, err
	}
	rows, err := e.q.ListCustomerNotificationTemplates(ctx, orgID)
	if err != nil {
		return model.ManagedTexts{}, err
	}
	out := model.ManagedTexts{
		Event: ev.Code, Variables: catalog.VariablesOf(ev), Required: append([]string{}, ev.Required...),
		Templates: make([]model.Template, 0, len(ev.Editable)), Voice: ev.Voice,
	}
	for _, c := range ev.Editable {
		d := catalog.DefaultTemplate(ev, c)
		tv := model.Template{Channel: c, Subject: d.Subject, Body: d.Body, DefaultSubject: d.Subject, DefaultBody: d.Body}
		for _, row := range rows {
			if row.EventCode != ev.Code || row.Channel != c {
				continue
			}
			tv.Subject, tv.Body, tv.Custom = row.Subject, row.Body, true
			out.Custom = true
			if t := row.UpdatedAt.Time; out.UpdatedAt == nil || t.After(*out.UpdatedAt) {
				out.UpdatedAt = &t
			}
		}
		out.Templates = append(out.Templates, tv)
	}
	return out, nil
}

// SaveManagedTexts replaces the texts of a managed event. A text that is
// empty or equal to the default goes back to the default; a channel left
// out keeps its text. The rule is not touched.
func (e *Engine) SaveManagedTexts(ctx context.Context, orgID, actorID int64, code string, in model.ManagedTextsInput) (model.ManagedTexts, error) {
	ev, err := managedEvent(code)
	if err != nil {
		return model.ManagedTexts{}, err
	}
	ve := &ValidationError{}
	if len(in.Templates) == 0 {
		ve.add("templates", "required", "send at least one text")
	}
	writes := validateTemplates(ve, ev, in.Templates, func(c string) bool { return slices.Contains(ev.Editable, c) })
	if err := ve.orNil(); err != nil {
		return model.ManagedTexts{}, err
	}
	if err := e.tx(ctx, func(q *db.Queries) error {
		return writeTemplates(ctx, q, orgID, actorID, ev.Code, writes)
	}); err != nil {
		return model.ManagedTexts{}, err
	}
	return e.ManagedTexts(ctx, orgID, ev.Code)
}

// ResetManagedTexts sets every text of a managed event back to the
// default.
func (e *Engine) ResetManagedTexts(ctx context.Context, orgID int64, code string) (model.ManagedTexts, error) {
	ev, err := managedEvent(code)
	if err != nil {
		return model.ManagedTexts{}, err
	}
	if err := e.q.DeleteCustomerNotificationTemplates(ctx, db.DeleteCustomerNotificationTemplatesParams{
		OrganizationID: orgID, EventCode: ev.Code,
	}); err != nil {
		return model.ManagedTexts{}, err
	}
	return e.ManagedTexts(ctx, orgID, ev.Code)
}
