package usecase

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/repository"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
)

const (
	otpTypeLoginCode           = "login_code"
	otpTypeOAuthLink           = "oauth_link"
	otpTypeAccountDeactivation = "account_deactivation"
	// emailCodeTTL is the lifetime of login / link / deactivation codes.
	emailCodeTTL = 10 * time.Minute
)

// ErrInvalidEmailCode covers wrong, expired, exhausted or already-used email codes.
var ErrInvalidEmailCode = errors.New("invalid email code")

// SetReviewAccounts installs the app-review allowlist (email -> fixed code).
// A fixed code only works for its own email; nil / empty disables the bypass.
func (u *AuthUseCase) SetReviewAccounts(accounts map[string]string) {
	out := make(map[string]string, len(accounts))
	for email, code := range accounts {
		email = strings.ToLower(strings.TrimSpace(email))
		code = strings.TrimSpace(code)
		if email != "" && code != "" {
			out[email] = code
		}
	}
	u.reviewAccounts = out
}

func (u *AuthUseCase) isReviewAccount(email string) bool {
	_, ok := u.reviewAccounts[email]
	return ok
}

func (u *AuthUseCase) reviewCodeMatches(email, code string) bool {
	fixed, ok := u.reviewAccounts[email]
	if !ok || code == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(fixed), []byte(code)) == 1
}

// RequestLoginCode emails a one-time sign-in code. It always succeeds from the
// caller's point of view so the response never reveals whether email exists.
func (u *AuthUseCase) RequestLoginCode(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	if email == "" || !strings.Contains(email, "@") {
		return fmt.Errorf("%w: a valid email is required", ErrInvalidRequest)
	}
	if u.isReviewAccount(email) {
		return nil
	}
	user, err := u.repo.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			if u.selfRegistrationOpen(ctx) {
				// Sign-up by email code: the code proves the mailbox and
				// verify creates the account. Same response as a known email.
				return u.sendEmailCode(ctx, model.User{Email: email}, email, otpTypeLoginCode, "auth.login_code", nil)
			}
			// Still "accepted" for the client (no account enumeration), but
			// leave a trace for "I never got my code" reports.
			if u.log != nil {
				u.log.InfoContext(ctx, "auth_email_code_unknown_account")
			}
			return nil
		}
		return err
	}
	// Disabled / deactivated users still get a code: once they prove the
	// mailbox, verify reports the account state instead of "invalid code".
	return u.sendEmailCode(ctx, user, user.Email, otpTypeLoginCode, "auth.login_code", nil)
}

// VerifyLoginCode exchanges an email code for tokens. When the user has
// authenticator 2FA, the first call without totpCode returns ErrMFARequired
// and leaves the code unused so the client can retry with the same code.
func (u *AuthUseCase) VerifyLoginCode(ctx context.Context, email, code, totpCode string, meta model.SessionMeta) (model.Tokens, error) {
	email = normalizeEmail(email)
	code = strings.TrimSpace(code)
	if email == "" || code == "" {
		return model.Tokens{}, fmt.Errorf("%w: email and code are required", ErrInvalidRequest)
	}
	user, err := u.repo.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return u.signUpWithEmailCode(ctx, email, code, meta)
		}
		return model.Tokens{}, err
	}
	otpID, err := u.checkEmailCode(ctx, email, otpTypeLoginCode, code)
	if err != nil {
		return model.Tokens{}, err
	}
	if err := userStatusError(user); err != nil {
		return model.Tokens{}, err
	}
	if err := u.checkLoginMFA(ctx, user.ID, totpCode, false); err != nil {
		u.countFailedSecondFactor(ctx, otpID, err)
		return model.Tokens{}, err
	}
	if err := u.consumeEmailCode(ctx, otpID); err != nil {
		return model.Tokens{}, err
	}
	if !user.EmailVerified {
		// The code proves control of the mailbox.
		if updated, err := u.repo.SetEmailVerified(ctx, user.ID); err == nil {
			user = updated
		}
	}
	return u.completeLogin(ctx, user, meta)
}

