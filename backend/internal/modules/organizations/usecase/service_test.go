package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestResolveOrgctxErrorsAndPerOrgPermissions(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	f := newFakeRepo(now)
	userID := int64(10)
	orgA := f.addOrg("acme", "active", now.Add(-time.Hour), nil)
	orgB := f.addOrg("beta", "active", now.Add(-time.Hour), nil)
	suspended := f.addOrg("sleepy", "suspended", now.Add(-time.Hour), nil)
	future := f.addOrg("future", "active", now.Add(time.Hour), nil)
	pastEnd := now.Add(-time.Minute)
	expired := f.addOrg("expired", "active", now.Add(-time.Hour), &pastEnd)
	f.addMembership(orgA.ID, userID, "owner")
	f.addMembership(orgB.ID, userID, "member")
	f.addMembership(suspended.ID, userID, "member")
	f.addMembership(future.ID, userID, "member")
	f.addMembership(expired.ID, userID, "member")
	f.perms["owner"] = []string{"tenant.settings.read", "tenant.roles.write"}
	f.perms["member"] = []string{"tenant.settings.read"}
	svc := New(f, nil)
	svc.now = func() time.Time { return now }

	scopeA, err := svc.Resolve(context.Background(), orgA.Slug, userID)
	if err != nil {
		t.Fatalf("resolve A: %v", err)
	}
	if scopeA.MemberRole != "owner" || !scopeA.Has("tenant.roles.write") {
		t.Fatalf("scope A permissions = role %q perms %v", scopeA.MemberRole, scopeA.Permissions)
	}
	scopeB, err := svc.Resolve(context.Background(), orgB.Uuid.String(), userID)
	if err != nil {
		t.Fatalf("resolve B: %v", err)
	}
	if scopeB.MemberRole != "member" || scopeB.Has("tenant.roles.write") {
		t.Fatalf("scope B permissions = role %q perms %v", scopeB.MemberRole, scopeB.Permissions)
	}
	if _, err := svc.Resolve(context.Background(), orgA.Slug, 999); !errors.Is(err, orgctx.ErrNotMember) {
		t.Fatalf("non-member err = %v", err)
	}
	if _, err := svc.Resolve(context.Background(), "missing", userID); !errors.Is(err, orgctx.ErrOrganizationNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	if _, err := svc.Resolve(context.Background(), suspended.Slug, userID); !errors.Is(err, orgctx.ErrSuspended) {
		t.Fatalf("suspended err = %v", err)
	}
	if _, err := svc.Resolve(context.Background(), future.Slug, userID); !errors.Is(err, orgctx.ErrAccessExpired) {
		t.Fatalf("future err = %v", err)
	}
	if _, err := svc.Resolve(context.Background(), expired.Slug, userID); !errors.Is(err, orgctx.ErrAccessExpired) {
		t.Fatalf("expired err = %v", err)
	}
}

func TestCreateOrganizationAllocatesSlugAndInvalidatesOnMutation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	f := newFakeRepo(now)
	f.users[10] = db.User{ID: 10, Uuid: uuid.New(), Email: "owner@example.test", Name: "Owner", Surname: "User", Status: "active"}
	f.existingSlugs["acme-inc"] = true
	inv := &fakeInvalidator{}
	svc := New(f, nil)
	svc.SetInvalidator(inv)

	org, err := svc.CreateOrganization(context.Background(), orgmodel.CreateOrganizationInput{Name: "Acme Inc", OwnerUserID: 10})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if org.Slug != "acme-inc-2" {
		t.Fatalf("slug = %q", org.Slug)
	}
	if got := f.memberships[org.ID][10]; got != "owner" {
		t.Fatalf("owner role = %q", got)
	}
	_, err = svc.SetStatus(context.Background(), org.UUID, orgctx.StatusSuspended)
	if err != nil {
		t.Fatalf("set status: %v", err)
	}
	if len(inv.ids) != 1 || inv.ids[0] != org.ID {
		t.Fatalf("invalidated ids = %v", inv.ids)
	}
}

