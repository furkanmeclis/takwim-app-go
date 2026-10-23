// Package usecase manages an organization's integration accounts (TEC-880,
// docs/design/integration-jobs.md): connect a provider with the organization's
// own credentials (sealed by the integrations Vault), test the connection,
// rotate keys, disable / enable. Answers carry masked hints only; the
// plaintext credentials leave this package solely as integrations.Account
// values handed to adapters inside the backend.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Account statuses.
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

var (
	// ErrNotFound: unknown account (or another organization's).
	ErrNotFound = errors.New("integration account not found")
	// ErrActiveExists: the organization already has an active account of
	// the kind (one active payment account at a time).
	ErrActiveExists = errors.New("an active account of this kind exists; disable it first")
	// ErrNoActiveAccount: no active account of the kind is connected.
	ErrNoActiveAccount = errors.New("no active integration account of this kind")
)

// Detail is a field-level validation problem.
type Detail struct{ Field, Code, Message string }

// ValidationError carries field details (400).
type ValidationError struct{ Details []Detail }

func (e *ValidationError) Error() string {
	if len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, Detail{Field: field, Code: code, Message: msg})
}

// CheckError is a failed connection test (the account is kept, marked).
type CheckError struct{ Message string }

func (e *CheckError) Error() string { return e.Message }

