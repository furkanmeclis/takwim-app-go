package usecase

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type captureNotifier struct {
	mu    sync.Mutex
	links []string
	vars  []map[string]string
}

func (c *captureNotifier) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.links = append(c.links, in.TemplateVars["link"])
	c.vars = append(c.vars, in.TemplateVars)
	return nil, nil
}

func (c *captureNotifier) lastToken(t *testing.T) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.links) == 0 {
		t.Fatal("no invitation e-mail was sent")
	}
	link := c.links[len(c.links)-1]
	i := strings.LastIndex(link, "/invite/")
	if i < 0 {
		t.Fatalf("link %q has no /invite/ token", link)
	}
	return link[i+len("/invite/"):]
}

type countingLimiter struct {
	limit int
	hits  map[string]int
}

func (l *countingLimiter) Allow(_ context.Context, action, subject string, _ int, _ time.Duration) (bool, time.Duration) {
	if l.hits == nil {
		l.hits = map[string]int{}
	}
	l.hits[action+"|"+subject]++
	return l.hits[action+"|"+subject] <= l.limit, time.Minute
}

type dbAccounts struct {
	q   *db.Queries
	ids *[]int64
}

func (a dbAccounts) RegisterInvited(ctx context.Context, email, name, surname, _ string) (int64, error) {
	u, err := a.q.CreateUser(ctx, db.CreateUserParams{Email: email, PasswordHash: "x", Name: name, Surname: surname, Status: "active"})
	if err != nil {
		return 0, err
	}
	*a.ids = append(*a.ids, u.ID)
	return u.ID, nil
}

