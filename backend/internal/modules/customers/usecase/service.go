// Package usecase holds the customer rules. Every method takes the
// organization id of the caller explicitly (from orgctx.Scope in handlers,
// from the job in workers).
//
// Customers are business-level: every member with tenant.customers.read
// sees every record of the business. A record belongs to one business only;
// the shared identity (customer_identities) is reached solely through
// LinkVerified, after the phone was proven, and never through the tenant API.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/phone"
)

// SearchSpec is the Cmd+K spec id of customers.
const SearchSpec = "customers"

// Limits of the customer fields.
const (
	maxNameLength  = 80
	maxEmailLength = 254
	maxNotesLength = 5000
	maxTags        = 20
	maxTagLength   = 32
)

var (
	ErrNotFound       = errors.New("customer not found")
	ErrPhoneTaken     = errors.New("another customer already has this phone")
	ErrIdentityLinked = errors.New("this person is already linked to another customer record")
	// ErrPhoneMismatch is returned when the proven phone is not the phone of
	// the record (a link needs the very number on file).
	ErrPhoneMismatch = errors.New("the verified phone does not match the customer's phone")
)

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Message string
	Code    string
}

// ValidationError carries field details; handlers answer 400 VALIDATION_ERROR.
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
	if len(e.Details) == 0 {
		return nil
	}
	return e
}

// IsValidation reports whether err is a field validation error.
func IsValidation(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// SearchIndexer schedules Cmd+K index updates (fail-soft).
type SearchIndexer interface {
	EnqueueUpsert(ctx context.Context, spec, id string)
	EnqueueDelete(ctx context.Context, spec, id string)
}

// Service implements the customer use cases.
type Service struct {
	repo    *repository.Postgres
	indexer SearchIndexer
	now     func() time.Time
}

// New builds the service. indexer may be nil.
func New(repo *repository.Postgres, indexer SearchIndexer) *Service {
	return &Service{repo: repo, indexer: indexer, now: time.Now}
}

// SortColumns are the sortable list fields.
var SortColumns = map[string]string{
	"name": "name", "phone": "phone", "email": "email", "source": "source",
	"created_at": "created_at", "updated_at": "updated_at",
}

// listArgs are the shared filter arguments of the list queries.
type listArgs struct {
	tags, sources                      []string
	smsConsent, emailConsent, verified pgtype.Bool
	q, qDigits                         pgtype.Text
}

func toListArgs(f model.ListFilter) listArgs {
	a := listArgs{
		tags: nilIfEmpty(f.Tags), sources: nilIfEmpty(f.Sources),
		smsConsent: boolArg(f.SMSConsent), emailConsent: boolArg(f.EmailConsent), verified: boolArg(f.Verified),
		q: textArg(strings.TrimSpace(f.Q)),
	}
	a.qDigits = textArg(phoneDigits(f.Q))
	return a
}

// phoneDigits returns the digits of a search term that looks like a phone
// fragment (at least 3 digits), without the national "0" prefix, so
// "0532 12" matches "+90532 12…".
func phoneDigits(q string) string {
	var b strings.Builder
	for _, r := range q {
		switch {
		case unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '+' || r == '.':
		default:
			return ""
		}
	}
	d := strings.TrimLeft(b.String(), "0")
	if len(d) < 3 {
		return ""
	}
	return d
}

// List returns one page of customers. The default order is first name,
// last name.
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Customer, int64, error) {
	if _, ok := SortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "", false
	}
	a := toListArgs(f)
	q := s.repo.Q()
	rows, err := q.ListCustomers(ctx, db.ListCustomersParams{
		OrganizationID: orgID, Tags: a.tags, Sources: a.sources, SmsConsent: a.smsConsent,
		EmailConsent: a.emailConsent, Verified: a.verified, Q: a.q, QDigits: a.qDigits,
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountCustomers(ctx, db.CountCustomersParams{
		OrganizationID: orgID, Tags: a.tags, Sources: a.sources, SmsConsent: a.smsConsent,
		EmailConsent: a.emailConsent, Verified: a.verified, Q: a.q, QDigits: a.qDigits,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Customer, 0, len(rows))
	for _, row := range rows {
		out = append(out, MapCustomer(db.GetCustomerRow(row)))
	}
	return out, total, nil
}

// Facets returns the tags and sources in use.
func (s *Service) Facets(ctx context.Context, orgID int64) (model.Facets, error) {
	q := s.repo.Q()
	out := model.Facets{Tag: []model.Facet{}, Source: []model.Facet{}}
	tags, err := q.ListCustomerTagFacets(ctx, orgID)
	if err != nil {
		return out, err
	}
	for _, row := range tags {
		out.Tag = append(out.Tag, model.Facet{Value: row.Value, Count: row.Count})
	}
	sources, err := q.ListCustomerSourceFacets(ctx, orgID)
	if err != nil {
		return out, err
	}
	for _, row := range sources {
		out.Source = append(out.Source, model.Facet{Value: row.Value, Count: row.Count})
	}
	return out, nil
}

// Get returns one customer of the organization.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Customer, error) {
	row, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return model.Customer{}, err
	}
	return MapCustomer(row), nil
}

