package usecase

import (
	"context"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
)

// userStatusError maps a non-active user to the error every login path returns.
func userStatusError(user model.User) error {
	if user.Status == "active" {
		return nil
	}
	if user.DeactivatedAt != nil {
		return ErrAccountDeactivated
	}
	return ErrUserDisabled
}

// checkLoginMFA enforces authenticator 2FA for a first-factor-verified user.
// applyPasswordPolicy also enforces the admin "password login requires 2FA"
// policy (password sign-in only; passwordless methods check user-level TOTP).
func (u *AuthUseCase) checkLoginMFA(ctx context.Context, userID int64, totpCode string, applyPasswordPolicy bool) error {
	hasTOTP, err := u.UserHasEnabledTOTP(ctx, userID)
	if err != nil {
		return err
	}
	if applyPasswordPolicy && !hasTOTP && u.accessPolicy != nil {
		required, err := u.accessPolicy.PasswordLoginTOTPRequired(ctx)
		if err != nil {
			return err
		}
		if required {
			return ErrMFANotEnrolled
		}
	}
	if !hasTOTP {
		return nil
	}
	code := strings.TrimSpace(totpCode)
	if code == "" {
		return ErrMFARequired
	}
	valid, _, err := u.verifyUserTOTPCode(ctx, userID, code, true)
	if err != nil {
		return err
	}
	if !valid {
		return ErrInvalidMFACode
	}
	return nil
}

// completeLogin records the login and issues tokens.
func (u *AuthUseCase) completeLogin(ctx context.Context, user model.User, meta model.SessionMeta) (model.Tokens, error) {
	if err := u.repo.UpdateLastLogin(ctx, user.ID); err != nil {
		return model.Tokens{}, err
	}
	return u.issueTokensForUser(ctx, user, meta)
}
