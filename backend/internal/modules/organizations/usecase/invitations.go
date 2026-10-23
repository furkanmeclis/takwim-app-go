package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Staff invitations (TEC-671) and platform organization management helpers
// (TEC-670).
//
// Tokens are 32 random bytes (base64url in the link); only their sha256 hex
// is stored. An invitation is single use, expires after InviteTTL (7 days)
// and is revocable. Accepting needs a signed-in account whose e-mail matches
// the invitation (case-insensitive).

var (
	ErrInvitationNotFound      = errors.New("invitation not found")
	ErrInvitationExpired       = errors.New("invitation has expired")
	ErrInvitationUsed          = errors.New("invitation was already used")
	ErrInvitationRevoked       = errors.New("invitation was revoked")
	ErrInvitationEmailMismatch = errors.New("invitation belongs to another e-mail address")
	ErrAlreadyMember           = errors.New("user is already a member")
	ErrAccountExists           = errors.New("an account with this e-mail already exists")
	ErrRateLimited             = errors.New("too many invitations, try again later")
	ErrRegistrationUnavailable = errors.New("account registration is unavailable")
)

const (
	defaultInviteTTL        = 7 * 24 * time.Hour
	defaultInviteRateLimit  = 30
	defaultInviteRateWindow = time.Hour
	invitationTemplate      = "organizations.invitation"
)

// ManagementRepository persists invitations and the platform management
// views (organization_invitations.sql, organization_management.sql).
type ManagementRepository interface {
	CreateInvitation(ctx context.Context, q *db.Queries, params db.CreateInvitationParams) (db.OrganizationInvitation, error)
	AddInvitationRoles(ctx context.Context, q *db.Queries, orgID, invitationID int64, roleIDs []int64) error
	AddInvitationBranches(ctx context.Context, q *db.Queries, orgID, invitationID int64, branchIDs []int64) error
	RevokeOpenInvitationsForEmail(ctx context.Context, q *db.Queries, orgID int64, email string, by *int64) error
	GetInvitationByUUID(ctx context.Context, orgID int64, id uuid.UUID) (db.OrganizationInvitation, error)
	GetInvitationPreview(ctx context.Context, tokenHash string) (db.GetInvitationPreviewByTokenHashRow, error)
	LockInvitationByTokenHash(ctx context.Context, q *db.Queries, tokenHash string) (db.OrganizationInvitation, error)
	ListInvitations(ctx context.Context, params db.ListInvitationsParams) ([]db.ListInvitationsRow, int64, error)
	ListInvitationRoles(ctx context.Context, q *db.Queries, orgID int64, ids []int64) ([]db.ListInvitationRolesRow, error)
	ListInvitationBranches(ctx context.Context, q *db.Queries, orgID int64, ids []int64) ([]db.ListInvitationBranchesRow, error)
	RevokeInvitation(ctx context.Context, orgID int64, id uuid.UUID, by *int64) (db.OrganizationInvitation, error)
	RotateInvitationToken(ctx context.Context, params db.RotateInvitationTokenParams) (db.OrganizationInvitation, error)
	MarkInvitationAccepted(ctx context.Context, q *db.Queries, orgID, invitationID, userID int64) error
	FindUserByEmail(ctx context.Context, email string) (db.User, error)
	ListOrganizationBranchOptions(ctx context.Context, orgID int64) ([]db.ListOrganizationBranchOptionsRow, error)
	ListOrganizationActivity(ctx context.Context, orgID int64, orgUUID uuid.UUID, limit, offset int32) ([]db.ListPlatformOrganizationActivityRow, int64, error)
	SetOrganizationLogo(ctx context.Context, orgID int64, key *string) (db.Organization, error)
}

// Notifier enqueues the invitation e-mail (notifications usecase).
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// InviteAccounts creates the account of an invitee (auth usecase). The
// account follows the normal password registration rules: unverified
// e-mail, verification e-mail sent.
type InviteAccounts interface {
	RegisterInvited(ctx context.Context, email, name, surname, password string) (int64, error)
}

// RateLimiter is the generic fixed-window limiter (platform/ratelimit).
type RateLimiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// ManagementConfig configures invitations.
type ManagementConfig struct {
	// FrontendURL is PUBLIC_FRONTEND_URL; links go to {FrontendURL}/invite/{token}.
	FrontendURL string
	// InviteTTL is the invitation lifetime (default 7 days).
	InviteTTL time.Duration
	// InviteRateLimit invitations (create + resend) per organization and
	// InviteRateWindow (default 30 per hour).
	InviteRateLimit  int
	InviteRateWindow time.Duration
}

// Actor is the person performing an invitation action.
type Actor struct {
	UserID int64
	Name   string
}