// TestInvitationsLifecycle covers TEC-671 against the real schema: create,
// e-mail with the token, accept, reuse, e-mail mismatch, expiry, revoke,
// resend (old link dies), re-invite, grant limits, rate limit, registration
// through the invitation, and the platform owner invitation of a new
// organization (TEC-670, default branch seeded). Runs with TEST_DATABASE_URL.
func TestInvitationsLifecycle(t *testing.T) {
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
	userIDs := []int64{}
	orgIDs := []int64{}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", orgIDs)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", userIDs)
	})
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-inv-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, u.ID)
		return u
	}
	owner, admin, desk := newUser("owner"), newUser("admin"), newUser("desk")
	repo := repository.NewPostgres(pool, q)
	inv := &fakeInvalidator{}
	svc := New(repo, branchusecase.NewOrganizationHooks(nil))
	svc.SetInvalidator(inv)
	notifier := &captureNotifier{}
	limiter := &countingLimiter{limit: 100}
	mgmt := NewManagement(svc, repo, ManagementConfig{FrontendURL: "https://app.example.test/"})
	mgmt.SetNotifier(notifier)
	mgmt.SetRateLimiter(limiter)
	mgmt.SetAccounts(dbAccounts{q: q, ids: &userIDs})

	org, err := svc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Davet " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	orgIDs = append(orgIDs, org.ID)
	ownerScope, err := svc.Resolve(ctx, org.Slug, owner.ID)
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
	branches, err := mgmt.PlatformBranchOptions(ctx, org.ID)
	if err != nil || len(branches) != 1 || branches[0].Code != "MRK" {
		t.Fatalf("default branch = %+v err %v", branches, err)
	}
	merkez := branches[0].UUID
	actor := Actor{UserID: owner.ID, Name: "Owner Test"}

	// Create: e-mail sent with the raw token, only the hash is stored.
	staffEmail := "Personel-" + suffix + "@Example.test"
	created, err := mgmt.CreateInvitation(ctx, ownerScope, actor, orgmodel.CreateInvitationInput{
		Email: staffEmail, RoleUUIDs: []uuid.UUID{roleUUID["resepsiyon"], roleUUID["sube_yoneticisi"]},
		BranchScope: orgctx.BranchScopeSelected, BranchUUIDs: []uuid.UUID{merkez},
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	if created.Email != strings.ToLower(staffEmail) || created.Status != orgmodel.InvitationPending || len(created.Roles) != 2 ||
		len(created.Branches) != 1 || created.InvitedBy == nil || created.InvitedBy.UUID != owner.Uuid {
		t.Fatalf("created = %+v", created)
	}
	if d := time.Until(created.ExpiresAt); d < 167*time.Hour || d > 169*time.Hour {
		t.Fatalf("expiry in %s, want 7 days", d)
	}
	token := notifier.lastToken(t)
	if !strings.HasPrefix(notifier.links[0], "https://app.example.test/invite/") || len(token) != 43 {
		t.Fatalf("link %q token %q", notifier.links[0], token)
	}
	var stored string
	if err := pool.QueryRow(ctx, "SELECT token_hash FROM organization_invitations WHERE uuid = $1", created.UUID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != HashInvitationToken(token) || strings.Contains(stored, token) {
		t.Fatal("token must be stored as its sha256 hash only")
	}
	if notifier.vars[0]["organization"] != org.Name || notifier.vars[0]["inviter"] != "Owner Test" {
		t.Fatalf("template vars = %v", notifier.vars[0])
	}

	// Public preview: masked e-mail, no address leak.
	preview, err := mgmt.PreviewInvitation(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Organization.Name != org.Name || preview.InviterName != "owner Test" || strings.Contains(preview.EmailMasked, "ersonel") ||
		preview.Status != orgmodel.InvitationPending {
		t.Fatalf("preview = %+v", preview)
	}
	if _, err := mgmt.PreviewInvitation(ctx, "not-a-token"); !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("unknown token err = %v", err)
	}

	// Wrong account e-mail.
	if _, err := mgmt.AcceptInvitation(ctx, token, desk.ID); !errors.Is(err, ErrInvitationEmailMismatch) {
		t.Fatalf("mismatch err = %v", err)
	}

	// Registration through the invitation: account for the invited e-mail,
	// membership with the invited roles and branch.
	res, err := mgmt.RegisterAndAccept(ctx, token, orgmodel.RegisterInvitedInput{Name: "Ayşe", Surname: "Kaya", Password: "s3cret-pass"})
	if err != nil {
		t.Fatalf("register and accept: %v", err)
	}
	if res.OrganizationSlug != org.Slug || res.AlreadyMember || res.Email != strings.ToLower(staffEmail) {
		t.Fatalf("accept result = %+v", res)
	}
	staff, err := q.GetUserByEmail(ctx, strings.ToLower(staffEmail))
	if err != nil {
		t.Fatal(err)
	}
	staffScope, err := svc.Resolve(ctx, org.Slug, staff.ID)
	if err != nil {
		t.Fatalf("invitee is not a member: %v", err)
	}
	if staffScope.AllBranches() || len(staffScope.BranchIDs) != 1 || !staffScope.Has("tenant.finance.read") {
		t.Fatalf("invitee scope = %+v", staffScope)
	}
	// Single use.
	if _, err := mgmt.AcceptInvitation(ctx, token, staff.ID); !errors.Is(err, ErrInvitationUsed) {
		t.Fatalf("reuse err = %v", err)
	}
	if p, _ := mgmt.PreviewInvitation(ctx, token); p.Status != orgmodel.InvitationAccepted {
		t.Fatalf("preview after accept = %+v", p)
	}
	// Inviting a member again is refused.
	if _, err := mgmt.CreateInvitation(ctx, ownerScope, actor, orgmodel.CreateInvitationInput{
		Email: staff.Email, RoleUUIDs: []uuid.UUID{roleUUID["member"]},
	}); !errors.Is(err, ErrAlreadyMember) {
		t.Fatalf("invite member err = %v", err)
	}

	// Expiry, case-insensitive match for an existing account.
	adminInv, err := mgmt.CreateInvitation(ctx, ownerScope, actor, orgmodel.CreateInvitationInput{
		Email: strings.ToUpper(admin.Email), RoleUUIDs: []uuid.UUID{roleUUID["admin"]},
	})
	if err != nil {
		t.Fatal(err)
	}
	adminToken := notifier.lastToken(t)
	mgmt.now = func() time.Time { return time.Now().Add(8 * 24 * time.Hour) }
	if _, err := mgmt.AcceptInvitation(ctx, adminToken, admin.ID); !errors.Is(err, ErrInvitationExpired) {
		t.Fatalf("expired err = %v", err)
	}
	mgmt.now = time.Now
	// Resend rotates the token: the old link dies, the new one works.
	if _, err := mgmt.ResendInvitation(ctx, ownerScope, actor, adminInv.UUID); err != nil {
		t.Fatalf("resend: %v", err)
	}
	newAdminToken := notifier.lastToken(t)
	if newAdminToken == adminToken {
		t.Fatal("resend must issue a new token")
	}
	if _, err := mgmt.AcceptInvitation(ctx, adminToken, admin.ID); !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("old token after resend err = %v", err)
	}
	if _, err := mgmt.AcceptInvitation(ctx, newAdminToken, admin.ID); err != nil {
		t.Fatalf("accept resent: %v", err)
	}
	adminScope, err := svc.Resolve(ctx, org.Slug, admin.ID)
	if err != nil || !adminScope.AllBranches() || adminScope.MemberRole != "admin" {
		t.Fatalf("admin scope = %+v err %v", adminScope, err)
	}

	// Revoke; re-invite revokes the previous open invitation.
	deskInv, err := mgmt.CreateInvitation(ctx, ownerScope, actor, orgmodel.CreateInvitationInput{
		Email: desk.Email, RoleUUIDs: []uuid.UUID{roleUUID["member"]},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstDeskToken := notifier.lastToken(t)
	deskInv2, err := mgmt.CreateInvitation(ctx, ownerScope, actor, orgmodel.CreateInvitationInput{
		Email: desk.Email, RoleUUIDs: []uuid.UUID{roleUUID["resepsiyon"]},
	})
	if err != nil {
		t.Fatalf("re-invite: %v", err)
	}
	if again, _ := mgmt.GetInvitation(ctx, org.ID, deskInv.UUID); again.Status != orgmodel.InvitationRevoked {
		t.Fatalf("re-invite must revoke the previous one: %+v", again)
	}
	if _, err := mgmt.AcceptInvitation(ctx, firstDeskToken, desk.ID); !errors.Is(err, ErrInvitationRevoked) {
		t.Fatalf("revoked (replaced) err = %v", err)
	}
	secondDeskToken := notifier.lastToken(t)
	if _, err := mgmt.RevokeInvitation(ctx, ownerScope, actor, deskInv2.UUID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := mgmt.RevokeInvitation(ctx, ownerScope, actor, deskInv2.UUID); !errors.Is(err, ErrConflict) {
		t.Fatalf("revoke twice err = %v", err)
	}
	if _, err := mgmt.AcceptInvitation(ctx, secondDeskToken, desk.ID); !errors.Is(err, ErrInvitationRevoked) {
		t.Fatalf("revoked err = %v", err)
	}
	list, total, err := mgmt.ListInvitations(ctx, org.ID, orgmodel.InvitationListFilter{Limit: 50, Statuses: []string{orgmodel.InvitationRevoked}})
	if err != nil || total != 2 || len(list) != 2 {
		t.Fatalf("revoked list = %d/%d err %v", len(list), total, err)
	}
	// Another organization's invitation is not found.
	if _, err := mgmt.GetInvitation(ctx, org.ID+100000, deskInv2.UUID); !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("cross-org get err = %v", err)
	}

	// Grant limits: admin cannot invite owners or roles with permissions it
	// lacks; a branch-restricted member cannot grant every branch.
	if _, err := mgmt.CreateInvitation(ctx, adminScope, actor, orgmodel.CreateInvitationInput{
		Email: "x-" + suffix + "@example.test", RoleUUIDs: []uuid.UUID{roleUUID["owner"]},
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("admin invites owner err = %v", err)
	}
	roleAdminName, rolePerms := "Rol Yöneticisi", []string{"tenant.roles.read", "tenant.roles.write"}
	roleAdmin, err := svc.CreateTenantRole(ctx, ownerScope, orgmodel.TenantRoleInput{Name: &roleAdminName, Permissions: &rolePerms})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgmt.CreateInvitation(ctx, adminScope, actor, orgmodel.CreateInvitationInput{
		Email: "y-" + suffix + "@example.test", RoleUUIDs: []uuid.UUID{roleAdmin.UUID},
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("admin invites a role with tenant.roles.write err = %v", err)
	}
	if _, err := mgmt.CreateInvitation(ctx, staffScope, actor, orgmodel.CreateInvitationInput{
		Email: "z-" + suffix + "@example.test", RoleUUIDs: []uuid.UUID{roleUUID["member"]},
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("restricted member grants all branches err = %v", err)
	}
	var ve *ValidationError
	if _, err := mgmt.CreateInvitation(ctx, ownerScope, actor, orgmodel.CreateInvitationInput{
		Email: "not-an-email", RoleUUIDs: nil,
	}); !errors.As(err, &ve) || len(ve.Details) != 2 {
		t.Fatalf("validation err = %v", err)
	}

	// Rate limit per organization.
	limiter.limit = 0
	if _, err := mgmt.CreateInvitation(ctx, ownerScope, actor, orgmodel.CreateInvitationInput{
		Email: "r-" + suffix + "@example.test", RoleUUIDs: []uuid.UUID{roleUUID["member"]},
	}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("rate limit err = %v", err)
	}
	limiter.limit = 1000

	// Platform: new organization with an owner invitation.
	ownerEmail := "ides-owner-" + suffix + "@example.test"
	ides, ownerInv, err := mgmt.CreateOrganizationWithOwnerInvitation(ctx, orgmodel.CreateOrganizationInput{Name: "İdes Car Rental " + suffix}, ownerEmail, Actor{UserID: owner.ID, Name: "Platform"})
	if err != nil {
		t.Fatalf("create with owner invitation: %v", err)
	}
	orgIDs = append(orgIDs, ides.ID)
	if !strings.HasPrefix(ides.Slug, "ides-car-rental-") {
		t.Fatalf("slug = %q (Turkish folding)", ides.Slug)
	}
	if len(ownerInv.Roles) != 1 || ownerInv.Roles[0].Key != orgctx.RoleOwner {
		t.Fatalf("owner invitation = %+v", ownerInv)
	}
	idesBranches, _ := mgmt.PlatformBranchOptions(ctx, ides.ID)
	if len(idesBranches) != 1 || idesBranches[0].Name != "Merkez" {
		t.Fatalf("OnCreated must seed the default branch: %+v", idesBranches)
	}
	summary, err := svc.GetOrganizationSummary(ctx, ides.UUID)
	if err != nil || summary.MemberCount != 0 || summary.Owner != nil || summary.PendingInvitationCount != 1 || summary.BranchCount != 1 {
		t.Fatalf("summary before accept = %+v err %v", summary, err)
	}
	newOwner := newUser("ides-owner")
	if _, err := pool.Exec(ctx, "UPDATE users SET email = $1 WHERE id = $2", ownerEmail, newOwner.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := mgmt.AcceptInvitation(ctx, notifier.lastToken(t), newOwner.ID); err != nil {
		t.Fatalf("owner accept: %v", err)
	}
	summary, _ = svc.GetOrganizationSummary(ctx, ides.UUID)
	if summary.Owner == nil || summary.Owner.UUID != newOwner.Uuid || summary.MemberCount != 1 {
		t.Fatalf("summary after accept = %+v", summary)
	}

	// Platform member operations: add with roles + branches, patch, last owner.
	idesMember, err := svc.PlatformAddMember(ctx, ides.UUID, orgmodel.PlatformMemberInput{
		UserUUID: desk.Uuid, RoleUUIDs: &[]uuid.UUID{roleUUID["resepsiyon"]},
		BranchScope: ptr(orgctx.BranchScopeSelected), BranchUUIDs: &[]uuid.UUID{idesBranches[0].UUID},
	})
	if err != nil {
		t.Fatalf("platform add member: %v", err)
	}
	if idesMember.BranchScope != orgctx.BranchScopeSelected || len(idesMember.Branches) != 1 || idesMember.Roles[0].Key != "resepsiyon" {
		t.Fatalf("platform member = %+v", idesMember)
	}
	if _, err := svc.PlatformAddMember(ctx, ides.UUID, orgmodel.PlatformMemberInput{UserUUID: desk.Uuid, RoleKey: "member"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate member err = %v", err)
	}
	if _, err := svc.PlatformPatchMember(ctx, ides.UUID, newOwner.Uuid, orgmodel.PlatformMemberInput{
		RoleUUIDs: &[]uuid.UUID{roleUUID["admin"]},
	}); !errors.Is(err, ErrLastOrganizationOwner) {
		t.Fatalf("platform demote last owner err = %v", err)
	}
	if err := svc.RemoveMember(ctx, ides.UUID, newOwner.Uuid); !errors.Is(err, ErrLastOrganizationOwner) {
		t.Fatalf("platform remove last owner err = %v", err)
	}
	if _, err := svc.PlatformPatchMember(ctx, ides.UUID, desk.Uuid, orgmodel.PlatformMemberInput{
		RoleUUIDs: &[]uuid.UUID{roleUUID["owner"]}, BranchScope: ptr(orgctx.BranchScopeAll),
	}); err != nil {
		t.Fatalf("platform promote: %v", err)
	}
	if err := svc.RemoveMember(ctx, ides.UUID, newOwner.Uuid); err != nil {
		t.Fatalf("remove one of two owners: %v", err)
	}
	// List filters: by access window and status.
	items, _, err := svc.ListOrganizations(ctx, orgmodel.ListFilter{Limit: 100, Q: suffix, Windows: []string{orgmodel.WindowUnlimited}, Statuses: []string{"active"}})
	if err != nil || len(items) != 2 {
		t.Fatalf("filtered list = %d err %v", len(items), err)
	}
	items, _, _ = svc.ListOrganizations(ctx, orgmodel.ListFilter{Limit: 100, Q: suffix, AccessStates: []string{orgmodel.AccessExpired}})
	if len(items) != 0 {
		t.Fatalf("expired list = %d", len(items))
	}
}

func ptr[T any](v T) *T { return &v }
