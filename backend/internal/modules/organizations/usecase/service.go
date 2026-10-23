package usecase

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	authmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/auth/model"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sector"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/slug"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/localtime"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
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
	ListOrganizations(ctx context.Context, params db.ListOrganizationsWithStatsParams) ([]db.ListOrganizationsWithStatsRow, int64, error)
	UpdateOrganization(ctx context.Context, params db.UpdateOrganizationParams) (db.Organization, error)
	UpdateOrganizationEntitlements(ctx context.Context, params db.UpdateOrganizationEntitlementsParams) (db.Organization, error)
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
	AccessRepository
}

// AccessRepository is the persistence of multi-role memberships, branch
// scope and organization-specific roles (tenant_access.sql).
type AccessRepository interface {
	ListMemberPermissionSlugs(ctx context.Context, orgID, memberID int64) ([]string, error)
	ListMemberRoles(ctx context.Context, q *db.Queries, orgID int64, memberIDs []int64) ([]db.ListMemberRolesRow, error)
	ListMemberBranchIDs(ctx context.Context, orgID, memberID int64) ([]int64, error)
	ListMemberBranches(ctx context.Context, orgID int64, memberIDs []int64) ([]db.ListMemberBranchesRow, error)
	ListBranchIDsByUUIDs(ctx context.Context, orgID int64, ids []uuid.UUID) ([]db.ListBranchIDsByUUIDsRow, error)
	ReplaceMemberRoles(ctx context.Context, q *db.Queries, orgID, memberID int64, roleIDs []int64, primaryRoleID int64) error
	ReplaceMemberBranches(ctx context.Context, q *db.Queries, orgID, memberID int64, scope string, branchIDs []int64) error
	ListTenantMembers(ctx context.Context, params db.ListTenantMembersParams) ([]db.ListTenantMembersRow, int64, error)
	GetTenantMember(ctx context.Context, orgID int64, userUUID uuid.UUID) (db.ListTenantMembersRow, error)
	ListTenantRoles(ctx context.Context, orgID int64) ([]repository.TenantRoleRow, error)
	GetTenantRoleByUUID(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (repository.TenantRoleRow, error)
	ListRolePermissionSlugs(ctx context.Context, roleIDs []int64) (map[int64][]string, error)
	ListTenantPermissions(ctx context.Context) ([]db.ListTenantPermissionsRow, error)
	TenantRoleSlugTaken(ctx context.Context, orgID int64, slug string) (bool, error)
	CreateTenantRole(ctx context.Context, q *db.Queries, orgID int64, name, slug, description string) (int64, uuid.UUID, error)
	UpdateTenantRole(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID, name, description string) (int64, error)
	DeleteTenantRole(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) error
	SetRolePermissions(ctx context.Context, q *db.Queries, roleID int64, slugs []string) error
}

type Service struct {
	repo        Repository
	hooks       OrganizationHooks
	invalidator orgctx.Invalidator
	staff       StaffLimiter
	now         func() time.Time
}

// StaffLimiter enforces the staff seat limit of the plan (TEC-883): q is
// the transaction that locked the organization row.
type StaffLimiter interface {
	CheckStaffAdd(ctx context.Context, q *db.Queries, scope orgctx.Scope) error
}

// SetStaffLimiter attaches the plan limit check of members and invitations.
func (s *Service) SetStaffLimiter(l StaffLimiter) {
	s.staff = l
}

func (s *Service) checkStaffAdd(ctx context.Context, q *db.Queries, scope orgctx.Scope) error {
	if s.staff == nil {
		return nil
	}
	return s.staff.CheckStaffAdd(ctx, q, scope)
}

func New(repo Repository, hooks OrganizationHooks) *Service {
	return &Service{repo: repo, hooks: hooks, now: time.Now}
}

func (s *Service) SetInvalidator(inv orgctx.Invalidator) {
	s.invalidator = inv
}

func (s *Service) CreateOrganization(ctx context.Context, in orgmodel.CreateOrganizationInput) (orgmodel.Organization, error) {
	return s.createOrganization(ctx, in, nil)
}

// createOrganization creates the organization, its owner membership (unless
// NoOwner) and runs OrganizationHooks.OnCreated in one transaction. after
// runs last inside the same transaction (owner invitation).
func (s *Service) createOrganization(
	ctx context.Context,
	in orgmodel.CreateOrganizationInput,
	after func(ctx context.Context, q *db.Queries, org db.Organization) error,
) (orgmodel.Organization, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || (!in.NoOwner && in.OwnerUserID <= 0 && in.OwnerUserUUID == uuid.Nil) {
		return orgmodel.Organization{}, fmt.Errorf("%w: name and owner user are required", ErrInvalidRequest)
	}
	var (
		owner db.User
		err   error
	)
	if !in.NoOwner {
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
	}
	var sectorParam pgtype.Text
	if v := strings.TrimSpace(in.Sector); v != "" {
		if !sector.Valid(v) {
			return orgmodel.Organization{}, fmt.Errorf("%w: sector is not known", ErrInvalidRequest)
		}
		sectorParam = pgtype.Text{String: v, Valid: true}
	}
	allocated, err := s.allocateSlug(ctx, in.Slug, name)
	if err != nil {
		return orgmodel.Organization{}, err
	}

	var created db.Organization
	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, tx pgx.Tx) error {
		row, err := s.repo.CreateOrganization(ctx, q, db.CreateOrganizationParams{
			Slug: allocated, Name: name, Sector: sectorParam,
		})
		if err != nil {
			return err
		}
		if !in.NoOwner {
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
		}
		created = row
		if s.hooks != nil {
			if err := s.hooks.OnCreated(ctx, tx, mapOrganization(row), mapUser(owner)); err != nil {
				return err
			}
		}
		if after != nil {
			return after(ctx, q, row)
		}
		return nil
	})
	if err != nil {
		return orgmodel.Organization{}, err
	}
	return mapOrganization(created), nil
}

