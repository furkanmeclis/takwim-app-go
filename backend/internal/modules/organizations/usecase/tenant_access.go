package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/slug"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Tenant member management (/v1/tenant/members) and organization roles
// (/v1/tenant/roles). Every method takes the caller's orgctx.Scope: the
// organization comes from it and the guards below compare the caller's own
// roles, permissions and branch scope with the requested change.
//
// Guards:
//   - only owners grant or revoke the owner role; the last owner stays (409)
//   - a non-owner can only grant roles / role permissions it holds itself
//   - a branch-restricted caller can only manage members whose current and
//     new branches are inside its own branch scope, and never grants 'all'
//   - template roles are read-only; organization roles in use cannot be deleted

var (
	// ErrForbidden is a guard violation (403).
	ErrForbidden = errors.New("forbidden")
	// ErrRoleReadOnly is returned for changes to template roles (403).
	ErrRoleReadOnly = errors.New("template roles are read-only")
	// ErrRoleInUse is returned when deleting a role members still hold (409).
	ErrRoleInUse = errors.New("role is assigned to members")
	// ErrRoleKeyTaken is returned for a duplicate role key (409).
	ErrRoleKeyTaken = errors.New("role key is already used")
)

// FieldError is one field-level validation problem.
type FieldError struct {
	Field   string
	Code    string
	Message string
}

// ValidationError carries field details (400 VALIDATION_ERROR).
type ValidationError struct {
	Details []FieldError
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, FieldError{Field: field, Code: code, Message: msg})
}

func (e *ValidationError) orNil() error {
	if len(e.Details) == 0 {
		return nil
	}
	return e
}

// MemberSortColumns are the sortable member list fields.
var MemberSortColumns = map[string]string{
	"name": "name", "email": "email", "branch_scope": "branch_scope",
	"last_login_at": "last_login_at", "created_at": "created_at",
}

// rolePrecedence picks the primary (display) role of a member.
var rolePrecedence = []string{
	orgctx.RoleOwner, orgctx.RoleAdmin, rbac.TenantRoleSubeYoneticisi,
	rbac.TenantRoleResepsiyon, orgctx.RoleMember,
}

func isOwnerScope(s orgctx.Scope) bool {
	return slices.Contains(s.MemberRoles, orgctx.RoleOwner) || s.MemberRole == orgctx.RoleOwner
}