// Actor is a user reference.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// Account is an integration account as the API shows it (no secrets).
type Account struct {
	UUID            uuid.UUID            `json:"uuid"`
	Kind            string               `json:"kind"`
	Provider        string               `json:"provider"`
	DisplayName     string               `json:"display_name"`
	Mode            string               `json:"mode"`
	Status          string               `json:"status"`
	CredentialsHint map[string]string    `json:"credentials_hint"`
	LastCheckedAt   *time.Time           `json:"last_checked_at"`
	LastCheckStatus string               `json:"last_check_status"`
	LastCheckError  string               `json:"last_check_error"`
	Fields          []integrations.Field `json:"fields,omitempty"`
	CreatedBy       *Actor               `json:"created_by,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

// ProviderInfo describes a connectable provider (connect form).
type ProviderInfo struct {
	Kind     string               `json:"kind"`
	Provider string               `json:"provider"`
	Fields   []integrations.Field `json:"fields"`
	Modes    []string             `json:"modes"`
}

// ConnectInput is POST /v1/tenant/integrations.
type ConnectInput struct {
	Kind        string            `json:"kind"`
	Provider    string            `json:"provider"`
	DisplayName string            `json:"display_name"`
	Mode        string            `json:"mode"`
	Credentials map[string]string `json:"credentials"`
}

// UpdateInput is PATCH /v1/tenant/integrations/{uuid}: rename, change the
// mode, rotate credentials (only the given keys are replaced).
type UpdateInput struct {
	DisplayName *string           `json:"display_name"`
	Mode        *string           `json:"mode"`
	Credentials map[string]string `json:"credentials"`
}

// ListFilter selects a page of accounts.
type ListFilter struct {
	Q               string
	Statuses, Kinds []string
	Providers       []string
	SortField       string
	SortDesc        bool
	Limit, Offset   int32
}

// Job is one integration job row.
type Job struct {
	UUID          uuid.UUID  `json:"uuid"`
	Direction     string     `json:"direction"`
	Provider      string     `json:"provider"`
	Kind          string     `json:"kind"`
	Status        string     `json:"status"`
	Priority      string     `json:"priority"`
	Attempts      int32      `json:"attempts"`
	MaxAttempts   int32      `json:"max_attempts"`
	Deliveries    int32      `json:"deliveries"`
	LastError     string     `json:"last_error"`
	RefType       string     `json:"ref_type"`
	RefUUID       *uuid.UUID `json:"ref_uuid"`
	NextAttemptAt *time.Time `json:"next_attempt_at"`
	SubmittedAt   *time.Time `json:"submitted_at"`
	CompletedAt   *time.Time `json:"completed_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// JobFilter selects a page of an account's jobs.
type JobFilter struct {
	Statuses, Kinds []string
	SortField       string
	SortDesc        bool
	Limit, Offset   int32
}

// Service implements the integration accounts.
type Service struct {
	q     *db.Queries
	vault *integrations.Vault
	reg   *integrations.Registry
	now   func() time.Time
}

// New builds the service.
func New(q *db.Queries, vault *integrations.Vault, reg *integrations.Registry) *Service {
	return &Service{q: q, vault: vault, reg: reg, now: time.Now}
}

// Registry returns the adapter registry.
func (s *Service) Registry() *integrations.Registry { return s.reg }

func actorID(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func hintOf(raw []byte) map[string]string {
	out := map[string]string{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func validMode(m string) bool { return m == integrations.ModeSandbox || m == integrations.ModeLive }

// Providers lists the connectable providers with their credential fields.
func (s *Service) Providers() []ProviderInfo {
	out := []ProviderInfo{}
	for _, a := range s.reg.Providers() {
		out = append(out, ProviderInfo{
			Kind: a.Kind(), Provider: a.Provider(), Fields: a.Fields(),
			Modes: []string{integrations.ModeSandbox, integrations.ModeLive},
		})
	}
	return out
}

// List returns a page of the organization's accounts.
func (s *Service) List(ctx context.Context, orgID int64, f ListFilter) ([]Account, int64, error) {
	p := db.ListIntegrationAccountsParams{
		OrganizationID: orgID, SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if p.SortField == "" {
		p.SortField, p.SortDesc = "created_at", true
	}
	if len(f.Statuses) > 0 {
		p.Statuses = f.Statuses
	}
	if len(f.Kinds) > 0 {
		p.Kinds = f.Kinds
	}
	if len(f.Providers) > 0 {
		p.Providers = f.Providers
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Q = pgtype.Text{String: q, Valid: true}
	}
	rows, err := s.q.ListIntegrationAccounts(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Account, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, Account{
			UUID: r.Uuid, Kind: r.Kind, Provider: r.Provider, DisplayName: r.DisplayName, Mode: r.Mode, Status: r.Status,
			CredentialsHint: hintOf(r.CredentialsHint), LastCheckedAt: timePtr(r.LastCheckedAt),
			LastCheckStatus: r.LastCheckStatus, LastCheckError: r.LastCheckError, CreatedAt: r.CreatedAt.Time.UTC(),
			UpdatedAt: r.UpdatedAt.Time.UTC(),
		})
	}
	return out, total, nil
}

func (s *Service) row(ctx context.Context, orgID int64, id uuid.UUID) (db.GetIntegrationAccountRow, error) {
	r, err := s.q.GetIntegrationAccount(ctx, db.GetIntegrationAccountParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// Get returns one account.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (Account, error) {
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return Account{}, err
	}
	a := Account{
		UUID: r.Uuid, Kind: r.Kind, Provider: r.Provider, DisplayName: r.DisplayName, Mode: r.Mode, Status: r.Status,
		CredentialsHint: hintOf(r.CredentialsHint), LastCheckedAt: timePtr(r.LastCheckedAt),
		LastCheckStatus: r.LastCheckStatus, LastCheckError: r.LastCheckError, CreatedAt: r.CreatedAt.Time.UTC(),
		UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
	if ad, ok := s.reg.Get(r.Kind, r.Provider); ok {
		a.Fields = ad.Fields()
	}
	if r.CreatedByUuid.Valid {
		a.CreatedBy = &Actor{
			UUID: r.CreatedByUuid.Bytes, Name: r.CreatedByName.String, Surname: r.CreatedBySurname.String,
			Email: r.CreatedByEmail.String,
		}
	}
	return a, nil
}

func uniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

// Connect stores a new account (active). Credentials are validated for
// shape here; Test proves them against the provider.
func (s *Service) Connect(ctx context.Context, orgID int64, in ConnectInput) (Account, error) {
	ve := &ValidationError{}
	in.Kind, in.Provider = strings.TrimSpace(in.Kind), strings.TrimSpace(in.Provider)
	if in.Kind == "" {
		// The provider names its kind (netgsm → sms).
		in.Kind = integrations.KindPayment
		for _, a := range s.reg.Providers() {
			if a.Provider() == in.Provider {
				in.Kind = a.Kind()
				break
			}
		}
	}
	ad, ok := s.reg.Get(in.Kind, in.Provider)
	if !ok {
		ve.add("provider", "invalid", "unknown provider for this kind")
	}
	name := strings.TrimSpace(in.DisplayName)
	if name == "" || len(name) > 120 {
		ve.add("display_name", "required", "display_name is required (at most 120 characters)")
	}
	if in.Mode == "" {
		in.Mode = integrations.ModeSandbox
	}
	if !validMode(in.Mode) {
		ve.add("mode", "invalid", "mode must be sandbox or live")
	}
	var creds integrations.Credentials
	if ok {
		var missing []string
		creds, missing = integrations.ValidateCredentials(ad, in.Credentials)
		for _, m := range missing {
			ve.add("credentials."+m, "required", m+" is required")
		}
	}
	if len(ve.Details) > 0 {
		return Account{}, ve
	}
	id := uuid.New()
	sealed, err := s.vault.Seal(orgID, id, creds)
	if err != nil {
		return Account{}, err
	}
	hint, _ := json.Marshal(integrations.Hint(ad, creds))
	_, err = s.q.InsertIntegrationAccount(ctx, db.InsertIntegrationAccountParams{
		Uuid: id, OrganizationID: orgID, Kind: in.Kind, Provider: in.Provider, DisplayName: name, Mode: in.Mode,
		CredentialsCiphertext: sealed, CredentialsHint: hint, Settings: []byte("{}"), CreatedBy: actorID(ctx),
	})
	if uniqueViolation(err) {
		return Account{}, ErrActiveExists
	}
	if err != nil {
		return Account{}, err
	}
	return s.Get(ctx, orgID, id)
}

// Update renames, switches the mode and / or rotates credentials. A
// rotation or a mode change resets the last connection check.
func (s *Service) Update(ctx context.Context, orgID int64, id uuid.UUID, in UpdateInput) (Account, []string, error) {
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return Account{}, nil, err
	}
	ad, ok := s.reg.Get(r.Kind, r.Provider)
	if !ok {
		return Account{}, nil, &ValidationError{Details: []Detail{{Field: "provider", Code: "invalid", Message: "provider is not available"}}}
	}
	ve := &ValidationError{}
	changed := []string{}
	name, mode := r.DisplayName, r.Mode
	if in.DisplayName != nil {
		name = strings.TrimSpace(*in.DisplayName)
		if name == "" || len(name) > 120 {
			ve.add("display_name", "required", "display_name is required (at most 120 characters)")
		} else if name != r.DisplayName {
			changed = append(changed, "display_name")
		}
	}
	if in.Mode != nil {
		mode = *in.Mode
		if !validMode(mode) {
			ve.add("mode", "invalid", "mode must be sandbox or live")
		} else if mode != r.Mode {
			changed = append(changed, "mode")
		}
	}
	creds, err := s.vault.Open(orgID, r.Uuid, r.CredentialsCiphertext)
	if err != nil {
		return Account{}, nil, err
	}
	known := map[string]bool{}
	for _, f := range ad.Fields() {
		known[f.Key] = true
	}
	for k, v := range in.Credentials {
		if !known[k] {
			ve.add("credentials."+k, "unknown", "unknown credential field")
			continue
		}
		if v = strings.TrimSpace(v); v != "" && v != creds[k] {
			creds[k] = v
			changed = append(changed, "credentials."+k)
		}
	}
	if len(ve.Details) > 0 {
		return Account{}, nil, ve
	}
	if len(changed) == 0 {
		a, err := s.Get(ctx, orgID, id)
		return a, changed, err
	}
	sealed, err := s.vault.Seal(orgID, r.Uuid, creds)
	if err != nil {
		return Account{}, nil, err
	}
	hint, _ := json.Marshal(integrations.Hint(ad, creds))
	if _, err := s.q.UpdateIntegrationAccount(ctx, db.UpdateIntegrationAccountParams{
		OrganizationID: orgID, ID: r.ID, DisplayName: name, Mode: mode, CredentialsCiphertext: sealed,
		CredentialsHint: hint, Settings: r.Settings, UpdatedBy: actorID(ctx),
	}); err != nil {
		return Account{}, nil, err
	}
	for _, c := range changed {
		if c != "display_name" {
			if err := s.q.SetIntegrationAccountCheck(ctx, db.SetIntegrationAccountCheckParams{
				OrganizationID: orgID, ID: r.ID, CheckedAt: pgtype.Timestamptz{}, CheckStatus: "", CheckError: "",
			}); err != nil {
				return Account{}, nil, err
			}
			break
		}
	}
	a, err := s.Get(ctx, orgID, id)
	return a, changed, err
}

// account builds the adapter view of a row.
func (s *Service) account(orgID int64, id int64, accUUID uuid.UUID, kind, provider, mode, ciphertext string) (integrations.Account, error) {
	creds, err := s.vault.Open(orgID, accUUID, ciphertext)
	if err != nil {
		return integrations.Account{}, err
	}
	return integrations.Account{
		ID: id, OrganizationID: orgID, UUID: accUUID, Kind: kind, Provider: provider, Mode: mode, Credentials: creds,
		Settings: map[string]any{},
	}, nil
}

// Test checks the connection with the provider and records the result.
func (s *Service) Test(ctx context.Context, orgID int64, id uuid.UUID) (Account, error) {
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return Account{}, err
	}
	ad, ok := s.reg.Get(r.Kind, r.Provider)
	if !ok {
		return Account{}, &CheckError{Message: "provider is not available"}
	}
	acc, err := s.account(orgID, r.ID, r.Uuid, r.Kind, r.Provider, r.Mode, r.CredentialsCiphertext)
	if err != nil {
		return Account{}, err
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	checkErr := ad.Check(cctx, acc)
	status, msg := "ok", ""
	if checkErr != nil {
		status, msg = "failed", checkErr.Error()
		if len(msg) > 500 {
			msg = msg[:500]
		}
	}
	if err := s.q.SetIntegrationAccountCheck(ctx, db.SetIntegrationAccountCheckParams{
		OrganizationID: orgID, ID: r.ID, CheckedAt: pgtype.Timestamptz{Time: s.now().UTC(), Valid: true},
		CheckStatus: status, CheckError: msg,
	}); err != nil {
		return Account{}, err
	}
	a, err := s.Get(ctx, orgID, id)
	if err != nil {
		return Account{}, err
	}
	if checkErr != nil {
		return a, &CheckError{Message: msg}
	}
	return a, nil
}

// SetStatus disables or (re-)enables an account.
func (s *Service) SetStatus(ctx context.Context, orgID int64, id uuid.UUID, status string) (Account, error) {
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return Account{}, err
	}
	if r.Status != status {
		_, err := s.q.SetIntegrationAccountStatus(ctx, db.SetIntegrationAccountStatusParams{
			OrganizationID: orgID, ID: r.ID, Status: status, UpdatedBy: actorID(ctx),
		})
		if uniqueViolation(err) {
			return Account{}, ErrActiveExists
		}
		if err != nil {
			return Account{}, err
		}
	}
	return s.Get(ctx, orgID, id)
}

// Jobs lists the integration jobs of an account.
func (s *Service) Jobs(ctx context.Context, orgID int64, id uuid.UUID, f JobFilter) ([]Job, int64, error) {
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return nil, 0, err
	}
	return s.listJobs(ctx, orgID, pgtype.Int8{Int64: r.ID, Valid: true}, f)
}

// AllJobs lists the organization's integration jobs across accounts (the
// failed / needs-verification work list filters by status).
func (s *Service) AllJobs(ctx context.Context, orgID int64, f JobFilter) ([]Job, int64, error) {
	return s.listJobs(ctx, orgID, pgtype.Int8{}, f)
}

func (s *Service) listJobs(ctx context.Context, orgID int64, accountID pgtype.Int8, f JobFilter) ([]Job, int64, error) {
	p := db.ListIntegrationJobsParams{
		OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, AccountID: accountID,
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if p.SortField == "" {
		p.SortField, p.SortDesc = "created_at", true
	}
	if len(f.Statuses) > 0 {
		p.Statuses = f.Statuses
	}
	if len(f.Kinds) > 0 {
		p.Kinds = f.Kinds
	}
	rows, err := s.q.ListIntegrationJobs(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Job, 0, len(rows))
	var total int64
	for _, j := range rows {
		total = j.Total
		job := Job{
			UUID: j.Uuid, Direction: j.Direction, Provider: j.Provider, Kind: j.Kind, Status: j.Status, Priority: j.Priority,
			Attempts: j.Attempts, MaxAttempts: j.MaxAttempts, Deliveries: j.Deliveries, LastError: j.LastError,
			RefType: j.RefType, NextAttemptAt: timePtr(j.NextAttemptAt), SubmittedAt: timePtr(j.SubmittedAt),
			CompletedAt: timePtr(j.CompletedAt), CreatedAt: j.CreatedAt.Time.UTC(), UpdatedAt: j.UpdatedAt.Time.UTC(),
		}
		if j.RefUuid.Valid {
			u := uuid.UUID(j.RefUuid.Bytes)
			job.RefUUID = &u
		}
		out = append(out, job)
	}
	return out, total, nil
}

// ActiveAccount returns the adapter view of the organization's active
// account of a kind (ErrNoActiveAccount when none).
func (s *Service) ActiveAccount(ctx context.Context, orgID int64, kind string) (integrations.Account, error) {
	r, err := s.q.GetActiveIntegrationAccount(ctx, db.GetActiveIntegrationAccountParams{OrganizationID: orgID, Kind: kind})
	if errors.Is(err, pgx.ErrNoRows) {
		return integrations.Account{}, ErrNoActiveAccount
	}
	if err != nil {
		return integrations.Account{}, err
	}
	return s.account(orgID, r.ID, r.Uuid, r.Kind, r.Provider, r.Mode, r.CredentialsCiphertext)
}

// AccountByUUID returns the adapter view of an account by uuid (any
// status: a connection test may target a disabled account).
func (s *Service) AccountByUUID(ctx context.Context, orgID int64, id uuid.UUID) (integrations.Account, error) {
	r, err := s.row(ctx, orgID, id)
	if err != nil {
		return integrations.Account{}, err
	}
	return s.account(orgID, r.ID, r.Uuid, r.Kind, r.Provider, r.Mode, r.CredentialsCiphertext)
}

// AccountByID returns the adapter view of an account by internal id (a
// payment keeps the account it was opened with, even after a rotation or
// when it was disabled later).
func (s *Service) AccountByID(ctx context.Context, orgID, id int64) (integrations.Account, error) {
	r, err := s.q.GetIntegrationAccountByID(ctx, db.GetIntegrationAccountByIDParams{OrganizationID: orgID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return integrations.Account{}, ErrNotFound
	}
	if err != nil {
		return integrations.Account{}, err
	}
	return s.account(orgID, r.ID, r.Uuid, r.Kind, r.Provider, r.Mode, r.CredentialsCiphertext)
}
