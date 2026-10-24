// Package integrations is the skeleton every external provider plugs into
// (TEC-880, docs/design/integration-jobs.md): an Adapter per provider (SMS,
// WhatsApp, e-mail and payment providers) registered in a Registry, the
// organization's own credentials sealed by a Vault, and the Account an
// adapter receives (decrypted credentials, mode, settings). Providers are
// swappable: a module talks to the kind's interface, never to a provider.
//
// Secrets never leave the backend: the API answers with Hint values only.
package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/google/uuid"
)

// Integration kinds.
const (
	KindPayment  = "payment"
	KindSMS      = "sms"
	KindWhatsApp = "whatsapp"
	KindEmail    = "email"
	// KindVoiceCall: automatic calls with keys (TEC-923).
	KindVoiceCall = "voice_call"
)

// Account modes.
const (
	ModeSandbox = "sandbox"
	ModeLive    = "live"
)

// Credentials are the secret values of an account (field key → value).
type Credentials map[string]string

// Field describes one credential field of a provider.
type Field struct {
	Key      string `json:"key"`
	LabelKey string `json:"label_key"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
}

// Account is what an adapter works with: one organization's connected
// account with its decrypted credentials. It is never serialized to a
// client.
type Account struct {
	ID             int64
	OrganizationID int64
	UUID           uuid.UUID
	Kind           string
	Provider       string
	Mode           string
	Credentials    Credentials
	Settings       map[string]any
}

// Adapter is one provider integration.
type Adapter interface {
	// Kind is the integration kind (KindPayment …).
	Kind() string
	// Provider is the provider code ("netgsm").
	Provider() string
	// Fields lists the credentials the provider needs.
	Fields() []Field
	// Check tests the connection with the account's credentials (a cheap
	// authenticated call; no money moves).
	Check(ctx context.Context, account Account) error
}

// ErrUnknownProvider: no adapter for the kind / provider.
var ErrUnknownProvider = errors.New("integrations: unknown provider")

// Registry holds the adapters by kind and provider.
type Registry struct {
	adapters map[string]Adapter
}

// NewRegistry registers adapters (nil entries are skipped).
func NewRegistry(adapters ...Adapter) *Registry {
	r := &Registry{adapters: map[string]Adapter{}}
	for _, a := range adapters {
		if a != nil {
			r.adapters[a.Kind()+"/"+a.Provider()] = a
		}
	}
	return r
}

// Get returns the adapter of kind / provider.
func (r *Registry) Get(kind, provider string) (Adapter, bool) {
	if r == nil {
		return nil, false
	}
	a, ok := r.adapters[kind+"/"+provider]
	return a, ok
}

// Providers lists the registered adapters in a stable order.
func (r *Registry) Providers() []Adapter {
	if r == nil {
		return nil
	}
	keys := make([]string, 0, len(r.adapters))
	for k := range r.adapters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Adapter, 0, len(keys))
	for _, k := range keys {
		out = append(out, r.adapters[k])
	}
	return out
}

// ValidateCredentials checks the required fields and drops unknown keys.
func ValidateCredentials(a Adapter, in Credentials) (Credentials, []string) {
	out := Credentials{}
	missing := []string{}
	for _, f := range a.Fields() {
		v := strings.TrimSpace(in[f.Key])
		if v == "" {
			if f.Required {
				missing = append(missing, f.Key)
			}
			continue
		}
		out[f.Key] = v
	}
	return out, missing
}

// Mask shows the first four and the last four characters of a secret of
// at least 12 characters, else only bullets ("sand••••1234").
func Mask(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if len(v) < 12 {
		return "••••"
	}
	return v[:4] + "••••" + v[len(v)-4:]
}

// Hint is the displayable form of credentials: every field masked.
func Hint(a Adapter, c Credentials) map[string]string {
	out := map[string]string{}
	for _, f := range a.Fields() {
		if v := c[f.Key]; v != "" {
			out[f.Key] = Mask(v)
		}
	}
	return out
}

// CredentialsPurpose is the field cipher purpose of account credentials.
const CredentialsPurpose = "integration.credentials"

// Vault seals account credentials with purpose-separated keys; the
// associated data binds a ciphertext to its organization and account.
type Vault struct {
	sealer *crypto.Sealer
}

// NewVault builds the vault on the field cipher.
func NewVault(fc *crypto.FieldCipher) (*Vault, error) {
	if fc == nil {
		return nil, errors.New("integrations: field cipher required")
	}
	s, err := fc.Sealer(CredentialsPurpose)
	if err != nil {
		return nil, err
	}
	return &Vault{sealer: s}, nil
}

func aad(orgID int64, account uuid.UUID) string {
	return "integration_accounts.credentials|org:" + strconv.FormatInt(orgID, 10) + "|account:" + account.String()
}

// Seal encrypts credentials for an account.
func (v *Vault) Seal(orgID int64, account uuid.UUID, c Credentials) (string, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return v.sealer.Seal(string(raw), aad(orgID, account))
}

// Open decrypts the credentials of an account.
func (v *Vault) Open(orgID int64, account uuid.UUID, ciphertext string) (Credentials, error) {
	plain, err := v.sealer.Open(ciphertext, aad(orgID, account))
	if err != nil {
		return nil, fmt.Errorf("integrations: open credentials: %w", err)
	}
	out := Credentials{}
	if plain == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(plain), &out); err != nil {
		return nil, fmt.Errorf("integrations: credentials: %w", err)
	}
	return out, nil
}
