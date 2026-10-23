package usecase

import (
	"context"
	"errors"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/repository"
	"github.com/google/uuid"
)

// QRLoginUser is the account that approves / receives a QR sign-in.
type QRLoginUser struct {
	UUID  uuid.UUID
	Email string
	Name  string
}

// CheckQRLoginApprover verifies the approving account is still allowed to sign
// in (a deactivated account returns ErrAccountDeactivated).
func (u *AuthUseCase) CheckQRLoginApprover(ctx context.Context, userUUID uuid.UUID) error {
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrInvalidCredentials
		}
		return err
	}
	return userStatusError(user)
}

// IssueQRLoginSession issues a web session for the user who approved a QR
// sign-in on their phone. The approving device already holds a signed-in session, so no second factor
// is asked again (same model as passkey / native sign-in).
func (u *AuthUseCase) IssueQRLoginSession(
	ctx context.Context,
	userUUID uuid.UUID,
	meta model.SessionMeta,
) (model.Tokens, QRLoginUser, error) {
	user, err := u.repo.FindUserByUUID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.Tokens{}, QRLoginUser{}, ErrInvalidCredentials
		}
		return model.Tokens{}, QRLoginUser{}, err
	}
	if err := userStatusError(user); err != nil {
		return model.Tokens{}, QRLoginUser{}, err
	}
	if err := u.repo.UpdateLastLogin(ctx, user.ID); err != nil {
		return model.Tokens{}, QRLoginUser{}, err
	}
	tokens, err := u.issueTokensForUser(ctx, user, meta)
	if err != nil {
		return model.Tokens{}, QRLoginUser{}, err
	}
	return tokens, QRLoginUser{UUID: user.UUID, Email: user.Email, Name: user.Name}, nil
}