// ListTenantMembers lists the members of the caller's organization.
func (s *Service) ListTenantMembers(ctx context.Context, scope orgctx.Scope, f orgmodel.TenantMemberListFilter) ([]orgmodel.TenantMember, int64, error) {
	if _, ok := MemberSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", false
	}
	params := db.ListTenantMembersParams{
		OrganizationID: scope.ID, Q: pgText(f.Q), BranchScope: pgText(f.BranchScope),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if len(f.RoleUUIDs) > 0 {
		ids := []int64{}
		for _, id := range f.RoleUUIDs {
			role, err := s.repo.GetTenantRoleByUUID(ctx, nil, scope.ID, id)
			if err == nil {
				ids = append(ids, role.ID)
			} else if !errors.Is(err, repository.ErrNotFound) {
				return nil, 0, err
			}
		}
		params.RoleIds = ids
	}
	rows, total, err := s.repo.ListTenantMembers(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	out, err := s.enrichMembers(ctx, scope.ID, rows)
	return out, total, err
}

// GetTenantMember returns one member of the caller's organization.
func (s *Service) GetTenantMember(ctx context.Context, scope orgctx.Scope, userUUID uuid.UUID) (orgmodel.TenantMember, error) {
	row, err := s.repo.GetTenantMember(ctx, scope.ID, userUUID)
	if err != nil {
		return orgmodel.TenantMember{}, mapRepoNotFound(err, ErrMemberNotFound)
	}
	out, err := s.enrichMembers(ctx, scope.ID, []db.ListTenantMembersRow{row})
	if err != nil {
		return orgmodel.TenantMember{}, err
	}
	return out[0], nil
}

// PatchTenantMember changes the roles and / or branch access of a member.
func (s *Service) PatchTenantMember(ctx context.Context, scope orgctx.Scope, userUUID uuid.UUID, in orgmodel.PatchTenantMemberInput) (orgmodel.TenantMember, error) {
	target, err := s.GetTenantMember(ctx, scope, userUUID)
	if err != nil {
		return orgmodel.TenantMember{}, err
	}
	if err := s.guardBranchCoverage(ctx, scope, target); err != nil {
		return orgmodel.TenantMember{}, err
	}
	ve := &ValidationError{}
	var (
		newRoles    []repository.TenantRoleRow
		newScope    string
		newBranches []int64
	)
	if in.RoleUUIDs != nil {
		if len(*in.RoleUUIDs) == 0 {
			ve.add("role_uuids", "required", "at least one role is required")
		}
		seen := map[uuid.UUID]bool{}
		for i, id := range *in.RoleUUIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			role, err := s.repo.GetTenantRoleByUUID(ctx, nil, scope.ID, id)
			if errors.Is(err, repository.ErrNotFound) {
				ve.add(fmt.Sprintf("role_uuids.%d", i), "not_found", "role was not found")
				continue
			}
			if err != nil {
				return orgmodel.TenantMember{}, err
			}
			newRoles = append(newRoles, role)
		}
	}
	if in.BranchScope != nil {
		newScope = strings.TrimSpace(*in.BranchScope)
		switch newScope {
		case orgctx.BranchScopeAll:
			if !scope.AllBranches() {
				return orgmodel.TenantMember{}, fmt.Errorf("%w: only members with access to every branch can grant it", ErrForbidden)
			}
		case orgctx.BranchScopeSelected:
			if in.BranchUUIDs == nil || len(*in.BranchUUIDs) == 0 {
				ve.add("branch_uuids", "required", "select at least one branch")
				break
			}
			found, err := s.repo.ListBranchIDsByUUIDs(ctx, scope.ID, *in.BranchUUIDs)
			if err != nil {
				return orgmodel.TenantMember{}, err
			}
			byUUID := map[uuid.UUID]int64{}
			for _, b := range found {
				byUUID[b.Uuid] = b.ID
			}
			for i, id := range *in.BranchUUIDs {
				branchID, ok := byUUID[id]
				switch {
				case !ok:
					ve.add(fmt.Sprintf("branch_uuids.%d", i), "not_found", "branch was not found")
				case !scope.AllowsBranch(branchID):
					// Same answer as an unknown branch: do not leak it.
					ve.add(fmt.Sprintf("branch_uuids.%d", i), "not_found", "branch was not found")
				case !slices.Contains(newBranches, branchID):
					newBranches = append(newBranches, branchID)
				}
			}
		default:
			ve.add("branch_scope", "invalid", "branch_scope must be all or selected")
		}
	} else if in.BranchUUIDs != nil {
		ve.add("branch_scope", "required", "branch_scope is required with branch_uuids")
	}
	if err := ve.orNil(); err != nil {
		return orgmodel.TenantMember{}, err
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		if err := s.repo.LockOrganizationRow(ctx, q, scope.ID); err != nil {
			return err
		}
		if in.RoleUUIDs != nil {
			if err := s.applyMemberRoles(ctx, q, scope, target, newRoles); err != nil {
				return err
			}
		}
		if in.BranchScope != nil {
			return s.repo.ReplaceMemberBranches(ctx, q, scope.ID, target.ID, newScope, newBranches)
		}
		return nil
	})
	if err != nil {
		return orgmodel.TenantMember{}, err
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, scope.ID)
	return s.GetTenantMember(ctx, scope, userUUID)
}

func (s *Service) applyMemberRoles(ctx context.Context, q *db.Queries, scope orgctx.Scope, target orgmodel.TenantMember, roles []repository.TenantRoleRow) error {
	current, err := s.repo.ListMemberRoles(ctx, q, scope.ID, []int64{target.ID})
	if err != nil {
		return err
	}
	held := map[int64]bool{}
	hadOwner := false
	for _, r := range current {
		held[r.ID] = true
		if r.IsTemplate && r.Slug == orgctx.RoleOwner {
			hadOwner = true
		}
	}
	willOwner := false
	ids := make([]int64, 0, len(roles))
	added := []int64{}
	for _, r := range roles {
		ids = append(ids, r.ID)
		if r.IsTemplate && r.Slug == orgctx.RoleOwner {
			willOwner = true
		}
		if !held[r.ID] {
			added = append(added, r.ID)
		}
	}
	callerOwner := isOwnerScope(scope)
	if hadOwner != willOwner && !callerOwner {
		return fmt.Errorf("%w: only owners can grant or revoke the owner role", ErrForbidden)
	}
	if len(added) > 0 && !callerOwner {
		perms, err := s.repo.ListRolePermissionSlugs(ctx, added)
		if err != nil {
			return err
		}
		for _, list := range perms {
			for _, p := range list {
				if !scope.Has(p) {
					return fmt.Errorf("%w: cannot grant a role with permission %s you do not hold", ErrForbidden, p)
				}
			}
		}
	}
	if hadOwner && !willOwner {
		if err := s.guardLastOwner(ctx, q, scope.ID); err != nil {
			return err
		}
	}
	return s.repo.ReplaceMemberRoles(ctx, q, scope.ID, target.ID, ids, primaryRole(roles).ID)
}