func (s *Service) ListOrganizations(ctx context.Context, f orgmodel.ListFilter) ([]orgmodel.OrganizationSummary, int64, error) {
	rows, total, err := s.repo.ListOrganizations(ctx, db.ListOrganizationsWithStatsParams{
		Statuses: nilIfEmpty(f.Statuses), AccessStates: nilIfEmpty(f.AccessStates), Windows: nilIfEmpty(f.Windows),
		Q: pgText(f.Q), SortField: sortOrDefault(f.SortField, "created_at"),
		SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	now := s.now().UTC()
	out := make([]orgmodel.OrganizationSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapOrganizationSummary(row, now))
	}
	return out, total, nil
}

// GetOrganizationSummary returns one organization with owner and counts.
func (s *Service) GetOrganizationSummary(ctx context.Context, id uuid.UUID) (orgmodel.OrganizationSummary, error) {
	rows, _, err := s.repo.ListOrganizations(ctx, db.ListOrganizationsWithStatsParams{
		OrganizationUuid: pgtype.UUID{Bytes: id, Valid: true}, SortField: "created_at", LimitCount: 1,
	})
	if err != nil {
		return orgmodel.OrganizationSummary{}, err
	}
	if len(rows) == 0 {
		return orgmodel.OrganizationSummary{}, ErrNotFound
	}
	return mapOrganizationSummary(rows[0], s.now().UTC()), nil
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
	if in.Timezone != nil {
		v := strings.TrimSpace(*in.Timezone)
		if !localtime.ValidZone(v) {
			return orgmodel.Organization{}, fmt.Errorf("%w: timezone is not a known IANA zone", ErrInvalidRequest)
		}
		params.Timezone = pgtype.Text{String: v, Valid: true}
	}
	if in.Currency != nil {
		v, err := money.ParseCurrency(strings.ToUpper(strings.TrimSpace(*in.Currency)))
		if err != nil {
			return orgmodel.Organization{}, fmt.Errorf("%w: currency must be a 3-letter ISO 4217 code", ErrInvalidRequest)
		}
		params.Currency = pgtype.Text{String: string(v), Valid: true}
	}
	if in.Sector != nil {
		v := strings.TrimSpace(*in.Sector)
		if !sector.Valid(v) {
			return orgmodel.Organization{}, fmt.Errorf("%w: sector is not known", ErrInvalidRequest)
		}
		params.Sector = pgtype.Text{String: v, Valid: true}
	}
	row, err := s.repo.UpdateOrganization(ctx, params)
	if err != nil {
		return orgmodel.Organization{}, mapRepoNotFound(err, ErrNotFound)
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, row.ID)
	return mapOrganization(row), nil
}

// SetEntitlements replaces the plan, add-ons and limit overrides of an
// organization (platform only; TEC-883).
func (s *Service) SetEntitlements(ctx context.Context, id uuid.UUID, in orgmodel.SetEntitlementsInput) (orgmodel.Organization, error) {
	plan := strings.TrimSpace(in.Plan)
	if !entitlement.ValidPlan(plan) {
		return orgmodel.Organization{}, fmt.Errorf("%w: plan is not known", ErrInvalidRequest)
	}
	addons := []string{}
	seen := map[string]bool{}
	for _, a := range in.Addons {
		a = strings.TrimSpace(a)
		if !entitlement.ValidAddon(a) {
			return orgmodel.Organization{}, fmt.Errorf("%w: add-on %q is not known", ErrInvalidRequest, a)
		}
		if !seen[a] {
			seen[a] = true
			addons = append(addons, a)
		}
	}
	for _, l := range []*int32{in.BranchLimit, in.StaffLimit} {
		if l != nil && *l < 1 {
			return orgmodel.Organization{}, fmt.Errorf("%w: a limit must be at least 1", ErrInvalidRequest)
		}
	}
	row, err := s.repo.UpdateOrganizationEntitlements(ctx, db.UpdateOrganizationEntitlementsParams{
		Uuid: id, Plan: plan, Addons: addons,
		BranchLimit: int4From(in.BranchLimit), StaffLimit: int4From(in.StaffLimit),
	})
	if err != nil {
		return orgmodel.Organization{}, mapRepoNotFound(err, ErrNotFound)
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, row.ID)
	return mapOrganization(row), nil
}

func int4Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	n := v.Int32
	return &n
}