func TestLastOwnerGuard(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	f := newFakeRepo(now)
	org := f.addOrg("acme", "active", now.Add(-time.Hour), nil)
	userUUID := uuid.New()
	f.users[10] = db.User{ID: 10, Uuid: userUUID, Email: "owner@example.test", Name: "Owner", Surname: "User", Status: "active"}
	f.addMembership(org.ID, 10, "owner")
	svc := New(f, nil)

	if _, err := svc.ChangeMemberRole(context.Background(), org.Uuid, userUUID, "member"); !errors.Is(err, ErrLastOrganizationOwner) {
		t.Fatalf("change last owner err = %v", err)
	}
	if err := svc.RemoveMember(context.Background(), org.Uuid, userUUID); !errors.Is(err, ErrLastOrganizationOwner) {
		t.Fatalf("remove last owner err = %v", err)
	}
}

func TestAddMemberRejectsExistingMember(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	f := newFakeRepo(now)
	org := f.addOrg("acme", "active", now.Add(-time.Hour), nil)
	userUUID := uuid.New()
	f.users[10] = db.User{ID: 10, Uuid: userUUID, Email: "owner@example.test", Name: "Owner", Surname: "User", Status: "active"}
	f.addMembership(org.ID, 10, "owner")
	svc := New(f, nil)

	if _, err := svc.AddMember(context.Background(), org.Uuid, userUUID, "member"); !errors.Is(err, ErrConflict) {
		t.Fatalf("add existing member err = %v", err)
	}
	if got := f.memberships[org.ID][10]; got != "owner" {
		t.Fatalf("role changed to %q", got)
	}
}

func TestOrganizationSettingsMarshalAsJSONObject(t *testing.T) {
	t.Parallel()
	org := mapOrganization(db.Organization{Settings: []byte(`{"locale":"tr"}`)})
	out, err := json.Marshal(org)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), `"settings":{"locale":"tr"}`) {
		t.Fatalf("settings not emitted as object: %s", out)
	}
}

type fakeRepo struct {
	now           time.Time
	nextOrgID     int64
	orgs          map[int64]db.Organization
	orgByUUID     map[uuid.UUID]int64
	orgBySlug     map[string]int64
	existingSlugs map[string]bool
	users         map[int64]db.User
	userByUUID    map[uuid.UUID]int64
	memberships   map[int64]map[int64]string
	roleIDs       map[string]int64
	perms         map[string][]string
	extraRoles    map[int64][]string
	branches      map[int64][]int64
}

// fakeMemberID derives a stable member id from organization and user.
func fakeMemberID(orgID, userID int64) int64 { return orgID*1000 + userID }

func newFakeRepo(now time.Time) *fakeRepo {
	return &fakeRepo{
		now: now, nextOrgID: 1, orgs: map[int64]db.Organization{}, orgByUUID: map[uuid.UUID]int64{},
		orgBySlug: map[string]int64{}, existingSlugs: map[string]bool{}, users: map[int64]db.User{},
		userByUUID: map[uuid.UUID]int64{}, memberships: map[int64]map[int64]string{},
		roleIDs: map[string]int64{"owner": 1, "admin": 2, "member": 3}, perms: map[string][]string{},
		extraRoles: map[int64][]string{}, branches: map[int64][]int64{},
	}
}

func (f *fakeRepo) addOrg(slug, status string, starts time.Time, ends *time.Time) db.Organization {
	row := db.Organization{
		ID: f.nextOrgID, Uuid: uuid.New(), Slug: slug, Name: slug, Status: status,
		AccessStartsAt: pgtype.Timestamptz{Time: starts, Valid: true},
		CreatedAt:      pgtype.Timestamptz{Time: f.now, Valid: true},
		UpdatedAt:      pgtype.Timestamptz{Time: f.now, Valid: true},
	}
	if ends != nil {
		row.AccessEndsAt = pgtype.Timestamptz{Time: *ends, Valid: true}
	}
	f.nextOrgID++
	f.orgs[row.ID] = row
	f.orgByUUID[row.Uuid] = row.ID
	f.orgBySlug[row.Slug] = row.ID
	f.existingSlugs[row.Slug] = true
	return row
}

func (f *fakeRepo) addMembership(orgID, userID int64, role string) {
	if f.memberships[orgID] == nil {
		f.memberships[orgID] = map[int64]string{}
	}
	f.memberships[orgID][userID] = role
}

func (f *fakeRepo) WithTx(ctx context.Context, fn repository.TxFunc) error {
	return fn(ctx, nil, nil)
}