func primaryRole(roles []repository.TenantRoleRow) repository.TenantRoleRow {
	for _, key := range rolePrecedence {
		for _, r := range roles {
			if r.IsTemplate && r.Slug == key {
				return r
			}
		}
	}
	return roles[0]
}

// RemoveTenantMember removes a member from the caller's organization.
func (s *Service) RemoveTenantMember(ctx context.Context, scope orgctx.Scope, userUUID uuid.UUID) error {
	target, err := s.GetTenantMember(ctx, scope, userUUID)
	if err != nil {
		return err
	}
	if err := s.guardBranchCoverage(ctx, scope, target); err != nil {
		return err
	}
	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		if err := s.repo.LockOrganizationRow(ctx, q, scope.ID); err != nil {
			return err
		}
		isOwner, err := s.memberHoldsOwner(ctx, q, scope.ID, target.ID)
		if err != nil {
			return err
		}
		if isOwner {
			if !isOwnerScope(scope) {
				return fmt.Errorf("%w: only owners can remove an owner", ErrForbidden)
			}
			if err := s.guardLastOwner(ctx, q, scope.ID); err != nil {
				return err
			}
		}
		member, err := s.repo.GetTenantMember(ctx, scope.ID, userUUID)
		if err != nil {
			return mapRepoNotFound(err, ErrMemberNotFound)
		}
		return s.repo.DeleteOrganizationMember(ctx, q, scope.ID, member.UserID)
	})
	if err != nil {
		return mapRepoNotFound(err, ErrMemberNotFound)
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, scope.ID)
	return nil
}

// guardBranchCoverage stops a branch-restricted caller from managing members
// that can see branches outside the caller's own scope.
func (s *Service) guardBranchCoverage(_ context.Context, scope orgctx.Scope, target orgmodel.TenantMember) error {
	if scope.AllBranches() {
		return nil
	}
	if target.BranchScope != orgctx.BranchScopeSelected {
		return fmt.Errorf("%w: member has access to every branch", ErrForbidden)
	}
	for _, id := range target.BranchIDs {
		if !scope.AllowsBranch(id) {
			return fmt.Errorf("%w: member has access to branches outside your scope", ErrForbidden)
		}
	}
	return nil
}

func (s *Service) enrichMembers(ctx context.Context, orgID int64, rows []db.ListTenantMembersRow) ([]orgmodel.TenantMember, error) {
	out := make([]orgmodel.TenantMember, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	roles, err := s.repo.ListMemberRoles(ctx, nil, orgID, ids)
	if err != nil {
		return nil, err
	}
	branches, err := s.repo.ListMemberBranches(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	rolesBy := map[int64][]orgmodel.TenantRoleRef{}
	for _, r := range roles {
		rolesBy[r.MemberID] = append(rolesBy[r.MemberID], orgmodel.TenantRoleRef{
			UUID: r.Uuid, Key: r.Slug, Name: r.Name, IsTemplate: r.IsTemplate,
		})
	}
	branchesBy := map[int64][]orgmodel.BranchRef{}
	branchIDsBy := map[int64][]int64{}
	for _, b := range branches {
		branchesBy[b.MemberID] = append(branchesBy[b.MemberID], orgmodel.BranchRef{UUID: b.Uuid, Code: b.Code, Name: b.Name})
		branchIDsBy[b.MemberID] = append(branchIDsBy[b.MemberID], b.ID)
	}
	for _, row := range rows {
		m := orgmodel.TenantMember{
			ID: row.ID,
			User: orgmodel.TenantMemberUser{
				UUID: row.UserUuid, Email: row.Email, Name: row.Name, Surname: row.Surname, Status: row.Status,
			},
			Roles: rolesBy[row.ID], BranchScope: row.BranchScope, Branches: branchesBy[row.ID],
			BranchIDs:   branchIDsBy[row.ID],
			LastLoginAt: timePtr(row.LastLoginAt),
			CreatedAt:   row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
		}
		if m.Roles == nil {
			m.Roles = []orgmodel.TenantRoleRef{}
		}
		if m.Branches == nil || m.BranchScope != orgctx.BranchScopeSelected {
			m.Branches = []orgmodel.BranchRef{}
		}
		out = append(out, m)
	}
	return out, nil
}

// --- roles ------------------------------------------------------------------

var roleKeyRE = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

// ListTenantRoles lists the template roles and the organization's own roles.
func (s *Service) ListTenantRoles(ctx context.Context, scope orgctx.Scope) ([]orgmodel.TenantRole, error) {
	rows, err := s.repo.ListTenantRoles(ctx, scope.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	perms, err := s.repo.ListRolePermissionSlugs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]orgmodel.TenantRole, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapTenantRole(row, perms[row.ID]))
	}
	return out, nil
}

