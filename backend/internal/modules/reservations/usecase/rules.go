package usecase

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	orgsettings "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
)

// The booking rules of a business (TEC-905): its booking form questions
// and the online booking limits. The same checks run for every caller of
// the online channel (booking page, widget, mobile app, partners), never
// only in a browser.

// checkAnswers validates the form answers of a booking from source against
// the current questions and returns the answers and the question snapshot
// to store (previous: the snapshot the record already keeps).
func checkAnswers(ve *ValidationError, set *orgsettings.Version, source string, in map[string]any, previous []orgsettings.CustomField) ([]byte, []byte) {
	var fields []orgsettings.CustomField
	if set != nil {
		fields = set.Settings.Booking.Fields
	}
	where := orgsettings.AskOnline
	if source == model.SourcePanel {
		where = orgsettings.AskPanel
	}
	answers, problems := orgsettings.CheckAnswers(fields, in, where)
	for _, p := range problems {
		ve.add(p.Field, p.Code, p.Message)
	}
	values, err := json.Marshal(answers)
	if err != nil {
		ve.add("custom_fields", "invalid", "custom fields must be an object")
	}
	defs, err := json.Marshal(orgsettings.Snapshot(fields, previous, answers))
	if err != nil {
		ve.add("custom_fields", "invalid", "custom fields must be an object")
	}
	return values, defs
}

// decodeDefs reads a stored question snapshot (empty on bad data).
func decodeDefs(raw []byte) []orgsettings.CustomField {
	var defs []orgsettings.CustomField
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &defs)
	}
	return defs
}

// answersOf pairs stored answers with their question snapshot, in the
// form's order; answers without a question follow by key.
func answersOf(rawValues, rawDefs []byte) []model.CustomAnswer {
	out := []model.CustomAnswer{}
	values := map[string]any{}
	if len(rawValues) > 0 {
		if err := json.Unmarshal(rawValues, &values); err != nil {
			return out
		}
	}
	done := map[string]bool{}
	for _, f := range decodeDefs(rawDefs) {
		v, ok := values[f.Key]
		if !ok || done[f.Key] {
			continue
		}
		done[f.Key] = true
		out = append(out, model.CustomAnswer{Key: f.Key, Label: f.Label, Type: f.Type, Value: v, Online: f.Online})
	}
	rest := make([]string, 0, len(values))
	for k := range values {
		if !done[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		v := values[k]
		kind := orgsettings.FieldText
		switch v.(type) {
		case bool:
			kind = orgsettings.FieldCheckbox
		case float64:
			kind = orgsettings.FieldNumber
		}
		out = append(out, model.CustomAnswer{Key: k, Label: k, Type: kind, Value: v})
	}
	return out
}

// checkOpenLimit refuses an online booking when the customer already holds
// the business's maximum of open bookings.
func checkOpenLimit(ctx context.Context, q *db.Queries, orgID int64, set *orgsettings.Version, customerID int64, now time.Time, exclude int64) error {
	if set == nil || set.Settings.Reservation.MaxOpenPerCustomer == nil {
		return nil
	}
	n, err := q.CountOpenCustomerReservations(ctx, db.CountOpenCustomerReservationsParams{
		OrganizationID: orgID, CustomerID: customerID, Now: ts(now), ExcludeID: pgtype.Int8{Int64: exclude, Valid: exclude > 0},
	})
	if err != nil {
		return err
	}
	if n >= int64(*set.Settings.Reservation.MaxOpenPerCustomer) {
		return &ValidationError{Details: []Detail{{Field: "customer", Code: "open_limit", Message: "the customer already has the most open bookings the business allows"}}}
	}
	return nil
}

// CheckDailyLimit is checkDailyLimit for the public checkout (TEC-905):
// a customer learns that the day is full before filling in the form.
func CheckDailyLimit(ctx context.Context, q *db.Queries, orgID int64, set *orgsettings.Version, branchID int64, start time.Time) error {
	return checkDailyLimit(ctx, q, orgID, set, branchID, start, 0)
}

// checkDailyLimit refuses an online booking starting at start when the
// branch already took its maximum of online bookings that local day.
func checkDailyLimit(ctx context.Context, q *db.Queries, orgID int64, set *orgsettings.Version, branchID int64, start time.Time, exclude int64) error {
	if set == nil || set.Settings.Reservation.MaxOnlinePerDay == nil {
		return nil
	}
	tz, err := q.GetReservationBranchTimezone(ctx, db.GetReservationBranchTimezoneParams{OrganizationID: orgID, ID: branchID})
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(tz)
	if err != nil || tz == "" {
		loc, _ = time.LoadLocation("Europe/Istanbul")
	}
	local := start.In(loc)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	n, err := q.CountOnlineBranchReservations(ctx, db.CountOnlineBranchReservationsParams{
		OrganizationID: orgID, BranchID: branchID, StartsFrom: ts(day), StartsTo: ts(day.AddDate(0, 0, 1)),
		ExcludeID: pgtype.Int8{Int64: exclude, Valid: exclude > 0},
	})
	if err != nil {
		return err
	}
	if n >= int64(*set.Settings.Reservation.MaxOnlinePerDay) {
		return &ValidationError{Details: []Detail{{Field: "starts_at", Code: "daily_limit", Message: "the branch takes no more online bookings that day"}}}
	}
	return nil
}

// Form returns the booking form questions and the hidden flow steps of
// the business (none without a settings reader).
func (s *Service) Form(ctx context.Context, orgID int64) (model.Form, error) {
	out := model.Form{Fields: []model.FormField{}, HiddenSteps: []string{}}
	if s.settings == nil {
		return out, nil
	}
	v, err := s.settings.Current(ctx, orgID)
	if err != nil {
		return out, err
	}
	out.HiddenSteps = append(out.HiddenSteps, v.Settings.Booking.HiddenSteps...)
	for _, f := range v.Settings.Booking.Fields {
		out.Fields = append(out.Fields, model.FormField{
			Key: f.Key, Label: f.Label, Help: f.Help, Type: f.Type, Options: append([]string{}, f.Options...),
			Required: f.Required, Online: f.Online, Panel: f.Panel,
		})
	}
	return out, nil
}

func settingsVersion(set *orgsettings.Version) pgtype.Int4 {
	if set == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(set.Version), Valid: true}
}