// signUpWithEmailCode creates an account for an unknown email once its code
// is verified (self-registration on) and signs it in.
// With registration off it keeps the old "invalid code" answer.
func (u *AuthUseCase) signUpWithEmailCode(ctx context.Context, email, code string, meta model.SessionMeta) (model.Tokens, error) {
	if !u.selfRegistrationOpen(ctx) {
		return model.Tokens{}, ErrInvalidEmailCode
	}
	otpID, err := u.checkEmailCode(ctx, email, otpTypeLoginCode, code)
	if err != nil {
		return model.Tokens{}, err
	}
	if err := u.consumeEmailCode(ctx, otpID); err != nil {
		return model.Tokens{}, err
	}
	user, err := u.createPasswordlessUser(ctx, email, emailLocalPart(email), "", true)
	if errors.Is(err, ErrConflict) {
		// Created concurrently (double submit): sign in the existing account.
		user, err = u.repo.FindUserByEmail(ctx, email)
		if err == nil {
			err = userStatusError(user)
		}
	}
	if err != nil {
		return model.Tokens{}, err
	}
	return u.completeLogin(ctx, user, meta)
}

func (u *AuthUseCase) selfRegistrationOpen(ctx context.Context) bool {
	if u.authSettings == nil {
		return false
	}
	enabled, err := u.authSettings.RegistrationEnabled(ctx)
	return err == nil && enabled
}

// emailLocalPart is the default display name for an email-code sign-up.
func emailLocalPart(email string) string {
	local, _, _ := strings.Cut(email, "@")
	local = strings.TrimSpace(local)
	if r := []rune(local); len(r) > 100 {
		local = string(r[:100])
	}
	return local
}

// sendEmailCode issues a fresh code of otpType for recipient and emails it.
func (u *AuthUseCase) sendEmailCode(
	ctx context.Context,
	user model.User,
	recipient, otpType, templateCode string,
	extraVars map[string]string,
) error {
	code, err := generateOTP()
	if err != nil {
		return err
	}
	_ = u.repo.InvalidateOTPs(ctx, recipient, otpType)
	// user.ID is 0 for a sign-up code (no account yet).
	var uidPtr *int64
	if user.ID != 0 {
		uid := user.ID
		uidPtr = &uid
	}
	if err := u.repo.CreateOTP(ctx, uidPtr, recipient, hashOTP(code), otpType, u.now().UTC().Add(emailCodeTTL)); err != nil {
		return err
	}
	if u.log != nil {
		u.log.Debug("otp_issued", "type", otpType, "email", recipient)
	}
	if u.notifier == nil {
		return nil
	}
	vars := map[string]string{
		"name": user.Name, "code": code, "expires_minutes": strconv.Itoa(int(emailCodeTTL.Minutes())),
	}
	for k, v := range extraVars {
		vars[k] = v
	}
	to := recipient
	_, err = u.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
		UserID: uidPtr, Channels: []string{notifmodel.ChannelEmail},
		TemplateCode: templateCode, SourceEvent: templateCode,
		SecurityEmail: true, Recipient: &to, Language: user.Locale,
		TemplateVars: vars,
	})
	return err
}

// checkEmailCode validates code against the newest active code without
// consuming it; a wrong code burns one of the code's attempts.
func (u *AuthUseCase) checkEmailCode(ctx context.Context, email, otpType, code string) (int64, error) {
	if otpType == otpTypeLoginCode || otpType == otpTypeAccountDeactivation {
		if u.reviewCodeMatches(email, code) {
			return 0, nil
		}
	}
	id, codeHash, attempts, maxAttempts, err := u.repo.GetActiveOTP(ctx, email, otpType)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return 0, ErrInvalidEmailCode
		}
		return 0, err
	}
	if attempts >= maxAttempts {
		_ = u.repo.ConsumeOTP(ctx, id)
		return 0, ErrInvalidEmailCode
	}
	if subtle.ConstantTimeCompare([]byte(hashOTP(code)), []byte(codeHash)) != 1 {
		if n, maxN, err := u.repo.IncrementOTPAttempts(ctx, id); err == nil && n >= maxN {
			_ = u.repo.ConsumeOTP(ctx, id)
		}
		return 0, ErrInvalidEmailCode
	}
	return id, nil
}

// consumeEmailCode marks a checked code used (id 0 = review bypass, nothing stored).
func (u *AuthUseCase) consumeEmailCode(ctx context.Context, id int64) error {
	if id == 0 {
		return nil
	}
	if err := u.repo.ConsumeOTP(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrInvalidEmailCode
		}
		return err
	}
	return nil
}

// countFailedSecondFactor charges a wrong authenticator code against the email
// code's attempt budget so the pair cannot be brute-forced indefinitely.
func (u *AuthUseCase) countFailedSecondFactor(ctx context.Context, otpID int64, err error) {
	if otpID == 0 || !errors.Is(err, ErrInvalidMFACode) {
		return
	}
	if n, maxN, incErr := u.repo.IncrementOTPAttempts(ctx, otpID); incErr == nil && n >= maxN {
		_ = u.repo.ConsumeOTP(ctx, otpID)
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