// Management bundles invitations and the platform management views.
type Management struct {
	orgs     *Service
	repo     ManagementRepository
	notifier Notifier
	accounts InviteAccounts
	limiter  RateLimiter
	log      *slog.Logger
	cfg      ManagementConfig
	now      func() time.Time
	newToken func() (string, error)
}

// NewManagement builds the invitation / management service.
func NewManagement(orgs *Service, repo ManagementRepository, cfg ManagementConfig) *Management {
	if cfg.InviteTTL <= 0 {
		cfg.InviteTTL = defaultInviteTTL
	}
	if cfg.InviteRateLimit <= 0 {
		cfg.InviteRateLimit = defaultInviteRateLimit
	}
	if cfg.InviteRateWindow <= 0 {
		cfg.InviteRateWindow = defaultInviteRateWindow
	}
	return &Management{orgs: orgs, repo: repo, cfg: cfg, now: time.Now, newToken: NewInvitationToken, log: slog.Default()}
}

// SetNotifier attaches the e-mail pipeline (nil = no e-mail).
func (m *Management) SetNotifier(n Notifier) { m.notifier = n }

// SetAccounts attaches invitee registration (nil = registration through the
// invitation is unavailable).
func (m *Management) SetAccounts(a InviteAccounts) { m.accounts = a }

// SetRateLimiter attaches the per-organization invitation limit.
func (m *Management) SetRateLimiter(l RateLimiter) { m.limiter = l }

// SetLogger sets the logger for fail-soft e-mail errors.
func (m *Management) SetLogger(l *slog.Logger) {
	if l != nil {
		m.log = l
	}
}

// NewInvitationToken returns 32 random bytes, base64url without padding.
func NewInvitationToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashInvitationToken is the stored form of a token (sha256 hex).
func HashInvitationToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

// MaskEmail hides most of an address: "ayse.kaya@example.com" →
// "a********@e******.com".
func MaskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "***"
	}
	local, domain := email[:at], email[at+1:]
	host, tld := domain, ""
	if dot := strings.LastIndex(domain, "."); dot > 0 {
		host, tld = domain[:dot], domain[dot:]
	}
	mask := func(s string) string {
		n := utf8.RuneCountInString(s)
		if n <= 1 {
			return "*"
		}
		r, _ := utf8.DecodeRuneInString(s)
		return string(r) + strings.Repeat("*", n-1)
	}
	return mask(local) + "@" + mask(host) + tld
}

func normalizeEmail(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" || len(v) > 254 {
		return v, false
	}
	addr, err := mail.ParseAddress(v)
	if err != nil || addr.Address != v || !strings.Contains(v, "@") {
		return v, false
	}
	return v, true
}

// --- create -----------------------------------------------------------------

type invitationGrant struct {
	email       string
	roles       []repository.TenantRoleRow
	branchScope string
	branchIDs   []int64
}

// resolveGrant validates an invitation body against the caller's scope:
// same rules as member updates (owner role only by owners, roles only with
// permissions the caller holds, branches only inside the caller's scope).
func (m *Management) resolveGrant(ctx context.Context, scope orgctx.Scope, in orgmodel.CreateInvitationInput) (invitationGrant, error) {
	ve := &ValidationError{}
	email, ok := normalizeEmail(in.Email)
	if !ok {
		ve.add("email", "invalid", "a valid e-mail address is required")
	}
	roles, err := m.orgs.resolveRoles(ctx, scope.ID, in.RoleUUIDs, "role_uuids", ve)
	if err != nil {
		return invitationGrant{}, err
	}
	branchScope, branchIDs, err := m.orgs.resolveBranchScope(ctx, scope, in.BranchScope, in.BranchUUIDs, ve)
	if err != nil {
		return invitationGrant{}, err
	}
	if err := ve.orNil(); err != nil {
		return invitationGrant{}, err
	}
	if err := m.orgs.checkGrant(ctx, scope, roles); err != nil {
		return invitationGrant{}, err
	}
	return invitationGrant{email: email, roles: roles, branchScope: branchScope, branchIDs: branchIDs}, nil
}

func (m *Management) allow(ctx context.Context, orgID int64) error {
	if m.limiter == nil {
		return nil
	}
	if ok, _ := m.limiter.Allow(ctx, "org_invitations", strconv.FormatInt(orgID, 10), m.cfg.InviteRateLimit, m.cfg.InviteRateWindow); !ok {
		return ErrRateLimited
	}
	return nil
}

