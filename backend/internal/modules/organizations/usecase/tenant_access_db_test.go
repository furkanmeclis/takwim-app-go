package usecase

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestTenantAccessMultiRoleAndGuards covers TEC-672 / TEC-689 against the
// real schema: permissions are the union over a member's roles, role and
// branch changes invalidate the cache, and the member/role guards hold.
// Runs when TEST_DATABASE_URL points at a migrated database.
func TestTenantAccessMultiRoleAndGuards(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	suffix := uuid.NewString()[:8]
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	owner, admin, carol := newUser("owner"), newUser("admin"), newUser("carol")
	inv := &fakeInvalidator{}
	svc := New(repository.NewPostgres(pool, q), nil)
	svc.SetInvalidator(inv)
	org, err := svc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Access " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{owner.ID, admin.ID, carol.ID})
	})
	if _, err := svc.AddMember(ctx, org.UUID, admin.Uuid, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddMember(ctx, org.UUID, carol.Uuid, "rezervasyon_satis"); err != nil {
		t.Fatal(err)
	}
	ownerScope, err := svc.Resolve(ctx, org.Slug, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	adminScope, err := svc.Resolve(ctx, org.Slug, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	roles, err := svc.ListTenantRoles(ctx, ownerScope)
	if err != nil {
		t.Fatal(err)
	}
	roleUUID := map[string]uuid.UUID{}
	for _, r := range roles {
		roleUUID[r.Key] = r.UUID
	}
	for _, key := range []string{"owner", "admin", "member", "merkez_yoneticisi", "sube_yoneticisi", "rezervasyon_satis", "muhasebe", "teslim_iade"} {
		if roleUUID[key] == uuid.Nil {
			t.Fatalf("template role %s missing from %v", key, roleUUID)
		}
	}

	// Multi-role: rezervasyon_satis + muhasebe = union of both.
	carolScope, _ := svc.Resolve(ctx, org.Slug, carol.ID)
	if carolScope.Has("tenant.payment.refund") {
		t.Fatal("rezervasyon_satis alone must not refund")
	}
	before := len(inv.ids)
	m, err := svc.PatchTenantMember(ctx, ownerScope, carol.Uuid, orgmodel.PatchTenantMemberInput{
		RoleUUIDs: &[]uuid.UUID{roleUUID["rezervasyon_satis"], roleUUID["muhasebe"]},
	})
	if err != nil {
		t.Fatalf("patch roles: %v", err)
	}
	if len(m.Roles) != 2 {
		t.Fatalf("carol roles = %v", m.Roles)
	}
	if len(inv.ids) == before {
		t.Fatal("role change must invalidate the organization cache")
	}
	carolScope, err = svc.Resolve(ctx, org.Slug, carol.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"tenant.payment.refund", "tenant.discount.apply", "tenant.report.export"} {
		if !carolScope.Has(p) {
			t.Fatalf("union missing %s: %v", p, carolScope.Permissions)
		}
	}
	if !slices.Contains(carolScope.MemberRoles, "muhasebe") || carolScope.MemberRole != "muhasebe" {
		t.Fatalf("roles = %v primary %q", carolScope.MemberRoles, carolScope.MemberRole)
	}
	if !carolScope.AllBranches() {
		t.Fatal("new members see every branch")
	}

	// Branch scope change invalidates too, and lands in the scope.
	var branchID int64
	var branchUUID uuid.UUID
	if err := pool.QueryRow(ctx,
		"INSERT INTO branches (organization_id, code, name) VALUES ($1, 'MRK', 'Merkez') RETURNING id, uuid", org.ID,
	).Scan(&branchID, &branchUUID); err != nil {
		t.Fatal(err)
	}
	before = len(inv.ids)
	selected := orgctx.BranchScopeSelected
	if _, err := svc.PatchTenantMember(ctx, ownerScope, carol.Uuid, orgmodel.PatchTenantMemberInput{
		BranchScope: &selected, BranchUUIDs: &[]uuid.UUID{branchUUID},
	}); err != nil {
		t.Fatalf("patch branches: %v", err)
	}
	if len(inv.ids) == before {
		t.Fatal("branch change must invalidate the organization cache")
	}
	carolScope, _ = svc.Resolve(ctx, org.Slug, carol.ID)
	if carolScope.AllBranches() || !carolScope.AllowsBranch(branchID) || carolScope.AllowsBranch(branchID+100000) {
		t.Fatalf("carol branch scope = %v", carolScope.BranchIDs)
	}
	// Unknown branch uuid is a validation error.
	var ve *ValidationError
	if _, err := svc.PatchTenantMember(ctx, ownerScope, carol.Uuid, orgmodel.PatchTenantMemberInput{
		BranchScope: &selected, BranchUUIDs: &[]uuid.UUID{uuid.New()},
	}); !errors.As(err, &ve) {
		t.Fatalf("unknown branch err = %v", err)
	}

	// Guards: a non-owner cannot grant owner, and cannot grant what it lacks.
	if _, err := svc.PatchTenantMember(ctx, adminScope, carol.Uuid, orgmodel.PatchTenantMemberInput{
		RoleUUIDs: &[]uuid.UUID{roleUUID["owner"]},
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("admin grants owner err = %v", err)
	}
	if _, err := svc.PatchTenantMember(ctx, adminScope, carol.Uuid, orgmodel.PatchTenantMemberInput{
		RoleUUIDs: &[]uuid.UUID{roleUUID["merkez_yoneticisi"]},
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("admin grants merkez_yoneticisi (has tenant.roles.write) err = %v", err)
	}
	// A branch-restricted caller cannot manage an all-branch member.
	if _, err := svc.PatchTenantMember(ctx, carolScope, admin.Uuid, orgmodel.PatchTenantMemberInput{
		RoleUUIDs: &[]uuid.UUID{roleUUID["member"]},
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("restricted caller err = %v", err)
	}
	// Last owner stays.
	if _, err := svc.PatchTenantMember(ctx, ownerScope, owner.Uuid, orgmodel.PatchTenantMemberInput{
		RoleUUIDs: &[]uuid.UUID{roleUUID["admin"]},
	}); !errors.Is(err, ErrLastOrganizationOwner) {
		t.Fatalf("demote last owner err = %v", err)
	}
	if err := svc.RemoveTenantMember(ctx, ownerScope, owner.Uuid); !errors.Is(err, ErrLastOrganizationOwner) {
		t.Fatalf("remove last owner err = %v", err)
	}

	// Organization roles: create with a permission matrix, template read-only,
	// in-use delete refused, unused delete works.
	name := "Gece Vardiyası"
	perms := []string{"tenant.branches.read", "tenant.payment.collect"}
	custom, err := svc.CreateTenantRole(ctx, ownerScope, orgmodel.TenantRoleInput{Name: &name, Permissions: &perms})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if custom.IsTemplate || custom.Key == "" || len(custom.Permissions) != 2 {
		t.Fatalf("custom role = %+v", custom)
	}
	if _, err := svc.CreateTenantRole(ctx, ownerScope, orgmodel.TenantRoleInput{Name: &name, Permissions: &perms}); !errors.Is(err, ErrRoleKeyTaken) {
		t.Fatalf("duplicate key err = %v", err)
	}
	badPerms := []string{"platform.users.read"}
	if _, err := svc.CreateTenantRole(ctx, ownerScope, orgmodel.TenantRoleInput{Name: &name, Permissions: &badPerms}); !errors.As(err, &ve) {
		t.Fatalf("platform permission err = %v", err)
	}
	if err := svc.DeleteTenantRole(ctx, ownerScope, roleUUID["muhasebe"]); !errors.Is(err, ErrRoleReadOnly) {
		t.Fatalf("delete template err = %v", err)
	}
	if _, err := svc.PatchTenantMember(ctx, ownerScope, carol.Uuid, orgmodel.PatchTenantMemberInput{
		RoleUUIDs: &[]uuid.UUID{custom.UUID},
	}); err != nil {
		t.Fatalf("assign custom role: %v", err)
	}
	carolScope, _ = svc.Resolve(ctx, org.Slug, carol.ID)
	if !carolScope.Has("tenant.payment.collect") || carolScope.Has("tenant.payment.refund") {
		t.Fatalf("custom role permissions = %v", carolScope.Permissions)
	}
	if err := svc.DeleteTenantRole(ctx, ownerScope, custom.UUID); !errors.Is(err, ErrRoleInUse) {
		t.Fatalf("delete in-use role err = %v", err)
	}
	// Editing the custom role's permissions changes the member's scope.
	more := []string{"tenant.branches.read", "tenant.payment.collect", "tenant.payment.refund"}
	if _, err := svc.PatchTenantRole(ctx, ownerScope, custom.UUID, orgmodel.TenantRoleInput{Permissions: &more}); err != nil {
		t.Fatal(err)
	}
	carolScope, _ = svc.Resolve(ctx, org.Slug, carol.ID)
	if !carolScope.Has("tenant.payment.refund") {
		t.Fatalf("patched role permissions = %v", carolScope.Permissions)
	}
	if _, err := svc.PatchTenantMember(ctx, ownerScope, carol.Uuid, orgmodel.PatchTenantMemberInput{
		RoleUUIDs: &[]uuid.UUID{roleUUID["member"]},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteTenantRole(ctx, ownerScope, custom.UUID); err != nil {
		t.Fatalf("delete unused role: %v", err)
	}

	// Removing a member works and the membership is gone.
	if err := svc.RemoveTenantMember(ctx, ownerScope, carol.Uuid); err != nil {
		t.Fatalf("remove carol: %v", err)
	}
	if _, err := svc.Resolve(ctx, org.Slug, carol.ID); !errors.Is(err, orgctx.ErrNotMember) {
		t.Fatalf("removed member resolve err = %v", err)
	}
}
