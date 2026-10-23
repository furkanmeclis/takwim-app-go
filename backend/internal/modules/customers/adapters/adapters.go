// Package adapters connects the customers resource to the shared engines:
// ioengine (export / import), bulkengine (tag / untag / delete) and
// searchengine (Cmd+K). The organization always comes from the context
// (orgctx.OrganizationID); without one every adapter call fails.
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/phone"
	"github.com/google/uuid"
)

// Resource is the engine slug of customers.
const Resource = model.Slug

// MaxBulkTargets caps the "all matching" bulk scope.
const MaxBulkTargets = 10000

// MaxExportRows caps one export.
const MaxExportRows = 50000

var errNoOrganization = errors.New("customers: organization scope is required")

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

// FilterFromQuery maps list query params (q, tag, source, sms_consent,
// email_consent, verified) to a filter.
func FilterFromQuery(query map[string]string) model.ListFilter {
	f := model.ListFilter{Q: strings.TrimSpace(query["q"])}
	f.Tags = apiquery.SplitCSV(query["tag"])
	f.Sources = apiquery.SplitCSV(query["source"])
	for _, b := range []struct {
		key string
		dst **bool
	}{{"sms_consent", &f.SMSConsent}, {"email_consent", &f.EmailConsent}, {"verified", &f.Verified}} {
		if v, err := strconv.ParseBool(strings.TrimSpace(query[b.key])); err == nil {
			*b.dst = &v
		}
	}
	return f
}

// ---------------------------------------------------------------- io engine

// IO exports and imports customers.
type IO struct {
	svc *usecase.Service
}

// NewIO builds the io adapter.
func NewIO(svc *usecase.Service) *IO {
	return &IO{svc: svc}
}

func (a *IO) Resource() string { return Resource }

func (a *IO) ExportColumns() []ioengine.Column {
	return []ioengine.Column{
		{Key: "first_name", LabelKey: "customer.first_name", Type: ioengine.ColumnTypeString},
		{Key: "last_name", LabelKey: "customer.last_name", Type: ioengine.ColumnTypeString},
		{Key: "phone", LabelKey: "customer.phone", Type: ioengine.ColumnTypeString},
		{Key: "email", LabelKey: "customer.email", Type: ioengine.ColumnTypeString},
		{Key: "birth_date", LabelKey: "customer.birth_date", Type: ioengine.ColumnTypeString},
		{Key: "tags", LabelKey: "customer.tags", Type: ioengine.ColumnTypeString},
		{Key: "sms_consent", LabelKey: "customer.sms_consent", Type: ioengine.ColumnTypeBoolean},
		{Key: "email_consent", LabelKey: "customer.email_consent", Type: ioengine.ColumnTypeBoolean},
		{Key: "verified", LabelKey: "customer.verified", Type: ioengine.ColumnTypeBoolean},
		{Key: "source", LabelKey: "customer.source", Type: ioengine.ColumnTypeEnum},
		{Key: "notes", LabelKey: "customer.notes", Type: ioengine.ColumnTypeString},
		{Key: "uuid", LabelKey: "customer.uuid", Type: ioengine.ColumnTypeUUID},
		{Key: "created_at", LabelKey: "customer.created_at", Type: ioengine.ColumnTypeDatetime},
	}
}

func (a *IO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.ExportRows(ctx, orgID, FilterFromQuery(query), MaxExportRows)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, c := range rows {
		p, birth := "", ""
		if c.Phone != nil {
			p = *c.Phone
		}
		if c.BirthDate != nil {
			birth = *c.BirthDate
		}
		out = append(out, map[string]any{
			"uuid": c.UUID.String(), "first_name": c.FirstName, "last_name": c.LastName, "phone": p,
			"email": c.Email, "birth_date": birth, "tags": strings.Join(c.Tags, ","),
			"sms_consent": c.SMSConsent, "email_consent": c.EmailConsent, "verified": c.Verification != nil,
			"source": c.Source, "notes": c.Notes, "created_at": c.CreatedAt,
		})
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: out}, nil
}