// CreateInvitation invites an e-mail into the caller's organization.
func (m *Management) CreateInvitation(ctx context.Context, scope orgctx.Scope, actor Actor, in orgmodel.CreateInvitationInput) (orgmodel.Invitation, error) {
	grant, err := m.resolveGrant(ctx, scope, in)
	if err != nil {
		return orgmodel.Invitation{}, err
	}
	if err := m.ensureNotMember(ctx, scope.ID, grant.email); err != nil {
		return orgmodel.Invitation{}, err
	}
	if err := m.allow(ctx, scope.ID); err != nil {
		return orgmodel.Invitation{}, err
	}
	var (
		row   db.OrganizationInvitation
		token string
	)
	err = m.orgs.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		if err := m.orgs.repo.LockOrganizationRow(ctx, q, scope.ID); err != nil {
			return mapRepoNotFound(err, ErrNotFound)
		}
		if err := m.orgs.checkStaffAdd(ctx, q, scope); err != nil {
			return err
		}
		row, token, err = m.insertInvitation(ctx, q, scope.ID, actor, grant)
		return err
	})
	if err != nil {
		return orgmodel.Invitation{}, err
	}
	inv, err := m.GetInvitation(ctx, scope.ID, row.Uuid)
	if err != nil {
		return orgmodel.Invitation{}, err
	}
	m.sendInvitation(ctx, scope.ID, actor, inv, token)
	return inv, nil
}

// CreatePlatformInvitation invites an e-mail into an organization from the
// platform (operator rights: no grant limits).
func (m *Management) CreatePlatformInvitation(ctx context.Context, orgUUID uuid.UUID, actor Actor, in orgmodel.CreateInvitationInput) (orgmodel.Invitation, error) {
	scope, err := m.orgs.PlatformScope(ctx, orgUUID)
	if err != nil {
		return orgmodel.Invitation{}, err
	}
	return m.CreateInvitation(ctx, scope, actor, in)
}

// CreateOrganizationWithOwnerInvitation creates an organization without
// members and invites ownerEmail as its owner (same transaction; the e-mail
// goes out after commit).
func (m *Management) CreateOrganizationWithOwnerInvitation(ctx context.Context, in orgmodel.CreateOrganizationInput, ownerEmail string, actor Actor) (orgmodel.Organization, orgmodel.Invitation, error) {
	email, ok := normalizeEmail(ownerEmail)
	if !ok {
		return orgmodel.Organization{}, orgmodel.Invitation{}, &ValidationError{Details: []FieldError{{
			Field: "owner_email", Code: "invalid", Message: "a valid e-mail address is required",
		}}}
	}
	in.NoOwner = true
	in.OwnerUserID, in.OwnerUserUUID = 0, uuid.Nil
	var (
		row   db.OrganizationInvitation
		token string
	)
	org, err := m.orgs.createOrganization(ctx, in, func(ctx context.Context, q *db.Queries, created db.Organization) error {
		role, err := m.orgs.repo.GetTenantRoleByKey(ctx, q, created.ID, orgctx.RoleOwner)
		if err != nil {
			return mapRepoNotFound(err, ErrRoleNotFound)
		}
		grant := invitationGrant{
			email: email, branchScope: orgctx.BranchScopeAll,
			roles: []repository.TenantRoleRow{{ID: role.ID, Uuid: role.Uuid, Slug: role.Slug, Name: role.Name, IsTemplate: true}},
		}
		row, token, err = m.insertInvitation(ctx, q, created.ID, actor, grant)
		return err
	})
	if err != nil {
		return orgmodel.Organization{}, orgmodel.Invitation{}, err
	}
	inv, err := m.GetInvitation(ctx, org.ID, row.Uuid)
	if err != nil {
		return orgmodel.Organization{}, orgmodel.Invitation{}, err
	}
	m.sendInvitation(ctx, org.ID, actor, inv, token)
	return org, inv, nil
}

func (m *Management) ensureNotMember(ctx context.Context, orgID int64, email string) error {
	user, err := m.repo.FindUserByEmail(ctx, email)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := m.orgs.repo.GetOrganizationMemberByUserUUID(ctx, orgID, user.Uuid); err == nil {
		return ErrAlreadyMember
	} else if !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	return nil
}

