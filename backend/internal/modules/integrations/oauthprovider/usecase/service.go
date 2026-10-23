package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/appleauth"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalidRequest  = errors.New("invalid request")
	ErrUnknownProvider = errors.New("unknown oauth provider")
)

var knownProviders = map[string]struct{}{
	"google":   {},
	"facebook": {},
	"apple":    {},
}

// IsKnownProvider reports whether provider is google/facebook/apple.
func IsKnownProvider(provider string) bool {
	_, ok := knownProviders[strings.ToLower(strings.TrimSpace(provider))]
	return ok
}

type settingsStore interface {
	GetOAuthProviderSettings(ctx context.Context, provider string) (db.OauthProviderSetting, error)
	ListOAuthProviderSettings(ctx context.Context) ([]db.OauthProviderSetting, error)
	UpdateOAuthProviderSettings(ctx context.Context, arg db.UpdateOAuthProviderSettingsParams) (db.OauthProviderSetting, error)
}

const providerApple = "apple"

// AppleWebClientSecretTTL is the lifetime of the Apple client_secret JWT
// generated for the web (NextAuth) client. Far above the frontend providers
// cache (5 min, which also refreshes before client_secret_expires_at) and far
// below Apple's 6-month cap.
const AppleWebClientSecretTTL = 24 * time.Hour

// appleIDPattern matches Apple Team IDs and Key IDs (10 uppercase alphanumerics).
var appleIDPattern = regexp.MustCompile(`^[A-Z0-9]{10}$`)

// AppleKeys resolves the active Sign in with Apple signing key (DB first,
// env fallback); implemented by appleauth.Resolver.
type AppleKeys interface {
	Configured(ctx context.Context) bool
	EnvConfigured() bool
	Source(ctx context.Context) string
	GenerateClientSecretTTL(ctx context.Context, clientID string, ttl time.Duration) (string, time.Time, error)
	Invalidate()
}

// Service manages oauth_provider_settings rows.
type Service struct {
	q     settingsStore
	box   *crypto.SecretBox
	apple AppleKeys
}

// New creates an OAuth provider settings service.
func New(q settingsStore, box *crypto.SecretBox) *Service {
	return &Service{q: q, box: box}
}

// SetAppleKeys attaches the Apple signing key resolver (web client secret
// generation, cache invalidation on settings updates).
func (s *Service) SetAppleKeys(k AppleKeys) { s.apple = k }

// Settings is the public admin payload (no secret values).
type Settings struct {
	Provider               string `json:"provider"`
	LoginEnabled           bool   `json:"login_enabled"`
	RegisterEnabled        bool   `json:"register_enabled"`
	ClientID               string `json:"client_id"`
	ClientSecretConfigured bool   `json:"client_secret_configured"`
	// Apple only: signing key (.p8) metadata. The key itself is never returned.
	TeamID               *string `json:"team_id,omitempty"`
	KeyID                *string `json:"key_id,omitempty"`
	PrivateKeyConfigured *bool   `json:"private_key_configured,omitempty"`
	// PrivateKeySource is the key in use at runtime: db, env or none.
	PrivateKeySource *string `json:"private_key_source,omitempty"`
}

// AdapterOAuthConfig is returned to NextAuth via the internal adapter API.
type AdapterOAuthConfig struct {
	Enabled         bool   `json:"enabled"`
	RegisterAllowed bool   `json:"register_allowed"`
	ClientID        string `json:"client_id"`
	ClientSecret    string `json:"client_secret"`
	// ClientSecretExpiresAt is set when the secret was generated (Apple key);
	// callers must not use the secret after it.
	ClientSecretExpiresAt *time.Time `json:"client_secret_expires_at,omitempty"`
}

// PatchInput partial update for provider settings.
type PatchInput struct {
	LoginEnabled    *bool   `json:"login_enabled"`
	RegisterEnabled *bool   `json:"register_enabled"`
	ClientID        *string `json:"client_id"`
	ClientSecret    *string `json:"client_secret"`
	// Apple only.
	TeamID           *string `json:"team_id"`
	KeyID            *string `json:"key_id"`
	PrivateKey       *string `json:"private_key"` // .p8 PEM text
	RemovePrivateKey *bool   `json:"remove_private_key"`
}

