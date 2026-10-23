package orgctx

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
)

// BranchHeaderName is the optional request header that narrows tenant lists
// to one branch (the branch picker in the panel header; a mobile client
// sends the same header). Its value is the branch uuid; no header means all
// branches the caller may access.
const BranchHeaderName = "X-Branch"

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

// BranchSelector looks up a live branch of an organization by uuid for the
// X-Branch header. It returns ErrBranchNotAllowed when there is none.
type BranchSelector interface {
	BranchIDByUUID(ctx context.Context, organizationID int64, branchUUID uuid.UUID) (int64, error)
}

// SelectBranch returns the scope narrowed to branchID for lists, or
// ErrBranchNotAllowed when the branch is outside the caller's branch scope.
func (s Scope) SelectBranch(branchID int64) (Scope, error) {
	if branchID <= 0 || !s.AllowsBranch(branchID) {
		return s, ErrBranchNotAllowed
	}
	s.SelectedBranchID = branchID
	return s, nil
}

// SelectedBranch returns the branch picked with X-Branch, if any.
func SelectedBranch(ctx context.Context) (int64, bool) {
	s, ok := From(ctx)
	if !ok || s.SelectedBranchID <= 0 {
		return 0, false
	}
	return s.SelectedBranchID, true
}

// AllBranches reports whether the scope covers every branch.
func (s Scope) AllBranches() bool {
	return s.BranchIDs == nil
}

// AllowsBranch reports whether the scope covers branchID.
func (s Scope) AllowsBranch(branchID int64) bool {
	return s.BranchIDs == nil || slices.Contains(s.BranchIDs, branchID)
}

// BranchScope returns the branch restriction of ctx for lists: nil means
// every branch, a non-nil slice (possibly empty) lists the allowed branch
// ids. A branch picked with X-Branch narrows it to that one branch.
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
	if !ok {
		return nil
	}
	if s.SelectedBranchID > 0 {
		return []int64{s.SelectedBranchID}
	}
	if s.BranchIDs == nil {
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
	if actorID > 0 {
		ctx = context.WithValue(ctx, jobActorKey{}, actorID)
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

type jobActorKey struct{}

// JobActor is the user who requested the tenant job a worker context runs
// (set by WithJobScope). Request contexts carry the principal instead; an
// engine adapter that needs the acting user (an own-records reach, history
// entries) reads the principal first and falls back to JobActor.
func JobActor(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(jobActorKey{}).(int64)
	return id, ok && id > 0
}