func (m *Management) insertInvitation(ctx context.Context, q *db.Queries, orgID int64, actor Actor, grant invitationGrant) (db.OrganizationInvitation, string, error) {
	token, err := m.newToken()
	if err != nil {
		return db.OrganizationInvitation{}, "", err
	}
	var by *int64
	if actor.UserID > 0 {
		by = &actor.UserID
	}
	// Re-inviting replaces the e-mail's open invitation (one open per e-mail).
	if err := m.repo.RevokeOpenInvitationsForEmail(ctx, q, orgID, grant.email, by); err != nil {
		return db.OrganizationInvitation{}, "", err
	}
	primary := primaryRole(grant.roles)
	row, err := m.repo.CreateInvitation(ctx, q, db.CreateInvitationParams{
		OrganizationID: orgID, Email: grant.email, RoleID: primary.ID, TokenHash: HashInvitationToken(token),
		ExpiresAt: pgtype.Timestamptz{Time: m.now().UTC().Add(m.cfg.InviteTTL), Valid: true},
		InvitedBy: pgInt8(by), BranchScope: grant.branchScope,
	})
	if err != nil {
		return db.OrganizationInvitation{}, "", err
	}
	ids := make([]int64, 0, len(grant.roles))
	for _, r := range grant.roles {
		ids = append(ids, r.ID)
	}
	if err := m.repo.AddInvitationRoles(ctx, q, orgID, row.ID, ids); err != nil {
		return db.OrganizationInvitation{}, "", err
	}
	if grant.branchScope == orgctx.BranchScopeSelected {
		if err := m.repo.AddInvitationBranches(ctx, q, orgID, row.ID, grant.branchIDs); err != nil {
			return db.OrganizationInvitation{}, "", err
		}
	}
	return row, token, nil
}

// InvitationLink is the accept page of a token.
func (m *Management) InvitationLink(token string) string {
	return strings.TrimRight(m.cfg.FrontendURL, "/") + "/invite/" + token
}

// sendInvitation enqueues the invitation e-mail (fail-soft: the
// invitation stays and can be resent).
func (m *Management) sendInvitation(ctx context.Context, orgID int64, actor Actor, inv orgmodel.Invitation, token string) {
	if m.notifier == nil {
		return
	}
	org, err := m.orgs.repo.GetOrganizationByID(ctx, orgID)
	if err != nil {
		m.log.WarnContext(ctx, "organization_invitation_email_failed", "error", err)
		return
	}
	lang := "tr"
	if user, err := m.repo.FindUserByEmail(ctx, inv.Email); err == nil && user.Locale != "" {
		lang = user.Locale
	}
	roleNames := make([]string, 0, len(inv.Roles))
	for _, r := range inv.Roles {
		roleNames = append(roleNames, r.Name)
	}
	inviter := strings.TrimSpace(actor.Name)
	if inviter == "" {
		inviter = org.Name
	}
	expires := inv.ExpiresAt.In(istanbul()).Format("02.01.2006 15:04")
	if lang == "en" {
		expires = inv.ExpiresAt.In(istanbul()).Format("Jan 2, 2006 15:04")
	}
	email := inv.Email
	link := m.InvitationLink(token)
	if _, err := m.notifier.Enqueue(ctx, notifmodel.EnqueueInput{
		Channels: []string{notifmodel.ChannelEmail}, Recipient: &email,
		TemplateCode: invitationTemplate, SourceEvent: invitationTemplate, Language: lang,
		ActionURL: &link, Priority: notifmodel.PriorityHigh,
		TemplateVars: map[string]string{
			"organization": org.Name, "inviter": inviter, "roles": strings.Join(roleNames, ", "),
			"link": link, "expires_at": expires, "email": inv.Email,
		},
	}); err != nil {
		m.log.WarnContext(ctx, "organization_invitation_email_failed", "error", err)
	}
}

func istanbul() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("TRT", 3*60*60)
}

// --- list / revoke / resend ---------------------------------------------------

// InvitationSortColumns are the sortable invitation list fields.
var InvitationSortColumns = map[string]string{"email": "email", "expires_at": "expires_at", "created_at": "created_at"}