func int4From(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
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
		// The organization row is locked, so this check cannot race another add.
		if _, err := s.repo.GetOrganizationMember(ctx, q, org.ID, user.ID); err == nil {
			return fmt.Errorf("%w: user is already a member", ErrConflict)
		} else if !errors.Is(err, repository.ErrNotFound) {
			return err
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
		isOwner, err := s.memberHoldsOwner(ctx, q, org.ID, current.ID)
		if err != nil {
			return err
		}
		if isOwner && (role.Slug != orgctx.RoleOwner || role.OrganizationID.Valid) {
			if err := s.guardLastOwner(ctx, q, org.ID); err != nil {
				return err
			}
		}
		// The platform API manages a single role: it replaces every role
		// the member holds.
		return s.repo.ReplaceMemberRoles(ctx, q, org.ID, current.ID, []int64{role.ID}, role.ID)
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
		isOwner, err := s.memberHoldsOwner(ctx, q, org.ID, current.ID)
		if err != nil {
			return err
		}
		if isOwner {
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
		perms, err := s.repo.ListMemberPermissionSlugs(ctx, row.OrganizationID, row.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, authmodel.MeOrganization{
			UUID: row.OrganizationUuid, Slug: row.OrganizationSlug, Name: row.OrganizationName,
			LogoURL: LogoURL(row.OrganizationUuid, row.LogoObjectKey), Status: row.OrganizationStatus,
			AccessEndsAt: timePtr(row.AccessEndsAt),
			Role:         authmodel.MeOrganizationRole{Key: row.RoleKey, Name: row.RoleName},
			Permissions:  perms,
		})
	}
	return out, nil
}

// BranchIDByUUID implements orgctx.BranchSelector (the X-Branch header):
// the id of a live branch of organizationID, or orgctx.ErrBranchNotAllowed.
func (s *Service) BranchIDByUUID(ctx context.Context, organizationID int64, branchUUID uuid.UUID) (int64, error) {
	rows, err := s.repo.ListBranchIDsByUUIDs(ctx, organizationID, []uuid.UUID{branchUUID})
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, orgctx.ErrBranchNotAllowed
	}
	return rows[0].ID, nil
}