// ImportSchema: a row updates the customer with the same phone, otherwise a
// new customer is created. Imported phones stay unverified.
func (a *IO) ImportSchema() []ioengine.ImportField {
	return []ioengine.ImportField{
		{Key: "first_name", LabelKey: "customer.first_name", Type: ioengine.ColumnTypeString, Required: true},
		{Key: "last_name", LabelKey: "customer.last_name", Type: ioengine.ColumnTypeString},
		{Key: "phone", LabelKey: "customer.phone", Type: ioengine.ColumnTypeString},
		{Key: "email", LabelKey: "customer.email", Type: ioengine.ColumnTypeString},
		{Key: "birth_date", LabelKey: "customer.birth_date", Type: ioengine.ColumnTypeString},
		{Key: "tags", LabelKey: "customer.tags", Type: ioengine.ColumnTypeString},
		{Key: "sms_consent", LabelKey: "customer.sms_consent", Type: ioengine.ColumnTypeBoolean, DefaultHint: "false"},
		{Key: "email_consent", LabelKey: "customer.email_consent", Type: ioengine.ColumnTypeBoolean, DefaultHint: "false"},
		{Key: "notes", LabelKey: "customer.notes", Type: ioengine.ColumnTypeString},
	}
}

// SampleRows implements ioengine.SampleProvider.
func (a *IO) SampleRows() []map[string]any {
	return []map[string]any{{
		"first_name": "Ayşe", "last_name": "Yılmaz", "phone": "0532 123 45 67", "email": "ayse@example.com",
		"birth_date": "1990-04-12", "tags": "vip,düzenli", "sms_consent": "true", "email_consent": "false",
		"notes": "Kısa kesim tercih eder",
	}}
}

// ApplyRow creates a customer, or updates the one with the same phone.
func (a *IO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	in, err := inputFromRow(row, defaults)
	if err != nil {
		return ioengine.RowResult{OK: false, Error: err.Error()}, nil
	}
	got, created, prev, err := a.svc.ImportRow(ctx, orgID, in)
	if err != nil {
		if usecase.IsValidation(err) || errors.Is(err, usecase.ErrNotFound) || errors.Is(err, usecase.ErrPhoneTaken) {
			return ioengine.RowResult{OK: false, Error: err.Error()}, nil
		}
		return ioengine.RowResult{}, err
	}
	res := ioengine.RowResult{OK: true, EntityType: "customer", EntityUUID: got.String(), Op: "create"}
	if !created && prev != nil {
		res.Op = "update"
		res.Previous = snapshot(*prev)
	}
	return res, nil
}

// RevertRow deletes an imported customer or restores an updated one.
func (a *IO) RevertRow(ctx context.Context, entityType, entityUUID string, previous map[string]any) error {
	if entityType != "customer" {
		return fmt.Errorf("unsupported entity %q", entityType)
	}
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	if len(previous) == 0 {
		_, err := a.svc.Delete(ctx, orgID, id)
		return err
	}
	raw, err := json.Marshal(previous)
	if err != nil {
		return err
	}
	var prev model.Customer
	if err := json.Unmarshal(raw, &prev); err != nil {
		return err
	}
	in := model.Input{
		FirstName: &prev.FirstName, LastName: &prev.LastName, Email: &prev.Email, Notes: &prev.Notes,
		Tags: &prev.Tags, SMSConsent: &prev.SMSConsent, EmailConsent: &prev.EmailConsent,
		Phone:     model.OptionalString{Set: true, Value: prev.Phone},
		BirthDate: model.OptionalString{Set: true, Value: prev.BirthDate},
	}
	_, err = a.svc.Patch(ctx, orgID, id, in)
	return err
}