// ListInvitations lists an organization's invitations.
func (m *Management) ListInvitations(ctx context.Context, orgID int64, f orgmodel.InvitationListFilter) ([]orgmodel.Invitation, int64, error) {
	if _, ok := InvitationSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", true
	}
	rows, total, err := m.repo.ListInvitations(ctx, db.ListInvitationsParams{
		OrganizationID: orgID, Q: pgText(f.Q), Statuses: nilIfEmpty(f.Statuses),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out, err := m.enrichInvitations(ctx, orgID, rows)
	return out, total, err
}

// GetInvitation returns one invitation of the organization.
func (m *Management) GetInvitation(ctx context.Context, orgID int64, id uuid.UUID) (orgmodel.Invitation, error) {
	row, err := m.repo.GetInvitationByUUID(ctx, orgID, id)
	if err != nil {
		return orgmodel.Invitation{}, mapRepoNotFound(err, ErrInvitationNotFound)
	}
	listRow := db.ListInvitationsRow{
		ID: row.ID, Uuid: row.Uuid, Email: row.Email, BranchScope: row.BranchScope, ExpiresAt: row.ExpiresAt,
		AcceptedAt: row.AcceptedAt, RevokedAt: row.RevokedAt, LastSentAt: row.LastSentAt, SendCount: row.SendCount,
		CreatedAt: row.CreatedAt, Status: invitationStatus(row.AcceptedAt, row.RevokedAt, row.ExpiresAt, m.now()),
	}
	if row.InvitedBy.Valid {
		if u, err := m.orgs.repo.GetUserByID(ctx, row.InvitedBy.Int64); err == nil {
			listRow.InviterUuid = pgtype.UUID{Bytes: u.Uuid, Valid: true}
			listRow.InviterEmail, listRow.InviterName, listRow.InviterSurname = u.Email, u.Name, u.Surname
		}
	}
	out, err := m.enrichInvitations(ctx, orgID, []db.ListInvitationsRow{listRow})
	if err != nil {
		return orgmodel.Invitation{}, err
	}
	return out[0], nil
}

// RevokeInvitation closes an open invitation. Callers can only revoke what
// they could have granted.
func (m *Management) RevokeInvitation(ctx context.Context, scope orgctx.Scope, actor Actor, id uuid.UUID) (orgmodel.Invitation, error) {
	inv, err := m.GetInvitation(ctx, scope.ID, id)
	if err != nil {
		return orgmodel.Invitation{}, err
	}
	if err := m.guardInvitation(ctx, scope, inv); err != nil {
		return orgmodel.Invitation{}, err
	}
	var by *int64
	if actor.UserID > 0 {
		by = &actor.UserID
	}
	if _, err := m.repo.RevokeInvitation(ctx, scope.ID, id, by); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return orgmodel.Invitation{}, fmt.Errorf("%w: invitation is no longer open", ErrConflict)
		}
		return orgmodel.Invitation{}, err
	}
	return m.GetInvitation(ctx, scope.ID, id)
}

// ResendInvitation issues a new token (the old link stops working), renews
// the expiry and sends the e-mail again.
func (m *Management) ResendInvitation(ctx context.Context, scope orgctx.Scope, actor Actor, id uuid.UUID) (orgmodel.Invitation, error) {
	inv, err := m.GetInvitation(ctx, scope.ID, id)
	if err != nil {
		return orgmodel.Invitation{}, err
	}
	if err := m.guardInvitation(ctx, scope, inv); err != nil {
		return orgmodel.Invitation{}, err
	}
	if inv.Status == orgmodel.InvitationAccepted || inv.Status == orgmodel.InvitationRevoked {
		return orgmodel.Invitation{}, fmt.Errorf("%w: invitation is no longer open", ErrConflict)
	}
	if err := m.allow(ctx, scope.ID); err != nil {
		return orgmodel.Invitation{}, err
	}
	token, err := m.newToken()
	if err != nil {
		return orgmodel.Invitation{}, err
	}
	if _, err := m.repo.RotateInvitationToken(ctx, db.RotateInvitationTokenParams{
		OrganizationID: scope.ID, Uuid: id, TokenHash: HashInvitationToken(token),
		ExpiresAt: pgtype.Timestamptz{Time: m.now().UTC().Add(m.cfg.InviteTTL), Valid: true},
	}); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return orgmodel.Invitation{}, fmt.Errorf("%w: invitation is no longer open", ErrConflict)
		}
		return orgmodel.Invitation{}, err
	}
	inv, err = m.GetInvitation(ctx, scope.ID, id)
	if err != nil {
		return orgmodel.Invitation{}, err
	}
	m.sendInvitation(ctx, scope.ID, actor, inv, token)
	return inv, nil
}

// guardInvitation applies the grant rules to an existing invitation.
func (m *Management) guardInvitation(ctx context.Context, scope orgctx.Scope, inv orgmodel.Invitation) error {
	if !scope.AllBranches() {
		if inv.BranchScope != orgctx.BranchScopeSelected {
			return fmt.Errorf("%w: invitation grants every branch", ErrForbidden)
		}
		ids, err := m.invitationBranchIDs(ctx, scope.ID, inv.ID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if !scope.AllowsBranch(id) {
				return fmt.Errorf("%w: invitation grants branches outside your scope", ErrForbidden)
			}
		}
	}
	rows, err := m.repo.ListInvitationRoles(ctx, nil, scope.ID, []int64{inv.ID})
	if err != nil {
		return err
	}
	return m.orgs.checkGrant(ctx, scope, invitationRoleRows(rows))
}

func (m *Management) invitationBranchIDs(ctx context.Context, orgID, invitationID int64) ([]int64, error) {
	rows, err := m.repo.ListInvitationBranches(ctx, nil, orgID, []int64{invitationID})
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(rows))
	for _, b := range rows {
		out = append(out, b.ID)
	}
	return out, nil
}