func (in PatchInput) touchesAppleKey() bool {
	return in.TeamID != nil || in.KeyID != nil || in.PrivateKey != nil || in.RemovePrivateKey != nil
}

func normalizeProvider(provider string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(provider))
	if !IsKnownProvider(p) {
		return "", ErrUnknownProvider
	}
	return p, nil
}

func (s *Service) mapSettings(ctx context.Context, row db.OauthProviderSetting) Settings {
	out := Settings{
		Provider:               row.Provider,
		LoginEnabled:           row.LoginEnabled,
		RegisterEnabled:        row.RegisterEnabled,
		ClientID:               row.ClientID,
		ClientSecretConfigured: hasStoredSecret(row),
	}
	if row.Provider == providerApple {
		teamID, keyID := row.AppleTeamID, row.AppleKeyID
		configured := hasStoredAppleKey(row)
		source := appleauth.SourceNone
		if s.apple != nil {
			source = s.apple.Source(ctx)
		}
		out.TeamID, out.KeyID, out.PrivateKeyConfigured, out.PrivateKeySource = &teamID, &keyID, &configured, &source
	}
	return out
}

func hasStoredSecret(row db.OauthProviderSetting) bool {
	return row.ClientSecretEnc.Valid && row.ClientSecretEnc.String != ""
}

func hasStoredAppleKey(row db.OauthProviderSetting) bool {
	return row.ApplePrivateKeyEnc.Valid && row.ApplePrivateKeyEnc.String != ""
}

// appleKeyActive reports whether an Apple signing key (DB or env) can sign secrets.
func (s *Service) appleKeyActive(ctx context.Context) bool {
	return s.apple != nil && s.apple.Configured(ctx)
}

// hasCredentials: client_id plus a pasted secret, or (Apple) a signing key
// from which the secret is generated.
func (s *Service) hasCredentials(ctx context.Context, row db.OauthProviderSetting) bool {
	if strings.TrimSpace(row.ClientID) == "" {
		return false
	}
	if hasStoredSecret(row) {
		return true
	}
	return row.Provider == providerApple && s.appleKeyActive(ctx)
}

// AppleSigningKey returns the admin-uploaded Apple key (implements
// appleauth.KeySource). ok=false when none is stored.
func (s *Service) AppleSigningKey(ctx context.Context) (appleauth.Config, bool, error) {
	row, err := s.q.GetOAuthProviderSettings(ctx, providerApple)
	if err != nil {
		return appleauth.Config{}, false, err
	}
	if !hasStoredAppleKey(row) {
		return appleauth.Config{}, false, nil
	}
	pemText, err := s.box.Decrypt(row.ApplePrivateKeyEnc.String)
	if err != nil {
		return appleauth.Config{}, false, fmt.Errorf("decrypt apple private key: %w", err)
	}
	return appleauth.Config{TeamID: row.AppleTeamID, KeyID: row.AppleKeyID, PrivateKey: pemText}, true, nil
}

// Get returns masked settings for one provider.
func (s *Service) Get(ctx context.Context, provider string) (Settings, error) {
	p, err := normalizeProvider(provider)
	if err != nil {
		return Settings{}, err
	}
	row, err := s.q.GetOAuthProviderSettings(ctx, p)
	if err != nil {
		return Settings{}, err
	}
	return s.mapSettings(ctx, row), nil
}

// IsLoginEnabled reports whether provider sign-in is active (credentials + flag).
func (s *Service) IsLoginEnabled(ctx context.Context, provider string) (bool, error) {
	p, err := normalizeProvider(provider)
	if err != nil {
		return false, err
	}
	row, err := s.q.GetOAuthProviderSettings(ctx, p)
	if err != nil {
		return false, err
	}
	return row.LoginEnabled && s.hasCredentials(ctx, row), nil
}

// IsRegisterEnabled reports whether provider self-register is active (credentials + flag).
func (s *Service) IsRegisterEnabled(ctx context.Context, provider string) (bool, error) {
	p, err := normalizeProvider(provider)
	if err != nil {
		return false, err
	}
	row, err := s.q.GetOAuthProviderSettings(ctx, p)
	if err != nil {
		return false, err
	}
	return row.RegisterEnabled && s.hasCredentials(ctx, row), nil
}