func (s *Service) Resolve(ctx context.Context, orgRef string, userID int64) (orgctx.Scope, error) {
	ref := strings.TrimSpace(orgRef)
	if ref == "" {
		return orgctx.Scope{}, orgctx.ErrOrganizationNotFound
	}
	var (
		rowID       int64
		rowUUID     uuid.UUID
		rowSlug     string
		rowName     string
		rowStatus   string
		rowSector   string
		rowPlan     string
		rowAddons   []string
		rowBranches pgtype.Int4
		rowStaff    pgtype.Int4
		rowZone     string
		rowCurrency string
		startsAt    pgtype.Timestamptz
		endsAt      pgtype.Timestamptz
		roleKey     string
		memberID    int64
		branchScope string
		err         error
	)
	if id, parseErr := uuid.Parse(ref); parseErr == nil {
		row, err := s.repo.ResolveOrganizationScopeByUUID(ctx, id, userID)
		if err != nil {
			return s.resolveMissingOrError(ctx, id, "", err)
		}
		rowID, rowUUID, rowSlug, rowName, rowStatus = row.ID, row.Uuid, row.Slug, row.Name, row.Status
		startsAt, endsAt, roleKey = row.AccessStartsAt, row.AccessEndsAt, row.RoleKey
		memberID, branchScope = row.MemberID, row.BranchScope
		rowSector, rowZone, rowCurrency = row.Sector, row.Timezone, row.Currency
		rowPlan, rowAddons, rowBranches, rowStaff = row.Plan, row.Addons, row.BranchLimit, row.StaffLimit
	} else {
		row, err := s.repo.ResolveOrganizationScopeBySlug(ctx, strings.ToLower(ref), userID)
		if err != nil {
			return s.resolveMissingOrError(ctx, uuid.Nil, strings.ToLower(ref), err)
		}
		rowID, rowUUID, rowSlug, rowName, rowStatus = row.ID, row.Uuid, row.Slug, row.Name, row.Status
		startsAt, endsAt, roleKey = row.AccessStartsAt, row.AccessEndsAt, row.RoleKey
		memberID, branchScope = row.MemberID, row.BranchScope
		rowSector, rowZone, rowCurrency = row.Sector, row.Timezone, row.Currency
		rowPlan, rowAddons, rowBranches, rowStaff = row.Plan, row.Addons, row.BranchLimit, row.StaffLimit
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
	perms, err := s.repo.ListMemberPermissionSlugs(ctx, rowID, memberID)
	if err != nil {
		return orgctx.Scope{}, err
	}
	roles, err := s.repo.ListMemberRoles(ctx, nil, rowID, []int64{memberID})
	if err != nil {
		return orgctx.Scope{}, err
	}
	roleKeys := make([]string, 0, len(roles))
	for _, r := range roles {
		roleKeys = append(roleKeys, r.Slug)
	}
	var branchIDs []int64
	if branchScope == orgctx.BranchScopeSelected {
		// Non-nil (possibly empty): the member sees only these branches.
		if branchIDs, err = s.repo.ListMemberBranchIDs(ctx, rowID, memberID); err != nil {
			return orgctx.Scope{}, err
		}
		if branchIDs == nil {
			branchIDs = []int64{}
		}
	}
	return orgctx.Scope{
		ID: rowID, UUID: rowUUID, Slug: rowSlug, Name: rowName, Status: rowStatus,
		AccessStartsAt: startsAt.Time.UTC(), AccessEndsAt: accessEnds,
		Sector: rowSector, Timezone: rowZone, Currency: rowCurrency,
		Plan: rowPlan, Addons: nonNilStrings(rowAddons), BranchLimit: int4Ptr(rowBranches), StaffLimit: int4Ptr(rowStaff),
		MemberRole: roleKey, MemberRoles: roleKeys, Permissions: perms, BranchIDs: branchIDs,
	}, nil
}

// ResolveMember implements orgctx.JobScopeLoader: the current scope of a
// member, used by worker jobs to run with the requester's permissions and
// branch scope.
func (s *Service) ResolveMember(ctx context.Context, organizationID, userID int64) (orgctx.Scope, error) {
	org, err := s.repo.GetOrganizationByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return orgctx.Scope{}, orgctx.ErrOrganizationNotFound
		}
		return orgctx.Scope{}, err
	}
	return s.Resolve(ctx, org.Uuid.String(), userID)
}

// memberHoldsOwner reports whether the member holds the template owner role.
func (s *Service) memberHoldsOwner(ctx context.Context, q *db.Queries, orgID, memberID int64) (bool, error) {
	roles, err := s.repo.ListMemberRoles(ctx, q, orgID, []int64{memberID})
	if err != nil {
		return false, err
	}
	for _, r := range roles {
		if r.IsTemplate && r.Slug == orgctx.RoleOwner {
			return true, nil
		}
	}
	return false, nil
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
		LogoURL: LogoURL(row.Uuid, row.LogoObjectKey), Status: row.Status,
		Timezone: row.Timezone, Currency: row.Currency, Sector: row.Sector,
		Plan: row.Plan, Addons: nonNilStrings(row.Addons),
		BranchLimit: int4Ptr(row.BranchLimit), StaffLimit: int4Ptr(row.StaffLimit),
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

// LogoURL is the API-owned logo stream of an organization; the version
// parameter changes with every upload (each upload gets a new object key).
func LogoURL(orgUUID uuid.UUID, key pgtype.Text) *string {
	if !key.Valid || strings.TrimSpace(key.String) == "" {
		return nil
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key.String))
	u := fmt.Sprintf("/v1/organizations/%s/logo?v=%x", orgUUID, h.Sum32())
	return &u
}
