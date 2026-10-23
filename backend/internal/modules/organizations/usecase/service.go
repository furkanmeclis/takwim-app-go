package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	authmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/slug"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type OrganizationHooks interface {
	OnCreated(ctx context.Context, tx pgx.Tx, org orgmodel.Organization, owner orgmodel.User) error
	OnDeleted(ctx context.Context, org orgmodel.Organization) error
}

type Repository interface {
	WithTx(ctx context.Context, fn repository.TxFunc) error
	CreateOrganization(ctx context.Context, q *db.Queries, params db.CreateOrganizationParams) (db.Organization, error)
	AddOrganizationMember(ctx context.Context, q *db.Queries, params db.AddOrganizationMemberParams) error
	GetUserByID(ctx context.Context, id int64) (db.User, error)
	GetUserByUUID(ctx context.Context, id uuid.UUID) (db.User, error)
	GetTenantRoleByKey(ctx context.Context, q *db.Queries, orgID int64, key string) (db.Role, error)
	GetOrganizationByUUID(ctx context.Context, id uuid.UUID) (db.Organization, error)
	GetOrganizationByID(ctx context.Context, id int64) (db.Organization, error)
	SlugExists(ctx context.Context, slug string) (bool, error)
	ListOrganizations(ctx context.Context, params db.ListOrganizationsFilteredParams) ([]db.Organization, int64, error)
	UpdateOrganization(ctx context.Context, params db.UpdateOrganizationParams) (db.Organization, error)
	SetOrganizationStatus(ctx context.Context, params db.SetOrganizationStatusParams) (db.Organization, error)
	SetOrganizationAccessWindow(ctx context.Context, params db.SetOrganizationAccessWindowParams) (db.Organization, error)
	SoftDeleteOrganization(ctx context.Context, id uuid.UUID) (db.Organization, error)
	LockOrganizationRow(ctx context.Context, q *db.Queries, orgID int64) error
	GetOrganizationMemberByUserUUID(ctx context.Context, orgID int64, userUUID uuid.UUID) (db.GetOrganizationMemberByUserUUIDRow, error)
	GetOrganizationMember(ctx context.Context, q *db.Queries, orgID, userID int64) (db.GetOrganizationMemberRow, error)
	ListOrganizationMembers(ctx context.Context, params db.ListOrganizationMembersByOrgParams) ([]db.ListOrganizationMembersByOrgRow, int64, error)
	ListOrganizationMembershipsByUser(ctx context.Context, userID int64) ([]db.ListOrganizationMembershipsByUserRow, error)
	UpdateOrganizationMemberRole(ctx context.Context, q *db.Queries, params db.UpdateOrganizationMemberRoleParams) error
	DeleteOrganizationMember(ctx context.Context, q *db.Queries, orgID, userID int64) error
	CountOrganizationOwners(ctx context.Context, q *db.Queries, orgID int64) (int64, error)
	ResolveOrganizationScopeBySlug(ctx context.Context, slug string, userID int64) (db.ResolveOrganizationScopeBySlugRow, error)
	ResolveOrganizationScopeByUUID(ctx context.Context, id uuid.UUID, userID int64) (db.ResolveOrganizationScopeByUUIDRow, error)
	ListTenantRolePermissionSlugsByRoleID(ctx context.Context, roleID int64) ([]string, error)
}

type Service struct {
	repo        Repository
	hooks       OrganizationHooks
	invalidator orgctx.Invalidator
	now         func() time.Time
}

func New(repo Repository, hooks OrganizationHooks) *Service {
	return &Service{repo: repo, hooks: hooks, now: time.Now}
}

func (s *Service) SetInvalidator(inv orgctx.Invalidator) {
	s.invalidator = inv
}