// GetAdapterOAuthConfig returns decrypted OAuth credentials when login or register is enabled.
func (s *Service) GetAdapterOAuthConfig(ctx context.Context, provider string) (AdapterOAuthConfig, error) {
	p, err := normalizeProvider(provider)
	if err != nil {
		return AdapterOAuthConfig{}, err
	}
	row, err := s.q.GetOAuthProviderSettings(ctx, p)
	if err != nil {
		return AdapterOAuthConfig{}, err
	}
	if (!row.LoginEnabled && !row.RegisterEnabled) || !s.hasCredentials(ctx, row) {
		return AdapterOAuthConfig{Enabled: false}, nil
	}
	out := AdapterOAuthConfig{
		Enabled:         true,
		RegisterAllowed: row.RegisterEnabled,
		ClientID:        strings.TrimSpace(row.ClientID),
	}
	// Apple with a signing key: generate a fresh client_secret JWT (sub = the
	// Services ID) instead of the hand-made one that silently expires.
	if p == providerApple && s.appleKeyActive(ctx) {
		secret, exp, err := s.apple.GenerateClientSecretTTL(ctx, out.ClientID, AppleWebClientSecretTTL)
		if err == nil {
			out.ClientSecret = secret
			out.ClientSecretExpiresAt = &exp
			return out, nil
		}
		if !hasStoredSecret(row) {
			return AdapterOAuthConfig{}, fmt.Errorf("generate apple client secret: %w", err)
		}
	}
	if !hasStoredSecret(row) {
		return AdapterOAuthConfig{Enabled: false}, nil
	}
	secret, err := s.box.Decrypt(row.ClientSecretEnc.String)
	if err != nil {
		return AdapterOAuthConfig{}, fmt.Errorf("decrypt client secret: %w", err)
	}
	out.ClientSecret = secret
	return out, nil
}

// ClientCredentials returns the configured web client id and decrypted secret
// regardless of the login/register flags. Native sign-in uses the id as an
// accepted id_token audience and the secret to revoke web-issued Apple tokens.
func (s *Service) ClientCredentials(ctx context.Context, provider string) (string, string, error) {
	p, err := normalizeProvider(provider)
	if err != nil {
		return "", "", err
	}
	row, err := s.q.GetOAuthProviderSettings(ctx, p)
	if err != nil {
		return "", "", err
	}
	secret := ""
	if row.ClientSecretEnc.Valid && row.ClientSecretEnc.String != "" {
		secret, err = s.box.Decrypt(row.ClientSecretEnc.String)
		if err != nil {
			return "", "", fmt.Errorf("decrypt client secret: %w", err)
		}
	}
	return strings.TrimSpace(row.ClientID), secret, nil
}

