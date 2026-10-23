package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/oidc"
	"github.com/google/uuid"
)

const (
	appleRelayDomain = "@privaterelay.appleid.com"
	linkTicketTTL    = 15 * time.Minute
	linkTicketV1     = 1
)

var (
	// ErrInvalidIDToken: provider id_token failed signature / claim checks.
	ErrInvalidIDToken = errors.New("invalid id token")
	// ErrOAuthProviderDisabled: native sign-in is not configured for provider.
	ErrOAuthProviderDisabled = errors.New("oauth provider is not enabled for native sign-in")
	// ErrOAuthNotLinked mirrors NextAuth OAuthAccountNotLinked: an account with
	// this email exists but the provider identity is not linked to it.
	ErrOAuthNotLinked = errors.New("oauth identity is not linked to the existing account")
	// ErrInvalidLinkTicket: link ticket is missing, tampered or expired.
	ErrInvalidLinkTicket = errors.New("invalid link ticket")
	// ErrLinkChoiceRequired is matched by *LinkChoiceRequiredError.
	ErrLinkChoiceRequired = errors.New("oauth link choice required")
)

// LinkChoiceRequiredError asks the client whether the Apple private-relay
// user already has an account ("Hesabım var mı?").
type LinkChoiceRequiredError struct {
	Ticket          string
	Provider        string
	ExpiresIn       int64
	RegisterAllowed bool
}

func (e *LinkChoiceRequiredError) Error() string { return ErrLinkChoiceRequired.Error() }

// Is lets errors.Is(err, ErrLinkChoiceRequired) match.
func (e *LinkChoiceRequiredError) Is(target error) bool { return target == ErrLinkChoiceRequired }

// IDTokenVerifier validates provider id_tokens (implemented by platform/oidc).
type IDTokenVerifier interface {
	Verify(ctx context.Context, provider, raw string, audiences []string, nonce string) (oidc.Claims, error)
}

// AppleTokenClient exchanges / revokes Sign in with Apple tokens.
type AppleTokenClient interface {
	Configured(ctx context.Context) bool
	ExchangeCode(ctx context.Context, clientID, code string) (string, error)
	Revoke(ctx context.Context, clientID, clientSecret, refreshToken string) error
}

// OAuthClientSource returns the web OAuth client credentials from settings.
type OAuthClientSource interface {
	ClientCredentials(ctx context.Context, provider string) (clientID, clientSecret string, err error)
}

// NativeOAuthConfig wires native (mobile) OAuth sign-in.
type NativeOAuthConfig struct {
	Verifier        IDTokenVerifier
	Apple           AppleTokenClient
	Clients         OAuthClientSource
	AppleClientIDs  []string
	GoogleClientIDs []string
}

// SetNativeOAuth attaches native OAuth configuration.
func (u *AuthUseCase) SetNativeOAuth(cfg NativeOAuthConfig) {
	u.native = cfg
}

// NativeOAuthInput is POST /v1/auth/oauth/{provider}/native.
type NativeOAuthInput struct {
	Provider          string
	IDToken           string
	Nonce             string
	GivenName         string
	FamilyName        string
	AuthorizationCode string
	TOTPCode          string
}

// linkTicket is the encrypted (AES-GCM, so also authenticated) payload handed
// to the client when an Apple private-relay sign-in needs a link decision.
type linkTicket struct {
	V            int    `json:"v"`
	Provider     string `json:"p"`
	Subject      string `json:"s"`
	Email        string `json:"e"`
	GivenName    string `json:"gn,omitempty"`
	FamilyName   string `json:"fn,omitempty"`
	ClientID     string `json:"c,omitempty"`
	RefreshToken string `json:"rt,omitempty"`
	ExpiresAt    int64  `json:"exp"`
}