func (s *Service) CreateOrganization(ctx context.Context, in orgmodel.CreateOrganizationInput) (orgmodel.Organization, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || (in.OwnerUserID <= 0 && in.OwnerUserUUID == uuid.Nil) {
		return orgmodel.Organization{}, fmt.Errorf("%w: name and owner user are required", ErrInvalidRequest)
	}
	var (
		owner db.User
		err   error
	)
	if in.OwnerUserUUID != uuid.Nil {
		owner, err = s.repo.GetUserByUUID(ctx, in.OwnerUserUUID)
	} else {
		owner, err = s.repo.GetUserByID(ctx, in.OwnerUserID)
	}
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return orgmodel.Organization{}, ErrUserNotFound
		}
		return orgmodel.Organization{}, err
	}
	allocated, err := s.allocateSlug(ctx, in.Slug, name)
	if err != nil {
		return orgmodel.Organization{}, err
	}

	var created db.Organization
	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, tx pgx.Tx) error {
		row, err := s.repo.CreateOrganization(ctx, q, db.CreateOrganizationParams{
			Slug: allocated, Name: name,
		})
		if err != nil {
			return err
		}
		role, err := s.repo.GetTenantRoleByKey(ctx, q, row.ID, orgctx.RoleOwner)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return ErrRoleNotFound
			}
			return err
		}
		if err := s.repo.AddOrganizationMember(ctx, q, db.AddOrganizationMemberParams{
			OrganizationID: row.ID, UserID: owner.ID, RoleID: role.ID,
		}); err != nil {
			return err
		}
		created = row
		if s.hooks != nil {
			if err := s.hooks.OnCreated(ctx, tx, mapOrganization(row), mapUser(owner)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return orgmodel.Organization{}, err
	}
	return mapOrganization(created), nil
}

func (s *Service) ListOrganizations(ctx context.Context, f orgmodel.ListFilter) ([]orgmodel.Organization, int64, error) {
	rows, total, err := s.repo.ListOrganizations(ctx, db.ListOrganizationsFilteredParams{
		Status: pgText(f.Status), Q: pgText(f.Q), SortField: sortOrDefault(f.SortField, "created_at"),
		SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]orgmodel.Organization, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapOrganization(row))
	}
	return out, total, nil
}

func (s *Service) GetOrganization(ctx context.Context, id uuid.UUID) (orgmodel.Organization, error) {
	row, err := s.repo.GetOrganizationByUUID(ctx, id)
	if err != nil {
		return orgmodel.Organization{}, mapRepoNotFound(err, ErrNotFound)
	}
	return mapOrganization(row), nil
}

func (s *Service) PatchOrganization(ctx context.Context, id uuid.UUID, in orgmodel.PatchOrganizationInput) (orgmodel.Organization, error) {
	params := db.UpdateOrganizationParams{Uuid: id}
	if in.Name != nil {
		v := strings.TrimSpace(*in.Name)
		if v == "" {
			return orgmodel.Organization{}, fmt.Errorf("%w: name cannot be empty", ErrInvalidRequest)
		}
		params.Name = pgtype.Text{String: v, Valid: true}
	}
	if in.Slug != nil {
		v := normalizeRequestedSlug(*in.Slug)
		if v == "" || reservedSlug(v) {
			return orgmodel.Organization{}, fmt.Errorf("%w: slug is invalid or reserved", ErrInvalidRequest)
		}
		params.Slug = pgtype.Text{String: v, Valid: true}
	}
	if in.Settings != nil {
		params.Settings = in.Settings
	}
	row, err := s.repo.UpdateOrganization(ctx, params)
	if err != nil {
		return orgmodel.Organization{}, mapRepoNotFound(err, ErrNotFound)
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, row.ID)
	return mapOrganization(row), nil
}

func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, status string) (orgmodel.Organization, error) {
	status = strings.TrimSpace(status)
	if status != orgctx.StatusActive && status != orgctx.StatusSuspended {
		return orgmodel.Organization{}, fmt.Errorf("%w: status must be active or suspended", ErrInvalidRequest)
	}
	row, err := s.repo.SetOrganizationStatus(ctx, db.SetOrganizationStatusParams{Uuid: id, Status: status})
	if err != nil {
		return orgmodel.Organization{}, mapRepoNotFound(err, ErrNotFound)
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, row.ID)
	return mapOrganization(row), nil
}

func (s *Service) SetAccessWindow(ctx context.Context, id uuid.UUID, startsAt *time.Time, endsAt *time.Time) (orgmodel.Organization, error) {
	params := db.SetOrganizationAccessWindowParams{Uuid: id}
	if startsAt != nil {
		params.AccessStartsAt = pgtype.Timestamptz{Time: startsAt.UTC(), Valid: true}
	}
	if endsAt != nil {
		params.AccessEndsAt = pgtype.Timestamptz{Time: endsAt.UTC(), Valid: true}
	}
	row, err := s.repo.SetOrganizationAccessWindow(ctx, params)
	if err != nil {
		return orgmodel.Organization{}, mapRepoNotFound(err, ErrNotFound)
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, row.ID)
	return mapOrganization(row), nil
}

