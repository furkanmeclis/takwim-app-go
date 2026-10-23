// Package orgctx carries the current organization (tenant) of a request.
//
// Tenant routes resolve the organization per request from the
// X-Organization header (slug or uuid) and the caller's membership; nothing
// about the organization lives in the access token. See
// docs/design/tenant-core.md.
//
// This file is the contract between the organizations module (persistence,
// Resolver implementation, membership mutations) and the HTTP layer
// (RequireOrganization middleware, caching).
package orgctx

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// HeaderName is the request header the tenant shell sends on /v1/tenant/*.
const HeaderName = "X-Organization"

// Organization statuses.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
)

// Template tenant role keys seeded by the tenant RBAC migration.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// Resolution errors. Resolvers return these (possibly wrapped) so the HTTP
// layer can map them to response codes.
var (
	ErrOrganizationNotFound = errors.New("orgctx: organization not found")
	ErrNotMember            = errors.New("orgctx: not a member of the organization")
	ErrSuspended            = errors.New("orgctx: organization suspended")
	ErrAccessExpired        = errors.New("orgctx: organization access window closed")
)

// Scope is the resolved organization + membership for one request.
type Scope struct {
	ID             int64
	UUID           uuid.UUID
	Slug           string
	Name           string
	Status         string
	AccessStartsAt time.Time
	AccessEndsAt   *time.Time
	// Sector is the business sector code (internal/platform/sector).
	Sector string
	// Timezone is the business IANA zone; Currency its ISO 4217 code.
	Timezone string
	Currency string
	// Plan, Addons and the limit overrides are the stored entitlements
	// (internal/platform/entitlement resolves the effective ones). A nil
	// limit override means the plan default.
	Plan        string
	Addons      []string
	BranchLimit *int32
	StaffLimit  *int32
	// MemberRole is the primary tenant role key of the caller in this
	// organization (owner, admin, member, a Rentopia template or a custom
	// role key). Kept for display; permissions come from MemberRoles.
	MemberRole string
	// MemberRoles are the keys of every role the caller holds here.
	MemberRoles []string
	// Permissions are the tenant.* slugs granted by the union of
	// MemberRoles in this organization only.
	Permissions []string
	// BranchIDs are the branches the caller may access. nil means every
	// branch of the organization (branch_scope 'all'); a non-nil slice,
	// possibly empty, limits branch-bound data to those ids. See branch.go.
	BranchIDs []int64
}

// Has reports whether the scope grants a tenant permission slug.
func (s Scope) Has(slug string) bool {
	for _, p := range s.Permissions {
		if p == slug {
			return true
		}
	}
	return false
}

// Resolver loads an organization by reference (slug or uuid string) and the
// membership of userID in it. Implementations must return ErrOrganizationNotFound,
// ErrNotMember, ErrSuspended or ErrAccessExpired for the matching cases and
// must ignore soft-deleted organizations.
type Resolver interface {
	Resolve(ctx context.Context, orgRef string, userID int64) (Scope, error)
}

// Invalidator drops cached resolutions of one organization. The
// organizations module calls it after any change that affects Resolve
// (status, access window, membership, role, role permissions, slug).
// A nil Invalidator is allowed: callers use Invalidate.
type Invalidator interface {
	InvalidateOrganization(ctx context.Context, organizationID int64) error
}

// Invalidate is a nil-safe helper around Invalidator.
func Invalidate(ctx context.Context, inv Invalidator, organizationID int64) error {
	if inv == nil {
		return nil
	}
	return inv.InvalidateOrganization(ctx, organizationID)
}

type ctxKey struct{}

// WithScope stores the scope on the context.
func WithScope(ctx context.Context, s Scope) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// From returns the scope if the request passed RequireOrganization.
func From(ctx context.Context) (Scope, bool) {
	s, ok := ctx.Value(ctxKey{}).(Scope)
	return s, ok
}

// OrganizationID returns the organization a context is scoped to: the
// request scope set by RequireOrganization, or the organization a background
// job carries (WithOrganizationID). Engines (io, bulk, search, activity) use
// it to stamp and filter organization_id; platform contexts return false.
func OrganizationID(ctx context.Context) (int64, bool) {
	s, ok := From(ctx)
	if !ok || s.ID <= 0 {
		return 0, false
	}
	return s.ID, true
}

// WithOrganizationID scopes a background context (worker job) to an
// organization. A request scope already on the context is kept when it names
// the same organization; otherwise a minimal scope carrying only the id is set.
func WithOrganizationID(ctx context.Context, organizationID int64) context.Context {
	if s, ok := From(ctx); ok && s.ID == organizationID {
		return ctx
	}
	return WithScope(ctx, Scope{ID: organizationID})
}

// Must returns the scope or panics (handlers behind RequireOrganization).
func Must(ctx context.Context) Scope {
	s, ok := From(ctx)
	if !ok {
		panic("orgctx: scope missing from context")
	}
	return s
}