// NativeOAuthLogin signs in with a provider id_token obtained by the mobile SDK.
func (u *AuthUseCase) NativeOAuthLogin(ctx context.Context, in NativeOAuthInput, meta model.SessionMeta) (model.Tokens, error) {
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	if provider != model.OAuthProviderApple && provider != model.OAuthProviderGoogle {
		return model.Tokens{}, ErrNotFound
	}
	if strings.TrimSpace(in.IDToken) == "" {
		return model.Tokens{}, fmt.Errorf("%w: id_token is required", ErrInvalidRequest)
	}
	if u.native.Verifier == nil || u.box == nil {
		return model.Tokens{}, ErrOAuthProviderDisabled
	}
	audiences := u.nativeAudiences(ctx, provider)
	if len(audiences) == 0 {
		return model.Tokens{}, ErrOAuthProviderDisabled
	}
	claims, err := u.native.Verifier.Verify(ctx, provider, in.IDToken, audiences, in.Nonce)
	if err != nil {
		if errors.Is(err, oidc.ErrInvalidToken) {
			// Keep the verifier's reason (audience, nonce, expiry...) for logs.
			return model.Tokens{}, fmt.Errorf("%w: %w", ErrInvalidIDToken, err)
		}
		return model.Tokens{}, err
	}
	givenName := firstNonBlank(in.GivenName, claims.GivenName)
	familyName := firstNonBlank(in.FamilyName, claims.FamilyName)

	// Already linked -> normal login.
	account, err := u.repo.GetOAuthAccountByProviderAccount(ctx, provider, claims.Subject)
	if err == nil {
		user, err := u.repo.FindUserByID(ctx, account.UserID)
		if err != nil {
			return model.Tokens{}, err
		}
		if err := userStatusError(user); err != nil {
			return model.Tokens{}, err
		}
		if err := u.checkLoginMFA(ctx, user.ID, in.TOTPCode, false); err != nil {
			return model.Tokens{}, err
		}
		if provider == model.OAuthProviderApple && strings.TrimSpace(in.AuthorizationCode) != "" {
			u.storeAppleRefreshToken(ctx, account.ID, claims.Audience, in.AuthorizationCode)
		}
		return u.completeLogin(ctx, user, meta)
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return model.Tokens{}, err
	}

	email := claims.Email
	if email == "" {
		return model.Tokens{}, fmt.Errorf("%w: the provider did not share an email address", ErrInvalidRequest)
	}
	if !claims.EmailVerified {
		return model.Tokens{}, fmt.Errorf("%w: the provider email is not verified", ErrInvalidRequest)
	}

	// Same policy as the web (NextAuth without dangerous email linking): an
	// existing account is never linked implicitly by email.
	if _, err := u.repo.FindUserByEmail(ctx, email); err == nil {
		return model.Tokens{}, ErrOAuthNotLinked
	} else if !errors.Is(err, repository.ErrNotFound) {
		return model.Tokens{}, err
	}

	refreshToken := ""
	if provider == model.OAuthProviderApple && strings.TrimSpace(in.AuthorizationCode) != "" {
		refreshToken = u.exchangeAppleCode(ctx, claims.Audience, in.AuthorizationCode)
	}

	if provider == model.OAuthProviderApple && strings.HasSuffix(email, appleRelayDomain) {
		ticket, err := u.sealLinkTicket(linkTicket{
			V: linkTicketV1, Provider: provider, Subject: claims.Subject, Email: email,
			GivenName: givenName, FamilyName: familyName,
			ClientID: claims.Audience, RefreshToken: refreshToken,
			ExpiresAt: u.now().Add(linkTicketTTL).Unix(),
		})
		if err != nil {
			return model.Tokens{}, err
		}
		registerAllowed := false
		if u.authSettings != nil {
			registerAllowed, _ = u.authSettings.RegistrationEnabled(ctx)
		}
		return model.Tokens{}, &LinkChoiceRequiredError{
			Ticket: ticket, Provider: provider,
			ExpiresIn: int64(linkTicketTTL.Seconds()), RegisterAllowed: registerAllowed,
		}
	}

	return u.createAndLinkOAuthUser(ctx, linkTicket{
		Provider: provider, Subject: claims.Subject, Email: email,
		GivenName: givenName, FamilyName: familyName,
		ClientID: claims.Audience, RefreshToken: refreshToken,
	}, meta)
}