func inputFromRow(row, defaults map[string]any) (model.Input, error) {
	get := func(key string) (string, bool) {
		if v := cell(row, key); v != "" {
			return v, true
		}
		if v := cell(defaults, key); v != "" {
			return v, true
		}
		return "", false
	}
	str := func(key string) *string {
		if v, ok := get(key); ok {
			return &v
		}
		return nil
	}
	var in model.Input
	first := cell(row, "first_name")
	in.FirstName = &first
	in.LastName = str("last_name")
	in.Email = str("email")
	in.Notes = str("notes")
	if v, ok := get("phone"); ok {
		in.Phone = model.OptionalString{Set: true, Value: &v}
	}
	if v, ok := get("birth_date"); ok {
		in.BirthDate = model.OptionalString{Set: true, Value: &v}
	}
	if v, ok := get("tags"); ok {
		tags := apiquery.SplitCSV(v)
		in.Tags = &tags
	}
	for _, b := range []struct {
		key string
		dst **bool
	}{{"sms_consent", &in.SMSConsent}, {"email_consent", &in.EmailConsent}} {
		if v, ok := get(b.key); ok {
			parsed, err := parseBool(b.key, v)
			if err != nil {
				return in, err
			}
			*b.dst = &parsed
		}
	}
	return in, nil
}

func snapshot(c model.Customer) map[string]any {
	raw, err := json.Marshal(c)
	if err != nil {
		return nil
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func cell(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func parseBool(key, v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "evet", "var", "onaylı", "onayli":
		return true, nil
	case "0", "false", "no", "hayır", "hayir", "yok":
		return false, nil
	}
	return false, fmt.Errorf("%s must be true or false", key)
}

// -------------------------------------------------------------- bulk engine

// Bulk tags, untags and deletes customers.
type Bulk struct {
	svc *usecase.Service
}

// NewBulk builds the bulk adapter. svc may be nil for metadata only.
func NewBulk(svc *usecase.Service) *Bulk {
	return &Bulk{svc: svc}
}

func (a *Bulk) Resource() string { return Resource }

// BulkActions: add_tag / remove_tag take the tag in params["tag"].
func (a *Bulk) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: "add_tag", LabelKey: "bulk.actions.customers.add_tag",
			Permission: rbac.PermTenantCustomersBulkTag, Reversible: true,
		},
		{
			ID: "remove_tag", LabelKey: "bulk.actions.customers.remove_tag",
			Permission: rbac.PermTenantCustomersBulkTag, Reversible: true,
		},
		{
			ID: "delete", LabelKey: "bulk.actions.customers.delete",
			Permission: rbac.PermTenantCustomersBulkDelete, Destructive: true, Reversible: true,
			ConfirmKey: "bulk.confirm.customers.delete",
		},
	}
}

func (a *Bulk) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return nil, err
	}
	if (action == "add_tag" || action == "remove_tag") && strings.TrimSpace(target.Params["tag"]) == "" {
		return nil, fmt.Errorf("params.tag is required")
	}
	switch target.Scope {
	case "ids":
		seen := map[string]bool{}
		out := make([]string, 0, len(target.IDs))
		for _, raw := range target.IDs {
			id, err := uuid.Parse(strings.TrimSpace(raw))
			if err != nil || seen[id.String()] {
				continue
			}
			seen[id.String()] = true
			out = append(out, id.String())
		}
		if len(out) > MaxBulkTargets {
			return nil, fmt.Errorf("at most %d targets are allowed", MaxBulkTargets)
		}
		return out, nil
	case "query":
		ids, err := a.svc.TargetUUIDs(ctx, orgID, FilterFromQuery(target.Query), MaxBulkTargets)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			out = append(out, id.String())
		}
		return out, nil
	default:
		return nil, fmt.Errorf("invalid target scope")
	}
}

func (a *Bulk) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "invalid uuid"}, nil
	}
	fail := func(err error) (bulkengine.BulkItemResult, error) {
		if errors.Is(err, usecase.ErrNotFound) {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "not found"}, nil
		}
		if usecase.IsValidation(err) {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: err.Error()}, nil
		}
		return bulkengine.BulkItemResult{}, err
	}
	switch action {
	case "add_tag", "remove_tag":
		tag := strings.Join(strings.Fields(bulkengine.Params(ctx)["tag"]), " ")
		c, err := a.svc.Get(ctx, orgID, id)
		if err != nil {
			return fail(err)
		}
		next := slices.DeleteFunc(slices.Clone(c.Tags), func(t string) bool { return strings.EqualFold(t, tag) })
		if action == "add_tag" {
			next = append(next, tag)
		}
		prev, err := a.svc.SetTags(ctx, orgID, id, next)
		if err != nil {
			return fail(err)
		}
		return bulkengine.BulkItemResult{
			EntityUUID: entityUUID, EntityType: "customer", OK: true, Op: "update",
			Previous: map[string]any{"tags": prev},
		}, nil
	case "delete":
		if _, err := a.svc.Delete(ctx, orgID, id); err != nil {
			return fail(err)
		}
		return bulkengine.BulkItemResult{
			EntityUUID: entityUUID, EntityType: "customer", OK: true, Op: "delete",
			Previous: map[string]any{"deleted": false},
		}, nil
	default:
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, OK: false, Error: "unknown action"}, nil
	}
}

