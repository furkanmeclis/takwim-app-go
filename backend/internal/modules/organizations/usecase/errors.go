package usecase

import "errors"

var (
	ErrNotFound              = errors.New("organization not found")
	ErrMemberNotFound        = errors.New("organization member not found")
	ErrUserNotFound          = errors.New("user not found")
	ErrRoleNotFound          = errors.New("tenant role not found")
	ErrInvalidRequest        = errors.New("invalid request")
	ErrConflict              = errors.New("conflict")
	ErrLastOrganizationOwner = errors.New("last organization owner")
)