// RequestOAuthLinkCode emails a link code to an existing account ("yes, I
// have an account"). Always succeeds for a valid ticket (no enumeration).
func (u *AuthUseCase) RequestOAuthLinkCode(ctx context.Context, rawTicket, email string) error {
	ticket, err := u.openLinkTicket(rawTicket)
	if err != nil {
		return err
	}
	email = normalizeEmail(email)
	if email == "" || !strings.Contains(email, "@") {
		return fmt.Errorf("%w: a valid email is required", ErrInvalidRequest)
	}
	user, err := u.repo.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	return u.sendEmailCode(ctx, user, user.Email, otpTypeOAuthLink, "auth.oauth_link_code", map[string]string{
		"provider": providerDisplayName(ticket.Provider),
	})
}

// VerifyOAuthLink links the ticket's identity to the account that proved
// control of email with a link code, then signs in.
func (u *AuthUseCase) VerifyOAuthLink(ctx context.Context, rawTicket, email, code, totpCode string, meta model.SessionMeta) (model.Tokens, error) {
	ticket, err := u.openLinkTicket(rawTicket)
	if err != nil {
		return model.Tokens{}, err
	}
	email = normalizeEmail(email)
	code = strings.TrimSpace(code)
	if email == "" || code == "" {
		return model.Tokens{}, fmt.Errorf("%w: email and code are required", ErrInvalidRequest)
	}
	user, err := u.repo.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.Tokens{}, ErrInvalidEmailCode
		}
		return model.Tokens{}, err
	}
	otpID, err := u.checkEmailCode(ctx, email, otpTypeOAuthLink, code)
	if err != nil {
		return model.Tokens{}, err
	}
	if err := userStatusError(user); err != nil {
		return model.Tokens{}, err
	}
	if err := u.ensureLinkable(ctx, user.ID, ticket); err != nil {
		return model.Tokens{}, err
	}
	if err := u.checkLoginMFA(ctx, user.ID, totpCode, false); err != nil {
		u.countFailedSecondFactor(ctx, otpID, err)
		return model.Tokens{}, err
	}
	if err := u.consumeEmailCode(ctx, otpID); err != nil {
		return model.Tokens{}, err
	}
	if err := u.linkIdentity(ctx, user.ID, ticket); err != nil {
		return model.Tokens{}, err
	}
	return u.completeLogin(ctx, user, meta)
}

// CreateAccountFromLinkTicket creates a new account for the ticket identity
// ("no, I don't have an account"), exactly like an OAuth sign-up.
func (u *AuthUseCase) CreateAccountFromLinkTicket(ctx context.Context, rawTicket, totpCode string, meta model.SessionMeta) (model.Tokens, error) {
	ticket, err := u.openLinkTicket(rawTicket)
	if err != nil {
		return model.Tokens{}, err
	}
	// Idempotent retry: the identity may already be linked by a previous call.
	if account, err := u.repo.GetOAuthAccountByProviderAccount(ctx, ticket.Provider, ticket.Subject); err == nil {
		user, err := u.repo.FindUserByID(ctx, account.UserID)
		if err != nil {
			return model.Tokens{}, err
		}
		if err := userStatusError(user); err != nil {
			return model.Tokens{}, err
		}
		if err := u.checkLoginMFA(ctx, user.ID, totpCode, false); err != nil {
			return model.Tokens{}, err
		}
		return u.completeLogin(ctx, user, meta)
	} else if !errors.Is(err, repository.ErrNotFound) {
		return model.Tokens{}, err
	}
	return u.createAndLinkOAuthUser(ctx, ticket, meta)
}

func (u *AuthUseCase) createAndLinkOAuthUser(ctx context.Context, t linkTicket, meta model.SessionMeta) (model.Tokens, error) {
	// Reuses OAuth self-registration: registration policy, default role,
	// welcome notification — the same path NextAuth's createUser takes.
	created, err := u.RegisterOAuthUser(ctx, t.Email, t.GivenName, t.FamilyName, true)
	if err != nil {
		return model.Tokens{}, err
	}
	user, err := u.repo.FindUserByEmail(ctx, strings.ToLower(created.Email))
	if err != nil {
		return model.Tokens{}, err
	}
	if err := u.linkIdentity(ctx, user.ID, t); err != nil {
		return model.Tokens{}, err
	}
	return u.completeLogin(ctx, user, meta)
}

