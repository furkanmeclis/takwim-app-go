package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/repository"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// ErrConfirmationRequired: deactivation needs a step-up grant or an email code.
var ErrConfirmationRequired = errors.New("confirmation required")

// RequestAccountDeactivationCode emails a confirmation code to the caller.
// Passwordless users (Apple / Google / email-code only) confirm with it.
func (u *AuthUseCase) RequestAccountDeactivationCode(ctx context.Context, userUUID uuid.UUID) error {
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	if u.isReviewAccount(normalizeEmail(user.Email)) {
		return nil
	}
	return u.sendEmailCode(ctx, user, normalizeEmail(user.Email), otpTypeAccountDeactivation, "auth.account_deactivation_code", nil)
}

// DeactivateAccount is self-service account deletion. No data is deleted: the
// user is marked disabled + deactivated_at, every refresh session is revoked
// and stored Apple tokens are revoked (best effort).
//
// Confirmation: stepUpVerified (recent /v1/auth/step-up/* grant) or a valid
// account-deactivation email code.
func (u *AuthUseCase) DeactivateAccount(ctx context.Context, userUUID uuid.UUID, code string, stepUpVerified bool) error {
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	if user.DeactivatedAt != nil {
		return ErrAccountDeactivated
	}
	isSuperAdmin, err := u.repo.UserHasRoleSlug(ctx, user.ID, rbac.RoleSuperAdmin)
	if err != nil {
		return err
	}
	if isSuperAdmin {
		n, err := u.repo.CountUsersWithRole(ctx, rbac.RoleSuperAdmin)
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrLastSuperAdmin
		}
	}

	var otpID int64
	code = strings.TrimSpace(code)
	switch {
	case code != "":
		otpID, err = u.checkEmailCode(ctx, normalizeEmail(user.Email), otpTypeAccountDeactivation, code)
		if err != nil {
			return err
		}
	case !stepUpVerified:
		return ErrConfirmationRequired
	}

	if _, err := u.repo.DeactivateUser(ctx, user.ID); err != nil {
		return err
	}
	if otpID != 0 {
		_ = u.repo.ConsumeOTP(ctx, otpID)
	}
	if err := u.repo.RevokeAllRefresh(ctx, user.ID); err != nil {
		return err
	}
	u.revokeAppleTokens(ctx, user.ID)
	u.indexUserSearch(ctx, user.UUID.String())
	if u.notifier != nil {
		uid := user.ID
		email := user.Email
		_, _ = u.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
			UserID: &uid, Channels: []string{notifmodel.ChannelEmail},
			TemplateCode: "auth.account_deactivated", SourceEvent: "auth.account_deactivated",
			SecurityEmail: true, Recipient: &email, Language: user.Locale,
			TemplateVars: map[string]string{"name": user.Name},
		})
	}
	return nil
}

// revokeAppleTokens revokes every stored Apple refresh token of the user.
// Failures are logged and skipped: deactivation must not depend on Apple.
func (u *AuthUseCase) revokeAppleTokens(ctx context.Context, userID int64) {
	if u.native.Apple == nil || u.box == nil {
		return
	}
	accounts, err := u.repo.ListOAuthAccountsByUserID(ctx, userID)
	if err != nil {
		return
	}
	for _, acc := range accounts {
		if acc.Provider != model.OAuthProviderApple || acc.RefreshTokenEnc == nil || *acc.RefreshTokenEnc == "" {
			continue
		}
		token, err := u.box.Decrypt(*acc.RefreshTokenEnc)
		if err != nil || token == "" {
			continue
		}
		clientID, clientSecret := "", ""
		if acc.ClientID != nil && *acc.ClientID != "" {
			clientID = *acc.ClientID // native: secret is generated from the .p8 key
		} else if u.native.Clients != nil {
			// Web (NextAuth) token: use the web client; prefer a generated
			// secret, fall back to the configured web client secret JWT.
			id, secret, err := u.native.Clients.ClientCredentials(ctx, model.OAuthProviderApple)
			if err == nil {
				clientID = id
				if !u.native.Apple.Configured(ctx) {
					clientSecret = secret
				}
			}
		}
		if clientID == "" || (clientSecret == "" && !u.native.Apple.Configured(ctx)) {
			continue
		}
		rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = u.native.Apple.Revoke(rctx, clientID, clientSecret, token)
		cancel()
		if err != nil {
			if u.log != nil {
				u.log.Warn("apple_token_revoke_failed", "user_id", userID, "error", err)
			}
			continue
		}
		cid := clientID
		if err := u.repo.UpdateOAuthAccountRefreshToken(ctx, acc.ID, nil, &cid); err != nil && u.log != nil {
			u.log.Warn("apple_token_clear_failed", "user_id", userID, "error", err)
		}
	}
}