func (f *fakeRepo) CreateOrganization(_ context.Context, _ *db.Queries, params db.CreateOrganizationParams) (db.Organization, error) {
	row := f.addOrg(params.Slug, "active", f.now, nil)
	row.Name = params.Name
	row.Sector = "hairdresser"
	if params.Sector.Valid {
		row.Sector = params.Sector.String
	}
	f.orgs[row.ID] = row
	return row, nil
}

func (f *fakeRepo) AddOrganizationMember(_ context.Context, _ *db.Queries, params db.AddOrganizationMemberParams) error {
	for key, id := range f.roleIDs {
		if id == params.RoleID {
			f.addMembership(params.OrganizationID, params.UserID, key)
			return nil
		}
	}
	return repository.ErrNotFound
}

func (f *fakeRepo) GetUserByID(_ context.Context, id int64) (db.User, error) {
	u, ok := f.users[id]
	if !ok {
		return db.User{}, repository.ErrNotFound
	}
	f.userByUUID[u.Uuid] = u.ID
	return u, nil
}

func (f *fakeRepo) GetUserByUUID(_ context.Context, id uuid.UUID) (db.User, error) {
	for _, u := range f.users {
		if u.Uuid == id {
			return u, nil
		}
	}
	return db.User{}, repository.ErrNotFound
}

func (f *fakeRepo) GetTenantRoleByKey(_ context.Context, _ *db.Queries, _ int64, key string) (db.Role, error) {
	id, ok := f.roleIDs[key]
	if !ok {
		return db.Role{}, repository.ErrNotFound
	}
	return db.Role{ID: id, Slug: key, Name: key, Scope: "tenant"}, nil
}

func (f *fakeRepo) GetOrganizationByUUID(_ context.Context, id uuid.UUID) (db.Organization, error) {
	if oid, ok := f.orgByUUID[id]; ok {
		return f.orgs[oid], nil
	}
	return db.Organization{}, repository.ErrNotFound
}

func (f *fakeRepo) GetOrganizationByID(_ context.Context, id int64) (db.Organization, error) {
	row, ok := f.orgs[id]
	if !ok {
		return db.Organization{}, repository.ErrNotFound
	}
	return row, nil
}

func (f *fakeRepo) SlugExists(_ context.Context, slug string) (bool, error) {
	return f.existingSlugs[slug], nil
}

func (f *fakeRepo) ListOrganizations(context.Context, db.ListOrganizationsWithStatsParams) ([]db.ListOrganizationsWithStatsRow, int64, error) {
	return nil, 0, nil
}

func (f *fakeRepo) UpdateOrganization(_ context.Context, params db.UpdateOrganizationParams) (db.Organization, error) {
	org, err := f.GetOrganizationByUUID(context.Background(), params.Uuid)
	if err != nil {
		return db.Organization{}, err
	}
	if params.Timezone.Valid {
		org.Timezone = params.Timezone.String
	}
	if params.Currency.Valid {
		org.Currency = params.Currency.String
	}
	if params.Sector.Valid {
		org.Sector = params.Sector.String
	}
	f.orgs[org.ID] = org
	return org, nil
}

func (f *fakeRepo) UpdateOrganizationEntitlements(_ context.Context, params db.UpdateOrganizationEntitlementsParams) (db.Organization, error) {
	org, err := f.GetOrganizationByUUID(context.Background(), params.Uuid)
	if err != nil {
		return db.Organization{}, err
	}
	org.Plan, org.Addons, org.BranchLimit, org.StaffLimit = params.Plan, params.Addons, params.BranchLimit, params.StaffLimit
	f.orgs[org.ID] = org
	return org, nil
}

func (f *fakeRepo) SetOrganizationStatus(_ context.Context, params db.SetOrganizationStatusParams) (db.Organization, error) {
	org, err := f.GetOrganizationByUUID(context.Background(), params.Uuid)
	if err != nil {
		return db.Organization{}, err
	}
	org.Status = params.Status
	f.orgs[org.ID] = org
	return org, nil
}

func (f *fakeRepo) SetOrganizationAccessWindow(context.Context, db.SetOrganizationAccessWindowParams) (db.Organization, error) {
	return db.Organization{}, nil
}

func (f *fakeRepo) SoftDeleteOrganization(context.Context, uuid.UUID) (db.Organization, error) {
	return db.Organization{}, nil
}

func (f *fakeRepo) LockOrganizationRow(context.Context, *db.Queries, int64) error { return nil }