// GetTenantRole returns one role visible to the organization.
func (s *Service) GetTenantRole(ctx context.Context, scope orgctx.Scope, id uuid.UUID) (orgmodel.TenantRole, error) {
	row, err := s.repo.GetTenantRoleByUUID(ctx, nil, scope.ID, id)
	if err != nil {
		return orgmodel.TenantRole{}, mapRepoNotFound(err, ErrRoleNotFound)
	}
	perms, err := s.repo.ListRolePermissionSlugs(ctx, []int64{row.ID})
	if err != nil {
		return orgmodel.TenantRole{}, err
	}
	return mapTenantRole(row, perms[row.ID]), nil
}

// ListTenantPermissions lists the assignable tenant permissions.
func (s *Service) ListTenantPermissions(ctx context.Context) ([]orgmodel.TenantPermission, error) {
	rows, err := s.repo.ListTenantPermissions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]orgmodel.TenantPermission, 0, len(rows))
	for _, row := range rows {
		out = append(out, orgmodel.TenantPermission{Slug: row.Slug, Name: row.Name})
	}
	return out, nil
}

// CreateTenantRole creates an organization-specific role.
func (s *Service) CreateTenantRole(ctx context.Context, scope orgctx.Scope, in orgmodel.TenantRoleInput) (orgmodel.TenantRole, error) {
	ve := &ValidationError{}
	name := ""
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	validateRoleName(ve, name)
	key := ""
	if in.Key != nil && strings.TrimSpace(*in.Key) != "" {
		key = strings.TrimSpace(strings.ToLower(*in.Key))
	} else {
		key = strings.ReplaceAll(slug.FromName(name), "-", "_")
	}
	if !roleKeyRE.MatchString(key) {
		ve.add("key", "invalid", "key must be 2-64 characters: lowercase letters, digits and _")
	}
	description := ""
	if in.Description != nil {
		description = strings.TrimSpace(*in.Description)
	}
	validateRoleDescription(ve, description)
	perms := []string{}
	if in.Permissions != nil {
		perms = *in.Permissions
	}
	perms, err := s.validateRolePermissions(ctx, ve, scope, perms)
	if err != nil {
		return orgmodel.TenantRole{}, err
	}
	if err := ve.orNil(); err != nil {
		return orgmodel.TenantRole{}, err
	}
	if taken, err := s.repo.TenantRoleSlugTaken(ctx, scope.ID, key); err != nil {
		return orgmodel.TenantRole{}, err
	} else if taken {
		return orgmodel.TenantRole{}, ErrRoleKeyTaken
	}
	var created uuid.UUID
	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		roleID, roleUUID, err := s.repo.CreateTenantRole(ctx, q, scope.ID, name, key, description)
		if err != nil {
			return err
		}
		created = roleUUID
		return s.repo.SetRolePermissions(ctx, q, roleID, perms)
	})
	if err != nil {
		return orgmodel.TenantRole{}, err
	}
	return s.GetTenantRole(ctx, scope, created)
}