func (a *Bulk) RevertItem(ctx context.Context, action string, entityUUID string, previous map[string]any) error {
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	if action == "delete" {
		return a.svc.Restore(ctx, orgID, id)
	}
	raw, ok := previous["tags"].([]any)
	if !ok {
		return fmt.Errorf("missing previous tags")
	}
	tags := make([]string, 0, len(raw))
	for _, t := range raw {
		if s, ok := t.(string); ok {
			tags = append(tags, s)
		}
	}
	_, err = a.svc.SetTags(ctx, orgID, id, tags)
	return err
}

// ------------------------------------------------------------ search engine

// Search indexes customers for the tenant Cmd+K palette.
type Search struct {
	q *db.Queries
}

// NewSearch builds the search adapter.
func NewSearch(q *db.Queries) *Search {
	return &Search{q: q}
}

func (a *Search) Spec() searchengine.Spec {
	return searchengine.Spec{
		ID:         usecase.SearchSpec,
		LabelKey:   "search.specs_customers",
		Permission: rbac.PermTenantCustomersRead,
		Icon:       "customers",
		Searchable: []string{"title", "subtitle", "keywords"},
		Scope:      searchengine.ScopeTenant,
		// Customers are business level: every reader sees every record.
	}
}

// ListAll walks every live organization (each query is organization-scoped).
func (a *Search) ListAll(ctx context.Context) ([]searchengine.Document, error) {
	orgIDs, err := a.q.ListLiveOrganizationIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]searchengine.Document, 0)
	for _, orgID := range orgIDs {
		rows, err := a.q.ListCustomerSearchDocuments(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, searchDocument(db.GetCustomerSearchDocumentRow(row)))
		}
	}
	return out, nil
}

// Document loads one customer by its tenant document id.
func (a *Search) Document(ctx context.Context, docID string) (searchengine.Document, error) {
	orgID, raw, err := searchengine.ParseTenantDocumentID(docID)
	if err != nil {
		return searchengine.Document{}, err
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return searchengine.Document{}, fmt.Errorf("customers search: invalid uuid")
	}
	row, err := a.q.GetCustomerSearchDocument(ctx, db.GetCustomerSearchDocumentParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return searchengine.Document{}, err
	}
	return searchDocument(row), nil
}

func searchDocument(row db.GetCustomerSearchDocumentRow) searchengine.Document {
	name := strings.TrimSpace(row.FirstName + " " + row.LastName)
	display, subtitle := "", row.Email
	keywords := []string{name, row.Email}
	if row.PhoneE164.Valid {
		display = phone.Display(row.PhoneE164.String, "")
		subtitle = display
		// Digits without the country code, so "532…" and "0532…" both hit.
		keywords = append(keywords, row.PhoneE164.String, strings.ReplaceAll(display, " ", ""))
	}
	return searchengine.Document{
		ID:             searchengine.TenantDocumentID(row.OrganizationID, row.Uuid.String()),
		Spec:           usecase.SearchSpec,
		Title:          name,
		Subtitle:       subtitle,
		Keywords:       keywords,
		Href:           "/t/" + row.OrganizationSlug + "/customers/" + row.Uuid.String(),
		Icon:           "customers",
		OrganizationID: row.OrganizationID,
	}
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ ioengine.SampleProvider  = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*Bulk)(nil)
	_ searchengine.Adapter     = (*Search)(nil)
)
