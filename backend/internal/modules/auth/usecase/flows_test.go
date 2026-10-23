package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/repository"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/oidc"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/password"
	"github.com/google/uuid"
	pqtotp "github.com/pquerna/otp/totp"
)

// ---- fakes -----------------------------------------------------------------

type otpRow struct {
	id        int64
	email     string
	typ       string
	hash      string
	attempts  int32
	max       int32
	expiresAt time.Time
	consumed  bool
}

// flowRepo adds working OTP, OAuth account, TOTP and refresh bookkeeping.
type flowRepo struct {
	*memRepo
	otps       []*otpRow
	accounts   []model.OAuthAccountRecord
	totp       map[int64]model.UserTOTP
	revokedAll map[int64]int
	cleared    map[int64]bool
}

func newFlowRepo() *flowRepo {
	return &flowRepo{
		memRepo: newMemRepo(), totp: map[int64]model.UserTOTP{},
		revokedAll: map[int64]int{}, cleared: map[int64]bool{},
	}
}

func (r *flowRepo) CreateOTP(_ context.Context, _ *int64, email, codeHash, otpType string, expiresAt time.Time) error {
	r.otps = append(r.otps, &otpRow{
		id: int64(len(r.otps) + 1), email: email, typ: otpType, hash: codeHash, max: 5, expiresAt: expiresAt,
	})
	return nil
}

func (r *flowRepo) InvalidateOTPs(_ context.Context, email, otpType string) error {
	for _, o := range r.otps {
		if o.email == email && o.typ == otpType {
			o.consumed = true
		}
	}
	return nil
}

func (r *flowRepo) GetActiveOTP(_ context.Context, email, otpType string) (int64, string, int32, int32, error) {
	for i := len(r.otps) - 1; i >= 0; i-- {
		o := r.otps[i]
		if o.email == email && o.typ == otpType && !o.consumed && time.Now().Before(o.expiresAt) {
			return o.id, o.hash, o.attempts, o.max, nil
		}
	}
	return 0, "", 0, 0, repository.ErrNotFound
}

func (r *flowRepo) IncrementOTPAttempts(_ context.Context, id int64) (int32, int32, error) {
	o := r.otps[id-1]
	o.attempts++
	return o.attempts, o.max, nil
}

func (r *flowRepo) ConsumeOTP(_ context.Context, id int64) error {
	o := r.otps[id-1]
	if o.consumed {
		return repository.ErrNotFound
	}
	o.consumed = true
	return nil
}

func (r *flowRepo) RevokeAllRefresh(_ context.Context, userID int64) error {
	r.revokedAll[userID]++
	return nil
}

func (r *flowRepo) SetEmailVerified(_ context.Context, userID int64) (model.User, error) {
	u := r.users[userID]
	u.EmailVerified = true
	r.users[u.ID], r.byEmail[u.Email], r.byUUID[u.UUID] = u, u, u
	return u, nil
}

func (r *flowRepo) GetUserTOTP(_ context.Context, userID int64) (model.UserTOTP, error) {
	row, ok := r.totp[userID]
	if !ok {
		return model.UserTOTP{}, repository.ErrNotFound
	}
	return row, nil
}

func (r *flowRepo) GetOAuthAccountByProviderAccount(_ context.Context, provider, subject string) (model.OAuthAccountRecord, error) {
	for _, a := range r.accounts {
		if a.Provider == provider && a.ProviderAccountID == subject {
			return a, nil
		}
	}
	return model.OAuthAccountRecord{}, repository.ErrNotFound
}

func (r *flowRepo) GetOAuthAccountByUserProvider(_ context.Context, userID int64, provider string) (model.OAuthAccountRecord, error) {
	for _, a := range r.accounts {
		if a.UserID == userID && a.Provider == provider {
			return a, nil
		}
	}
	return model.OAuthAccountRecord{}, repository.ErrNotFound
}