func (s *Service) DeleteOrganization(ctx context.Context, id uuid.UUID) error {
	row, err := s.repo.SoftDeleteOrganization(ctx, id)
	if err != nil {
		return mapRepoNotFound(err, ErrNotFound)
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, row.ID)
	if s.hooks != nil {
		return s.hooks.OnDeleted(ctx, mapOrganization(row))
	}
	return nil
}

func (s *Service) ListMembers(ctx context.Context, orgUUID uuid.UUID, f orgmodel.MemberListFilter) ([]orgmodel.Member, int64, error) {
	org, err := s.repo.GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		return nil, 0, mapRepoNotFound(err, ErrNotFound)
	}
	rows, total, err := s.repo.ListOrganizationMembers(ctx, db.ListOrganizationMembersByOrgParams{
		OrganizationID: org.ID, Q: pgText(f.Q), SortField: sortOrDefault(f.SortField, "created_at"),
		SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]orgmodel.Member, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapMember(row))
	}
	return out, total, nil
}

func (s *Service) AddMember(ctx context.Context, orgUUID, userUUID uuid.UUID, roleKey string) (orgmodel.Member, error) {
	org, user, err := s.loadOrgUser(ctx, orgUUID, userUUID)
	if err != nil {
		return orgmodel.Member{}, err
	}
	roleKey = normalizeRole(roleKey)
	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		if err := s.repo.LockOrganizationRow(ctx, q, org.ID); err != nil {
			return err
		}
		role, err := s.repo.GetTenantRoleByKey(ctx, q, org.ID, roleKey)
		if err != nil {
			return mapRepoNotFound(err, ErrRoleNotFound)
		}
		if err := s.repo.AddOrganizationMember(ctx, q, db.AddOrganizationMemberParams{
			OrganizationID: org.ID, UserID: user.ID, RoleID: role.ID,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return orgmodel.Member{}, err
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, org.ID)
	return s.GetMember(ctx, org.ID, user.Uuid)
}

func (s *Service) ChangeMemberRole(ctx context.Context, orgUUID, userUUID uuid.UUID, roleKey string) (orgmodel.Member, error) {
	org, user, err := s.loadOrgUser(ctx, orgUUID, userUUID)
	if err != nil {
		return orgmodel.Member{}, err
	}
	roleKey = normalizeRole(roleKey)
	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		if err := s.repo.LockOrganizationRow(ctx, q, org.ID); err != nil {
			return err
		}
		current, err := s.repo.GetOrganizationMember(ctx, q, org.ID, user.ID)
		if err != nil {
			return mapRepoNotFound(err, ErrMemberNotFound)
		}
		role, err := s.repo.GetTenantRoleByKey(ctx, q, org.ID, roleKey)
		if err != nil {
			return mapRepoNotFound(err, ErrRoleNotFound)
		}
		if current.RoleKey == orgctx.RoleOwner && role.Slug != orgctx.RoleOwner {
			if err := s.guardLastOwner(ctx, q, org.ID); err != nil {
				return err
			}
		}
		return s.repo.UpdateOrganizationMemberRole(ctx, q, db.UpdateOrganizationMemberRoleParams{
			OrganizationID: org.ID, UserID: user.ID, RoleID: role.ID,
		})
	})
	if err != nil {
		return orgmodel.Member{}, err
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, org.ID)
	return s.GetMember(ctx, org.ID, user.Uuid)
}

func (s *Service) RemoveMember(ctx context.Context, orgUUID, userUUID uuid.UUID) error {
	org, user, err := s.loadOrgUser(ctx, orgUUID, userUUID)
	if err != nil {
		return err
	}
	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		if err := s.repo.LockOrganizationRow(ctx, q, org.ID); err != nil {
			return err
		}
		current, err := s.repo.GetOrganizationMember(ctx, q, org.ID, user.ID)
		if err != nil {
			return mapRepoNotFound(err, ErrMemberNotFound)
		}
		if current.RoleKey == orgctx.RoleOwner {
			if err := s.guardLastOwner(ctx, q, org.ID); err != nil {
				return err
			}
		}
		return s.repo.DeleteOrganizationMember(ctx, q, org.ID, user.ID)
	})
	if err != nil {
		return err
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, org.ID)
	return nil
}

