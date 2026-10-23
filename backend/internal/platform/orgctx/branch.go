package orgctx

import (
	"context"
	"errors"
	"slices"
)

// Branch scope values of organization_members.branch_scope.
const (
	BranchScopeAll      = "all"
	BranchScopeSelected = "selected"
)

// Branch scope errors. Modules map ErrBranchNotAllowed to 404 (the record
// must not leak, same rule as another organization's uuid) and
// ErrAllBranchesRequired to 403.
var (
	ErrBranchNotAllowed    = errors.New("orgctx: branch is outside the caller's branch scope")
	ErrAllBranchesRequired = errors.New("orgctx: action needs access to every branch")
)

// AllBranches reports whether the scope covers every branch.
func (s Scope) AllBranches() bool {
	return s.BranchIDs == nil
}

// AllowsBranch reports whether the scope covers branchID.
func (s Scope) AllowsBranch(branchID int64) bool {
	return s.BranchIDs == nil || slices.Contains(s.BranchIDs, branchID)
}

// BranchScope returns the branch restriction of ctx: nil means every branch,
// a non-nil slice (possibly empty) lists the allowed branch ids.
//
// Branch-bound modules pass it to their list/count/export/uuid queries as
//
//	AND (sqlc.narg(branch_ids)::bigint[] IS NULL OR t.branch_id = ANY(sqlc.narg(branch_ids)::bigint[]))
//
// (pgx encodes a nil slice as NULL and an empty one as '{}', which matches
// nothing). Contexts without an organization scope are not restricted:
// platform code never reads tenant tables, and worker jobs re-scope the
// context with WithJobScope first.
func BranchScope(ctx context.Context) []int64 {
	s, ok := From(ctx)
	if !ok || s.BranchIDs == nil {
		return nil
	}
	return slices.Clone(s.BranchIDs)
}

// CheckBranch returns ErrBranchNotAllowed when the record of branchID is
// outside the caller's branch scope. Use it after loading a single
// branch-bound record (get / patch / delete / bulk item).
func CheckBranch(ctx context.Context, branchID int64) error {
	s, ok := From(ctx)
	if !ok || s.AllowsBranch(branchID) {
		return nil
	}
	return ErrBranchNotAllowed
}

// RequireAllBranches returns ErrAllBranchesRequired unless the caller may
// access every branch (for example creating a branch, or changing data that
// is shared by all branches).
func RequireAllBranches(ctx context.Context) error {
	s, ok := From(ctx)
	if !ok || s.AllBranches() {
		return nil
	}
	return ErrAllBranchesRequired
}

// JobScopeLoader resolves the scope a background job runs with: the current
// membership of the user who requested it (permissions and branch scope).
type JobScopeLoader interface {
	ResolveMember(ctx context.Context, organizationID, userID int64) (Scope, error)
}

// WithJobScope scopes a worker context to a tenant job of organizationID
// requested by actorID. A request scope of the same organization already on
// the context (synchronous processing) is kept. Otherwise the loader
// re-resolves the actor's membership, so the job sees only what the actor may
// see now; a nil loader falls back to WithOrganizationID (organization only,
// every branch).
func WithJobScope(ctx context.Context, loader JobScopeLoader, organizationID, actorID int64) (context.Context, error) {
	if s, ok := From(ctx); ok && s.ID == organizationID && s.Slug != "" {
		return ctx, nil
	}
	if loader == nil || actorID <= 0 {
		return WithOrganizationID(ctx, organizationID), nil
	}
	scope, err := loader.ResolveMember(ctx, organizationID, actorID)
	if err != nil {
		return ctx, err
	}
	return WithScope(ctx, scope), nil
}