// Create validates and stores a new customer. source is "panel", "import"
// or "online"; whatever the source, the record starts unlinked.
func (s *Service) Create(ctx context.Context, orgID int64, in model.Input, source string) (model.Customer, error) {
	if !slices.Contains(model.Sources, source) {
		source = "panel"
	}
	f := fields{Tags: []string{}}
	ve := &ValidationError{}
	f.apply(ve, in, true, s.now())
	if err := ve.orNil(); err != nil {
		return model.Customer{}, err
	}
	if err := s.checkPhone(ctx, orgID, f.Phone, nil); err != nil {
		return model.Customer{}, err
	}
	row, err := s.repo.Q().CreateCustomer(ctx, db.CreateCustomerParams{
		OrganizationID: orgID, FirstName: f.FirstName, LastName: f.LastName, PhoneE164: f.Phone,
		Email: f.Email, BirthDate: f.BirthDate, Notes: f.Notes, Tags: f.Tags, SmsConsent: f.SMSConsent,
		EmailConsent: f.EmailConsent, ConsentUpdatedAt: f.ConsentUpdatedAt, Source: source,
	})
	if err != nil {
		return model.Customer{}, mapErr(err)
	}
	s.index(ctx, orgID, row.Uuid)
	return s.Get(ctx, orgID, row.Uuid)
}