func (m *Management) enrichInvitations(ctx context.Context, orgID int64, rows []db.ListInvitationsRow) ([]orgmodel.Invitation, error) {
	out := make([]orgmodel.Invitation, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	roles, err := m.repo.ListInvitationRoles(ctx, nil, orgID, ids)
	if err != nil {
		return nil, err
	}
	branches, err := m.repo.ListInvitationBranches(ctx, nil, orgID, ids)
	if err != nil {
		return nil, err
	}
	rolesBy := map[int64][]orgmodel.TenantRoleRef{}
	for _, r := range roles {
		rolesBy[r.InvitationID] = append(rolesBy[r.InvitationID], orgmodel.TenantRoleRef{
			UUID: r.Uuid, Key: r.Slug, Name: r.Name, IsTemplate: r.IsTemplate,
		})
	}
	branchesBy := map[int64][]orgmodel.BranchRef{}
	for _, b := range branches {
		branchesBy[b.InvitationID] = append(branchesBy[b.InvitationID], orgmodel.BranchRef{UUID: b.Uuid, Code: b.Code, Name: b.Name})
	}
	for _, row := range rows {
		inv := orgmodel.Invitation{
			ID: row.ID, UUID: row.Uuid, Email: row.Email, Status: row.Status,
			Roles: rolesBy[row.ID], BranchScope: row.BranchScope, Branches: branchesBy[row.ID],
			ExpiresAt: row.ExpiresAt.Time.UTC(), AcceptedAt: timePtr(row.AcceptedAt), RevokedAt: timePtr(row.RevokedAt),
			LastSentAt: row.LastSentAt.Time.UTC(), SendCount: row.SendCount, CreatedAt: row.CreatedAt.Time.UTC(),
		}
		if inv.Roles == nil {
			inv.Roles = []orgmodel.TenantRoleRef{}
		}
		if inv.Branches == nil || inv.BranchScope != orgctx.BranchScopeSelected {
			inv.Branches = []orgmodel.BranchRef{}
		}
		if row.InviterUuid.Valid {
			inv.InvitedBy = &orgmodel.UserRef{
				UUID: row.InviterUuid.Bytes, Email: row.InviterEmail, Name: row.InviterName, Surname: row.InviterSurname,
			}
		}
		out = append(out, inv)
	}
	return out, nil
}

func invitationStatus(accepted, revoked, expires pgtype.Timestamptz, now time.Time) string {
	switch {
	case revoked.Valid:
		return orgmodel.InvitationRevoked
	case accepted.Valid:
		return orgmodel.InvitationAccepted
	case !now.Before(expires.Time):
		return orgmodel.InvitationExpired
	default:
		return orgmodel.InvitationPending
	}
}

func invitationRoleRows(rows []db.ListInvitationRolesRow) []repository.TenantRoleRow {
	out := make([]repository.TenantRoleRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, repository.TenantRoleRow{ID: r.ID, Uuid: r.Uuid, Slug: r.Slug, Name: r.Name, IsTemplate: r.IsTemplate})
	}
	return out
}

// --- public: preview / accept / register ------------------------------------

// PreviewInvitation is the public view of a token: organization, inviter,
// masked e-mail and state. Unknown tokens are ErrInvitationNotFound.
func (m *Management) PreviewInvitation(ctx context.Context, token string) (orgmodel.InvitationPreview, error) {
	if strings.TrimSpace(token) == "" {
		return orgmodel.InvitationPreview{}, ErrInvitationNotFound
	}
	row, err := m.repo.GetInvitationPreview(ctx, HashInvitationToken(token))
	if err != nil {
		return orgmodel.InvitationPreview{}, mapRepoNotFound(err, ErrInvitationNotFound)
	}
	return orgmodel.InvitationPreview{
		Organization: orgmodel.InvitationPreviewOrganization{
			UUID: row.OrganizationUuid, Slug: row.OrganizationSlug, Name: row.OrganizationName,
			LogoURL: LogoURL(row.OrganizationUuid, row.LogoObjectKey),
		},
		InviterName: strings.TrimSpace(row.InviterName + " " + row.InviterSurname),
		EmailMasked: MaskEmail(row.Email),
		Status:      invitationStatus(row.AcceptedAt, row.RevokedAt, row.ExpiresAt, m.now()),
		ExpiresAt:   row.ExpiresAt.Time.UTC(),
	}, nil
}