// Patch updates provider settings.
func (s *Service) Patch(ctx context.Context, provider string, in PatchInput) (Settings, error) {
	p, err := normalizeProvider(provider)
	if err != nil {
		return Settings{}, err
	}
	current, err := s.q.GetOAuthProviderSettings(ctx, p)
	if err != nil {
		return Settings{}, err
	}

	params := db.UpdateOAuthProviderSettingsParams{Provider: p}
	if in.LoginEnabled != nil {
		params.LoginEnabled = pgtype.Bool{Bool: *in.LoginEnabled, Valid: true}
	}
	if in.RegisterEnabled != nil {
		params.RegisterEnabled = pgtype.Bool{Bool: *in.RegisterEnabled, Valid: true}
	}
	if in.ClientID != nil {
		params.ClientID = pgtype.Text{String: strings.TrimSpace(*in.ClientID), Valid: true}
	}
	if in.ClientSecret != nil {
		trimmed := strings.TrimSpace(*in.ClientSecret)
		if trimmed != "" {
			enc, err := s.box.Encrypt(trimmed)
			if err != nil {
				return Settings{}, err
			}
			params.ClientSecretEnc = pgtype.Text{String: enc, Valid: true}
		}
	}

	keyAvailable := false
	if p == providerApple {
		keyAvailable, err = s.applyAppleKeyPatch(current, in, &params)
		if err != nil {
			return Settings{}, err
		}
	} else if in.touchesAppleKey() {
		return Settings{}, fmt.Errorf("%w: team_id, key_id and private_key are only supported for apple", ErrInvalidRequest)
	}

	nextLogin := current.LoginEnabled
	if in.LoginEnabled != nil {
		nextLogin = *in.LoginEnabled
	}
	nextRegister := current.RegisterEnabled
	if in.RegisterEnabled != nil {
		nextRegister = *in.RegisterEnabled
	}
	nextClientID := current.ClientID
	if in.ClientID != nil {
		nextClientID = strings.TrimSpace(*in.ClientID)
	}
	hasSecret := hasStoredSecret(current)
	if in.ClientSecret != nil && strings.TrimSpace(*in.ClientSecret) != "" {
		hasSecret = true
	}
	if (nextLogin || nextRegister) && (nextClientID == "" || (!hasSecret && !keyAvailable)) {
		if p == providerApple {
			return Settings{}, fmt.Errorf("%w: client_id and either a signing key (.p8) or client_secret are required to enable Apple sign-in", ErrInvalidRequest)
		}
		return Settings{}, fmt.Errorf("%w: client_id and client_secret are required to enable OAuth", ErrInvalidRequest)
	}

	row, err := s.q.UpdateOAuthProviderSettings(ctx, params)
	if err != nil {
		return Settings{}, err
	}
	if p == providerApple && in.touchesAppleKey() && s.apple != nil {
		s.apple.Invalidate()
	}
	return s.mapSettings(ctx, row), nil
}

// applyAppleKeyPatch validates the Apple signing key fields, fills params and
// reports whether a key (stored after this update, or env) can sign secrets.
func (s *Service) applyAppleKeyPatch(current db.OauthProviderSetting, in PatchInput, params *db.UpdateOAuthProviderSettingsParams) (bool, error) {
	nextTeam := current.AppleTeamID
	if in.TeamID != nil {
		nextTeam = strings.ToUpper(strings.TrimSpace(*in.TeamID))
		params.AppleTeamID = pgtype.Text{String: nextTeam, Valid: true}
	}
	nextKeyID := current.AppleKeyID
	if in.KeyID != nil {
		nextKeyID = strings.ToUpper(strings.TrimSpace(*in.KeyID))
		params.AppleKeyID = pgtype.Text{String: nextKeyID, Valid: true}
	}
	hasKey := hasStoredAppleKey(current)
	newKey := ""
	if in.PrivateKey != nil {
		newKey = strings.TrimSpace(*in.PrivateKey)
	}
	remove := in.RemovePrivateKey != nil && *in.RemovePrivateKey
	switch {
	case remove && newKey != "":
		return false, fmt.Errorf("%w: private_key and remove_private_key cannot be combined", ErrInvalidRequest)
	case remove:
		params.ClearApplePrivateKey = true
		hasKey = false
		if in.KeyID == nil {
			nextKeyID = ""
			params.AppleKeyID = pgtype.Text{String: "", Valid: true}
		}
	case newKey != "":
		if _, err := appleauth.ParsePrivateKey(newKey); err != nil {
			return false, fmt.Errorf("%w: private_key must be an Apple .p8 key (PEM, PKCS#8, EC P-256)", ErrInvalidRequest)
		}
		enc, err := s.box.Encrypt(newKey)
		if err != nil {
			return false, err
		}
		params.ApplePrivateKeyEnc = pgtype.Text{String: enc, Valid: true}
		hasKey = true
	}
	if nextTeam != "" && !appleIDPattern.MatchString(nextTeam) {
		return false, fmt.Errorf("%w: team_id must be 10 letters or digits", ErrInvalidRequest)
	}
	if nextKeyID != "" && !appleIDPattern.MatchString(nextKeyID) {
		return false, fmt.Errorf("%w: key_id must be 10 letters or digits", ErrInvalidRequest)
	}
	if hasKey && (nextTeam == "" || nextKeyID == "") {
		return false, fmt.Errorf("%w: team_id and key_id are required with a private key", ErrInvalidRequest)
	}
	return hasKey || (s.apple != nil && s.apple.EnvConfigured()), nil
}