// Patch updates the given fields. Changing the phone of a linked record
// drops the link: the new number has not been proven.
func (s *Service) Patch(ctx context.Context, orgID int64, id uuid.UUID, in model.Input) (model.Customer, error) {
	q := s.repo.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Customer{}, err
	}
	f := fieldsFromRow(row)
	ve := &ValidationError{}
	f.apply(ve, in, false, s.now())
	if err := ve.orNil(); err != nil {
		return model.Customer{}, err
	}
	if err := s.checkPhone(ctx, orgID, f.Phone, &row.ID); err != nil {
		return model.Customer{}, err
	}
	phoneChanged := f.Phone != row.PhoneE164
	err = s.repo.InTx(ctx, func(tx *db.Queries) error {
		if _, err := tx.UpdateCustomer(ctx, db.UpdateCustomerParams{
			OrganizationID: orgID, ID: row.ID, FirstName: f.FirstName, LastName: f.LastName, PhoneE164: f.Phone,
			Email: f.Email, BirthDate: f.BirthDate, Notes: f.Notes, Tags: f.Tags, SmsConsent: f.SMSConsent,
			EmailConsent: f.EmailConsent, ConsentUpdatedAt: f.ConsentUpdatedAt,
		}); err != nil {
			return err
		}
		if phoneChanged && row.LinkSource.Valid {
			if _, err := tx.DeleteCustomerIdentityLink(ctx, db.DeleteCustomerIdentityLinkParams{OrganizationID: orgID, CustomerID: row.ID}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Customer{}, mapNotFound(mapErr(err))
	}
	s.index(ctx, orgID, id)
	return s.Get(ctx, orgID, id)
}

// SetTags replaces the tags of a customer and returns the previous ones
// (bulk engine).
func (s *Service) SetTags(ctx context.Context, orgID int64, id uuid.UUID, tags []string) ([]string, error) {
	row, err := s.load(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return nil, err
	}
	prev := row.Tags
	if prev == nil {
		prev = []string{}
	}
	ve := &ValidationError{}
	clean := cleanTags(ve, tags)
	if err := ve.orNil(); err != nil {
		return prev, err
	}
	if slices.Equal(clean, prev) {
		return prev, nil
	}
	if err := s.repo.Q().SetCustomerTags(ctx, db.SetCustomerTagsParams{OrganizationID: orgID, ID: row.ID, Tags: clean}); err != nil {
		return prev, err
	}
	return prev, nil
}

// Delete soft-deletes a customer. The phone becomes free; a verified link
// stays with the deleted row and is not reused.
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) (model.Customer, error) {
	q := s.repo.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Customer{}, err
	}
	n, err := q.SoftDeleteCustomer(ctx, db.SoftDeleteCustomerParams{OrganizationID: orgID, ID: row.ID})
	if err != nil {
		return model.Customer{}, err
	}
	if n == 0 {
		return model.Customer{}, ErrNotFound
	}
	if s.indexer != nil {
		s.indexer.EnqueueDelete(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
	return MapCustomer(row), nil
}

// Restore brings back a soft-deleted customer (bulk delete rollback). It
// fails with ErrPhoneTaken when a newer record took the phone.
func (s *Service) Restore(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := s.repo.Q().RestoreCustomer(ctx, db.RestoreCustomerParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return ErrNotFound
	}
	s.index(ctx, orgID, id)
	return nil
}

// LinkVerified links the business's record to the shared identity of a
// person who has just proven phoneE164 (OTP or customer panel login). The
// record must carry that very phone. Staff-typed and imported numbers never
// call this, so they never open shared history.
func (s *Service) LinkVerified(ctx context.Context, orgID int64, id uuid.UUID, phoneE164, source string, verifiedAt time.Time) (model.Customer, error) {
	return s.LinkVerifiedFrom(ctx, orgID, id, phoneE164, source, verifiedAt, nil)
}

// LinkVerifiedFrom is LinkVerified with a visibility start: with visibleFrom
// set, the person sees only the record's reservations made from then on
// (TEC-910: a record older than the phone's last change of hands). A record
// already linked to another identity stays with it (ErrIdentityLinked): a
// proven phone never takes over somebody else's history.
func (s *Service) LinkVerifiedFrom(ctx context.Context, orgID int64, id uuid.UUID, phoneE164, source string, verifiedAt time.Time, visibleFrom *time.Time) (model.Customer, error) {
	if !slices.Contains(model.LinkSources, source) {
		return model.Customer{}, fmt.Errorf("customers: unknown link source %q", source)
	}
	normalized, err := phone.Normalize(phoneE164, "")
	if err != nil || normalized == "" {
		return model.Customer{}, ErrPhoneMismatch
	}
	q := s.repo.Q()
	row, err := s.load(ctx, q, orgID, id)
	if err != nil {
		return model.Customer{}, err
	}
	if !row.PhoneE164.Valid || row.PhoneE164.String != normalized {
		return model.Customer{}, ErrPhoneMismatch
	}
	err = s.repo.InTx(ctx, func(tx *db.Queries) error {
		identity, err := tx.UpsertCustomerIdentity(ctx, db.UpsertCustomerIdentityParams{
			PhoneE164: normalized, VerifiedAt: pgtype.Timestamptz{Time: verifiedAt, Valid: true},
		})
		if err != nil {
			return err
		}
		if row.LinkSource.Valid {
			link, err := tx.GetCustomerIdentityLink(ctx, db.GetCustomerIdentityLinkParams{OrganizationID: orgID, CustomerID: row.ID})
			if err != nil {
				return err
			}
			if link.IdentityID == identity.ID {
				return nil
			}
			return ErrIdentityLinked
		}
		var from pgtype.Timestamptz
		if visibleFrom != nil {
			from = pgtype.Timestamptz{Time: *visibleFrom, Valid: true}
		}
		_, err = tx.CreateCustomerIdentityLink(ctx, db.CreateCustomerIdentityLinkParams{
			OrganizationID: orgID, CustomerID: row.ID, IdentityID: identity.ID, Source: source,
			VerifiedPhoneE164: normalized, VerifiedAt: pgtype.Timestamptz{Time: verifiedAt, Valid: true},
			VisibleFrom: from,
		})
		return err
	})
	if err != nil {
		return model.Customer{}, mapErr(err)
	}
	return s.Get(ctx, orgID, id)
}

// LinkedCustomer returns this business's record of the person who proved
// phoneE164, or ErrNotFound. Another business's records are never reached:
// the lookup goes identity → link of orgID → record of orgID.
func (s *Service) LinkedCustomer(ctx context.Context, orgID int64, phoneE164 string) (model.Customer, error) {
	normalized, err := phone.Normalize(phoneE164, "")
	if err != nil || normalized == "" {
		return model.Customer{}, ErrNotFound
	}
	q := s.repo.Q()
	identity, err := q.GetCustomerIdentityByPhone(ctx, normalized)
	if err != nil {
		return model.Customer{}, mapNotFound(repository.MapErr(err))
	}
	ref, err := q.GetLinkedCustomer(ctx, db.GetLinkedCustomerParams{OrganizationID: orgID, IdentityID: identity.ID})
	if err != nil {
		return model.Customer{}, mapNotFound(repository.MapErr(err))
	}
	return s.Get(ctx, orgID, ref.Uuid)
}

// TargetUUIDs resolves the customers matching a list filter (bulk "all
// matching" scope), capped at limit.
func (s *Service) TargetUUIDs(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]uuid.UUID, error) {
	a := toListArgs(f)
	return s.repo.Q().ListCustomerUUIDs(ctx, db.ListCustomerUUIDsParams{
		OrganizationID: orgID, Tags: a.tags, Sources: a.sources, SmsConsent: a.smsConsent,
		EmailConsent: a.emailConsent, Verified: a.verified, Q: a.q, QDigits: a.qDigits, LimitCount: limit,
	})
}

// ExportRows returns every customer matching a list filter (capped at limit).
func (s *Service) ExportRows(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]model.Customer, error) {
	a := toListArgs(f)
	rows, err := s.repo.Q().ListCustomersForExport(ctx, db.ListCustomersForExportParams{
		OrganizationID: orgID, Tags: a.tags, Sources: a.sources, SmsConsent: a.smsConsent,
		EmailConsent: a.emailConsent, Verified: a.verified, Q: a.q, QDigits: a.qDigits, LimitCount: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.Customer, 0, len(rows))
	for _, row := range rows {
		out = append(out, MapCustomer(db.GetCustomerRow(row)))
	}
	return out, nil
}

// ImportRow updates the live customer with the same phone, or creates one
// with source "import". It returns the previous state of an update. An
// imported phone stays unverified.
func (s *Service) ImportRow(ctx context.Context, orgID int64, in model.Input) (uuid.UUID, bool, *model.Customer, error) {
	if in.Phone.Value != nil {
		normalized, err := phone.Normalize(*in.Phone.Value, "")
		if err == nil && normalized != "" {
			existing, err := s.repo.Q().GetCustomerByPhone(ctx, db.GetCustomerByPhoneParams{OrganizationID: orgID, PhoneE164: pgtype.Text{String: normalized, Valid: true}})
			if err == nil {
				prev, err := s.Get(ctx, orgID, existing.Uuid)
				if err != nil {
					return uuid.Nil, false, nil, err
				}
				c, err := s.Patch(ctx, orgID, existing.Uuid, in)
				if err != nil {
					return uuid.Nil, false, nil, err
				}
				return c.UUID, false, &prev, nil
			}
			if !errors.Is(repository.MapErr(err), repository.ErrNotFound) {
				return uuid.Nil, false, nil, err
			}
		}
	}
	c, err := s.Create(ctx, orgID, in, "import")
	if err != nil {
		return uuid.Nil, false, nil, err
	}
	return c.UUID, true, nil, nil
}

// Activity lists the audit events of a customer.
func (s *Service) Activity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	if _, err := s.load(ctx, s.repo.Q(), orgID, id); err != nil {
		return nil, 0, err
	}
	q := s.repo.Q()
	rows, err := q.ListOrganizationResourceActivity(ctx, db.ListOrganizationResourceActivityParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Resource: model.Slug,
		ResourceUuid: pgtype.UUID{Bytes: id, Valid: true}, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountOrganizationResourceActivity(ctx, db.CountOrganizationResourceActivityParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, Resource: model.Slug,
		ResourceUuid: pgtype.UUID{Bytes: id, Valid: true},
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ActivityEntry, 0, len(rows))
	for _, row := range rows {
		e := model.ActivityEntry{UUID: row.Uuid, Action: row.Action, Payload: json.RawMessage(row.Payload), CreatedAt: row.CreatedAt.Time}
		if row.ActorUuid.Valid {
			e.Actor = &model.ActivityActor{
				UUID: uuid.UUID(row.ActorUuid.Bytes), Name: row.ActorName.String,
				Surname: row.ActorSurname.String, Email: row.ActorEmail.String,
			}
		}
		out = append(out, e)
	}
	return out, total, nil
}

// load reads one live customer of the organization.
func (s *Service) load(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetCustomerRow, error) {
	row, err := q.GetCustomer(ctx, db.GetCustomerParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return db.GetCustomerRow{}, mapNotFound(repository.MapErr(err))
	}
	return row, nil
}

// checkPhone answers ErrPhoneTaken when another live record of the business
// has the phone (the unique index is the backstop).
func (s *Service) checkPhone(ctx context.Context, orgID int64, p pgtype.Text, exclude *int64) error {
	if !p.Valid {
		return nil
	}
	excl := pgtype.Int8{}
	if exclude != nil {
		excl = pgtype.Int8{Int64: *exclude, Valid: true}
	}
	taken, err := s.repo.Q().CustomerPhoneTaken(ctx, db.CustomerPhoneTakenParams{OrganizationID: orgID, PhoneE164: p, ExcludeID: excl})
	if err != nil {
		return err
	}
	if taken {
		return ErrPhoneTaken
	}
	return nil
}

func (s *Service) index(ctx context.Context, orgID int64, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, SearchSpec, searchengine.TenantDocumentID(orgID, id.String()))
	}
}