// AcceptInvitation turns a token into a membership for the signed-in user.
// The account e-mail must match the invitation (case-insensitive). A user
// who already is a member only closes the invitation (roles unchanged).
func (m *Management) AcceptInvitation(ctx context.Context, token string, userID int64) (orgmodel.AcceptInvitationResult, error) {
	if strings.TrimSpace(token) == "" {
		return orgmodel.AcceptInvitationResult{}, ErrInvitationNotFound
	}
	user, err := m.orgs.repo.GetUserByID(ctx, userID)
	if err != nil {
		return orgmodel.AcceptInvitationResult{}, mapRepoNotFound(err, ErrUserNotFound)
	}
	hash := HashInvitationToken(token)
	var (
		inv     db.OrganizationInvitation
		already bool
	)
	err = m.orgs.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		row, err := m.repo.LockInvitationByTokenHash(ctx, q, hash)
		if err != nil {
			return mapRepoNotFound(err, ErrInvitationNotFound)
		}
		inv = row
		switch invitationStatus(row.AcceptedAt, row.RevokedAt, row.ExpiresAt, m.now()) {
		case orgmodel.InvitationRevoked:
			return ErrInvitationRevoked
		case orgmodel.InvitationAccepted:
			return ErrInvitationUsed
		case orgmodel.InvitationExpired:
			return ErrInvitationExpired
		}
		if !strings.EqualFold(strings.TrimSpace(user.Email), strings.TrimSpace(row.Email)) {
			return ErrInvitationEmailMismatch
		}
		if err := m.orgs.repo.LockOrganizationRow(ctx, q, row.OrganizationID); err != nil {
			return mapRepoNotFound(err, ErrInvitationNotFound)
		}
		if _, err := m.orgs.repo.GetOrganizationMember(ctx, q, row.OrganizationID, user.ID); err == nil {
			already = true
			return m.repo.MarkInvitationAccepted(ctx, q, row.OrganizationID, row.ID, user.ID)
		} else if !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		roleRows, err := m.repo.ListInvitationRoles(ctx, q, row.OrganizationID, []int64{row.ID})
		if err != nil {
			return err
		}
		roles := invitationRoleRows(roleRows)
		if len(roles) == 0 {
			// Every invited role was deleted meanwhile: plain member.
			role, err := m.orgs.repo.GetTenantRoleByKey(ctx, q, row.OrganizationID, orgctx.RoleMember)
			if err != nil {
				return mapRepoNotFound(err, ErrRoleNotFound)
			}
			roles = []repository.TenantRoleRow{{ID: role.ID, Uuid: role.Uuid, Slug: role.Slug, Name: role.Name, IsTemplate: true}}
		}
		var branchIDs []int64
		if row.BranchScope == orgctx.BranchScopeSelected {
			branchRows, err := m.repo.ListInvitationBranches(ctx, q, row.OrganizationID, []int64{row.ID})
			if err != nil {
				return err
			}
			for _, b := range branchRows {
				branchIDs = append(branchIDs, b.ID)
			}
		}
		if err := m.orgs.insertMember(ctx, q, row.OrganizationID, user.ID, roles, row.BranchScope, branchIDs); err != nil {
			return err
		}
		return m.repo.MarkInvitationAccepted(ctx, q, row.OrganizationID, row.ID, user.ID)
	})
	if err != nil {
		return orgmodel.AcceptInvitationResult{}, err
	}
	_ = orgctx.Invalidate(ctx, m.orgs.invalidator, inv.OrganizationID)
	org, err := m.orgs.repo.GetOrganizationByID(ctx, inv.OrganizationID)
	if err != nil {
		return orgmodel.AcceptInvitationResult{}, mapRepoNotFound(err, ErrInvitationNotFound)
	}
	return orgmodel.AcceptInvitationResult{
		OrganizationUUID: org.Uuid, OrganizationSlug: org.Slug, AlreadyMember: already, Email: user.Email,
	}, nil
}