func (f *fakeRepo) GetOrganizationMemberByUserUUID(_ context.Context, orgID int64, userUUID uuid.UUID) (db.GetOrganizationMemberByUserUUIDRow, error) {
	u, err := f.GetUserByUUID(context.Background(), userUUID)
	if err != nil {
		return db.GetOrganizationMemberByUserUUIDRow{}, err
	}
	role, ok := f.memberships[orgID][u.ID]
	if !ok {
		return db.GetOrganizationMemberByUserUUIDRow{}, repository.ErrNotFound
	}
	return db.GetOrganizationMemberByUserUUIDRow{OrganizationID: orgID, UserID: u.ID, UserUuid: u.Uuid, Email: u.Email, Name: u.Name, Surname: u.Surname, Status: u.Status, RoleKey: role, RoleName: role}, nil
}

func (f *fakeRepo) GetOrganizationMember(_ context.Context, _ *db.Queries, orgID, userID int64) (db.GetOrganizationMemberRow, error) {
	role, ok := f.memberships[orgID][userID]
	if !ok {
		return db.GetOrganizationMemberRow{}, repository.ErrNotFound
	}
	return db.GetOrganizationMemberRow{ID: fakeMemberID(orgID, userID), OrganizationID: orgID, UserID: userID, RoleKey: role, RoleName: role}, nil
}

func (f *fakeRepo) ListOrganizationMembers(context.Context, db.ListOrganizationMembersByOrgParams) ([]db.ListOrganizationMembersByOrgRow, int64, error) {
	return nil, 0, nil
}

func (f *fakeRepo) ListOrganizationMembershipsByUser(context.Context, int64) ([]db.ListOrganizationMembershipsByUserRow, error) {
	return nil, nil
}

func (f *fakeRepo) UpdateOrganizationMemberRole(_ context.Context, _ *db.Queries, params db.UpdateOrganizationMemberRoleParams) error {
	for key, id := range f.roleIDs {
		if id == params.RoleID {
			f.memberships[params.OrganizationID][params.UserID] = key
			return nil
		}
	}
	return repository.ErrNotFound
}

func (f *fakeRepo) DeleteOrganizationMember(_ context.Context, _ *db.Queries, orgID, userID int64) error {
	if _, ok := f.memberships[orgID][userID]; !ok {
		return repository.ErrNotFound
	}
	delete(f.memberships[orgID], userID)
	return nil
}