func (r *flowRepo) ListOAuthAccountsByUserID(_ context.Context, userID int64) ([]model.OAuthAccountRecord, error) {
	var out []model.OAuthAccountRecord
	for _, a := range r.accounts {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (r *flowRepo) CreateOAuthAccount(_ context.Context, in model.CreateOAuthAccountInput) (model.OAuthAccountRecord, error) {
	u := r.users[in.UserID]
	row := model.OAuthAccountRecord{
		ID: int64(len(r.accounts) + 1), UUID: uuid.New(), UserID: in.UserID, UserUUID: u.UUID,
		Provider: in.Provider, ProviderAccountID: in.ProviderAccountID, Type: in.Type,
		RefreshTokenEnc: in.RefreshTokenEnc, ClientID: in.ClientID,
	}
	r.accounts = append(r.accounts, row)
	return row, nil
}

func (r *flowRepo) UpdateOAuthAccountRefreshToken(_ context.Context, id int64, enc, clientID *string) error {
	r.accounts[id-1].RefreshTokenEnc = enc
	r.accounts[id-1].ClientID = clientID
	if enc == nil {
		r.cleared[id] = true
	}
	return nil
}

type captureNotifier struct{ sent []notifmodel.EnqueueInput }

func (n *captureNotifier) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	n.sent = append(n.sent, in)
	return nil, nil
}

func (n *captureNotifier) lastCode(t *testing.T, template string) string {
	t.Helper()
	for i := len(n.sent) - 1; i >= 0; i-- {
		if n.sent[i].TemplateCode == template {
			return n.sent[i].TemplateVars["code"]
		}
	}
	t.Fatalf("no %s notification sent", template)
	return ""
}

func (n *captureNotifier) count(template string) int {
	c := 0
	for _, s := range n.sent {
		if s.TemplateCode == template {
			c++
		}
	}
	return c
}

type fakeVerifier struct {
	claims oidc.Claims
	err    error
	gotAud []string
}

func (f *fakeVerifier) Verify(_ context.Context, provider, _ string, audiences []string, _ string) (oidc.Claims, error) {
	f.gotAud = audiences
	if f.err != nil {
		return oidc.Claims{}, f.err
	}
	c := f.claims
	c.Provider = provider
	return c, nil
}

type fakeApple struct {
	revoked []string
}

func (f *fakeApple) Configured(context.Context) bool { return true }
func (f *fakeApple) ExchangeCode(context.Context, string, string) (string, error) {
	return "apple-refresh", nil
}

func (f *fakeApple) Revoke(_ context.Context, _, _, token string) error {
	f.revoked = append(f.revoked, token)
	return nil
}

type fakeClients struct{ id string }

func (f fakeClients) ClientCredentials(context.Context, string) (string, string, error) {
	return f.id, "web-secret", nil
}

type flowEnv struct {
	repo   *flowRepo
	uc     *AuthUseCase
	notif  *captureNotifier
	verify *fakeVerifier
	apple  *fakeApple
}

func newFlowEnv(t *testing.T) *flowEnv {
	t.Helper()
	repo := newFlowRepo()
	tokens, err := jwt.NewManager("test-secret-key-32-bytes-minimum!", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	uc := New(repo, tokens)
	notif := &captureNotifier{}
	uc.SetNotifier(notif)
	uc.SetSecretBox(testOAuthBox(t), "Test")
	uc.SetAuthSettings(testAuthGate{passwordLogin: true, passkeyLogin: true, registration: true, passwordRegister: true})
	v := &fakeVerifier{}
	apple := &fakeApple{}
	uc.SetNativeOAuth(NativeOAuthConfig{
		Verifier: v, Apple: apple, Clients: fakeClients{id: "web.apps.googleusercontent.com"},
		AppleClientIDs: []string{"com.example.app"}, GoogleClientIDs: []string{"ios.apps.googleusercontent.com"},
	})
	return &flowEnv{repo: repo, uc: uc, notif: notif, verify: v, apple: apple}
}

func (e *flowEnv) user(t *testing.T, email string) model.User {
	t.Helper()
	hash, _ := password.Hash("Secret123!")
	u, err := e.repo.CreateUser(context.Background(), model.User{
		Email: email, PasswordHash: hash, Name: "Ada", Surname: "L", Status: "active", Locale: "en",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (e *flowEnv) enableTOTP(t *testing.T, userID int64) string {
	t.Helper()
	key, err := pqtotp.Generate(pqtotp.GenerateOpts{Issuer: "Test", AccountName: "x"})
	if err != nil {
		t.Fatal(err)
	}
	enc, _ := e.uc.box.Encrypt(key.Secret())
	e.repo.totp[userID] = model.UserTOTP{UserID: userID, SecretEnc: enc, Enabled: true}
	return key.Secret()
}

// ---- email code login --------------------------------------------------------

func TestEmailCodeLoginNoEnumeration(t *testing.T) {
	e := newFlowEnv(t)
	e.uc.SetAuthSettings(testAuthGate{passwordLogin: true, registration: false})
	if err := e.uc.RequestLoginCode(context.Background(), "nobody@example.com"); err != nil {
		t.Fatalf("unknown email must be accepted: %v", err)
	}
	if len(e.notif.sent) != 0 || len(e.repo.otps) != 0 {
		t.Fatal("no code may be issued for unknown email while registration is off")
	}
	if _, err := e.uc.VerifyLoginCode(context.Background(), "nobody@example.com", "123456", "", model.SessionMeta{}); !errors.Is(err, ErrInvalidEmailCode) {
		t.Fatalf("got %v", err)
	}
	if _, ok := e.repo.byEmail["nobody@example.com"]; ok {
		t.Fatal("no account may be created while registration is off")
	}
}

func TestEmailCodeSignUpCreatesAccount(t *testing.T) {
	e := newFlowEnv(t) // registration enabled
	ctx := context.Background()
	if err := e.uc.RequestLoginCode(ctx, " New.Person@Example.com "); err != nil {
		t.Fatalf("request: %v", err)
	}
	if len(e.repo.otps) != 1 {
		t.Fatalf("sign-up code must be issued, otps=%d", len(e.repo.otps))
	}
	sent := e.notif.sent[len(e.notif.sent)-1]
	if sent.UserID != nil || sent.Recipient == nil || *sent.Recipient != "new.person@example.com" {
		t.Fatalf("sign-up code mail must go to the address without a user id: %+v", sent)
	}
	code := e.notif.lastCode(t, "auth.login_code")

	if _, err := e.uc.VerifyLoginCode(ctx, "new.person@example.com", "000000", "", model.SessionMeta{}); !errors.Is(err, ErrInvalidEmailCode) {
		t.Fatalf("wrong code: %v", err)
	}
	if _, ok := e.repo.byEmail["new.person@example.com"]; ok {
		t.Fatal("wrong code must not create an account")
	}

	tok, err := e.uc.VerifyLoginCode(ctx, "new.person@example.com", code, "", model.SessionMeta{})
	if err != nil || tok.AccessToken == "" || tok.RefreshToken == "" {
		t.Fatalf("verify: %v %+v", err, tok)
	}
	u, ok := e.repo.byEmail["new.person@example.com"]
	if !ok {
		t.Fatal("account not created")
	}
	if !u.EmailVerified || u.PasswordSet || u.Name != "new.person" || u.Surname != "" || u.Status != "active" {
		t.Fatalf("user = %+v", u)
	}
	if e.notif.count("auth.welcome") != 1 || e.notif.count("auth.email_verification") != 0 {
		t.Fatalf("welcome=%d verification=%d", e.notif.count("auth.welcome"), e.notif.count("auth.email_verification"))
	}
	if _, err := e.uc.VerifyLoginCode(ctx, "new.person@example.com", code, "", model.SessionMeta{}); !errors.Is(err, ErrInvalidEmailCode) {
		t.Fatalf("code reuse: %v", err)
	}
}

func TestEmailCodeSignUpRegistrationTurnedOff(t *testing.T) {
	e := newFlowEnv(t)
	ctx := context.Background()
	if err := e.uc.RequestLoginCode(ctx, "late@example.com"); err != nil {
		t.Fatal(err)
	}
	code := e.notif.lastCode(t, "auth.login_code")
	e.uc.SetAuthSettings(testAuthGate{passwordLogin: true, registration: false})
	if _, err := e.uc.VerifyLoginCode(ctx, "late@example.com", code, "", model.SessionMeta{}); !errors.Is(err, ErrInvalidEmailCode) {
		t.Fatalf("got %v", err)
	}
	if _, ok := e.repo.byEmail["late@example.com"]; ok {
		t.Fatal("no account may be created after registration was turned off")
	}
}

func TestEmailCodeLoginSingleUse(t *testing.T) {
	e := newFlowEnv(t)
	e.user(t, "ada@example.com")
	ctx := context.Background()
	if err := e.uc.RequestLoginCode(ctx, " ADA@example.com "); err != nil {
		t.Fatal(err)
	}
	code := e.notif.lastCode(t, "auth.login_code")
	if len(code) != 6 {
		t.Fatalf("code %q", code)
	}
	if e.repo.otps[0].hash == code {
		t.Fatal("code must be stored hashed")
	}
	if ttl := time.Until(e.repo.otps[0].expiresAt); ttl > 10*time.Minute || ttl < 9*time.Minute {
		t.Fatalf("ttl = %v", ttl)
	}
	if e.notif.sent[0].Language != "en" || !e.notif.sent[0].SecurityEmail {
		t.Fatalf("notification = %+v", e.notif.sent[0])
	}
	tok, err := e.uc.VerifyLoginCode(ctx, "ada@example.com", code, "", model.SessionMeta{})
	if err != nil || tok.AccessToken == "" {
		t.Fatalf("verify: %v", err)
	}
	if !e.repo.byEmail["ada@example.com"].EmailVerified {
		t.Fatal("email should be marked verified")
	}
	if _, err := e.uc.VerifyLoginCode(ctx, "ada@example.com", code, "", model.SessionMeta{}); !errors.Is(err, ErrInvalidEmailCode) {
		t.Fatalf("reuse: got %v", err)
	}
}

func TestEmailCodeMaxAttempts(t *testing.T) {
	e := newFlowEnv(t)
	e.user(t, "ada@example.com")
	ctx := context.Background()
	_ = e.uc.RequestLoginCode(ctx, "ada@example.com")
	code := e.notif.lastCode(t, "auth.login_code")
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for i := 0; i < 5; i++ {
		if _, err := e.uc.VerifyLoginCode(ctx, "ada@example.com", wrong, "", model.SessionMeta{}); !errors.Is(err, ErrInvalidEmailCode) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, err := e.uc.VerifyLoginCode(ctx, "ada@example.com", code, "", model.SessionMeta{}); !errors.Is(err, ErrInvalidEmailCode) {
		t.Fatalf("code must be dead after 5 failures, got %v", err)
	}
}

func TestEmailCodeRequiresTOTPWhenEnabled(t *testing.T) {
	e := newFlowEnv(t)
	u := e.user(t, "ada@example.com")
	secret := e.enableTOTP(t, u.ID)
	ctx := context.Background()
	_ = e.uc.RequestLoginCode(ctx, "ada@example.com")
	code := e.notif.lastCode(t, "auth.login_code")

	if _, err := e.uc.VerifyLoginCode(ctx, "ada@example.com", code, "", model.SessionMeta{}); !errors.Is(err, ErrMFARequired) {
		t.Fatalf("expected MFA required, got %v", err)
	}
	if _, err := e.uc.VerifyLoginCode(ctx, "ada@example.com", code, "000000", model.SessionMeta{}); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("expected invalid mfa, got %v", err)
	}
	totpCode, _ := pqtotp.GenerateCode(secret, time.Now())
	if _, err := e.uc.VerifyLoginCode(ctx, "ada@example.com", code, totpCode, model.SessionMeta{}); err != nil {
		t.Fatalf("email code must survive the MFA round-trip: %v", err)
	}
}

func TestEmailCodeReviewAccount(t *testing.T) {
	e := newFlowEnv(t)
	e.user(t, "review@example.com")
	e.user(t, "other@example.com")
	e.uc.SetReviewAccounts(map[string]string{"Review@Example.com": "246810"})
	ctx := context.Background()
	if err := e.uc.RequestLoginCode(ctx, "review@example.com"); err != nil {
		t.Fatal(err)
	}
	if len(e.notif.sent) != 0 {
		t.Fatal("review account must not receive an email")
	}
	if _, err := e.uc.VerifyLoginCode(ctx, "review@example.com", "246810", "", model.SessionMeta{}); err != nil {
		t.Fatalf("review code: %v", err)
	}
	if _, err := e.uc.VerifyLoginCode(ctx, "other@example.com", "246810", "", model.SessionMeta{}); !errors.Is(err, ErrInvalidEmailCode) {
		t.Fatalf("review code must only work for its email, got %v", err)
	}
}

// ---- deactivation ------------------------------------------------------------

func TestDeactivateAccountBlocksLogins(t *testing.T) {
	e := newFlowEnv(t)
	u := e.user(t, "ada@example.com")
	enc, _ := e.uc.box.Encrypt("stored-apple-rt")
	cid := "com.example.app"
	e.repo.accounts = append(e.repo.accounts, model.OAuthAccountRecord{
		ID: 1, UserID: u.ID, UserUUID: u.UUID, Provider: "apple", ProviderAccountID: "apple-sub",
		RefreshTokenEnc: &enc, ClientID: &cid,
	})
	ctx := context.Background()

	if err := e.uc.DeactivateAccount(ctx, u.UUID, "", false); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("expected confirmation required, got %v", err)
	}
	if err := e.uc.DeactivateAccount(ctx, u.UUID, "999999", false); !errors.Is(err, ErrInvalidEmailCode) {
		t.Fatalf("expected invalid code, got %v", err)
	}
	if err := e.uc.RequestAccountDeactivationCode(ctx, u.UUID); err != nil {
		t.Fatal(err)
	}
	code := e.notif.lastCode(t, "auth.account_deactivation_code")
	if err := e.uc.DeactivateAccount(ctx, u.UUID, code, false); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	got := e.repo.byUUID[u.UUID]
	if got.Status != "disabled" || got.DeactivatedAt == nil {
		t.Fatalf("user not deactivated: %+v", got)
	}
	if e.repo.revokedAll[u.ID] != 1 {
		t.Fatal("refresh sessions must be revoked")
	}
	if len(e.apple.revoked) != 1 || e.apple.revoked[0] != "stored-apple-rt" || !e.repo.cleared[1] {
		t.Fatalf("apple token not revoked: %v", e.apple.revoked)
	}
	if e.notif.count("auth.account_deactivated") != 1 {
		t.Fatal("confirmation email missing")
	}

	if _, err := e.uc.Login(ctx, "ada@example.com", "Secret123!", "", model.SessionMeta{}); !errors.Is(err, ErrAccountDeactivated) {
		t.Fatalf("password login: %v", err)
	}
	if _, err := e.uc.IssueSessionForUser(ctx, u.UUID, model.SessionMeta{}, "passkey"); !errors.Is(err, ErrAccountDeactivated) {
		t.Fatalf("passkey/web oauth session: %v", err)
	}
	_ = e.uc.RequestLoginCode(ctx, "ada@example.com")
	loginCode := e.notif.lastCode(t, "auth.login_code")
	if _, err := e.uc.VerifyLoginCode(ctx, "ada@example.com", loginCode, "", model.SessionMeta{}); !errors.Is(err, ErrAccountDeactivated) {
		t.Fatalf("email code login: %v", err)
	}
	e.verify.claims = oidc.Claims{Subject: "apple-sub", Email: "ada@example.com", EmailVerified: true, Audience: "com.example.app"}
	if _, err := e.uc.NativeOAuthLogin(ctx, NativeOAuthInput{Provider: "apple", IDToken: "t"}, model.SessionMeta{}); !errors.Is(err, ErrAccountDeactivated) {
		t.Fatalf("native oauth: %v", err)
	}
	if !errors.Is(ErrAccountDeactivated, ErrUserDisabled) {
		t.Fatal("ErrAccountDeactivated must wrap ErrUserDisabled")
	}
}

func TestDeactivateWithStepUp(t *testing.T) {
	e := newFlowEnv(t)
	u := e.user(t, "ada@example.com")
	if err := e.uc.DeactivateAccount(context.Background(), u.UUID, "", true); err != nil {
		t.Fatal(err)
	}
	if err := e.uc.DeactivateAccount(context.Background(), u.UUID, "", true); !errors.Is(err, ErrAccountDeactivated) {
		t.Fatalf("second call: %v", err)
	}
}

// ---- native oauth ------------------------------------------------------------

func TestNativeOAuthLinkedLogin(t *testing.T) {
	e := newFlowEnv(t)
	u := e.user(t, "ada@example.com")
	e.repo.accounts = append(e.repo.accounts, model.OAuthAccountRecord{ID: 1, UserID: u.ID, Provider: "google", ProviderAccountID: "g-1"})
	e.verify.claims = oidc.Claims{Subject: "g-1", Email: "ada@example.com", EmailVerified: true}
	tok, err := e.uc.NativeOAuthLogin(context.Background(), NativeOAuthInput{Provider: "google", IDToken: "t"}, model.SessionMeta{})
	if err != nil || tok.AccessToken == "" {
		t.Fatalf("login: %v", err)
	}
	aud := strings.Join(e.verify.gotAud, ",")
	if !strings.Contains(aud, "ios.apps.googleusercontent.com") || !strings.Contains(aud, "web.apps.googleusercontent.com") {
		t.Fatalf("google audiences = %v", e.verify.gotAud)
	}

	secret := e.enableTOTP(t, u.ID)
	if _, err := e.uc.NativeOAuthLogin(context.Background(), NativeOAuthInput{Provider: "google", IDToken: "t"}, model.SessionMeta{}); !errors.Is(err, ErrMFARequired) {
		t.Fatalf("expected MFA required, got %v", err)
	}
	code, _ := pqtotp.GenerateCode(secret, time.Now())
	if _, err := e.uc.NativeOAuthLogin(context.Background(), NativeOAuthInput{Provider: "google", IDToken: "t", TOTPCode: code}, model.SessionMeta{}); err != nil {
		t.Fatalf("with totp: %v", err)
	}
}

func TestNativeOAuthInvalidToken(t *testing.T) {
	e := newFlowEnv(t)
	e.verify.err = oidc.ErrInvalidToken
	if _, err := e.uc.NativeOAuthLogin(context.Background(), NativeOAuthInput{Provider: "apple", IDToken: "t"}, model.SessionMeta{}); !errors.Is(err, ErrInvalidIDToken) {
		t.Fatalf("got %v", err)
	}
}

func TestNativeOAuthExistingEmailNotLinked(t *testing.T) {
	e := newFlowEnv(t)
	e.user(t, "ada@example.com")
	e.verify.claims = oidc.Claims{Subject: "g-1", Email: "ada@example.com", EmailVerified: true}
	if _, err := e.uc.NativeOAuthLogin(context.Background(), NativeOAuthInput{Provider: "google", IDToken: "t"}, model.SessionMeta{}); !errors.Is(err, ErrOAuthNotLinked) {
		t.Fatalf("got %v", err)
	}
}

func TestNativeOAuthSignUp(t *testing.T) {
	e := newFlowEnv(t)
	e.verify.claims = oidc.Claims{Subject: "g-2", Email: "new@example.com", EmailVerified: true, GivenName: "New", FamilyName: "Person"}
	tok, err := e.uc.NativeOAuthLogin(context.Background(), NativeOAuthInput{Provider: "google", IDToken: "t"}, model.SessionMeta{})
	if err != nil || tok.AccessToken == "" {
		t.Fatalf("sign-up: %v", err)
	}
	u := e.repo.byEmail["new@example.com"]
	if u.Name != "New" || !u.EmailVerified || len(e.repo.accounts) != 1 || e.repo.accounts[0].UserID != u.ID {
		t.Fatalf("user/link wrong: %+v %+v", u, e.repo.accounts)
	}

	e.uc.SetAuthSettings(testAuthGate{registration: false})
	e.verify.claims = oidc.Claims{Subject: "g-3", Email: "x@example.com", EmailVerified: true}
	if _, err := e.uc.NativeOAuthLogin(context.Background(), NativeOAuthInput{Provider: "google", IDToken: "t"}, model.SessionMeta{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("registration disabled: %v", err)
	}
}

func TestNativeAppleRelayLinkExistingAccount(t *testing.T) {
	e := newFlowEnv(t)
	u := e.user(t, "ada@example.com")
	ctx := context.Background()
	e.verify.claims = oidc.Claims{
		Subject: "apple-9", Email: "abc@privaterelay.appleid.com", EmailVerified: true,
		IsPrivateEmail: true, Audience: "com.example.app",
	}
	_, err := e.uc.NativeOAuthLogin(ctx, NativeOAuthInput{
		Provider: "apple", IDToken: "t", GivenName: "Ada", AuthorizationCode: "code",
	}, model.SessionMeta{})
	var choice *LinkChoiceRequiredError
	if !errors.As(err, &choice) || !errors.Is(err, ErrLinkChoiceRequired) {
		t.Fatalf("expected link choice, got %v", err)
	}
	if len(e.repo.users) != 1 {
		t.Fatal("no account may be created before the user chooses")
	}

	if err := e.uc.RequestOAuthLinkCode(ctx, choice.Ticket, "unknown@example.com"); err != nil || len(e.notif.sent) != 0 {
		t.Fatalf("unknown email must be silently accepted: %v", err)
	}
	if err := e.uc.RequestOAuthLinkCode(ctx, "tampered", "ada@example.com"); !errors.Is(err, ErrInvalidLinkTicket) {
		t.Fatalf("tampered ticket: %v", err)
	}
	if err := e.uc.RequestOAuthLinkCode(ctx, choice.Ticket, "ada@example.com"); err != nil {
		t.Fatal(err)
	}
	code := e.notif.lastCode(t, "auth.oauth_link_code")
	tok, err := e.uc.VerifyOAuthLink(ctx, choice.Ticket, "ada@example.com", code, "", model.SessionMeta{})
	if err != nil || tok.AccessToken == "" {
		t.Fatalf("link verify: %v", err)
	}
	if len(e.repo.accounts) != 1 || e.repo.accounts[0].UserID != u.ID || e.repo.accounts[0].RefreshTokenEnc == nil {
		t.Fatalf("apple identity not linked with token: %+v", e.repo.accounts)
	}
	// Next native sign-in is a plain login.
	if _, err := e.uc.NativeOAuthLogin(ctx, NativeOAuthInput{Provider: "apple", IDToken: "t"}, model.SessionMeta{}); err != nil {
		t.Fatalf("linked login: %v", err)
	}
}

func TestNativeAppleRelayCreateAccount(t *testing.T) {
	e := newFlowEnv(t)
	ctx := context.Background()
	e.verify.claims = oidc.Claims{Subject: "apple-7", Email: "zz@privaterelay.appleid.com", EmailVerified: true, Audience: "com.example.app"}
	_, err := e.uc.NativeOAuthLogin(ctx, NativeOAuthInput{Provider: "apple", IDToken: "t", GivenName: "Zed"}, model.SessionMeta{})
	var choice *LinkChoiceRequiredError
	if !errors.As(err, &choice) || !choice.RegisterAllowed {
		t.Fatalf("expected link choice, got %v", err)
	}
	tok, err := e.uc.CreateAccountFromLinkTicket(ctx, choice.Ticket, "", model.SessionMeta{})
	if err != nil || tok.AccessToken == "" {
		t.Fatalf("create: %v", err)
	}
	u := e.repo.byEmail["zz@privaterelay.appleid.com"]
	if u.Name != "Zed" || len(e.repo.accounts) != 1 || e.repo.accounts[0].UserID != u.ID {
		t.Fatalf("account wrong: %+v %+v", u, e.repo.accounts)
	}
	// Retrying with the same ticket signs into the same account.
	if _, err := e.uc.CreateAccountFromLinkTicket(ctx, choice.Ticket, "", model.SessionMeta{}); err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	if len(e.repo.users) != 1 {
		t.Fatal("retry must not create a second user")
	}
}

func TestLinkTicketExpires(t *testing.T) {
	e := newFlowEnv(t)
	e.verify.claims = oidc.Claims{Subject: "apple-5", Email: "q@privaterelay.appleid.com", EmailVerified: true, Audience: "com.example.app"}
	_, err := e.uc.NativeOAuthLogin(context.Background(), NativeOAuthInput{Provider: "apple", IDToken: "t"}, model.SessionMeta{})
	var choice *LinkChoiceRequiredError
	if !errors.As(err, &choice) {
		t.Fatal(err)
	}
	e.uc.now = func() time.Time { return time.Now().Add(16 * time.Minute) }
	if _, err := e.uc.CreateAccountFromLinkTicket(context.Background(), choice.Ticket, "", model.SessionMeta{}); !errors.Is(err, ErrInvalidLinkTicket) {
		t.Fatalf("expired ticket: %v", err)
	}
}

func TestNativeOAuthDisabledWithoutAudiences(t *testing.T) {
	e := newFlowEnv(t)
	e.uc.SetNativeOAuth(NativeOAuthConfig{Verifier: e.verify})
	if _, err := e.uc.NativeOAuthLogin(context.Background(), NativeOAuthInput{Provider: "apple", IDToken: "t"}, model.SessionMeta{}); !errors.Is(err, ErrOAuthProviderDisabled) {
		t.Fatalf("got %v", err)
	}
}

func TestLinkNativeIdentity(t *testing.T) {
	e := newFlowEnv(t)
	ctx := context.Background()
	u := e.user(t, "ada@example.com")
	e.verify.claims = oidc.Claims{Subject: "a-1", Audience: "com.example.app"}
	in := NativeLinkInput{Provider: "apple", IDToken: "t", AuthorizationCode: "code"}
	got, err := e.uc.LinkNativeIdentity(ctx, u.UUID, in)
	if err != nil || got.Provider != "apple" {
		t.Fatalf("link: %+v %v", got, err)
	}
	if len(e.repo.accounts) != 1 || e.repo.accounts[0].UserID != u.ID || e.repo.accounts[0].RefreshTokenEnc == nil {
		t.Fatalf("account not stored with refresh token: %+v", e.repo.accounts)
	}
	// Idempotent for the same user.
	if _, err := e.uc.LinkNativeIdentity(ctx, u.UUID, in); err != nil || len(e.repo.accounts) != 1 {
		t.Fatalf("relink: %v (%d accounts)", err, len(e.repo.accounts))
	}
	// Same identity, another user -> conflict.
	other := e.user(t, "bob@example.com")
	if _, err := e.uc.LinkNativeIdentity(ctx, other.UUID, in); !errors.Is(err, ErrConflict) {
		t.Fatalf("other user: %v", err)
	}
	// Different Apple identity for a user that already has one -> conflict.
	e.verify.claims = oidc.Claims{Subject: "a-2", Audience: "com.example.app"}
	if _, err := e.uc.LinkNativeIdentity(ctx, u.UUID, in); !errors.Is(err, ErrConflict) {
		t.Fatalf("second apple identity: %v", err)
	}
	e.verify.err = oidc.ErrInvalidToken
	if _, err := e.uc.LinkNativeIdentity(ctx, u.UUID, NativeLinkInput{Provider: "google", IDToken: "t"}); !errors.Is(err, ErrInvalidIDToken) {
		t.Fatalf("invalid token: %v", err)
	}
}

func TestNativeSignUpHasNoPassword(t *testing.T) {
	e := newFlowEnv(t)
	e.verify.claims = oidc.Claims{Subject: "g-9", Email: "np@example.com", EmailVerified: true}
	if _, err := e.uc.NativeOAuthLogin(context.Background(), NativeOAuthInput{Provider: "google", IDToken: "t"}, model.SessionMeta{}); err != nil {
		t.Fatal(err)
	}
	if e.repo.byEmail["np@example.com"].PasswordSet {
		t.Fatal("OAuth sign-up must not count as a password")
	}
	if !e.user(t, "pw@example.com").PasswordSet {
		t.Fatal("password user must have PasswordSet")
	}
}

func TestPasswordResetSetsPasswordSet(t *testing.T) {
	e := newFlowEnv(t)
	ctx := context.Background()
	e.verify.claims = oidc.Claims{Subject: "g-10", Email: "oauth@example.com", EmailVerified: true}
	if _, err := e.uc.NativeOAuthLogin(ctx, NativeOAuthInput{Provider: "google", IDToken: "t"}, model.SessionMeta{}); err != nil {
		t.Fatal(err)
	}
	if e.repo.byEmail["oauth@example.com"].PasswordSet {
		t.Fatal("OAuth sign-up must start without a password")
	}
	if err := e.uc.ForgotPassword(ctx, "oauth@example.com"); err != nil {
		t.Fatal(err)
	}
	code := e.notif.lastCode(t, "auth.password_reset")
	if err := e.uc.ResetPassword(ctx, "oauth@example.com", code, "NewSecret123!"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if !e.repo.byEmail["oauth@example.com"].PasswordSet {
		t.Fatal("password reset must set password_set")
	}
}