// RegisterAndAccept creates the account of an invitee (e-mail taken from
// the invitation, so it cannot differ) and accepts the invitation. The
// account is a normal password account: its e-mail stays unverified and the
// usual verification e-mail goes out. An existing account must sign in
// instead (ErrAccountExists).
func (m *Management) RegisterAndAccept(ctx context.Context, token string, in orgmodel.RegisterInvitedInput) (orgmodel.AcceptInvitationResult, error) {
	if m.accounts == nil {
		return orgmodel.AcceptInvitationResult{}, ErrRegistrationUnavailable
	}
	if strings.TrimSpace(token) == "" {
		return orgmodel.AcceptInvitationResult{}, ErrInvitationNotFound
	}
	ve := &ValidationError{}
	name, surname := strings.TrimSpace(in.Name), strings.TrimSpace(in.Surname)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		ve.add("name", "required", "name is required (max 100 characters)")
	}
	if surname == "" || utf8.RuneCountInString(surname) > 100 {
		ve.add("surname", "required", "surname is required (max 100 characters)")
	}
	if utf8.RuneCountInString(in.Password) < 8 || len(in.Password) > 128 {
		ve.add("password", "too_short", "password must be 8-128 characters")
	}
	if err := ve.orNil(); err != nil {
		return orgmodel.AcceptInvitationResult{}, err
	}
	row, err := m.repo.GetInvitationPreview(ctx, HashInvitationToken(token))
	if err != nil {
		return orgmodel.AcceptInvitationResult{}, mapRepoNotFound(err, ErrInvitationNotFound)
	}
	switch invitationStatus(row.AcceptedAt, row.RevokedAt, row.ExpiresAt, m.now()) {
	case orgmodel.InvitationRevoked:
		return orgmodel.AcceptInvitationResult{}, ErrInvitationRevoked
	case orgmodel.InvitationAccepted:
		return orgmodel.AcceptInvitationResult{}, ErrInvitationUsed
	case orgmodel.InvitationExpired:
		return orgmodel.AcceptInvitationResult{}, ErrInvitationExpired
	}
	if _, err := m.repo.FindUserByEmail(ctx, strings.ToLower(row.Email)); err == nil {
		return orgmodel.AcceptInvitationResult{}, ErrAccountExists
	} else if !errors.Is(err, repository.ErrNotFound) {
		return orgmodel.AcceptInvitationResult{}, err
	}
	userID, err := m.accounts.RegisterInvited(ctx, strings.ToLower(row.Email), name, surname, in.Password)
	if err != nil {
		return orgmodel.AcceptInvitationResult{}, err
	}
	return m.AcceptInvitation(ctx, token, userID)
}

// --- organization profile, logo and activity ---------------------------------

// SetLogo stores a new logo object key (nil clears it) and returns the
// organization plus the previous key so the caller can delete the object.
func (m *Management) SetLogo(ctx context.Context, orgID int64, key *string) (orgmodel.Organization, string, error) {
	current, err := m.orgs.repo.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return orgmodel.Organization{}, "", mapRepoNotFound(err, ErrNotFound)
	}
	row, err := m.repo.SetOrganizationLogo(ctx, orgID, key)
	if err != nil {
		return orgmodel.Organization{}, "", mapRepoNotFound(err, ErrNotFound)
	}
	_ = orgctx.Invalidate(ctx, m.orgs.invalidator, orgID)
	previous := ""
	if current.LogoObjectKey.Valid {
		previous = current.LogoObjectKey.String
	}
	return mapOrganization(row), previous, nil
}

// LogoObjectKey returns the logo key of an organization (ErrNotFound when
// it has none).
func (m *Management) LogoObjectKey(ctx context.Context, orgUUID uuid.UUID) (string, error) {
	row, err := m.orgs.repo.GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		return "", mapRepoNotFound(err, ErrNotFound)
	}
	if !row.LogoObjectKey.Valid || row.LogoObjectKey.String == "" {
		return "", ErrNotFound
	}
	// Only stream objects under the organization's own prefix (a legacy or
	// tampered key never leaks another object).
	if !strings.HasPrefix(row.LogoObjectKey.String, "organizations/"+row.Uuid.String()+"/") {
		return "", ErrNotFound
	}
	return row.LogoObjectKey.String, nil
}

// ListActivity returns the platform operations on an organization plus the
// activity recorded inside it.
func (m *Management) ListActivity(ctx context.Context, orgUUID uuid.UUID, limit, offset int32) ([]orgmodel.ActivityEntry, int64, error) {
	org, err := m.orgs.repo.GetOrganizationByUUID(ctx, orgUUID)
	if err != nil {
		return nil, 0, mapRepoNotFound(err, ErrNotFound)
	}
	rows, total, err := m.repo.ListOrganizationActivity(ctx, org.ID, org.Uuid, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	out := make([]orgmodel.ActivityEntry, 0, len(rows))
	for _, row := range rows {
		entry := orgmodel.ActivityEntry{
			UUID: row.Uuid, Action: row.Action, Resource: row.Resource, Payload: map[string]any{},
			CreatedAt: row.CreatedAt.Time.UTC(),
		}
		if row.ResourceUuid.Valid {
			id := uuid.UUID(row.ResourceUuid.Bytes)
			entry.ResourceUUID = &id
		}
		if len(row.Payload) > 0 {
			_ = json.Unmarshal(row.Payload, &entry.Payload)
		}
		if row.ActorUuid.Valid {
			entry.Actor = &orgmodel.UserRef{
				UUID: row.ActorUuid.Bytes, Email: row.ActorEmail.String,
				Name: row.ActorName.String, Surname: row.ActorSurname.String,
			}
		}
		out = append(out, entry)
	}
	return out, total, nil
}

func pgInt8(v *int64) pgtype.Int8 {
	if v == nil || *v <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