func (s *Service) GetMember(ctx context.Context, orgID int64, userUUID uuid.UUID) (orgmodel.Member, error) {
	row, err := s.repo.GetOrganizationMemberByUserUUID(ctx, orgID, userUUID)
	if err != nil {
		return orgmodel.Member{}, mapRepoNotFound(err, ErrMemberNotFound)
	}
	return mapMemberFromDetail(row), nil
}

func (s *Service) ListForUser(ctx context.Context, userID int64) ([]authmodel.MeOrganization, error) {
	rows, err := s.repo.ListOrganizationMembershipsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]authmodel.MeOrganization, 0, len(rows))
	for _, row := range rows {
		perms, err := s.repo.ListTenantRolePermissionSlugsByRoleID(ctx, row.RoleID)
		if err != nil {
			return nil, err
		}
		out = append(out, authmodel.MeOrganization{
			UUID: row.OrganizationUuid, Slug: row.OrganizationSlug, Name: row.OrganizationName,
			LogoURL: logoURL(row.LogoObjectKey), Status: row.OrganizationStatus,
			AccessEndsAt: timePtr(row.AccessEndsAt),
			Role:         authmodel.MeOrganizationRole{Key: row.RoleKey, Name: row.RoleName},
			Permissions:  perms,
		})
	}
	return out, nil
}

func (s *Service) Resolve(ctx context.Context, orgRef string, userID int64) (orgctx.Scope, error) {
	ref := strings.TrimSpace(orgRef)
	if ref == "" {
		return orgctx.Scope{}, orgctx.ErrOrganizationNotFound
	}
	var (
		rowID     int64
		rowUUID   uuid.UUID
		rowSlug   string
		rowName   string
		rowStatus string
		startsAt  pgtype.Timestamptz
		endsAt    pgtype.Timestamptz
		roleID    int64
		roleKey   string
		err       error
	)
	if id, parseErr := uuid.Parse(ref); parseErr == nil {
		row, err := s.repo.ResolveOrganizationScopeByUUID(ctx, id, userID)
		if err != nil {
			return s.resolveMissingOrError(ctx, id, "", err)
		}
		rowID, rowUUID, rowSlug, rowName, rowStatus = row.ID, row.Uuid, row.Slug, row.Name, row.Status
		startsAt, endsAt, roleID, roleKey = row.AccessStartsAt, row.AccessEndsAt, row.RoleID, row.RoleKey
	} else {
		row, err := s.repo.ResolveOrganizationScopeBySlug(ctx, strings.ToLower(ref), userID)
		if err != nil {
			return s.resolveMissingOrError(ctx, uuid.Nil, strings.ToLower(ref), err)
		}
		rowID, rowUUID, rowSlug, rowName, rowStatus = row.ID, row.Uuid, row.Slug, row.Name, row.Status
		startsAt, endsAt, roleID, roleKey = row.AccessStartsAt, row.AccessEndsAt, row.RoleID, row.RoleKey
	}
	if rowStatus == orgctx.StatusSuspended {
		return orgctx.Scope{}, orgctx.ErrSuspended
	}
	now := s.now().UTC()
	if startsAt.Valid && now.Before(startsAt.Time) {
		return orgctx.Scope{}, orgctx.ErrAccessExpired
	}
	var accessEnds *time.Time
	if endsAt.Valid {
		t := endsAt.Time.UTC()
		accessEnds = &t
		if now.After(t) {
			return orgctx.Scope{}, orgctx.ErrAccessExpired
		}
	}
	perms, err := s.repo.ListTenantRolePermissionSlugsByRoleID(ctx, roleID)
	if err != nil {
		return orgctx.Scope{}, err
	}
	return orgctx.Scope{
		ID: rowID, UUID: rowUUID, Slug: rowSlug, Name: rowName, Status: rowStatus,
		AccessStartsAt: startsAt.Time.UTC(), AccessEndsAt: accessEnds,
		MemberRole: roleKey, Permissions: perms,
	}, nil
}

func (s *Service) resolveMissingOrError(ctx context.Context, id uuid.UUID, slug string, err error) (orgctx.Scope, error) {
	if !errors.Is(err, repository.ErrNotFound) {
		return orgctx.Scope{}, err
	}
	if id != uuid.Nil {
		if _, orgErr := s.repo.GetOrganizationByUUID(ctx, id); orgErr == nil {
			return orgctx.Scope{}, orgctx.ErrNotMember
		}
	} else if slug != "" {
		if _, orgErr := s.repo.ResolveOrganizationScopeBySlug(ctx, slug, -1); orgErr == nil {
			return orgctx.Scope{}, orgctx.ErrNotMember
		}
		if exists, _ := s.repo.SlugExists(ctx, slug); exists {
			return orgctx.Scope{}, orgctx.ErrNotMember
		}
	}
	return orgctx.Scope{}, orgctx.ErrOrganizationNotFound
}