// ensureLinkable rejects linking when the identity belongs to someone else or
// the user already has a different identity for this provider.
func (u *AuthUseCase) ensureLinkable(ctx context.Context, userID int64, t linkTicket) error {
	existing, err := u.repo.GetOAuthAccountByProviderAccount(ctx, t.Provider, t.Subject)
	if err == nil {
		if existing.UserID != userID {
			return fmt.Errorf("%w: this sign-in is already linked to another account", ErrConflict)
		}
		return nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	owned, err := u.repo.GetOAuthAccountByUserProvider(ctx, userID, t.Provider)
	if err == nil && owned.ProviderAccountID != t.Subject {
		return fmt.Errorf("%w: the account already has a different %s sign-in linked", ErrConflict, providerDisplayName(t.Provider))
	}
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	return nil
}

func (u *AuthUseCase) linkIdentity(ctx context.Context, userID int64, t linkTicket) error {
	if err := u.ensureLinkable(ctx, userID, t); err != nil {
		return err
	}
	if _, err := u.repo.GetOAuthAccountByProviderAccount(ctx, t.Provider, t.Subject); err == nil {
		return nil
	}
	in := model.CreateOAuthAccountInput{
		UserID: userID, Provider: t.Provider, ProviderAccountID: t.Subject, Type: "oidc",
	}
	if t.ClientID != "" {
		cid := t.ClientID
		in.ClientID = &cid
	}
	if t.RefreshToken != "" && u.box != nil {
		if enc, err := u.box.Encrypt(t.RefreshToken); err == nil {
			in.RefreshTokenEnc = &enc
		}
	}
	_, err := u.repo.CreateOAuthAccount(ctx, in)
	return err
}

func (u *AuthUseCase) nativeAudiences(ctx context.Context, provider string) []string {
	var out []string
	switch provider {
	case model.OAuthProviderApple:
		out = append(out, u.native.AppleClientIDs...)
	case model.OAuthProviderGoogle:
		out = append(out, u.native.GoogleClientIDs...)
		// Android Credential Manager / iOS serverClientID tokens carry the
		// web client id as aud.
		if u.native.Clients != nil {
			if id, _, err := u.native.Clients.ClientCredentials(ctx, provider); err == nil && strings.TrimSpace(id) != "" {
				out = append(out, strings.TrimSpace(id))
			}
		}
	}
	return uniqueStrings(out)
}

// exchangeAppleCode best-effort trades an authorization code for a refresh
// token (kept only to revoke it on account deletion).
func (u *AuthUseCase) exchangeAppleCode(ctx context.Context, clientID, code string) string {
	if u.native.Apple == nil || !u.native.Apple.Configured(ctx) || clientID == "" {
		return ""
	}
	rt, err := u.native.Apple.ExchangeCode(ctx, clientID, code)
	if err != nil {
		if u.log != nil {
			u.log.Warn("apple_code_exchange_failed", "error", err)
		}
		return ""
	}
	return rt
}

func (u *AuthUseCase) storeAppleRefreshToken(ctx context.Context, accountID int64, clientID, code string) {
	rt := u.exchangeAppleCode(ctx, clientID, code)
	if rt == "" || u.box == nil {
		return
	}
	enc, err := u.box.Encrypt(rt)
	if err != nil {
		return
	}
	cid := clientID
	if err := u.repo.UpdateOAuthAccountRefreshToken(ctx, accountID, &enc, &cid); err != nil && u.log != nil {
		u.log.Warn("apple_refresh_token_store_failed", "error", err)
	}
}

func (u *AuthUseCase) sealLinkTicket(t linkTicket) (string, error) {
	if u.box == nil {
		return "", ErrOAuthProviderDisabled
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	return u.box.Encrypt(string(raw))
}

func (u *AuthUseCase) openLinkTicket(raw string) (linkTicket, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || u.box == nil {
		return linkTicket{}, ErrInvalidLinkTicket
	}
	plain, err := u.box.Decrypt(raw)
	if err != nil || plain == "" {
		return linkTicket{}, fmt.Errorf("%w: undecryptable", ErrInvalidLinkTicket)
	}
	var t linkTicket
	if err := json.Unmarshal([]byte(plain), &t); err != nil {
		return linkTicket{}, fmt.Errorf("%w: malformed", ErrInvalidLinkTicket)
	}
	if t.V != linkTicketV1 || t.Subject == "" || t.Email == "" || !model.IsKnownOAuthProvider(t.Provider) {
		return linkTicket{}, fmt.Errorf("%w: incomplete", ErrInvalidLinkTicket)
	}
	if u.now().Unix() > t.ExpiresAt {
		return linkTicket{}, fmt.Errorf("%w: expired", ErrInvalidLinkTicket)
	}
	return t, nil
}

func providerDisplayName(provider string) string {
	switch provider {
	case model.OAuthProviderApple:
		return "Apple"
	case model.OAuthProviderGoogle:
		return "Google"
	default:
		return provider
	}
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// NativeLinkInput is POST /v1/auth/identities/{provider}/native.
type NativeLinkInput struct {
	Provider          string
	IDToken           string
	Nonce             string
	AuthorizationCode string
}

// LinkNativeIdentity links a provider id_token obtained by the mobile SDK to
// the signed-in user. Idempotent when the identity is already linked to them.
func (u *AuthUseCase) LinkNativeIdentity(ctx context.Context, userUUID uuid.UUID, in NativeLinkInput) (model.LinkedIdentity, error) {
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	if provider != model.OAuthProviderApple && provider != model.OAuthProviderGoogle {
		return model.LinkedIdentity{}, ErrNotFound
	}
	if strings.TrimSpace(in.IDToken) == "" {
		return model.LinkedIdentity{}, fmt.Errorf("%w: id_token is required", ErrInvalidRequest)
	}
	if u.native.Verifier == nil || u.box == nil {
		return model.LinkedIdentity{}, ErrOAuthProviderDisabled
	}
	audiences := u.nativeAudiences(ctx, provider)
	if len(audiences) == 0 {
		return model.LinkedIdentity{}, ErrOAuthProviderDisabled
	}
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.LinkedIdentity{}, ErrNotFound
		}
		return model.LinkedIdentity{}, err
	}
	if err := userStatusError(user); err != nil {
		return model.LinkedIdentity{}, err
	}
	claims, err := u.native.Verifier.Verify(ctx, provider, in.IDToken, audiences, in.Nonce)
	if err != nil {
		if errors.Is(err, oidc.ErrInvalidToken) {
			return model.LinkedIdentity{}, fmt.Errorf("%w: %w", ErrInvalidIDToken, err)
		}
		return model.LinkedIdentity{}, err
	}
	t := linkTicket{Provider: provider, Subject: claims.Subject, ClientID: claims.Audience}
	if err := u.ensureLinkable(ctx, user.ID, t); err != nil {
		return model.LinkedIdentity{}, err
	}
	hasCode := provider == model.OAuthProviderApple && strings.TrimSpace(in.AuthorizationCode) != ""

	// Already linked to this user: refresh the stored Apple token only.
	if existing, err := u.repo.GetOAuthAccountByProviderAccount(ctx, provider, claims.Subject); err == nil {
		if hasCode {
			u.storeAppleRefreshToken(ctx, existing.ID, claims.Audience, in.AuthorizationCode)
		}
		return model.LinkedIdentity{Provider: provider, LinkedAt: existing.CreatedAt}, nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return model.LinkedIdentity{}, err
	}

	if hasCode {
		t.RefreshToken = u.exchangeAppleCode(ctx, claims.Audience, in.AuthorizationCode)
	}
	if err := u.linkIdentity(ctx, user.ID, t); err != nil {
		return model.LinkedIdentity{}, err
	}
	account, err := u.repo.GetOAuthAccountByProviderAccount(ctx, provider, claims.Subject)
	if err != nil {
		return model.LinkedIdentity{}, err
	}
	return model.LinkedIdentity{Provider: provider, LinkedAt: account.CreatedAt}, nil
}