func (f *fakeRepo) CountOrganizationOwners(_ context.Context, _ *db.Queries, orgID int64) (int64, error) {
	var n int64
	for _, role := range f.memberships[orgID] {
		if role == "owner" {
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) ResolveOrganizationScopeBySlug(_ context.Context, slug string, userID int64) (db.ResolveOrganizationScopeBySlugRow, error) {
	oid, ok := f.orgBySlug[slug]
	if !ok {
		return db.ResolveOrganizationScopeBySlugRow{}, repository.ErrNotFound
	}
	return f.resolveRow(oid, userID)
}

func (f *fakeRepo) ResolveOrganizationScopeByUUID(_ context.Context, id uuid.UUID, userID int64) (db.ResolveOrganizationScopeByUUIDRow, error) {
	oid, ok := f.orgByUUID[id]
	if !ok {
		return db.ResolveOrganizationScopeByUUIDRow{}, repository.ErrNotFound
	}
	row, err := f.resolveRow(oid, userID)
	if err != nil {
		return db.ResolveOrganizationScopeByUUIDRow{}, err
	}
	return db.ResolveOrganizationScopeByUUIDRow(row), nil
}

func (f *fakeRepo) resolveRow(oid, userID int64) (db.ResolveOrganizationScopeBySlugRow, error) {
	org := f.orgs[oid]
	role, ok := f.memberships[oid][userID]
	if !ok {
		return db.ResolveOrganizationScopeBySlugRow{}, repository.ErrNotFound
	}
	scope := orgctx.BranchScopeAll
	if _, ok := f.branches[fakeMemberID(oid, userID)]; ok {
		scope = orgctx.BranchScopeSelected
	}
	return db.ResolveOrganizationScopeBySlugRow{
		ID: org.ID, Uuid: org.Uuid, Slug: org.Slug, Name: org.Name, Status: org.Status,
		AccessStartsAt: org.AccessStartsAt, AccessEndsAt: org.AccessEndsAt,
		RoleID: f.roleIDs[role], RoleKey: role, MemberID: fakeMemberID(oid, userID), BranchScope: scope,
	}, nil
}

func (f *fakeRepo) ListTenantRolePermissionSlugsByRoleID(_ context.Context, roleID int64) ([]string, error) {
	for key, id := range f.roleIDs {
		if id == roleID {
			return append([]string(nil), f.perms[key]...), nil
		}
	}
	return nil, repository.ErrNotFound
}

type fakeInvalidator struct {
	ids []int64
}

func (f *fakeInvalidator) InvalidateOrganization(_ context.Context, id int64) error {
	f.ids = append(f.ids, id)
	return nil
}

func (f *fakeRepo) memberRoleKeys(memberID int64) []string {
	orgID, userID := memberID/1000, memberID%1000
	role, ok := f.memberships[orgID][userID]
	if !ok {
		return nil
	}
	return append([]string{role}, f.extraRoles[memberID]...)
}

func (f *fakeRepo) ListMemberPermissionSlugs(_ context.Context, _ int64, memberID int64) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, role := range f.memberRoleKeys(memberID) {
		for _, p := range f.perms[role] {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out, nil
}

func (f *fakeRepo) ListMemberRoles(_ context.Context, _ *db.Queries, _ int64, memberIDs []int64) ([]db.ListMemberRolesRow, error) {
	out := []db.ListMemberRolesRow{}
	for _, id := range memberIDs {
		for _, role := range f.memberRoleKeys(id) {
			out = append(out, db.ListMemberRolesRow{MemberID: id, ID: f.roleIDs[role], Slug: role, Name: role, IsTemplate: true})
		}
	}
	return out, nil
}

func (f *fakeRepo) ListMemberBranchIDs(_ context.Context, _ int64, memberID int64) ([]int64, error) {
	return append([]int64{}, f.branches[memberID]...), nil
}

func (f *fakeRepo) ListMemberBranches(context.Context, int64, []int64) ([]db.ListMemberBranchesRow, error) {
	return nil, nil
}

func (f *fakeRepo) ListBranchIDsByUUIDs(context.Context, int64, []uuid.UUID) ([]db.ListBranchIDsByUUIDsRow, error) {
	return nil, nil
}

func (f *fakeRepo) ReplaceMemberRoles(_ context.Context, _ *db.Queries, orgID, memberID int64, roleIDs []int64, primaryRoleID int64) error {
	userID := memberID % 1000
	extra := []string{}
	for key, id := range f.roleIDs {
		if id == primaryRoleID {
			f.memberships[orgID][userID] = key
		} else if slices.Contains(roleIDs, id) {
			extra = append(extra, key)
		}
	}
	f.extraRoles[memberID] = extra
	return nil
}

func (f *fakeRepo) ReplaceMemberBranches(_ context.Context, _ *db.Queries, _ int64, memberID int64, scope string, branchIDs []int64) error {
	if scope == orgctx.BranchScopeAll {
		delete(f.branches, memberID)
		return nil
	}
	f.branches[memberID] = append([]int64{}, branchIDs...)
	return nil
}

func (f *fakeRepo) ListTenantMembers(context.Context, db.ListTenantMembersParams) ([]db.ListTenantMembersRow, int64, error) {
	return nil, 0, nil
}

func (f *fakeRepo) GetTenantMember(context.Context, int64, uuid.UUID) (db.ListTenantMembersRow, error) {
	return db.ListTenantMembersRow{}, repository.ErrNotFound
}

func (f *fakeRepo) ListTenantRoles(context.Context, int64) ([]repository.TenantRoleRow, error) {
	return nil, nil
}

func (f *fakeRepo) GetTenantRoleByUUID(context.Context, *db.Queries, int64, uuid.UUID) (repository.TenantRoleRow, error) {
	return repository.TenantRoleRow{}, repository.ErrNotFound
}

func (f *fakeRepo) ListRolePermissionSlugs(context.Context, []int64) (map[int64][]string, error) {
	return map[int64][]string{}, nil
}

func (f *fakeRepo) ListTenantPermissions(context.Context) ([]db.ListTenantPermissionsRow, error) {
	return nil, nil
}

func (f *fakeRepo) TenantRoleSlugTaken(context.Context, int64, string) (bool, error) {
	return false, nil
}

func (f *fakeRepo) CreateTenantRole(context.Context, *db.Queries, int64, string, string, string) (int64, uuid.UUID, error) {
	return 0, uuid.Nil, nil
}

func (f *fakeRepo) UpdateTenantRole(context.Context, *db.Queries, int64, uuid.UUID, string, string) (int64, error) {
	return 0, nil
}

func (f *fakeRepo) DeleteTenantRole(context.Context, *db.Queries, int64, uuid.UUID) error { return nil }

func (f *fakeRepo) SetRolePermissions(context.Context, *db.Queries, int64, []string) error {
	return nil
}

var _ Repository = (*fakeRepo)(nil)

func TestPatchOrganizationTimezoneAndCurrency(t *testing.T) {
	t.Parallel()
	f := newFakeRepo(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC))
	f.users[10] = db.User{ID: 10, Uuid: uuid.New(), Email: "owner@example.test", Name: "Owner", Surname: "User", Status: "active"}
	svc := New(f, nil)
	org, err := svc.CreateOrganization(context.Background(), orgmodel.CreateOrganizationInput{Name: "Salon", OwnerUserID: 10})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	tz, cur := "Europe/Berlin", " eur "
	got, err := svc.PatchOrganization(context.Background(), org.UUID, orgmodel.PatchOrganizationInput{Timezone: &tz, Currency: &cur})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if got.Timezone != "Europe/Berlin" || got.Currency != "EUR" {
		t.Fatalf("timezone/currency = %q %q", got.Timezone, got.Currency)
	}
	for _, in := range []orgmodel.PatchOrganizationInput{
		{Timezone: ptr("Mars/Olympus")},
		{Timezone: ptr("")},
		{Currency: ptr("TL")},
	} {
		if _, err := svc.PatchOrganization(context.Background(), org.UUID, in); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("patch %+v err = %v", in, err)
		}
	}
}

func TestOrganizationSector(t *testing.T) {
	t.Parallel()
	f := newFakeRepo(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC))
	f.users[10] = db.User{ID: 10, Uuid: uuid.New(), Email: "owner@example.test", Name: "Owner", Surname: "User", Status: "active"}
	svc := New(f, nil)
	ctx := context.Background()
	def, err := svc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Salon", OwnerUserID: 10})
	if err != nil || def.Sector != "hairdresser" {
		t.Fatalf("default sector = %q, %v", def.Sector, err)
	}
	org, err := svc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Lokanta", Sector: "restaurant", OwnerUserID: 10})
	if err != nil || org.Sector != "restaurant" {
		t.Fatalf("restaurant = %q, %v", org.Sector, err)
	}
	if _, err := svc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Fırın", Sector: "bakery", OwnerUserID: 10}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unknown sector err = %v", err)
	}
	got, err := svc.PatchOrganization(ctx, org.UUID, orgmodel.PatchOrganizationInput{Sector: ptr("car_wash")})
	if err != nil || got.Sector != "car_wash" {
		t.Fatalf("patch sector = %q, %v", got.Sector, err)
	}
	if _, err := svc.PatchOrganization(ctx, org.UUID, orgmodel.PatchOrganizationInput{Sector: ptr("")}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("empty sector err = %v", err)
	}
}