// fields are the writable columns of a customer.
type fields struct {
	FirstName, LastName      string
	Phone                    pgtype.Text
	Email                    string
	BirthDate                pgtype.Date
	Notes                    string
	Tags                     []string
	SMSConsent, EmailConsent bool
	ConsentUpdatedAt         pgtype.Timestamptz
}

func fieldsFromRow(row db.GetCustomerRow) fields {
	tags := row.Tags
	if tags == nil {
		tags = []string{}
	}
	return fields{
		FirstName: row.FirstName, LastName: row.LastName, Phone: row.PhoneE164, Email: row.Email,
		BirthDate: row.BirthDate, Notes: row.Notes, Tags: tags, SMSConsent: row.SmsConsent,
		EmailConsent: row.EmailConsent, ConsentUpdatedAt: row.ConsentUpdatedAt,
	}
}

func (f *fields) apply(ve *ValidationError, in model.Input, create bool, now time.Time) {
	text := func(field string, src *string, dst *string, max int, required bool) {
		if src == nil {
			if create && required {
				ve.add(field, "required", field+" is required")
			}
			return
		}
		v := strings.TrimSpace(*src)
		switch {
		case required && v == "":
			ve.add(field, "required", field+" is required")
		case utf8.RuneCountInString(v) > max:
			ve.add(field, "too_long", fmt.Sprintf("%s must be at most %d characters", field, max))
		default:
			*dst = v
		}
	}
	text("first_name", in.FirstName, &f.FirstName, maxNameLength, true)
	text("last_name", in.LastName, &f.LastName, maxNameLength, false)
	text("notes", in.Notes, &f.Notes, maxNotesLength, false)
	if in.Email != nil {
		v := strings.TrimSpace(*in.Email)
		switch {
		case v == "":
			f.Email = ""
		case len(v) > maxEmailLength:
			ve.add("email", "too_long", fmt.Sprintf("email must be at most %d characters", maxEmailLength))
		default:
			if addr, err := mail.ParseAddress(v); err != nil || addr.Address != v || !strings.Contains(v, "@") {
				ve.add("email", "invalid", "email is not a valid address")
			} else {
				f.Email = strings.ToLower(v)
			}
		}
	}
	if in.Phone.Set {
		raw := ""
		if in.Phone.Value != nil {
			raw = *in.Phone.Value
		}
		normalized, err := phone.Normalize(raw, "")
		switch {
		case err != nil:
			ve.add("phone", "invalid", "phone is not a valid number")
		case normalized == "":
			f.Phone = pgtype.Text{}
		default:
			f.Phone = pgtype.Text{String: normalized, Valid: true}
		}
	}
	if in.BirthDate.Set {
		raw := ""
		if in.BirthDate.Value != nil {
			raw = strings.TrimSpace(*in.BirthDate.Value)
		}
		if raw == "" {
			f.BirthDate = pgtype.Date{}
		} else if d, err := time.Parse(time.DateOnly, raw); err != nil || d.Year() < 1900 || d.After(now) {
			ve.add("birth_date", "invalid", "birth_date must be a past date (YYYY-MM-DD)")
		} else {
			f.BirthDate = pgtype.Date{Time: d, Valid: true}
		}
	}
	if in.Tags != nil {
		f.Tags = cleanTags(ve, *in.Tags)
	}
	consentChanged := false
	if in.SMSConsent != nil && *in.SMSConsent != f.SMSConsent {
		f.SMSConsent, consentChanged = *in.SMSConsent, true
	}
	if in.EmailConsent != nil && *in.EmailConsent != f.EmailConsent {
		f.EmailConsent, consentChanged = *in.EmailConsent, true
	}
	if consentChanged {
		f.ConsentUpdatedAt = pgtype.Timestamptz{Time: now, Valid: true}
	}
}