// PatchTenantRole updates an organization-specific role.
func (s *Service) PatchTenantRole(ctx context.Context, scope orgctx.Scope, id uuid.UUID, in orgmodel.TenantRoleInput) (orgmodel.TenantRole, error) {
	current, err := s.GetTenantRole(ctx, scope, id)
	if err != nil {
		return orgmodel.TenantRole{}, err
	}
	if current.IsTemplate {
		return orgmodel.TenantRole{}, ErrRoleReadOnly
	}
	ve := &ValidationError{}
	if in.Key != nil && strings.TrimSpace(*in.Key) != current.Key {
		ve.add("key", "immutable", "key cannot be changed")
	}
	name, description := current.Name, current.Description
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		validateRoleName(ve, name)
	}
	if in.Description != nil {
		description = strings.TrimSpace(*in.Description)
		validateRoleDescription(ve, description)
	}
	var perms []string
	if in.Permissions != nil {
		if perms, err = s.validateRolePermissions(ctx, ve, scope, *in.Permissions); err != nil {
			return orgmodel.TenantRole{}, err
		}
		// Removing permissions the caller does not hold would also be an
		// escalation path in reverse (locking others out), so non-owners may
		// only touch permissions they hold.
		if !isOwnerScope(scope) {
			for _, p := range current.Permissions {
				if !slices.Contains(perms, p) && !scope.Has(p) {
					ve.add("permissions", "forbidden", "cannot remove permission "+p+" you do not hold")
				}
			}
		}
	}
	if err := ve.orNil(); err != nil {
		return orgmodel.TenantRole{}, err
	}
	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		roleID, err := s.repo.UpdateTenantRole(ctx, q, scope.ID, id, name, description)
		if err != nil {
			return mapRepoNotFound(err, ErrRoleNotFound)
		}
		if in.Permissions != nil {
			return s.repo.SetRolePermissions(ctx, q, roleID, perms)
		}
		return nil
	})
	if err != nil {
		return orgmodel.TenantRole{}, err
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, scope.ID)
	return s.GetTenantRole(ctx, scope, id)
}

// DeleteTenantRole deletes an organization-specific role nobody holds.
func (s *Service) DeleteTenantRole(ctx context.Context, scope orgctx.Scope, id uuid.UUID) error {
	current, err := s.GetTenantRole(ctx, scope, id)
	if err != nil {
		return err
	}
	if current.IsTemplate {
		return ErrRoleReadOnly
	}
	if current.MemberCount > 0 {
		return ErrRoleInUse
	}
	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		return s.repo.DeleteTenantRole(ctx, q, scope.ID, id)
	})
	switch {
	case errors.Is(err, repository.ErrInUse):
		return ErrRoleInUse
	case err != nil:
		return mapRepoNotFound(err, ErrRoleNotFound)
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, scope.ID)
	return nil
}

func (s *Service) validateRolePermissions(ctx context.Context, ve *ValidationError, scope orgctx.Scope, in []string) ([]string, error) {
	catalog, err := s.repo.ListTenantPermissions(ctx)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, p := range catalog {
		known[p.Slug] = true
	}
	out := []string{}
	owner := isOwnerScope(scope)
	for i, raw := range in {
		p := strings.TrimSpace(raw)
		switch {
		case !known[p]:
			ve.add(fmt.Sprintf("permissions.%d", i), "unknown", "unknown tenant permission "+p)
		case !owner && !scope.Has(p):
			ve.add(fmt.Sprintf("permissions.%d", i), "forbidden", "cannot grant permission "+p+" you do not hold")
		case !slices.Contains(out, p):
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out, nil
}

func validateRoleName(ve *ValidationError, name string) {
	switch {
	case name == "":
		ve.add("name", "required", "name is required")
	case utf8.RuneCountInString(name) > 100:
		ve.add("name", "too_long", "name must be at most 100 characters")
	}
}

func validateRoleDescription(ve *ValidationError, v string) {
	if utf8.RuneCountInString(v) > 500 {
		ve.add("description", "too_long", "description must be at most 500 characters")
	}
}

func mapTenantRole(row repository.TenantRoleRow, perms []string) orgmodel.TenantRole {
	if perms == nil {
		perms = []string{}
	}
	return orgmodel.TenantRole{
		ID: row.ID, UUID: row.Uuid, Key: row.Slug, Name: row.Name, Description: textOrEmpty(row.Description),
		IsTemplate: row.IsTemplate, MemberCount: row.MemberCount, Permissions: perms,
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
}

func textOrEmpty(v pgtype.Text) string {
	if !v.Valid {
		return ""
	}
	return v.String
}