func TestSetEntitlements(t *testing.T) {
	t.Parallel()
	f := newFakeRepo(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC))
	f.users[10] = db.User{ID: 10, Uuid: uuid.New(), Email: "owner@example.test", Name: "Owner", Surname: "User", Status: "active"}
	svc := New(f, nil)
	ctx := context.Background()
	org, err := svc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Salon", OwnerUserID: 10})
	if err != nil {
		t.Fatal(err)
	}
	two := int32(2)
	got, err := svc.SetEntitlements(ctx, org.UUID, orgmodel.SetEntitlementsInput{
		Plan: "growth", Addons: []string{"confirmation", "confirmation"}, BranchLimit: &two,
	})
	if err != nil || got.Plan != "growth" || len(got.Addons) != 1 || got.BranchLimit == nil || *got.BranchLimit != 2 || got.StaffLimit != nil {
		t.Fatalf("set = %+v, %v", got, err)
	}
	zero := int32(0)
	for _, in := range []orgmodel.SetEntitlementsInput{
		{Plan: "gold"},
		{Plan: "starter", Addons: []string{"sms"}},
		{Plan: "starter", StaffLimit: &zero},
	} {
		if _, err := svc.SetEntitlements(ctx, org.UUID, in); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("set %+v err = %v", in, err)
		}
	}
}