// cleanTags trims tags, drops empty ones and duplicates (case-insensitive,
// first spelling wins) and enforces the limits.
func cleanTags(ve *ValidationError, in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for i, raw := range in {
		t := strings.Join(strings.Fields(raw), " ")
		if t == "" {
			continue
		}
		if utf8.RuneCountInString(t) > maxTagLength {
			ve.add(fmt.Sprintf("tags.%d", i), "too_long", fmt.Sprintf("a tag must be at most %d characters", maxTagLength))
			continue
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	if len(out) > maxTags {
		ve.add("tags", "too_many", fmt.Sprintf("at most %d tags", maxTags))
	}
	return out
}

// MapCustomer maps a customer row (list, detail and export share the columns).
func MapCustomer(row db.GetCustomerRow) model.Customer {
	tags := row.Tags
	if tags == nil {
		tags = []string{}
	}
	c := model.Customer{
		UUID: row.Uuid, FirstName: row.FirstName, LastName: row.LastName,
		FullName: strings.TrimSpace(row.FirstName + " " + row.LastName), Email: row.Email, Notes: row.Notes,
		Tags: tags, SMSConsent: row.SmsConsent, EmailConsent: row.EmailConsent, Source: row.Source,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.PhoneE164.Valid {
		p := row.PhoneE164.String
		c.Phone = &p
		c.PhoneDisplay = phone.Display(p, "")
	}
	if row.BirthDate.Valid {
		d := row.BirthDate.Time.Format(time.DateOnly)
		c.BirthDate = &d
	}
	if row.ConsentUpdatedAt.Valid {
		t := row.ConsentUpdatedAt.Time
		c.ConsentUpdatedAt = &t
	}
	if row.LinkSource.Valid {
		c.Verification = &model.Verification{Source: row.LinkSource.String, VerifiedAt: row.LinkVerifiedAt.Time}
	}
	return c
}

func mapNotFound(err error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func mapErr(err error) error {
	switch mapped := repository.MapErr(err); {
	case errors.Is(mapped, repository.ErrPhoneTaken):
		return ErrPhoneTaken
	case errors.Is(mapped, repository.ErrIdentityLinked):
		return ErrIdentityLinked
	default:
		return mapped
	}
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func textArg(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func boolArg(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}