func (s *Service) loadOrgUser(ctx context.Context, orgUUID, userUUID uuid.UUID) (db.Organization, db.User, error) {
	org, err := s.repo.GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		return db.Organization{}, db.User{}, mapRepoNotFound(err, ErrNotFound)
	}
	user, err := s.repo.GetUserByUUID(ctx, userUUID)
	if err != nil {
		return db.Organization{}, db.User{}, mapRepoNotFound(err, ErrUserNotFound)
	}
	return org, user, nil
}

func (s *Service) guardLastOwner(ctx context.Context, q *db.Queries, orgID int64) error {
	count, err := s.repo.CountOrganizationOwners(ctx, q, orgID)
	if err != nil {
		return err
	}
	if count <= 1 {
		return ErrLastOrganizationOwner
	}
	return nil
}

func (s *Service) allocateSlug(ctx context.Context, requested, name string) (string, error) {
	base := ""
	if strings.TrimSpace(requested) != "" {
		base = normalizeRequestedSlug(requested)
	}
	if base == "" {
		base = normalizeRequestedSlug(name)
	}
	if base == "" {
		return "", fmt.Errorf("%w: slug is invalid", ErrInvalidRequest)
	}
	if reservedSlug(base) {
		base = base + "-org"
	}
	base = trimSlug(base, 64)
	for i := 0; i < 1000; i++ {
		candidate := base
		if i > 0 {
			suffix := fmt.Sprintf("-%d", i+1)
			candidate = trimSlug(base, 64-len(suffix)) + suffix
		}
		exists, err := s.repo.SlugExists(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: could not allocate slug", ErrConflict)
}

func normalizeRequestedSlug(v string) string {
	return trimSlug(slug.FromName(strings.TrimSpace(v)), 64)
}

func reservedSlug(v string) bool {
	switch v {
	case "admin", "api", "platform", "login", "t", "www":
		return true
	default:
		return false
	}
}

func trimSlug(v string, max int) string {
	if len(v) <= max {
		return strings.Trim(v, "-")
	}
	return strings.Trim(v[:max], "-")
}

func normalizeRole(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return orgctx.RoleMember
	}
	return v
}

func sortOrDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return strings.TrimSpace(v)
}

func mapRepoNotFound(err, target error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return target
	}
	return err
}

func pgText(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func mapOrganization(row db.Organization) orgmodel.Organization {
	return orgmodel.Organization{
		ID: row.ID, UUID: row.Uuid, Slug: row.Slug, Name: row.Name,
		LogoURL: logoURL(row.LogoObjectKey), Status: row.Status,
		AccessStartsAt: row.AccessStartsAt.Time.UTC(), AccessEndsAt: timePtr(row.AccessEndsAt),
		Settings: row.Settings, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
}

func mapUser(row db.User) orgmodel.User {
	return orgmodel.User{ID: row.ID, UUID: row.Uuid, Email: row.Email, Name: row.Name, Surname: row.Surname, Status: row.Status}
}

func mapMember(row db.ListOrganizationMembersByOrgRow) orgmodel.Member {
	return orgmodel.Member{
		ID:        row.ID,
		User:      orgmodel.User{ID: row.UserID, UUID: row.UserUuid, Email: row.Email, Name: row.Name, Surname: row.Surname, Status: row.Status},
		Role:      orgmodel.RoleSummary{Key: row.RoleKey, Name: row.RoleName},
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
}

func mapMemberFromDetail(row db.GetOrganizationMemberByUserUUIDRow) orgmodel.Member {
	return orgmodel.Member{
		ID:        row.ID,
		User:      orgmodel.User{ID: row.UserID, UUID: row.UserUuid, Email: row.Email, Name: row.Name, Surname: row.Surname, Status: row.Status},
		Role:      orgmodel.RoleSummary{Key: row.RoleKey, Name: row.RoleName},
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
}

func timePtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time.UTC()
	return &t
}

func logoURL(v pgtype.Text) *string {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return nil
	}
	u := "/v1/platform/organizations/logo/" + v.String
	return &u
}
